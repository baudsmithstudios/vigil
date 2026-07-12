package alert

import (
	"testing"

	"vigil/internal/config"
)

func TestEngine_Fires(t *testing.T) {
	rules := []config.Alert{
		{Metric: "cpu_percent", Threshold: 50.0, Above: true, Message: "CPU high"},
	}
	eng := New(rules)

	fired, _ := eng.Evaluate(map[string]float64{"cpu_percent": 75.0})
	if len(fired) != 1 {
		t.Fatalf("expected 1 alert fired, got %d", len(fired))
	}
	if fired[0].Name != "cpu_percent" {
		t.Errorf("unexpected alert name: %q", fired[0].Name)
	}
}

func TestEngine_DoesNotFireBelowThreshold(t *testing.T) {
	rules := []config.Alert{
		{Metric: "cpu_percent", Threshold: 90.0, Above: true, Message: "CPU high"},
	}
	eng := New(rules)

	fired, _ := eng.Evaluate(map[string]float64{"cpu_percent": 40.0})
	if len(fired) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(fired))
	}
}

func TestEngine_FiresBelow(t *testing.T) {
	rules := []config.Alert{
		{Metric: "cpu_temp", Threshold: 10.0, Above: false, Message: "temp too low"},
	}
	eng := New(rules)

	fired, _ := eng.Evaluate(map[string]float64{"cpu_temp": 5.0})
	if len(fired) != 1 {
		t.Fatalf("expected 1 alert fired, got %d", len(fired))
	}
}

func TestEngine_FiresOnceUntilResolved(t *testing.T) {
	rules := []config.Alert{
		{Metric: "cpu_percent", Threshold: 50.0, Above: true, Message: "CPU high"},
	}
	eng := New(rules)

	// First evaluation while above threshold → fires
	fired, _ := eng.Evaluate(map[string]float64{"cpu_percent": 80.0})
	if len(fired) != 1 {
		t.Fatalf("expected 1 alert on first fire, got %d", len(fired))
	}

	// Second evaluation still above threshold → does NOT re-fire
	fired, _ = eng.Evaluate(map[string]float64{"cpu_percent": 80.0})
	if len(fired) != 0 {
		t.Errorf("expected 0 alerts while already firing, got %d", len(fired))
	}
}

func TestEngine_Restore(t *testing.T) {
	rules := []config.Alert{
		{Metric: "cpu_percent", Threshold: 50.0, Above: true, Message: "CPU high"},
	}
	eng := New(rules)

	// Restore a previously-firing alert.
	eng.Restore([]State{{Name: "cpu_percent", Message: "CPU high"}})

	// Should not re-fire since it's already in firing state.
	fired, _ := eng.Evaluate(map[string]float64{"cpu_percent": 80.0})
	if len(fired) != 0 {
		t.Errorf("expected 0 alerts after restore (already firing), got %d", len(fired))
	}

	// Should resolve normally.
	_, resolved := eng.Evaluate(map[string]float64{"cpu_percent": 30.0})
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved alert, got %d", len(resolved))
	}
}

func TestEngine_DeltaAlert(t *testing.T) {
	rules := []config.Alert{
		{Metric: "cpu_percent", DeltaThreshold: 30.0},
	}
	eng := New(rules)

	// First tick — no previous value, no fire.
	fired, _ := eng.Evaluate(map[string]float64{"cpu_percent": 20.0})
	if len(fired) != 0 {
		t.Fatalf("expected 0 alerts on first tick, got %d", len(fired))
	}

	// Second tick — spike of 50 (20 → 70), exceeds delta_threshold of 30.
	fired, _ = eng.Evaluate(map[string]float64{"cpu_percent": 70.0})
	if len(fired) != 1 {
		t.Fatalf("expected 1 delta alert, got %d", len(fired))
	}
	if fired[0].Name != "cpu_percent:delta" {
		t.Errorf("expected name cpu_percent:delta, got %q", fired[0].Name)
	}

	// Third tick — small change (70 → 72), delta resolves.
	_, resolved := eng.Evaluate(map[string]float64{"cpu_percent": 72.0})
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved delta alert, got %d", len(resolved))
	}
}

