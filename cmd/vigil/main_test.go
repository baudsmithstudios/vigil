package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vigil/internal/alert"
	"vigil/internal/checker"
	"vigil/internal/collector"
	"vigil/internal/config"
	"vigil/internal/metric"
	"vigil/internal/store"
)

func tempDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "vigil.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func runLoopOnce(db *store.DB, snap collector.Snapshot) {
	snapshots := make(chan collector.Snapshot, 1)
	snapshots <- snap
	close(snapshots)
	runLoop(
		context.Background(),
		db,
		snapshots,
		alerting{
			engine:   alert.New(nil),
			mounts:   alert.NewMountHandler(),
			services: alert.NewServiceHandler(2),
			throttle: &alert.ThrottleAlertHandler{},
		},
		nil,
		loopConfig{flushInterval: time.Hour, purgeInterval: time.Hour},
	)
}

func TestSnapshotToValues_PerMountDiskKeys(t *testing.T) {
	snap := collector.Snapshot{
		Disks: []collector.DiskSnapshot{
			{MountPoint: "/", Percent: 95.0},
			{MountPoint: "/mnt/data", Percent: 20.0},
		},
	}
	values := snapshotMetricValues(snap)

	if v, ok := values[metric.PrefixDiskPercent+"/"]; !ok || v != 95.0 {
		t.Errorf("expected disk_percent:/ = 95.0, got %v (present=%v)", v, ok)
	}
	if v, ok := values[metric.PrefixDiskPercent+"/mnt/data"]; !ok || v != 20.0 {
		t.Errorf("expected disk_percent:/mnt/data = 20.0, got %v (present=%v)", v, ok)
	}
	if _, ok := values["disk_percent"]; ok {
		t.Error("bare 'disk_percent' key should not exist in values map")
	}
}

func TestBuildNotifierSendsNtfyNotification(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
	}))
	defer srv.Close()

	oldTransport := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	t.Cleanup(func() {
		http.DefaultTransport = oldTransport
	})

	notifier, _, err := buildNotifier(config.Notifications{
		NtfyTopic:  "vigil-alerts",
		NtfyServer: srv.URL,
	})
	if err != nil {
		t.Fatalf("buildNotifier() error = %v", err)
	}
	if err := notifier.Send(context.Background(), alert.State{Name: "cpu_percent"}, false); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if gotPath != "/vigil-alerts" {
		t.Fatalf("expected request path /vigil-alerts, got %q", gotPath)
	}
	if !strings.Contains(gotBody, "cpu_percent") {
		t.Fatalf("expected ntfy body to include alert name, got %q", gotBody)
	}
}

