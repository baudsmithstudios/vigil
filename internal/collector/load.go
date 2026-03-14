package collector

import (
	"log"

	"github.com/shirou/gopsutil/v3/load"
)

func collectLoad() LoadSnapshot {
	avg, err := load.Avg()
	if err != nil {
		log.Printf("load average collection error: %v", err)
		return LoadSnapshot{}
	}
	return LoadSnapshot{
		Load1:  avg.Load1,
		Load5:  avg.Load5,
		Load15: avg.Load15,
	}
}
