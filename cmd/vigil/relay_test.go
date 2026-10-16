package main

import (
	"context"
	"reflect"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"vigil/internal/alert"
	"vigil/internal/collector"
	"vigil/internal/tui"
)

func TestTUIRelay_StalledDashboardDoesNotBlockLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Unbuffered, so send blocks like a stalled TUI.
	got := make(chan tea.Msg)
	r := newTUIRelay()
	go r.run(ctx, func(m tea.Msg) { got <- m })

	want := []tui.AlertMsg{
		{Fired: []alert.State{{Name: "a1"}}},
		{Resolved: []alert.State{{Name: "a1"}}},
		{Fired: []alert.State{{Name: "a2"}}},
	}
	returned := make(chan struct{})
	go func() {
		for _, m := range want {
			r.alert(m.Fired, m.Resolved)
		}
		for i := int64(1); i <= 100; i++ {
			r.snapshot(collector.Snapshot{Timestamp: time.Unix(i, 0)})
		}
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("relay blocked the caller while the dashboard was stalled")
	}

	var alerts []tui.AlertMsg
	var snaps []time.Time
	for len(snaps) == 0 || !snaps[len(snaps)-1].Equal(time.Unix(100, 0)) {
		var msg tea.Msg
		select {
		case msg = <-got:
		case <-time.After(time.Second):
			t.Fatal("relay delivered nothing to the dashboard")
		}
		switch m := msg.(type) {
		case tui.AlertMsg:
			alerts = append(alerts, m)
		case tui.SnapshotMsg:
			snaps = append(snaps, m.Timestamp)
		}
	}

	if !reflect.DeepEqual(alerts, want) {
		t.Fatalf("alerts lost or reordered: got %+v", alerts)
	}
	// One batch may predate the stall; the rest coalesce.
	if len(snaps) > 2 {
		t.Fatalf("got %d snapshots, want stale ones dropped", len(snaps))
	}
}
