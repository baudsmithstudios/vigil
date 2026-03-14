package collector

import (
	"log"

	"github.com/shirou/gopsutil/v3/host"
)

func collectUptime() uint64 {
	sec, err := host.Uptime()
	if err != nil {
		log.Printf("uptime collection error: %v", err)
		return 0
	}
	return sec
}