func TestBuildNotifierRejectsInvalidNtfyTopic(t *testing.T) {
	_, _, err := buildNotifier(config.Notifications{
		NtfyTopic:  "vigil/alerts",
		NtfyServer: "https://ntfy.example.com",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestOnceOutputJSONContractUsesStableKeys(t *testing.T) {
	data, err := json.Marshal(onceOutputFromSnapshot(collector.Snapshot{}))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	for _, key := range []string{
		"timestamp", "cpu", "memory", "disks", "disk_io", "sd_errors",
		"network", "load", "temperature", "containers", "throttle",
		"mounts", "services", "uptime_sec",
	} {
		if _, ok := out[key]; !ok {
			t.Fatalf("expected top-level key %q in %s", key, data)
		}
	}

	cpu := out["cpu"].(map[string]any)
	if _, ok := cpu["percent_total"]; !ok {
		t.Fatalf("expected cpu.percent_total in %s", data)
	}
	if cpu["percent_per_core"] == nil {
		t.Fatalf("expected empty cpu.percent_per_core array, got null in %s", data)
	}

	for _, key := range []string{
		"disks", "disk_io", "sd_errors", "network", "temperature",
		"containers", "mounts", "services",
	} {
		if out[key] == nil {
			t.Fatalf("expected empty %s array, got null in %s", key, data)
		}
	}
}

func TestOnceOutputMapsRepresentativeValues(t *testing.T) {
	snap := collector.Snapshot{
		Disks:       []collector.DiskSnapshot{{MountPoint: "/"}},
		DiskIO:      []collector.DiskIOSnapshot{{Device: "mmcblk0", LatencyMs: 3.5}},
		Temperature: []collector.TempSnapshot{{SensorKey: "cpu_thermal"}},
		Throttle:    collector.ThrottleSnapshot{UnderVoltage: true, Available: true},
		Services:    []checker.ServiceStatus{{Name: "web", Latency: 25 * time.Millisecond}},
	}

	out := onceOutputFromSnapshot(snap)
	if got := out.Disks[0].MountPoint; got != "/" {
		t.Fatalf("expected disk mount point /, got %q", got)
	}
	if got := out.DiskIO[0].LatencyMs; got != 3.5 {
		t.Fatalf("expected disk_io latency_ms 3.5, got %v", got)
	}
	if got := out.Temperature[0].SensorKey; got != "cpu_thermal" {
		t.Fatalf("expected temperature sensor_key cpu_thermal, got %q", got)
	}
	if got := out.Throttle.Status; got != "THROTTLED" {
		t.Fatalf("expected throttle status THROTTLED, got %q", got)
	}
	if got := out.Services[0].LatencyMs; got != 25 {
		t.Fatalf("expected service latency_ms 25, got %v", got)
	}
}

func TestRunOnceJSONDoesNotOpenDatabase(t *testing.T) {
	cfg := config.Defaults()
	cfg.DBPath = filepath.Join(t.TempDir(), "missing", "vigil.db")
	cfg.Docker.Socket = ""
	cfg.MountChecks = nil
	cfg.HTTPChecks = nil
	cfg.PortChecks = nil

	var out bytes.Buffer
	if err := runOnceJSON(&out, cfg); err != nil {
		t.Fatalf("runOnceJSON() error = %v", err)
	}
	if !json.Valid(out.Bytes()) {
		t.Fatalf("runOnceJSON wrote invalid JSON: %s", out.String())
	}
}

func TestSnapshotToValues_NetworkMetrics(t *testing.T) {
	snap := collector.Snapshot{
		Network: []collector.NetSnapshot{
			{Interface: "eth0", SendRate: 1024.0, RecvRate: 2048.0, DropRate: 5.0, ErrRate: 2.0},
			{Interface: "wlan0", DropRate: 0.0, ErrRate: 0.0},
		},
	}
	values := snapshotMetricValues(snap)

	want := map[string]float64{
		metric.PrefixNetSend + "eth0":   1024.0,
		metric.PrefixNetRecv + "eth0":   2048.0,
		metric.PrefixNetDrops + "eth0":  5.0,
		metric.PrefixNetErrors + "eth0": 2.0,
		metric.PrefixNetDrops + "wlan0": 0.0,
	}
	for k, wantV := range want {
		if v, ok := values[k]; !ok || v != wantV {
			t.Errorf("expected %s = %v, got %v (present=%v)", k, wantV, v, ok)
		}
	}
}

func TestSnapshotToValues_DiskIOMetrics(t *testing.T) {
	snap := collector.Snapshot{
		Disks: []collector.DiskSnapshot{
			{MountPoint: "/", Device: "mmcblk0p2"},
		},
		DiskIO: []collector.DiskIOSnapshot{
			{Device: "mmcblk0", ReadRate: 4096.0, WriteRate: 8192.0, UtilPercent: 92.5, LatencyMs: 34.0},
		},
		Memory: collector.MemSnapshot{
			SwapInRate:  512.0,
			SwapOutRate: 1024.0,
		},
		SDErrors: []collector.SDErrorSnapshot{
			{Host: "mmc0", Delta: 3},
		},
	}
	values := snapshotMetricValues(snap)

	want := map[string]float64{
		metric.PrefixDiskRead + "mmcblk0":    4096.0,
		metric.PrefixDiskWrite + "mmcblk0":   8192.0,
		metric.PrefixDiskUtil + "mmcblk0":    92.5,
		metric.PrefixDiskLatency + "mmcblk0": 34.0,
		metric.SwapIn:                        512.0,
		metric.SwapOut:                       1024.0,
		metric.PrefixSDErrors + "mmc0":       3.0,
	}
	for k, wantV := range want {
		if got := values[k]; got != wantV {
			t.Errorf("expected %s = %v, got %v", k, wantV, got)
		}
	}
}

func TestSnapshotToValues_DiskIOMetricsFilteredToTrackedDevices(t *testing.T) {
	snap := collector.Snapshot{
		Disks: []collector.DiskSnapshot{
			{MountPoint: "/", Device: "mmcblk0p2"},
		},
		DiskIO: []collector.DiskIOSnapshot{
			{Device: "mmcblk0", UtilPercent: 10.0, LatencyMs: 1.0},
			{Device: "loop0", UtilPercent: 99.0, LatencyMs: 99.0},
		},
	}
	values := snapshotMetricValues(snap)

	if _, ok := values[metric.PrefixDiskUtil+"mmcblk0"]; !ok {
		t.Fatal("expected tracked device mmcblk0 in values")
	}
	if _, ok := values[metric.PrefixDiskUtil+"loop0"]; ok {
		t.Fatal("unexpected untracked device loop0 in values")
	}
}

func TestSnapshotToValues_TemperatureUsesPrefixedKey(t *testing.T) {
	snap := collector.Snapshot{
		Temperature: []collector.TempSnapshot{
			{SensorKey: "cpu_thermal", Celsius: 71.5},
		},
	}
	values := snapshotMetricValues(snap)

	if got := values[metric.PrefixTemp+"cpu_thermal"]; got != 71.5 {
		t.Fatalf("expected %s = 71.5, got %v", metric.PrefixTemp+"cpu_thermal", got)
	}
	if _, ok := values["cpu_thermal"]; ok {
		t.Fatalf("did not expect unprefixed cpu_thermal key in values")
	}
}

func TestRunLoop_FlushesOnSnapshotChannelClose(t *testing.T) {
	db := tempDB(t)
	ts := time.Now()
	runLoopOnce(db, collector.Snapshot{Timestamp: ts, Memory: collector.MemSnapshot{Percent: 42.0}})

	readings, err := db.QueryRecent(metric.MemPercent, 1)
	if err != nil {
		t.Fatalf("QueryRecent: %v", err)
	}
	if len(readings) != 1 || readings[0].Value != 42.0 || !readings[0].Timestamp.Equal(ts) {
		t.Fatalf("expected mem_percent 42.0 at %v flushed on close, got %+v", ts, readings)
	}
}

func TestRunLoop_PersistsThrottleAlert(t *testing.T) {
	db := tempDB(t)
	runLoopOnce(db, collector.Snapshot{
		Timestamp: time.Now(),
		Throttle:  collector.ThrottleSnapshot{Available: true, UnderVoltage: true},
	})

	active, err := db.ActiveAlerts()
	if err != nil {
		t.Fatalf("ActiveAlerts: %v", err)
	}
	if len(active) != 1 || active[0].Name != metric.AlertThrottle {
		t.Fatalf("expected active throttle alert, got %+v", active)
	}
}
