package collector

import (
	"log"

	"github.com/shirou/gopsutil/v3/mem"
)

func collectMemory() MemSnapshot {
	v, err := mem.VirtualMemory()
	if err != nil {
		log.Printf("memory collection error: %v", err)
		return MemSnapshot{}
	}

	snap := MemSnapshot{
		TotalBytes:     v.Total,
		UsedBytes:      v.Used,
		AvailableBytes: v.Available,
		CachedBytes:    v.Cached,
		BuffersBytes:   v.Buffers,
		Percent:        v.UsedPercent,
	}

	s, err := mem.SwapMemory()
	if err != nil {
		log.Printf("swap collection error: %v", err)
		return snap
	}
	snap.SwapTotalBytes = s.Total
	snap.SwapUsedBytes = s.Used
	snap.SwapPercent = s.UsedPercent
	snap.SwapReady = true
	snap.SwapInBytes = s.Sin
	snap.SwapOutBytes = s.Sout

	return snap
}
