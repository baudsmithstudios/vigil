package main

import (
	"context"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"vigil/internal/alert"
	"vigil/internal/collector"
	"vigil/internal/tui"
)

// tuiRelay feeds the dashboard without blocking runLoop, so a stalled terminal
// can't stop alerting. Snapshots coalesce; alerts queue in order.
type tuiRelay struct {
	mu     sync.Mutex
	snap   *tui.SnapshotMsg
	alerts []tui.AlertMsg
	wake   chan struct{}
}

func newTUIRelay() *tuiRelay {
	return &tuiRelay{wake: make(chan struct{}, 1)}
}

func (r *tuiRelay) snapshot(s collector.Snapshot) {
	msg := tui.SnapshotMsg(s)
	r.mu.Lock()
	r.snap = &msg
	r.mu.Unlock()
	r.signal()
}

func (r *tuiRelay) alert(fired, resolved []alert.State) {
	r.mu.Lock()
	r.alerts = append(r.alerts, tui.AlertMsg{Fired: fired, Resolved: resolved})
	r.mu.Unlock()
	r.signal()
}

func (r *tuiRelay) signal() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// run sends alerts before the snapshot, matching runLoop's order.
func (r *tuiRelay) run(ctx context.Context, send func(tea.Msg)) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.wake:
		}
		r.mu.Lock()
		alerts, snap := r.alerts, r.snap
		r.alerts, r.snap = nil, nil
		r.mu.Unlock()
		for _, m := range alerts {
			send(m)
		}
		if snap != nil {
			send(*snap)
		}
	}
}
