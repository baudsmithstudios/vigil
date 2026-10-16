package main

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"vigil/internal/alert"
	"vigil/internal/collector"
	"vigil/internal/tui"
)

func snapAt(sec int64) collector.Snapshot {
	return collector.Snapshot{Timestamp: time.Unix(sec, 0)}
}

func recv(t *testing.T, got <-chan tea.Msg) tea.Msg {
	t.Helper()
	select {
	case m := <-got:
		return m
	case <-time.After(time.Second):
		t.Fatal("relay delivered nothing to the dashboard")
		return nil
	}
}

func TestTUIRelay_StalledDashboardDoesNotBlockLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Unbuffered: send blocks until the test reads, standing in for a stalled TUI.
	got := make(chan tea.Msg)
	r := newTUIRelay()
	go r.run(ctx, func(m tea.Msg) { got <- m })

	r.snapshot(snapAt(0))
	if m := recv(t, got).(tui.SnapshotMsg); !m.Timestamp.Equal(time.Unix(0, 0)) {
		t.Fatalf("first snapshot: got %v", m.Timestamp)
	}

	returned := make(chan struct{})
	go func() {
		r.alert([]alert.State{{Name: "a1"}}, nil)
		for i := int64(1); i <= 100; i++ {
			r.snapshot(snapAt(i))
		}
		r.alert(nil, []alert.State{{Name: "a1"}})
		r.snapshot(snapAt(101))
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("relay blocked the caller while the dashboard was stalled")
	}

	var alerts []tui.AlertMsg
	var snaps []time.Time
	for len(snaps) == 0 || !snaps[len(snaps)-1].Equal(time.Unix(101, 0)) {
		switch m := recv(t, got).(type) {
		case tui.AlertMsg:
			alerts = append(alerts, m)
		case tui.SnapshotMsg:
			snaps = append(snaps, m.Timestamp)
		}
	}

	if len(alerts) != 2 || len(alerts[0].Fired) != 1 || len(alerts[1].Resolved) != 1 {
		t.Fatalf("alerts must arrive fire-then-resolve, got %+v", alerts)
	}
	// Stale frames are coalesced: at most one batch was in flight when the stall began.
	if len(snaps) > 2 {
		t.Fatalf("got %d snapshots, want stale ones dropped", len(snaps))
	}
}
