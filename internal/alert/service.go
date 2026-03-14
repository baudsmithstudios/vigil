package alert

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"vigil/internal/checker"
	"vigil/internal/metric"
)

type serviceAlertStatus int

const (
	serviceOK       serviceAlertStatus = iota
	serviceDegraded
	serviceFiring
)

type serviceState struct {
	status       serviceAlertStatus
	failureCount int
}

// ServiceAlertHandler tracks consecutive service failures and fires alerts when a threshold is met.
type ServiceAlertHandler struct {
	mu        sync.Mutex
	states    map[string]*serviceState
	threshold int
}

// NewServiceHandler returns a ServiceAlertHandler that fires after failuresBeforeAlert consecutive failures.
func NewServiceHandler(failuresBeforeAlert int) *ServiceAlertHandler {
	return &ServiceAlertHandler{
		states:    make(map[string]*serviceState),
		threshold: failuresBeforeAlert,
	}
}

// Restore loads previously-firing alerts into the handler so they survive restarts.
func (h *ServiceAlertHandler) Restore(states []State) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, s := range states {
		if name, ok := strings.CutPrefix(s.Name, metric.PrefixServiceDown); ok {
			st := h.getOrCreate(name)
			st.status = serviceFiring
			st.failureCount = h.threshold
		}
	}
}

// Dismiss resets the state for the named alert, allowing it to re-fire after reaching the threshold again.
func (h *ServiceAlertHandler) Dismiss(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if svcName, ok := strings.CutPrefix(name, metric.PrefixServiceDown); ok {
		if st, exists := h.states[svcName]; exists {
			st.status = serviceDegraded
			st.failureCount = 0
		}
	}
}

// Evaluate runs the state machine for each service and returns newly fired and resolved alerts.
func (h *ServiceAlertHandler) Evaluate(results []checker.ServiceStatus) (fired, resolved []State) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range results {
		st := h.getOrCreate(r.Name)
		if r.Up {
			switch st.status {
			case serviceFiring:
				resolved = append(resolved, State{
					Name:    metric.PrefixServiceDown + r.Name,
					Message: fmt.Sprintf("%s is reachable again", r.Name),
					FiredAt: time.Now(),
				})
			}
			st.status = serviceOK
			st.failureCount = 0
		} else {
			switch st.status {
			case serviceOK, serviceDegraded:
				st.failureCount++
				if st.failureCount >= h.threshold {
					st.status = serviceFiring
					fired = append(fired, State{
						Name:    metric.PrefixServiceDown + r.Name,
						Message: fmt.Sprintf("%s is not reachable", r.Name),
						FiredAt: time.Now(),
					})
				} else {
					st.status = serviceDegraded
				}
			case serviceFiring:
				// Already firing — no-op.
			}
		}
	}
	return fired, resolved
}

// getOrCreate returns the serviceState for the given name, creating it if needed.
// Caller must hold h.mu.
func (h *ServiceAlertHandler) getOrCreate(name string) *serviceState {
	if st, ok := h.states[name]; ok {
		return st
	}
	st := &serviceState{}
	h.states[name] = st
	return st
}
