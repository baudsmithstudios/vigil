package collector

import (
	"log"
	"path/filepath"
	"time"

	"github.com/shirou/gopsutil/v3/disk"
)

type diskCollector struct {
	prev     map[string]disk.IOCountersStat
	prevTime time.Time
}

func newDiskCollector() *diskCollector {
	return &diskCollector{prev: make(map[string]disk.IOCountersStat)}
}

// collectSpace returns usage for all mounted partitions. In a container with
// host /proc mounted, gopsutil enumerates real host partitions.
func (d *diskCollector) collectSpace() []DiskSnapshot {
	parts, err := disk.Partitions(false)
	if err != nil {
		log.Printf("disk partition collection error: %v", err)
		return nil
	}

	var out []DiskSnapshot
	for _, p := range parts {
		if !ShouldCollectFS(p.Fstype) {
			continue
		}
		usage, err := disk.Usage(p.Mountpoint)
		if err != nil {
			continue
		}
		out = append(out, DiskSnapshot{
			MountPoint: p.Mountpoint,
			Device:     normalizeDeviceName(p.Device),
			TotalBytes: usage.Total,
			UsedBytes:  usage.Used,
			FreeBytes:  usage.Free,
			Percent:    usage.UsedPercent,
		})
	}
	return out
}

// collectIO returns per-device I/O rates; zero on the first call (no prev sample).
func (d *diskCollector) collectIO() []DiskIOSnapshot {
	counters, err := disk.IOCounters()
	if err != nil {
		log.Printf("disk I/O collection error: %v", err)
		return nil
	}
	now := time.Now()
	elapsed := now.Sub(d.prevTime).Seconds()

	var out []DiskIOSnapshot
	for name, cur := range counters {
		snap := DiskIOSnapshot{Device: name}
		if prev, ok := d.prev[name]; ok && elapsed > 0 {
			applyDiskRates(&snap, prev, cur, elapsed)
		}
		out = append(out, snap)
		d.prev[name] = cur
	}
	d.prevTime = now
	return out
}

func normalizeDeviceName(device string) string {
	if device == "" {
		return device
	}
	if filepath.IsAbs(device) {
		if resolved, err := filepath.EvalSymlinks(device); err == nil && resolved != "" {
			device = resolved
		}
	}
	return filepath.Base(device)
}

// applyDiskRates computes interval rates from cumulative disk counters.
// Caller must pass elapsed > 0. Counter rollback is treated as unavailable
// data for the interval.
func applyDiskRates(snap *DiskIOSnapshot, prev, curr disk.IOCountersStat, elapsed float64) {
	if curr.ReadBytes >= prev.ReadBytes {
		snap.ReadRate = float64(curr.ReadBytes-prev.ReadBytes) / elapsed
	}
	if curr.WriteBytes >= prev.WriteBytes {
		snap.WriteRate = float64(curr.WriteBytes-prev.WriteBytes) / elapsed
	}

	if curr.IoTime >= prev.IoTime {
		util := float64(curr.IoTime-prev.IoTime) / (elapsed * 1000) * 100
		if util > 100 {
			util = 100
		}
		snap.UtilPercent = util
	}

	if curr.ReadCount >= prev.ReadCount && curr.WriteCount >= prev.WriteCount &&
		curr.ReadTime >= prev.ReadTime && curr.WriteTime >= prev.WriteTime {
		ops := (curr.ReadCount - prev.ReadCount) + (curr.WriteCount - prev.WriteCount)
		if ops > 0 {
			totalMs := (curr.ReadTime - prev.ReadTime) + (curr.WriteTime - prev.WriteTime)
			snap.LatencyMs = float64(totalMs) / float64(ops)
		}
	}
}