func TestEngine_DeltaAlertDoesNotResolveOnFiringTick(t *testing.T) {
	rules := []config.Alert{
		{Metric: "cpu_percent", DeltaThreshold: 30.0},
	}
	eng := New(rules)

	eng.Evaluate(map[string]float64{"cpu_percent": 20.0})

	fired, resolved := eng.Evaluate(map[string]float64{"cpu_percent": 70.0})
	if len(fired) != 1 {
		t.Fatalf("expected 1 delta alert, got %d", len(fired))
	}
	if len(resolved) != 0 {
		t.Fatalf("delta alert resolved on the same tick it fired: %v", resolved)
	}
}

func TestEngine_ZeroThreshold(t *testing.T) {
	rules := []config.Alert{
		{Metric: "balance", Threshold: 0, Above: false, Message: "balance negative"},
	}
	eng := New(rules)

	fired, _ := eng.Evaluate(map[string]float64{"balance": -5.0})
	if len(fired) != 1 {
		t.Fatalf("expected 1 alert for value below zero threshold, got %d", len(fired))
	}
}

func TestEngine_DeltaOnlyDoesNotThresholdFire(t *testing.T) {
	rules := []config.Alert{
		{Metric: "cpu_percent", DeltaThreshold: 30.0},
	}
	eng := New(rules)

	// Should not fire a threshold alert even though value (50) is above zero threshold.
	fired, _ := eng.Evaluate(map[string]float64{"cpu_percent": 50.0})
	if len(fired) != 0 {
		t.Fatalf("expected 0 alerts for delta-only rule on first tick, got %d", len(fired))
	}

	// Small change — still no fire.
	fired, _ = eng.Evaluate(map[string]float64{"cpu_percent": 52.0})
	for _, f := range fired {
		if f.Name == "cpu_percent" {
			t.Error("delta-only rule should not produce a threshold alert")
		}
	}
}

func TestEngine_Resolves(t *testing.T) {
	rules := []config.Alert{
		{Metric: "cpu_percent", Threshold: 50.0, Above: true, Message: "CPU high"},
	}
	eng := New(rules)
	eng.Evaluate(map[string]float64{"cpu_percent": 80.0}) // fires

	_, resolved := eng.Evaluate(map[string]float64{"cpu_percent": 30.0})
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved alert, got %d", len(resolved))
	}
	if resolved[0].Name != "cpu_percent" {
		t.Errorf("unexpected resolved name: %q", resolved[0].Name)
	}
}

func TestEngine_PrefixMatchFiresPerMount(t *testing.T) {
	rules := []config.Alert{
		{Metric: "disk_percent", Threshold: 90.0, Above: true, Message: "Disk usage above 90%"},
	}
	eng := New(rules)

	fired, _ := eng.Evaluate(map[string]float64{
		"disk_percent:/":         95.0,
		"disk_percent:/mnt/data": 20.0,
	})
	if len(fired) != 1 {
		t.Fatalf("expected 1 alert fired, got %d", len(fired))
	}
	if fired[0].Name != "disk_percent:/" {
		t.Errorf("expected alert name %q, got %q", "disk_percent:/", fired[0].Name)
	}
	if fired[0].Message != "Disk usage above 90% (/)" {
		t.Errorf("expected message with mount suffix, got %q", fired[0].Message)
	}
}

func TestEngine_PrefixExactMountMatch(t *testing.T) {
	rules := []config.Alert{
		{Metric: "disk_percent:/mnt/data", Threshold: 80.0, Above: true, Message: "Data disk high"},
	}
	eng := New(rules)

	fired, _ := eng.Evaluate(map[string]float64{
		"disk_percent:/":         95.0,
		"disk_percent:/mnt/data": 85.0,
	})
	if len(fired) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(fired))
	}
	if fired[0].Name != "disk_percent:/mnt/data" {
		t.Errorf("expected alert for /mnt/data only, got %q", fired[0].Name)
	}
	if fired[0].Message != "Data disk high" {
		t.Errorf("exact match should not append suffix, got %q", fired[0].Message)
	}
}

