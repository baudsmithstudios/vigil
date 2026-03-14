package alert

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"vigil/internal/config"
)

// State represents an alert that has fired and may or may not still be active.
type State struct {
	Name       string
	Message    string
	FiredAt    time.Time
	Resolved   bool
	ResolvedAt time.Time
}

// Engine evaluates threshold rules against metric snapshots and tracks firing state.
type Engine struct {
	mu       sync.Mutex
	rules    []config.Alert
	firing   map[string]State   // keyed by alert key (metric or metric:delta)
	prev     map[string]float64 // previous tick values for delta calculation
	sustains map[string]int     // consecutive ticks above threshold per rule+key
}

// New creates an Engine from the given alert rules.
func New(rules []config.Alert) *Engine {
	return &Engine{
		rules:    rules,
		firing:   make(map[string]State),
		prev:     make(map[string]float64),
		sustains: make(map[string]int),
	}
}

// Restore reloads previously-firing alerts into the engine so they survive restarts.
func (e *Engine) Restore(states []State) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range states {
		e.firing[s.Name] = s
	}
}

// Dismiss removes an alert from the firing set without requiring resolution.
// This allows manual dismissal from the TUI without the alert immediately re-firing.
func (e *Engine) Dismiss(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.firing, name)
}

// deltaKey returns the firing map key for a delta alert.
func deltaKey(metric string) string {
	return metric + ":delta"
}

// matchingKeys returns value-map keys that a rule should evaluate against.
// If rule.Metric exists as an exact key, return just that key.
// If rule.Metric contains no ":" and keys exist with a "rule.Metric:" prefix,
// return all such prefixed keys. Otherwise return nil.
func matchingKeys(ruleMetric string, values map[string]float64) []string {
	if _, ok := values[ruleMetric]; ok {
		return []string{ruleMetric}
	}
	if strings.Contains(ruleMetric, ":") {
		return nil
	}
	prefix := ruleMetric + ":"
	var keys []string
	for k := range values {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	return keys
}

// keySuffix extracts the part after the first ":" in a key, wrapped in parens.
// Returns empty string if there's no ":".
func keySuffix(key string) string {
	i := strings.Index(key, ":")
	if i < 0 {
		return ""
	}
	return " (" + key[i+1:] + ")"
}

// Evaluate checks values against all rules and returns newly-fired alerts.
// Already-firing alerts are not re-returned until they resolve and re-fire.
func (e *Engine) Evaluate(values map[string]float64) []State {
	e.mu.Lock()
	defer e.mu.Unlock()
	var fired []State
	for _, rule := range e.rules {
		keys := matchingKeys(rule.Metric, values)
		for _, key := range keys {
			v := values[key]

			// Threshold alert: evaluate unless this is a delta-only rule.
			if rule.Threshold != 0 || rule.DeltaThreshold == 0 {
				triggered := (rule.Above && v > rule.Threshold) || (!rule.Above && v < rule.Threshold)
				if triggered {
					if _, alreadyFiring := e.firing[key]; !alreadyFiring {
						e.sustains[key]++
						if rule.SustainedTicks > 0 && e.sustains[key] < rule.SustainedTicks {
							continue
						}
						msg := rule.Message
						if key != rule.Metric {
							msg += keySuffix(key)
						}
						s := State{Name: key, Message: msg, FiredAt: time.Now()}
						e.firing[key] = s
						fired = append(fired, s)
						delete(e.sustains, key)
					}
				} else {
					delete(e.sustains, key)
				}
			}

			// Delta alert.
			if rule.DeltaThreshold > 0 {
				dk := deltaKey(key)
				if prev, hasPrev := e.prev[key]; hasPrev {
					delta := v - prev
					if delta < 0 {
						delta = -delta
					}
					if delta >= rule.DeltaThreshold {
						if _, alreadyFiring := e.firing[dk]; !alreadyFiring {
							msg := fmt.Sprintf("%s spiked %+.1f%% in one tick (%.1f%% → %.1f%%)",
								key, v-prev, prev, v)
							s := State{Name: dk, Message: msg, FiredAt: time.Now()}
							e.firing[dk] = s
							fired = append(fired, s)
						}
					}
				}
			}
		}
	}

	// Store current values for next tick's delta calculation.
	for k, v := range values {
		e.prev[k] = v
	}

	return fired
}

// Resolved checks values against firing alerts and returns those that have cleared.
// Cleared alerts are removed from the active-firing set.
func (e *Engine) Resolved(values map[string]float64) []State {
	e.mu.Lock()
	defer e.mu.Unlock()
	var resolved []State
	for _, rule := range e.rules {
		keys := matchingKeys(rule.Metric, values)
		for _, key := range keys {
			v := values[key]

			// Threshold resolution.
			if _, firing := e.firing[key]; firing {
				stillTriggered := (rule.Above && v > rule.Threshold) || (!rule.Above && v < rule.Threshold)
				if !stillTriggered {
					resolved = append(resolved, e.firing[key])
					delete(e.firing, key)
				}
			}

			// Delta resolution: auto-resolve when the delta drops below threshold.
			if rule.DeltaThreshold > 0 {
				dk := deltaKey(key)
				if _, firing := e.firing[dk]; firing {
					if prev, hasPrev := e.prev[key]; hasPrev {
						delta := v - prev
						if delta < 0 {
							delta = -delta
						}
						if delta < rule.DeltaThreshold {
							resolved = append(resolved, e.firing[dk])
							delete(e.firing, dk)
						}
					}
				}
			}
		}
	}
	return resolved
}
