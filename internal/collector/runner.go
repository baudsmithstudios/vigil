package collector

import (
	"context"
	"time"

	"vigil/internal/checker"
	"vigil/internal/config"
)

type Runner struct {
	interval    time.Duration
	cpu         *cpuCollector
	disk        *diskCollector
	net         *netCollector
	sd          *sdErrorCollector
	docker      *dockerCollector // nil if Docker monitoring is disabled
	mount       *mountCollector
	services    *checker.ServiceChecker // nil if no checks configured
	prevSwapIn  uint64
	prevSwapOut uint64
	prevSwapAt  time.Time
	swapReady   bool
	out         chan Snapshot
}

// New returns a Runner and the channel it emits Snapshots on. Each subsystem
// is enabled by the corresponding argument: non-empty dockerSocket, non-empty
// mountChecks, or non-nil services.
func New(interval time.Duration, dockerSocket string, mountChecks []config.MountCheck, services *checker.ServiceChecker) (*Runner, <-chan Snapshot) {
	ch := make(chan Snapshot, 4)
	r := &Runner{
		interval: interval,
		cpu:      newCPUCollector(),
		disk:     newDiskCollector(),
		net:      newNetCollector(),
		sd:       newSDErrorCollector(),
		services: services,
		out:      ch,
	}
	if dockerSocket != "" {
		r.docker = newDockerCollector(dockerSocket)
	}
	if len(mountChecks) > 0 {
		r.mount = newMountCollector(mountChecks, "")
	}
	return r, ch
}

// Run blocks until ctx is cancelled, ticking once per interval.
func (r *Runner) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	defer close(r.out)

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			snap := r.Collect(t)
			select {
			case r.out <- snap:
			default:
				// Drop snapshot if consumer is slow rather than blocking.
			}
		}
	}
}

// Collect gathers one snapshot synchronously (used by --once mode).
func (r *Runner) Collect(t time.Time) Snapshot {
	mem := collectMemory()
	if mem.SwapReady {
		if r.swapReady {
			if elapsed := t.Sub(r.prevSwapAt).Seconds(); elapsed > 0 {
				mem.SwapInRate, mem.SwapOutRate = applySwapRates(
					r.prevSwapIn, r.prevSwapOut, mem.SwapInBytes, mem.SwapOutBytes, elapsed,
				)
			}
		}
		r.prevSwapIn = mem.SwapInBytes
		r.prevSwapOut = mem.SwapOutBytes
		r.prevSwapAt = t
		r.swapReady = true
	}

	snap := Snapshot{
		Timestamp:   t,
		CPU:         r.cpu.collect(),
		Memory:      mem,
		Disks:       r.disk.collectSpace(),
		DiskIO:      r.disk.collectIO(),
		SDErrors:    r.sd.collect(),
		Network:     r.net.collect(),
		Load:        collectLoad(),
		Temperature: collectTemperature(),
		Throttle:    collectThrottle(),
		UptimeSec:   collectUptime(),
	}
	if r.docker != nil {
		snap.Containers = r.docker.collect()
	}
	if r.mount != nil {
		snap.Mounts = r.mount.collect()
	}
	if r.services != nil {
		snap.Services, snap.ServiceCycleTime = r.services.Snapshot()
	}
	return snap
}
