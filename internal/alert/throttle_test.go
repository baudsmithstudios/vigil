package alert

import (
	"testing"

	"vigil/internal/collector"
	"vigil/internal/metric"
)

var (
	throttled = collector.ThrottleSnapshot{Available: true, UnderVoltage: true}
	cleared   = collector.ThrottleSnapshot{Available: true}
)

func TestThrottleHandler_FiresOnceWhileActive(t *testing.T) {
	var h ThrottleAlertHandler
	fired, _ := h.Evaluate(throttled)
	if len(fired) != 1 || fired[0].Name != metric.AlertThrottle || fired[0].Message != throttled.ActiveMessage() {
		t.Fatalf("expected one throttle alert, got %+v", fired)
	}
	if fired, _ := h.Evaluate(throttled); len(fired) != 0 {
		t.Fatalf("expected no re-fire while active, got %+v", fired)
	}
}

func TestThrottleHandler_ResolvesWhenCleared(t *testing.T) {
	var h ThrottleAlertHandler
	h.Evaluate(throttled)
	_, resolved := h.Evaluate(cleared)
	if len(resolved) != 1 || resolved[0].Name != metric.AlertThrottle {
		t.Fatalf("expected throttle resolved, got %+v", resolved)
	}
}

func TestThrottleHandler_UnavailableKeepsState(t *testing.T) {
	var h ThrottleAlertHandler
	h.Evaluate(throttled)
	if _, resolved := h.Evaluate(collector.ThrottleSnapshot{}); len(resolved) != 0 {
		t.Fatalf("expected a failed read not to resolve, got %+v", resolved)
	}
	if _, resolved := h.Evaluate(cleared); len(resolved) != 1 {
		t.Fatalf("expected alert still firing after failed read, got %+v", resolved)
	}
}

func TestThrottleHandler_Restore(t *testing.T) {
	var h ThrottleAlertHandler
	h.Restore([]State{{Name: metric.AlertThrottle}})
	if fired, _ := h.Evaluate(throttled); len(fired) != 0 {
		t.Fatalf("expected no re-fire after restore, got %+v", fired)
	}
	if _, resolved := h.Evaluate(cleared); len(resolved) != 1 {
		t.Fatalf("expected 1 resolved after restore, got %+v", resolved)
	}
}
