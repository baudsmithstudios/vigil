package alert

import (
	"time"

	"vigil/internal/collector"
	"vigil/internal/metric"
)

// ThrottleAlertHandler fires while the Pi reports an active throttle condition.
type ThrottleAlertHandler struct {
	firing bool
}

// Restore reloads a previously-firing alert so it survives restarts.
func (h *ThrottleAlertHandler) Restore(states []State) {
	for _, s := range states {
		if s.Name == metric.AlertThrottle {
			h.firing = true
		}
	}
}

// Evaluate ignores unavailable readings, so a failed read can't resolve the alert.
func (h *ThrottleAlertHandler) Evaluate(t collector.ThrottleSnapshot) (fired, resolved []State) {
	if !t.Available {
		return nil, nil
	}
	active := t.ActiveNow()
	switch {
	case active && !h.firing:
		fired = []State{{Name: metric.AlertThrottle, Message: t.ActiveMessage(), FiredAt: time.Now()}}
	case !active && h.firing:
		resolved = []State{{Name: metric.AlertThrottle, Message: "throttle condition cleared"}}
	}
	h.firing = active
	return fired, resolved
}
