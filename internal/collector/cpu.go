package collector

import (
	"log"

	"github.com/shirou/gopsutil/v3/cpu"
)

type cpuCollector struct {
	warmedUp  bool
	prevTimes cpu.TimesStat
}

func newCPUCollector() *cpuCollector {
	return &cpuCollector{}
}

// collect returns a CPUSnapshot. The first call primes the counters and returns
// Ready=false. Subsequent calls return the actual utilisation and time breakdown.
func (c *cpuCollector) collect() CPUSnapshot {
	// interval=0 means "delta since last call", which requires two calls.
	percents, err := cpu.Percent(0, true)
	if err != nil {
		log.Printf("cpu percent collection error: %v", err)
	}

	times, timesErr := cpu.Times(false) // aggregate (not per-core)
	if timesErr != nil {
		log.Printf("cpu times collection error: %v", timesErr)
	}

	if !c.warmedUp {
		c.warmedUp = true
		if timesErr == nil && len(times) > 0 {
			c.prevTimes = times[0]
		}
		return CPUSnapshot{Ready: false}
	}

	snap := CPUSnapshot{Ready: err == nil}
	if err == nil {
		total := 0.0
		for _, p := range percents {
			total += p
		}
		if len(percents) > 0 {
			total /= float64(len(percents))
		}
		snap.PercentPerCore = percents
		snap.PercentTotal = total
	}

	if timesErr == nil && len(times) > 0 {
		cur := times[0]
		prev := c.prevTimes
		delta := (cur.User - prev.User) +
			(cur.System - prev.System) +
			(cur.Iowait - prev.Iowait) +
			(cur.Idle - prev.Idle) +
			(cur.Nice - prev.Nice) +
			(cur.Irq - prev.Irq) +
			(cur.Softirq - prev.Softirq) +
			(cur.Steal - prev.Steal)
		if delta > 0 {
			snap.UserPercent = (cur.User - prev.User) / delta * 100
			snap.SystemPercent = (cur.System - prev.System) / delta * 100
			snap.IOWaitPercent = (cur.Iowait - prev.Iowait) / delta * 100
			snap.IdlePercent = (cur.Idle - prev.Idle) / delta * 100
		}
		c.prevTimes = cur
	}

	return snap
}
