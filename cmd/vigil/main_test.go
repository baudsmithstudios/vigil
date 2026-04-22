package main

import (
	"testing"
	"time"

	"vigil/internal/collector"
	"vigil/internal/metric"
)

func TestSnapshotToValues_PerMountDiskKeys(t *testing.T) {
	snap := collector.Snapshot{
		Disks: []collector.DiskSnapshot{
			{MountPoint: "/", Percent: 95.0},
			{MountPoint: "/mnt/data", Percent: 20.0},
		},
	}
	values := snapshotToValues(snap)

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

func TestSnapshotToValues_NetDropsAndErrors(t *testing.T) {
	snap := collector.Snapshot{
		Network: []collector.NetSnapshot{
			{Interface: "eth0", DropRate: 5.0, ErrRate: 2.0},
			{Interface: "wlan0", DropRate: 0.0, ErrRate: 0.0},
		},
	}
	values := snapshotToValues(snap)

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
	values := snapshotToValues(snap)

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
	values := snapshotToValues(snap)

	if _, ok := values[metric.PrefixDiskUtil+"mmcblk0"]; !ok {
		t.Fatal("expected tracked device mmcblk0 in values")
	}
	if _, ok := values[metric.PrefixDiskUtil+"loop0"]; ok {
		t.Fatal("unexpected untracked device loop0 in values")
	}
}
