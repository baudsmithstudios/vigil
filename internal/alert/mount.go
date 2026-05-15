package alert

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"vigil/internal/collector"
	"vigil/internal/metric"
)

type mountAlertStatus int

const (
	mountOK         mountAlertStatus = iota
	mountDebouncing
	mountFiring
)

type mountState struct {
	status      mountAlertStatus
	debounceLeft int

	flapCount  int
	flapStart  time.Time
	flapFiring bool
}

// MountAlertHandler tracks mount presence with debounce and flap detection.
type MountAlertHandler struct {
	mu            sync.Mutex
	states        map[string]*mountState
	debounceTicks int
	flapThreshold int
	flapWindow    time.Duration
}

// NewMountHandler: 3-tick debounce, 3 flaps in 5 minutes for unstable.
func NewMountHandler() *MountAlertHandler {
	return &MountAlertHandler{
		states:        make(map[string]*mountState),
		debounceTicks: 3,
		flapThreshold: 3,
		flapWindow:    5 * time.Minute,
	}
}

// Restore reloads previously-firing alerts so they survive restarts.
func (h *MountAlertHandler) Restore(states []State) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, s := range states {
		if path, ok := strings.CutPrefix(s.Name, metric.PrefixMountMissing); ok {
			st := h.getOrCreate(path)
			st.status = mountFiring
		} else if path, ok := strings.CutPrefix(s.Name, metric.PrefixMountUnstable); ok {
			st := h.getOrCreate(path)
			st.flapFiring = true
		}
	}
}

// Dismiss clears state for the named alert so it can re-fire.
func (h *MountAlertHandler) Dismiss(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if path, ok := strings.CutPrefix(name, metric.PrefixMountMissing); ok {
		if st, exists := h.states[path]; exists {
			st.status = mountOK
			st.debounceLeft = 0
		}
	} else if path, ok := strings.CutPrefix(name, metric.PrefixMountUnstable); ok {
		if st, exists := h.states[path]; exists {
			st.flapFiring = false
			st.flapCount = 0
		}
	}
}

func (h *MountAlertHandler) Evaluate(mounts []collector.MountStatus) (fired, resolved []State) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, m := range mounts {
		path := m.Path
		st := h.getOrCreate(path)

		// Check if the flap window has expired; if so, reset flap tracking.
		if st.flapCount > 0 && !st.flapStart.IsZero() && time.Since(st.flapStart) > h.flapWindow {
			if st.flapFiring {
				resolved = append(resolved, State{
					Name:    metric.PrefixMountUnstable + path,
					Message: fmt.Sprintf("mount %s is no longer flapping", displayName(m)),
					FiredAt: time.Now(),
				})
				st.flapFiring = false
			}
			st.flapCount = 0
			st.flapStart = time.Time{}
		}

		switch st.status {
		case mountOK:
			if !m.Mounted {
				st.status = mountDebouncing
				// debounceTicks counts the transition tick itself, so we need
				// debounceTicks-1 additional ticks before firing.
				st.debounceLeft = h.debounceTicks - 1
			}

		case mountDebouncing:
			if m.Mounted {
				// Mount reappeared during debounce — cancel and count as a flap.
				st.status = mountOK
				st.debounceLeft = 0
				f := h.incrementFlap(st, m)
				if f != nil {
					fired = append(fired, *f)
				}
			} else {
				st.debounceLeft--
				if st.debounceLeft <= 0 {
					st.status = mountFiring
					s := State{
						Name:    metric.PrefixMountMissing + path,
						Message: fmt.Sprintf("mount %s is not present", displayName(m)),
						FiredAt: time.Now(),
					}
					fired = append(fired, s)
				}
			}

		case mountFiring:
			if m.Mounted {
				// Mount came back — resolve the missing alert and count as a flap.
				resolved = append(resolved, State{
					Name:    metric.PrefixMountMissing + path,
					Message: fmt.Sprintf("mount %s is present again", displayName(m)),
					FiredAt: time.Now(),
				})
				st.status = mountOK
				f := h.incrementFlap(st, m)
				if f != nil {
					fired = append(fired, *f)
				}
			}
		}
	}

	return fired, resolved
}

func (h *MountAlertHandler) IsUnstable(path string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if st, ok := h.states[path]; ok {
		return st.flapFiring
	}
	return false
}

// Caller must hold h.mu.
func (h *MountAlertHandler) getOrCreate(path string) *mountState {
	if st, ok := h.states[path]; ok {
		return st
	}
	st := &mountState{}
	h.states[path] = st
	return st
}

// incrementFlap records a reappearance and returns an unstable alert when the
// flap threshold is crossed. Caller must hold h.mu.
func (h *MountAlertHandler) incrementFlap(st *mountState, m collector.MountStatus) *State {
	now := time.Now()
	if st.flapStart.IsZero() {
		st.flapStart = now
	}
	st.flapCount++
	if !st.flapFiring && st.flapCount >= h.flapThreshold {
		st.flapFiring = true
		s := State{
			Name:    metric.PrefixMountUnstable + m.Path,
			Message: fmt.Sprintf("mount %s is flapping", displayName(m)),
			FiredAt: now,
		}
		return &s
	}
	return nil
}

// displayName falls back to path when no label is configured.
func displayName(m collector.MountStatus) string {
	if m.Label != "" {
		return m.Label
	}
	return m.Path
}
