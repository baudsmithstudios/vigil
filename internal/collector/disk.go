package collector

import (
	"log"
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
			Device:     p.Device,
			TotalBytes: usage.Total,
			UsedBytes:  usage.Used,
			FreeBytes:  usage.Free,
			Percent:    usage.UsedPercent,
		})
	}
	return out
}

// collectIO returns per-device read/write throughput since the last call.
// Rates are zero on the first call.
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
			if cur.ReadBytes >= prev.ReadBytes {
				snap.ReadRate = float64(cur.ReadBytes-prev.ReadBytes) / elapsed
			}
			if cur.WriteBytes >= prev.WriteBytes {
				snap.WriteRate = float64(cur.WriteBytes-prev.WriteBytes) / elapsed
			}
		}
		out = append(out, snap)
		d.prev[name] = cur
	}
	d.prevTime = now
	return out
}
