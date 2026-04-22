package collector

import "testing"

func TestApplySwapRates_ComputesRates(t *testing.T) {
	inRate, outRate := applySwapRates(4096, 8192, 12288, 16384, 2.0)

	if inRate != 4096.0 {
		t.Errorf("expected inRate 4096.0, got %f", inRate)
	}
	if outRate != 4096.0 {
		t.Errorf("expected outRate 4096.0, got %f", outRate)
	}
}

func TestApplySwapRates_RolloverProtected(t *testing.T) {
	inRate, outRate := applySwapRates(100, 100, 10, 20, 1.0)

	if inRate != 0 {
		t.Errorf("expected inRate 0 on rollover, got %f", inRate)
	}
	if outRate != 0 {
		t.Errorf("expected outRate 0 on rollover, got %f", outRate)
	}
}
