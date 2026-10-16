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

func TestEngine_FiresOnceUntilResolved(t *testing.T) {
	rules := []config.Alert{
		{Metric: "cpu_percent", Threshold: 50.0, Above: true, Message: "CPU high"},
	}
	eng := New(rules)

	fired, _ := eng.Evaluate(map[string]float64{"cpu_percent": 80.0})
	if len(fired) != 1 {
		t.Fatalf("expected 1 alert on first fire, got %d", len(fired))
	}

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
	eng.Restore([]State{{Name: "cpu_percent", Message: "CPU high"}})

	fired, _ := eng.Evaluate(map[string]float64{"cpu_percent": 80.0})
	if len(fired) != 0 {
		t.Errorf("expected 0 alerts after restore (already firing), got %d", len(fired))
	}

	_, resolved := eng.Evaluate(map[string]float64{"cpu_percent": 30.0})
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved alert, got %d", len(resolved))
	}
}

func TestEngine_DismissRefiresAfterFullSustainWindow(t *testing.T) {
	rules := []config.Alert{
		{Metric: "cpu_percent", Threshold: 50.0, Above: true, Message: "CPU high", SustainedTicks: 2},
	}
	eng := New(rules)
	high := map[string]float64{"cpu_percent": 80.0}
	eng.Evaluate(high)
	if fired, _ := eng.Evaluate(high); len(fired) != 1 {
		t.Fatalf("setup: expected fire on sustained tick, got %d", len(fired))
	}

	eng.Dismiss("cpu_percent")
	if fired, _ := eng.Evaluate(high); len(fired) != 0 {
		t.Fatalf("expected no immediate re-fire after dismiss, got %d", len(fired))
	}
	if fired, _ := eng.Evaluate(high); len(fired) != 1 {
		t.Fatalf("expected re-fire after a full sustain window post-dismiss, got %d", len(fired))
	}
}

func TestEngine_DeltaAlert(t *testing.T) {
	rules := []config.Alert{
		{Metric: "cpu_percent", DeltaThreshold: 30.0},
	}
	eng := New(rules)

	fired, _ := eng.Evaluate(map[string]float64{"cpu_percent": 20.0})
	if len(fired) != 0 {
		t.Fatalf("expected 0 alerts on first tick, got %d", len(fired))
	}

	fired, resolved := eng.Evaluate(map[string]float64{"cpu_percent": 70.0})
	if len(fired) != 1 {
		t.Fatalf("expected 1 delta alert, got %d", len(fired))
	}
	if fired[0].Name != "cpu_percent:delta" {
		t.Errorf("expected name cpu_percent:delta, got %q", fired[0].Name)
	}
	if len(resolved) != 0 {
		t.Fatalf("delta alert resolved on the same tick it fired: %v", resolved)
	}

	_, resolved = eng.Evaluate(map[string]float64{"cpu_percent": 72.0})
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved delta alert, got %d", len(resolved))
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
		{Metric: "cpu_percent", Above: true, DeltaThreshold: 30.0},
	}
	eng := New(rules)

	fired, _ := eng.Evaluate(map[string]float64{"cpu_percent": 50.0})
	if len(fired) != 0 {
		t.Fatalf("delta-only rule produced a threshold alert: %+v", fired)
	}
}

func TestEngine_Resolves(t *testing.T) {
	rules := []config.Alert{
		{Metric: "cpu_percent", Threshold: 50.0, Above: true, Message: "CPU high"},
	}
	eng := New(rules)
	eng.Evaluate(map[string]float64{"cpu_percent": 80.0})

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
}

func TestEngine_PrefixIndependentResolution(t *testing.T) {
	rules := []config.Alert{
		{Metric: "disk_percent", Threshold: 90.0, Above: true, Message: "Disk high"},
	}
	eng := New(rules)

	fired, _ := eng.Evaluate(map[string]float64{
		"disk_percent:/":         95.0,
		"disk_percent:/mnt/data": 92.0,
	})
	if len(fired) != 2 {
		t.Fatalf("expected 2 alerts fired, got %d", len(fired))
	}

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

	eng.Evaluate(map[string]float64{
		"disk_percent:/":         20.0,
		"disk_percent:/mnt/data": 50.0,
	})

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

func TestEngine_SustainedTicks_DelaysFiring(t *testing.T) {
	rules := []config.Alert{
		{Metric: "net_drops", Threshold: 0, Above: true, Message: "Drops detected", SustainedTicks: 3},
	}
	eng := New(rules)
	vals := map[string]float64{"net_drops:eth0": 5.0}

	if fired, _ := eng.Evaluate(vals); len(fired) != 0 {
		t.Fatalf("tick 1: expected 0 alerts, got %d", len(fired))
	}
	if fired, _ := eng.Evaluate(vals); len(fired) != 0 {
		t.Fatalf("tick 2: expected 0 alerts, got %d", len(fired))
	}
	if fired, _ := eng.Evaluate(vals); len(fired) != 1 {
		t.Fatalf("tick 3: expected 1 alert, got %d", len(fired))
	}
}

func TestEngine_SustainedTicks_ResetsOnDrop(t *testing.T) {
	rules := []config.Alert{
		{Metric: "net_drops", Threshold: 0, Above: true, Message: "Drops detected", SustainedTicks: 3},
	}
	eng := New(rules)
	above := map[string]float64{"net_drops:eth0": 5.0}
	below := map[string]float64{"net_drops:eth0": 0.0}

	eng.Evaluate(above)
	eng.Evaluate(above)
	eng.Evaluate(below)

	if fired, _ := eng.Evaluate(above); len(fired) != 0 {
		t.Fatalf("expected 0 alerts after counter reset, got %d", len(fired))
	}
	if fired, _ := eng.Evaluate(above); len(fired) != 0 {
		t.Fatalf("expected 0 alerts on tick 2 after reset, got %d", len(fired))
	}
	if fired, _ := eng.Evaluate(above); len(fired) != 1 {
		t.Fatalf("expected 1 alert on 3rd sustained tick, got %d", len(fired))
	}
}

func TestEngine_SpecificRuleShadowsGeneric(t *testing.T) {
	rules := []config.Alert{
		{Metric: "disk_latency_ms", Threshold: 50.0, Above: true, Message: "Disk latency high"},
		{Metric: "disk_latency_ms:mmcblk0", Threshold: 200.0, Above: true, Message: "SD card latency high"},
	}
	eng := New(rules)

	// mmcblk0 sits between the generic and specific thresholds.
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

	fired, _ := eng.Evaluate(map[string]float64{
		"disk_latency_ms:mmcblk0": 250.0,
	})
	if len(fired) != 1 {
		t.Fatalf("expected 1 alert from specific rule, got %d: %+v", len(fired), fired)
	}
	// Both rules emit the same key, so only the message shows which one fired.
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
	eng.Evaluate(map[string]float64{"disk_latency_ms:mmcblk0": 250.0})

	// Drops between the generic and specific thresholds.
	fired, resolved := eng.Evaluate(map[string]float64{"disk_latency_ms:mmcblk0": 100.0})
	if len(resolved) != 1 || resolved[0].Name != "disk_latency_ms:mmcblk0" {
		t.Fatalf("expected specific rule to resolve, got %+v", resolved)
	}
	if len(fired) != 0 {
		t.Errorf("generic rule must not fire on shadowed key, got %+v", fired)
	}
}
