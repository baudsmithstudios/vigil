package alert

import (
	"testing"
	"time"

	"vigil/internal/checker"
)

func TestServiceHandler_NoAlertWhenUp(t *testing.T) {
	h := NewServiceHandler(2)
	results := []checker.ServiceStatus{{Name: "web", Up: true, CheckedAt: time.Now()}}
	fired, resolved := h.Evaluate(results)
	if len(fired) != 0 || len(resolved) != 0 {
		t.Errorf("expected no alerts, got fired=%d resolved=%d", len(fired), len(resolved))
	}
}

func TestServiceHandler_FiresAfterConsecutiveFailures(t *testing.T) {
	h := NewServiceHandler(2)
	down := []checker.ServiceStatus{{Name: "web", Up: false, CheckedAt: time.Now()}}
	fired, _ := h.Evaluate(down)
	if len(fired) != 0 {
		t.Error("expected no fire on first failure")
	}
	fired, _ = h.Evaluate(down)
	if len(fired) != 1 {
		t.Fatalf("expected 1 fire after 2 failures, got %d", len(fired))
	}
	if fired[0].Name != "service_down:web" {
		t.Errorf("unexpected alert name: %q", fired[0].Name)
	}
}

func TestServiceHandler_NoReFireWhileFiring(t *testing.T) {
	h := NewServiceHandler(2)
	down := []checker.ServiceStatus{{Name: "web", Up: false, CheckedAt: time.Now()}}
	h.Evaluate(down)
	h.Evaluate(down) // fires
	fired, _ := h.Evaluate(down)
	if len(fired) != 0 {
		t.Error("expected no re-fire while already firing")
	}
}

func TestServiceHandler_ResolvesOnRecovery(t *testing.T) {
	h := NewServiceHandler(2)
	down := []checker.ServiceStatus{{Name: "web", Up: false, CheckedAt: time.Now()}}
	h.Evaluate(down)
	h.Evaluate(down) // fires
	up := []checker.ServiceStatus{{Name: "web", Up: true, CheckedAt: time.Now()}}
	_, resolved := h.Evaluate(up)
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved, got %d", len(resolved))
	}
}

func TestServiceHandler_DegradedResetsOnRecovery(t *testing.T) {
	h := NewServiceHandler(3)
	down := []checker.ServiceStatus{{Name: "web", Up: false, CheckedAt: time.Now()}}
	h.Evaluate(down) // count=1
	up := []checker.ServiceStatus{{Name: "web", Up: true, CheckedAt: time.Now()}}
	fired, resolved := h.Evaluate(up)
	if len(fired) != 0 || len(resolved) != 0 {
		t.Error("expected silent reset from degraded")
	}
	// Should need full threshold again.
	h.Evaluate(down)
	fired, _ = h.Evaluate(down)
	if len(fired) != 0 {
		t.Error("expected no fire — threshold should restart from 0")
	}
	fired, _ = h.Evaluate(down) // 3rd failure
	if len(fired) != 1 {
		t.Error("expected fire after 3 consecutive failures")
	}
}

func TestServiceHandler_Dismiss(t *testing.T) {
	h := NewServiceHandler(2)
	down := []checker.ServiceStatus{{Name: "web", Up: false, CheckedAt: time.Now()}}
	h.Evaluate(down)
	h.Evaluate(down) // fires
	h.Dismiss("service_down:web")
	fired, _ := h.Evaluate(down)
	if len(fired) != 0 {
		t.Error("expected no immediate re-fire after dismiss")
	}
	fired, _ = h.Evaluate(down)
	if len(fired) != 1 {
		t.Error("expected re-fire after reaching threshold post-dismiss")
	}
}

func TestServiceHandler_Restore(t *testing.T) {
	h := NewServiceHandler(2)
	h.Restore([]State{{Name: "service_down:web"}})
	down := []checker.ServiceStatus{{Name: "web", Up: false, CheckedAt: time.Now()}}
	fired, _ := h.Evaluate(down)
	if len(fired) != 0 {
		t.Error("expected no re-fire after restore")
	}
	up := []checker.ServiceStatus{{Name: "web", Up: true, CheckedAt: time.Now()}}
	_, resolved := h.Evaluate(up)
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved after restore, got %d", len(resolved))
	}
}