func TestEngine_PrefixIndependentResolution(t *testing.T) {
	rules := []config.Alert{
		{Metric: "disk_percent", Threshold: 90.0, Above: true, Message: "Disk high"},
	}
	eng := New(rules)

	// Both mounts fire.
	fired, _ := eng.Evaluate(map[string]float64{
		"disk_percent:/":         95.0,
		"disk_percent:/mnt/data": 92.0,
	})
	if len(fired) != 2 {
		t.Fatalf("expected 2 alerts fired, got %d", len(fired))
	}

	// Root drops below threshold, data stays above.
	_, resolved := eng.Evaluate(map[string]float64{
		"disk_percent:/":         80.0,
		"disk_percent:/mnt/data": 92.0,
	})
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved, got %d", len(resolved))
	}
	if resolved[0].Name != "disk_percent:/" {
		t.Errorf("expected root to resolve, got %q", resolved[0].Name)
	}
}

func TestEngine_PrefixDelta(t *testing.T) {
	rules := []config.Alert{
		{Metric: "disk_percent", DeltaThreshold: 30.0},
	}
	eng := New(rules)

	// First tick — establish baseline.
	eng.Evaluate(map[string]float64{
		"disk_percent:/":         20.0,
		"disk_percent:/mnt/data": 50.0,
	})

	// Second tick — root spikes, data is stable.
	fired, _ := eng.Evaluate(map[string]float64{
		"disk_percent:/":         60.0,
		"disk_percent:/mnt/data": 52.0,
	})
	if len(fired) != 1 {
		t.Fatalf("expected 1 delta alert, got %d", len(fired))
	}
	if fired[0].Name != "disk_percent:/:delta" {
		t.Errorf("expected delta alert for root, got %q", fired[0].Name)
	}
}

func TestEngine_PrefixRestore(t *testing.T) {
	rules := []config.Alert{
		{Metric: "disk_percent", Threshold: 90.0, Above: true, Message: "Disk high"},
	}
	eng := New(rules)
	eng.Restore([]State{{Name: "disk_percent:/", Message: "Disk high (/)"}})

	// Should not re-fire since already restored.
	fired, _ := eng.Evaluate(map[string]float64{"disk_percent:/": 95.0})
	if len(fired) != 0 {
		t.Errorf("expected 0 alerts after restore, got %d", len(fired))
	}

	// Should resolve when value drops.
	_, resolved := eng.Evaluate(map[string]float64{"disk_percent:/": 80.0})
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved, got %d", len(resolved))
	}
}

func TestEngine_SustainedTicks_DelaysFiring(t *testing.T) {
	rules := []config.Alert{
		{Metric: "net_drops", Threshold: 0, Above: true, Message: "Drops detected", SustainedTicks: 3},
	}
	eng := New(rules)

	vals := map[string]float64{"net_drops:eth0": 5.0}

	// Ticks 1 and 2: above threshold but should NOT fire yet.
	if fired, _ := eng.Evaluate(vals); len(fired) != 0 {
		t.Fatalf("tick 1: expected 0 alerts, got %d", len(fired))
	}
	if fired, _ := eng.Evaluate(vals); len(fired) != 0 {
		t.Fatalf("tick 2: expected 0 alerts, got %d", len(fired))
	}

	// Tick 3: sustained threshold reached, should fire.
	fired, _ := eng.Evaluate(vals)
	if len(fired) != 1 {
		t.Fatalf("tick 3: expected 1 alert, got %d", len(fired))
	}
	if fired[0].Name != "net_drops:eth0" {
		t.Errorf("unexpected alert name: %q", fired[0].Name)
	}
}

func TestEngine_SustainedTicks_ResetsOnDrop(t *testing.T) {
	rules := []config.Alert{
		{Metric: "net_drops", Threshold: 0, Above: true, Message: "Drops detected", SustainedTicks: 3},
	}
	eng := New(rules)

	above := map[string]float64{"net_drops:eth0": 5.0}
	below := map[string]float64{"net_drops:eth0": 0.0}

	// 2 ticks above, then 1 below — counter should reset.
	eng.Evaluate(above)
	eng.Evaluate(above)
	eng.Evaluate(below)

	// 2 more ticks above — should NOT fire (counter reset).
	if fired, _ := eng.Evaluate(above); len(fired) != 0 {
		t.Fatalf("expected 0 alerts after counter reset, got %d", len(fired))
	}
	if fired, _ := eng.Evaluate(above); len(fired) != 0 {
		t.Fatalf("expected 0 alerts on tick 2 after reset, got %d", len(fired))
	}

	// 3rd tick above after reset — NOW it should fire.
	fired, _ := eng.Evaluate(above)
	if len(fired) != 1 {
		t.Fatalf("expected 1 alert on 3rd sustained tick, got %d", len(fired))
	}
}

