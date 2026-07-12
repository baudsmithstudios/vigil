package collector

import (
	"testing"

	"github.com/shirou/gopsutil/v3/net"
)

func TestApplyRates_ComputesDropAndErrorRates(t *testing.T) {
	prev := net.IOCountersStat{
		BytesSent: 1000,
		BytesRecv: 2000,
		Dropin:    10,
		Dropout:   5,
		Errin:     2,
		Errout:    1,
	}
	curr := net.IOCountersStat{
		BytesSent: 1200,
		BytesRecv: 2400,
		Dropin:    20,
		Dropout:   15,
		Errin:     7,
		Errout:    4,
	}
	elapsed := 2.0

	snap := NetSnapshot{}
	applyRates(&snap, prev, curr, elapsed)

	// (20+15) - (10+5) = 20 drops over 2s = 10/s
	if snap.DropRate != 10.0 {
		t.Errorf("expected DropRate 10.0, got %f", snap.DropRate)
	}
	// (7+4) - (2+1) = 8 errors over 2s = 4/s
	if snap.ErrRate != 4.0 {
		t.Errorf("expected ErrRate 4.0, got %f", snap.ErrRate)
	}

	// (1200-1000) / 2 = 100 bytes/s
	if snap.SendRate != 100.0 {
		t.Errorf("expected SendRate 100.0, got %f", snap.SendRate)
	}
	// (2400-2000) / 2 = 200 bytes/s
	if snap.RecvRate != 200.0 {
		t.Errorf("expected RecvRate 200.0, got %f", snap.RecvRate)
	}
}

func TestApplyRates_CounterRollover(t *testing.T) {
	// Counters that appear to have gone backward (rollover) should produce 0 rate, not negative.
	prev := net.IOCountersStat{Dropin: 1000, Dropout: 1000}
	curr := net.IOCountersStat{Dropin: 10, Dropout: 10}

	snap := NetSnapshot{}
	applyRates(&snap, prev, curr, 1.0)

	if snap.DropRate != 0 {
		t.Errorf("expected DropRate 0 on rollover, got %f", snap.DropRate)
	}
}
