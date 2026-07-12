package collector

import (
	"testing"
	"time"
)

// gopsutil needs two calls to compute a delta, so the first tick is never Ready.
func TestCPUCollector_FirstTickNotReady(t *testing.T) {
	c := newCPUCollector()
	snap := c.collect()
	if snap.Ready {
		t.Error("expected Ready=false on first tick")
	}
}

func TestCPUCollector_SecondTickReady(t *testing.T) {
	c := newCPUCollector()
	c.collect() // warm-up
	time.Sleep(100 * time.Millisecond)
	snap := c.collect()
	if !snap.Ready {
		t.Error("expected Ready=true on second tick")
	}
	if snap.PercentTotal < 0 || snap.PercentTotal > 100 {
		t.Errorf("PercentTotal out of range [0,100]: %f", snap.PercentTotal)
	}
	for _, v := range []float64{snap.UserPercent, snap.SystemPercent, snap.IOWaitPercent, snap.IdlePercent} {
		if v < 0 || v > 100 {
			t.Errorf("CPU time breakdown value out of range: %f", v)
		}
	}
	sum := snap.UserPercent + snap.SystemPercent + snap.IOWaitPercent + snap.IdlePercent
	// sum may be less than 100 because we only track a subset of states
	if sum > 100.5 {
		t.Errorf("CPU time breakdown sum > 100: %f", sum)
	}
}

func TestMemory_SwapFieldsPresent(t *testing.T) {
	snap := collectMemory()
	if snap.TotalBytes == 0 {
		t.Error("expected non-zero TotalBytes")
	}
	if snap.SwapPercent < 0 || snap.SwapPercent > 100 {
		t.Errorf("SwapPercent out of range: %f", snap.SwapPercent)
	}
}

func TestLoad_InRange(t *testing.T) {
	snap := collectLoad()
	if snap.Load1 < 0 || snap.Load5 < 0 || snap.Load15 < 0 {
		t.Errorf("negative load average: 1m=%f 5m=%f 15m=%f", snap.Load1, snap.Load5, snap.Load15)
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
