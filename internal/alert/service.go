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
	serviceOK serviceAlertStatus = iota
	serviceFiring
)

type serviceState struct {
	status       serviceAlertStatus
	failureCount int
}

// ServiceAlertHandler fires alerts after N consecutive service-check failures.
type ServiceAlertHandler struct {
	mu        sync.Mutex
	states    map[string]*serviceState
	threshold int
}

func NewServiceHandler(failuresBeforeAlert int) *ServiceAlertHandler {
	return &ServiceAlertHandler{
		states:    make(map[string]*serviceState),
		threshold: failuresBeforeAlert,
	}
}

// Restore reloads previously-firing alerts so they survive restarts.
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

// Dismiss clears state so re-firing requires another full threshold of failures.
func (h *ServiceAlertHandler) Dismiss(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if svcName, ok := strings.CutPrefix(name, metric.PrefixServiceDown); ok {
		if st, exists := h.states[svcName]; exists {
			st.status = serviceOK
			st.failureCount = 0
		}
	}
}

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
			case serviceOK:
				st.failureCount++
				if st.failureCount >= h.threshold {
					st.status = serviceFiring
					fired = append(fired, State{
						Name:    metric.PrefixServiceDown + r.Name,
						Message: fmt.Sprintf("%s is not reachable", r.Name),
						FiredAt: time.Now(),
					})
				}
			case serviceFiring:
			}
		}
	}
	return fired, resolved
}

// Caller must hold h.mu.
func (h *ServiceAlertHandler) getOrCreate(name string) *serviceState {
	if st, ok := h.states[name]; ok {
		return st
	}
	st := &serviceState{}
	h.states[name] = st
	return st
}
