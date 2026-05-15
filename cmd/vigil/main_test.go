package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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

func tempDB(t *testing.T) (*store.DB, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "vigil-main-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	db, err := store.Open(f.Name())
	if err != nil {
		os.Remove(f.Name())
		t.Fatal(err)
	}
	return db, func() {
		db.Close()
		os.Remove(f.Name())
	}
}

func TestSnapshotToValues_PerMountDiskKeys(t *testing.T) {
	snap := collector.Snapshot{
		Disks: []collector.DiskSnapshot{
			{MountPoint: "/", Percent: 95.0},
			{MountPoint: "/mnt/data", Percent: 20.0},
		},
	}
	values := snapshotMetricValues(snap)

	// Each mount should have its own key.
	if v, ok := values[metric.PrefixDiskPercent+"/"]; !ok || v != 95.0 {
		t.Errorf("expected disk_percent:/ = 95.0, got %v (present=%v)", v, ok)
	}
	if v, ok := values[metric.PrefixDiskPercent+"/mnt/data"]; !ok || v != 20.0 {
		t.Errorf("expected disk_percent:/mnt/data = 20.0, got %v (present=%v)", v, ok)
	}

	// Bare "disk_percent" key must not exist.
	if _, ok := values["disk_percent"]; ok {
		t.Error("bare 'disk_percent' key should not exist in values map")
	}
}

func TestBuildNotifierSendsNtfyNotification(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	oldTransport := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	t.Cleanup(func() {
		http.DefaultTransport = oldTransport
	})

	notifier, mute, err := buildNotifier(config.Notifications{
		NtfyTopic:  "vigil-alerts",
		NtfyServer: srv.URL,
	})
	if err != nil {
		t.Fatalf("buildNotifier() error = %v", err)
	}
	if mute == nil {
		t.Fatal("expected mute toggle")
	}
	if notifier == nil {
		t.Fatal("expected notifier")
	}
	err = notifier.Send(context.Background(), alert.State{
		Name:    "cpu_percent",
		Message: "CPU usage above 90%",
		FiredAt: time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC),
	}, false)
	if err != nil {
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
	if _, ok := out["Timestamp"]; ok {
		t.Fatalf("did not expect Go struct key Timestamp in %s", data)
	}

	cpu := out["cpu"].(map[string]any)
	if _, ok := cpu["percent_total"]; !ok {
		t.Fatalf("expected cpu.percent_total in %s", data)
	}
	if _, ok := cpu["PercentTotal"]; ok {
		t.Fatalf("did not expect Go struct key cpu.PercentTotal in %s", data)
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
	ts := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	snap := collector.Snapshot{
		Timestamp: ts,
		CPU: collector.CPUSnapshot{
			PercentPerCore: []float64{10, 20},
			PercentTotal:   15,
			Ready:          true,
		},
		Memory: collector.MemSnapshot{
			Percent: 50,
		},
		Disks: []collector.DiskSnapshot{{
			MountPoint: "/",
			Percent:    25,
			Device:     "mmcblk0p2",
		}},
		DiskIO: []collector.DiskIOSnapshot{{
			Device:    "mmcblk0",
			LatencyMs: 3.5,
		}},
		Load:        collector.LoadSnapshot{Load1: 1, Load5: 2, Load15: 3},
		Temperature: []collector.TempSnapshot{{SensorKey: "cpu_thermal", Celsius: 42.5}},
		Throttle: collector.ThrottleSnapshot{
			Raw:                   1,
			UnderVoltage:          true,
			UnderVoltageSinceBoot: true,
			Available:             true,
		},
		Mounts: []collector.MountStatus{{
			Path:     "/mnt/data",
			Label:    "Data",
			Mounted:  true,
			Unstable: false,
		}},
		Services: []checker.ServiceStatus{{
			Name:       "web",
			CheckType:  "http",
			Up:         true,
			StatusCode: 200,
			Latency:    25 * time.Millisecond,
			CheckedAt:  ts,
		}},
		ServiceCycleTime: ts,
		UptimeSec:        12345,
	}

	out := onceOutputFromSnapshot(snap)
	if got := out.Timestamp; !got.Equal(ts) {
		t.Fatalf("expected timestamp %s, got %s", ts, got)
	}
	if got := out.CPU.PercentTotal; got != 15 {
		t.Fatalf("expected cpu percent_total 15, got %v", got)
	}
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

	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("runOnceJSON wrote invalid JSON: %v\n%s", err, out.String())
	}
	if _, ok := decoded["timestamp"]; !ok {
		t.Fatalf("expected timestamp in JSON output: %s", out.String())
	}
}

func TestSnapshotToValues_NetDropsAndErrors(t *testing.T) {
	snap := collector.Snapshot{
		Network: []collector.NetSnapshot{
			{Interface: "eth0", DropRate: 5.0, ErrRate: 2.0},
			{Interface: "wlan0", DropRate: 0.0, ErrRate: 0.0},
		},
	}
	values := snapshotMetricValues(snap)

	if v, ok := values[metric.PrefixNetDrops+"eth0"]; !ok || v != 5.0 {
		t.Errorf("expected net_drops:eth0 = 5.0, got %v (present=%v)", v, ok)
	}
	if v, ok := values[metric.PrefixNetErrors+"eth0"]; !ok || v != 2.0 {
		t.Errorf("expected net_errors:eth0 = 2.0, got %v (present=%v)", v, ok)
	}
	if v, ok := values[metric.PrefixNetDrops+"wlan0"]; !ok || v != 0.0 {
		t.Errorf("expected net_drops:wlan0 = 0.0, got %v (present=%v)", v, ok)
	}
}

