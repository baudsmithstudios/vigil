package collector

import "testing"

func TestParseThrottleBitmask(t *testing.T) {
	tests := []struct {
		name     string
		raw      uint32
		wantAct  bool
		wantBoot bool
		wantStat string
	}{
		{"all clear", 0x0, false, false, "OK"},
		{"under-voltage now", 0x1, true, false, "THROTTLED"},
		{"freq capped now", 0x2, true, false, "THROTTLED"},
		{"throttled now", 0x4, true, false, "THROTTLED"},
		{"soft temp limit now", 0x8, true, false, "THROTTLED"},
		{"under-voltage since boot only", 0x10000, false, true, "WARN"},
		{"throttled since boot only", 0x40000, false, true, "WARN"},
		{"active and historical", 0x50005, true, true, "THROTTLED"},
		{"all flags set", 0xF000F, true, true, "THROTTLED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap := decodeThrottleBitmask(tt.raw)
			if snap.ActiveNow() != tt.wantAct {
				t.Errorf("ActiveNow() = %v, want %v", snap.ActiveNow(), tt.wantAct)
			}
			if snap.SinceBoot() != tt.wantBoot {
				t.Errorf("SinceBoot() = %v, want %v", snap.SinceBoot(), tt.wantBoot)
			}
			if snap.Status() != tt.wantStat {
				t.Errorf("Status() = %q, want %q", snap.Status(), tt.wantStat)
			}
		})
	}
}

func TestDecodeIndividualFlags(t *testing.T) {
	snap := decodeThrottleBitmask(0x50005)
	if !snap.UnderVoltage {
		t.Error("expected UnderVoltage set")
	}
	if !snap.Throttled {
		t.Error("expected Throttled set")
	}
	if !snap.UnderVoltageSinceBoot {
		t.Error("expected UnderVoltageSinceBoot set")
	}
	if !snap.ThrottledSinceBoot {
		t.Error("expected ThrottledSinceBoot set")
	}
	if snap.FreqCapped {
		t.Error("expected FreqCapped not set")
	}
	if !snap.Available {
		t.Error("expected Available set")
	}
}
