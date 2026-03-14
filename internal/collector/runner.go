package collector

import (
	"context"
	"time"

	"vigil/internal/checker"
	"vigil/internal/config"
)

// Runner polls all metrics on a fixed interval and emits Snapshots.
type Runner struct {
	interval time.Duration
	cpu      *cpuCollector
	disk     *diskCollector
	net      *netCollector
	docker   *dockerCollector         // nil if Docker monitoring is disabled
	mount    *mountCollector
	services *checker.ServiceChecker  // nil if no checks configured
	out      chan Snapshot
}

// New creates a Runner that emits snapshots on the returned channel.
// If dockerSocket is non-empty, Docker container monitoring is enabled.
// If mountChecks is non-empty, mount point monitoring is enabled.
// If services is non-nil, service health check results are included in snapshots.
func New(interval time.Duration, dockerSocket string, mountChecks []config.MountCheck, services *checker.ServiceChecker) (*Runner, <-chan Snapshot) {
	ch := make(chan Snapshot, 4)
	r := &Runner{
		interval: interval,
		cpu:      newCPUCollector(),
		disk:     newDiskCollector(),
		net:      newNetCollector(),
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

// Run starts the collection loop. It blocks until ctx is cancelled.
func (r *Runner) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	defer close(r.out)

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			snap := Snapshot{
				Timestamp:   t,
				CPU:         r.cpu.collect(),
				Memory:      collectMemory(),
				Disks:       r.disk.collectSpace(),
				DiskIO:      r.disk.collectIO(),
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
				snap.Services = r.services.Results()
			}
			select {
			case r.out <- snap:
			default:
				// Drop snapshot if consumer is slow rather than blocking.
			}
		}
	}
}