func TestEngine_SustainedTicks_ZeroMeansImmediate(t *testing.T) {
	rules := []config.Alert{
		{Metric: "cpu_percent", Threshold: 90.0, Above: true, Message: "CPU high", SustainedTicks: 0},
	}
	eng := New(rules)

	// Should fire immediately (backward compatible).
	fired, _ := eng.Evaluate(map[string]float64{"cpu_percent": 95.0})
	if len(fired) != 1 {
		t.Fatalf("expected immediate fire with SustainedTicks=0, got %d", len(fired))
	}
}

func TestEngine_PrefixZeroMatch(t *testing.T) {
	rules := []config.Alert{
		{Metric: "disk_percent", Threshold: 90.0, Above: true, Message: "Disk high"},
	}
	eng := New(rules)

	// No disk_percent keys in values at all.
	fired, _ := eng.Evaluate(map[string]float64{"cpu_percent": 50.0})
	if len(fired) != 0 {
		t.Errorf("expected 0 alerts with no matching keys, got %d", len(fired))
	}
}

func TestEngine_SpecificRuleShadowsGeneric(t *testing.T) {
	rules := []config.Alert{
		{Metric: "disk_latency_ms", Threshold: 50.0, Above: true, Message: "Disk latency high"},
		{Metric: "disk_latency_ms:mmcblk0", Threshold: 200.0, Above: true, Message: "SD card latency high"},
	}
	eng := New(rules)

	// mmcblk0 at 100ms: above generic (50) but below specific (200). Must not fire.
	// sda at 75ms: above generic, no specific rule. Must fire from generic.
	fired, _ := eng.Evaluate(map[string]float64{
		"disk_latency_ms:mmcblk0": 100.0,
		"disk_latency_ms:sda":     75.0,
	})
	if len(fired) != 1 {
		t.Fatalf("expected exactly 1 alert (sda only), got %d: %+v", len(fired), fired)
	}
	if fired[0].Name != "disk_latency_ms:sda" {
		t.Errorf("expected sda to fire, got %q", fired[0].Name)
	}
}

func TestEngine_SpecificRuleStillFiresWhenGenericShadowed(t *testing.T) {
	rules := []config.Alert{
		{Metric: "disk_latency_ms", Threshold: 50.0, Above: true, Message: "Disk latency high"},
		{Metric: "disk_latency_ms:mmcblk0", Threshold: 200.0, Above: true, Message: "SD card latency high"},
	}
	eng := New(rules)

	// mmcblk0 at 250ms: above its specific threshold. Generic must not double-fire.
	fired, _ := eng.Evaluate(map[string]float64{
		"disk_latency_ms:mmcblk0": 250.0,
	})
	if len(fired) != 1 {
		t.Fatalf("expected 1 alert from specific rule, got %d: %+v", len(fired), fired)
	}
	if fired[0].Name != "disk_latency_ms:mmcblk0" {
		t.Errorf("expected specific rule fire, got %q", fired[0].Name)
	}
	if fired[0].Message != "SD card latency high" {
		t.Errorf("expected specific rule message, got %q", fired[0].Message)
	}
}

func TestEngine_ShadowedKeyDoesNotFireGenericOnResolve(t *testing.T) {
	rules := []config.Alert{
		{Metric: "disk_latency_ms", Threshold: 50.0, Above: true, Message: "Disk latency high"},
		{Metric: "disk_latency_ms:mmcblk0", Threshold: 200.0, Above: true, Message: "SD card latency high"},
	}
	eng := New(rules)

	// mmcblk0 fires specific at 250ms.
	eng.Evaluate(map[string]float64{"disk_latency_ms:mmcblk0": 250.0})

	// Drops to 100ms: below specific (200) but above generic (50).
	// Specific must resolve; generic must NOT fire on the shadowed key.
	fired, resolved := eng.Evaluate(map[string]float64{"disk_latency_ms:mmcblk0": 100.0})
	if len(resolved) != 1 || resolved[0].Name != "disk_latency_ms:mmcblk0" {
		t.Fatalf("expected specific rule to resolve, got %+v", resolved)
	}
	if len(fired) != 0 {
		t.Errorf("generic rule must not fire on shadowed key, got %+v", fired)
	}
}
