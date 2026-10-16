package main

import (
	"context"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"vigil/internal/alert"
	"vigil/internal/collector"
	"vigil/internal/tui"
)

// tuiRelay hands runLoop output to the dashboard without ever blocking
// runLoop, so a stalled terminal cannot stop alert evaluation. Only the latest
// snapshot is kept; alert messages queue in order so every fire and resolve
// reaches the dashboard.
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
	default: // a wake-up is already pending
	}
}

// run delivers pending messages until ctx is done. Alerts go first, matching
// the order runLoop produces them within a tick.
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
