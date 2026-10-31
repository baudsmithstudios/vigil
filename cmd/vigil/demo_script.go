//go:build demo

package main

import (
	"context"
	"math"
	"time"

	"vigil/internal/checker"
	"vigil/internal/collector"
)

const (
	demoTick = time.Second

	mqttDownFrom, mqttDownTo = 26, 45
)

const (
	kib = 1 << 10
	mib = 1 << 20
	gib = 1 << 30
)

<<<<<<< HEAD
// playDemo stamps each snapshot on the script's clock, so service cycles land on
// the same ticks as in TestDemoTimeline.
=======
// playDemo timestamps each snapshot from its tick number, so service checks happen
// on the same ticks as in TestDemoTimeline.
>>>>>>> origin/wip/demo-mode
func playDemo(ctx context.Context, out chan<- collector.Snapshot) {
	start := time.Now()
	ticker := time.NewTicker(demoTick)
	defer ticker.Stop()
	for tick := 0; ; tick++ {
		select {
		case out <- demoSnapshot(tick, start.Add(time.Duration(tick)*demoTick)):
		case <-ctx.Done():
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

<<<<<<< HEAD
// demoSnapshot is a fictional Pi; nothing in it comes from the host running the demo.
=======
// demoSnapshot returns made-up metrics; nothing is read from the host.
>>>>>>> origin/wip/demo-mode
func demoSnapshot(tick int, now time.Time) collector.Snapshot {
	during := func(from, to int) bool { return tick >= from && tick <= to }
	drift := func(period, amplitude float64) float64 {
		return amplitude * math.Sin(2*math.Pi*float64(tick)/period)
	}
	cpuBusy := during(13, 45)
	mqttDown := during(mqttDownFrom, mqttDownTo)

	cpu := 11.5 + drift(9, 3.5)
	temp := 47.5 + drift(17, 1.2)
	load := collector.LoadSnapshot{Load1: 0.42 + drift(11, 0.08), Load5: 0.38, Load15: 0.35}
	haCPU := 2.1 + drift(7, 0.6)
	if cpuBusy {
		cpu = 94 + drift(5, 3)
		temp = 60 + drift(6, 1.5)
		load = collector.LoadSnapshot{Load1: 3.7 + drift(7, 0.2), Load5: 2.1, Load15: 0.9}
		haCPU = 78 + drift(5, 4)
	}

	memPercent := 30 + drift(23, 3)
	memUsed := uint64(4 * gib * memPercent / 100)

	mosquitto := "running"
	switch {
	case mqttDown:
		mosquitto = "exited"
	case tick == mqttDownTo+1:
		mosquitto = "restarting"
	}

<<<<<<< HEAD
	// Production checks every 30s; every other tick fits the two-failure debounce in the take.
=======
	// Check services every 2 ticks, not every 30s, so the two failures an alert needs happen during the recording.
>>>>>>> origin/wip/demo-mode
	checkedAt := now.Add(-time.Duration(tick%2) * demoTick)

	return collector.Snapshot{
		Timestamp: now,
		CPU: collector.CPUSnapshot{
			PercentPerCore: []float64{cpu + 2, cpu - 2, cpu + 1, cpu - 1},
			PercentTotal:   cpu,
			UserPercent:    cpu * 0.7,
			SystemPercent:  cpu * 0.25,
			IOWaitPercent:  cpu * 0.05,
			IdlePercent:    100 - cpu,
			Ready:          true,
		},
		Memory: collector.MemSnapshot{
			TotalBytes:     4 * gib,
			UsedBytes:      memUsed,
			CachedBytes:    1100 * mib,
			BuffersBytes:   90 * mib,
			Percent:        memPercent,
			SwapTotalBytes: 512 * mib,
			SwapUsedBytes:  40 * mib,
			SwapPercent:    40.0 / 512 * 100,
			SwapReady:      true,
		},
		Disks: []collector.DiskSnapshot{
			{MountPoint: "/", Device: "mmcblk0p2", TotalBytes: 29 * gib, UsedBytes: 29 * gib * 28 / 100, Percent: 28},
			{MountPoint: "/data", Device: "sda1", TotalBytes: 916 * gib, UsedBytes: 916 * gib * 41 / 100, Percent: 41},
		},
		DiskIO: []collector.DiskIOSnapshot{
			{Device: "mmcblk0", ReadRate: 12*kib + drift(7, 4*kib), WriteRate: 48*kib + drift(9, 12*kib), UtilPercent: 2 + drift(7, 0.5), LatencyMs: 1.5},
			{Device: "sda", ReadRate: 400*kib + drift(11, 120*kib), WriteRate: 1.1*mib + drift(13, 300*kib), UtilPercent: 6 + drift(11, 1.5), LatencyMs: 4.2},
		},
		Network: []collector.NetSnapshot{
			{Interface: "eth0", RecvRate: 180*kib + drift(8, 40*kib), SendRate: 45*kib + drift(10, 10*kib)},
			{Interface: "wlan0", RecvRate: 1.2*kib + drift(12, 0.3*kib), SendRate: 0.4 * kib},
		},
		Load:        load,
		Temperature: []collector.TempSnapshot{{SensorKey: "cpu_thermal", Celsius: temp}},
		Containers: []collector.ContainerSnapshot{
			{Name: "hass", ID: "3f9a1c2e7b4d", Status: "running", CPUPercent: haCPU, MemUsed: 412 * mib},
			{Name: "pihole", ID: "a81e5d0c9f62", Status: "running", CPUPercent: 0.6 + drift(9, 0.2), MemUsed: 96 * mib},
			{Name: "mosquitto", ID: "c47b20e8d1a5", Status: mosquitto, CPUPercent: 0.2, MemUsed: 9 * mib},
			{Name: "grafana", ID: "e2d6f8a03b19", Status: "running", CPUPercent: 1.4 + drift(11, 0.5), MemUsed: 154 * mib},
		},
		Throttle: collector.ThrottleSnapshot{Available: true, UnderVoltage: during(30, 47)},
		Mounts: []collector.MountStatus{
			{Path: "/mnt/nas", Label: "NAS Storage", Mounted: !during(17, 43)},
<<<<<<< HEAD
			// Three reappearances trip the mount handler's unstable alert.
=======
			// Three reappearances fire the mount handler's unstable alert.
>>>>>>> origin/wip/demo-mode
			{Path: "/media/usb0", Label: "USB Backup", Mounted: !(during(21, 25) && tick%2 == 1)},
		},
		Services: []checker.ServiceStatus{
			{Name: "hass", CheckType: "http", Up: true, StatusCode: 200, Latency: time.Duration(18+drift(9, 4)) * time.Millisecond, CheckedAt: checkedAt},
			{Name: "grafana", CheckType: "http", Up: true, StatusCode: 200, Latency: time.Duration(32+drift(11, 6)) * time.Millisecond, CheckedAt: checkedAt},
			{Name: "mqtt", CheckType: "tcp", Up: !mqttDown, Latency: time.Duration(3+drift(13, 1)) * time.Millisecond, CheckedAt: checkedAt},
		},
		ServiceCycleTime: checkedAt,
		UptimeSec:        (12*24+4)*3600 + uint64(tick),
	}
}