func TestAppendReadings_NetworkMetrics(t *testing.T) {
	ts := time.Now()
	snap := collector.Snapshot{
		Timestamp: ts,
		Network: []collector.NetSnapshot{
			{Interface: "eth0", SendRate: 1024.0, RecvRate: 2048.0, DropRate: 3.0, ErrRate: 1.5},
			{Interface: "wlan0", SendRate: 512.0, RecvRate: 256.0, DropRate: 0.0, ErrRate: 0.0},
		},
	}
	buf := appendReadings(nil, snap)

	want := map[string]float64{
		metric.PrefixNetSend + "eth0":    1024.0,
		metric.PrefixNetRecv + "eth0":    2048.0,
		metric.PrefixNetDrops + "eth0":   3.0,
		metric.PrefixNetErrors + "eth0":  1.5,
		metric.PrefixNetSend + "wlan0":   512.0,
		metric.PrefixNetRecv + "wlan0":   256.0,
		metric.PrefixNetDrops + "wlan0":  0.0,
		metric.PrefixNetErrors + "wlan0": 0.0,
	}

	found := make(map[string]float64)
	for _, r := range buf {
		if _, ok := want[r.Metric]; ok {
			found[r.Metric] = r.Value
		}
	}
	for k, v := range want {
		if got, ok := found[k]; !ok {
			t.Errorf("missing metric %q", k)
		} else if got != v {
			t.Errorf("metric %q: expected %.1f, got %.1f", k, v, got)
		}
	}
}

func TestSnapshotToValues_DiskIOMetrics(t *testing.T) {
	snap := collector.Snapshot{
		Disks: []collector.DiskSnapshot{
			{MountPoint: "/", Device: "mmcblk0p2"},
		},
		DiskIO: []collector.DiskIOSnapshot{
			{Device: "mmcblk0", UtilPercent: 92.5, LatencyMs: 34.0},
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

	if got := values[metric.PrefixDiskUtil+"mmcblk0"]; got != 92.5 {
		t.Errorf("expected disk_util:mmcblk0 = 92.5, got %v", got)
	}
	if got := values[metric.PrefixDiskLatency+"mmcblk0"]; got != 34.0 {
		t.Errorf("expected disk_latency_ms:mmcblk0 = 34.0, got %v", got)
	}
	if got := values[metric.SwapIn]; got != 512.0 {
		t.Errorf("expected swap_in = 512.0, got %v", got)
	}
	if got := values[metric.SwapOut]; got != 1024.0 {
		t.Errorf("expected swap_out = 1024.0, got %v", got)
	}
	if got := values[metric.PrefixSDErrors+"mmc0"]; got != 3.0 {
		t.Errorf("expected sd_errors:mmc0 = 3.0, got %v", got)
	}
}

func TestAppendReadings_DiskIOMetrics(t *testing.T) {
	ts := time.Now()
	snap := collector.Snapshot{
		Timestamp: ts,
		Disks: []collector.DiskSnapshot{
			{MountPoint: "/", Device: "mmcblk0p2"},
		},
		DiskIO: []collector.DiskIOSnapshot{
			{Device: "mmcblk0", ReadRate: 4096.0, WriteRate: 8192.0, UtilPercent: 88.0, LatencyMs: 12.5},
		},
		Memory: collector.MemSnapshot{
			SwapInRate:  64.0,
			SwapOutRate: 32.0,
		},
		SDErrors: []collector.SDErrorSnapshot{
			{Host: "mmc0", Delta: 1},
		},
	}
	buf := appendReadings(nil, snap)

	want := map[string]float64{
		metric.PrefixDiskRead + "mmcblk0":    4096.0,
		metric.PrefixDiskWrite + "mmcblk0":   8192.0,
		metric.PrefixDiskUtil + "mmcblk0":    88.0,
		metric.PrefixDiskLatency + "mmcblk0": 12.5,
		metric.SwapIn:                        64.0,
		metric.SwapOut:                       32.0,
		metric.PrefixSDErrors + "mmc0":       1.0,
	}
	found := make(map[string]float64)
	for _, r := range buf {
		if _, ok := want[r.Metric]; ok {
			found[r.Metric] = r.Value
		}
	}
	for k, v := range want {
		if got, ok := found[k]; !ok {
			t.Errorf("missing metric %q", k)
		} else if got != v {
			t.Errorf("metric %q: expected %.1f, got %.1f", k, v, got)
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
	db, cleanup := tempDB(t)
	defer cleanup()

	snapshots := make(chan collector.Snapshot, 1)
	snapshots <- collector.Snapshot{
		Timestamp: time.Now(),
		Memory: collector.MemSnapshot{
			Percent: 42.0,
		},
	}
	close(snapshots)

	runLoop(
		context.Background(),
		db,
		snapshots,
		alert.New(nil),
		nil,
		alert.NewMountHandler(),
		alert.NewServiceHandler(2),
		loopConfig{
			writeBufCap:     8,
			containerBufCap: 4,
			flushInterval:   time.Hour,
			purgeInterval:   time.Hour,
			retention:       time.Hour,
		},
	)

	readings, err := db.QueryRecent(metric.MemPercent, 1)
	if err != nil {
		t.Fatalf("QueryRecent: %v", err)
	}
	if len(readings) != 1 {
		t.Fatalf("expected 1 reading flushed on channel close, got %d", len(readings))
	}
	if readings[0].Value != 42.0 {
		t.Fatalf("expected mem_percent 42.0, got %v", readings[0].Value)
	}
}
