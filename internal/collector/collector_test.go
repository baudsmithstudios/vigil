package collector

import (
	"testing"
	"time"
)

func TestCPUCollector_FirstTickNotReady(t *testing.T) {
	c := newCPUCollector()
	snap := c.collect()
	if snap.Ready {
		t.Error("expected Ready=false on first tick")
	}
}

func TestCPUCollector_SecondTickReady(t *testing.T) {
	c := newCPUCollector()
	c.collect()
	time.Sleep(100 * time.Millisecond)
	snap := c.collect()
	if !snap.Ready {
		t.Error("expected Ready=true on second tick")
	}
	if snap.PercentTotal < 0 || snap.PercentTotal > 100 {
		t.Errorf("PercentTotal out of range [0,100]: %f", snap.PercentTotal)
	}
	// Only a subset of CPU states is tracked, so the sum can fall short of 100.
	sum := snap.UserPercent + snap.SystemPercent + snap.IOWaitPercent + snap.IdlePercent
	if sum > 100.5 {
		t.Errorf("CPU time breakdown sum > 100: %f", sum)
	}
}

func TestNetCollector_FirstTickZeroRates(t *testing.T) {
	nc := newNetCollector()
	snaps := nc.collect()
	for _, s := range snaps {
		if s.SendRate != 0 || s.RecvRate != 0 {
			t.Errorf("expected zero rates on first tick for %q, got send=%f recv=%f",
				s.Interface, s.SendRate, s.RecvRate)
		}
	}
}

func TestDiskIOCollector_FirstTickZeroRates(t *testing.T) {
	dc := newDiskCollector()
	snaps := dc.collectIO()
	for _, s := range snaps {
		if s.ReadRate != 0 || s.WriteRate != 0 {
			t.Errorf("expected zero rates on first tick for %q, got read=%f write=%f",
				s.Device, s.ReadRate, s.WriteRate)
		}
	}
}
