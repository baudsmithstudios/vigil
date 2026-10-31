//go:build demo

package main

import (
	"context"
	"reflect"
	"testing"
	"time"

	"vigil/internal/alert"
	"vigil/internal/collector"
	"vigil/internal/config"
)

// Every tick after demoEndTick repeats the calm state.
const demoEndTick = 60

func TestDemoTimeline(t *testing.T) {
	base := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	const lastTick = 90 // past demoEndTick: the calm tail must stay quiet
	snapshots := make(chan collector.Snapshot, lastTick+1)
	for tick := 0; tick <= lastTick; tick++ {
		snapshots <- demoSnapshot(tick, base.Add(time.Duration(tick)*demoTick))
	}
	close(snapshots)

	type event struct {
		tick int
		name string
	}
	var fired, resolved []event
	tick := 0
	cfg := config.Defaults()
	runLoop(context.Background(), tempDB(t), snapshots, demoAlerting(cfg), nil, loopConfig{
		writeBufCap: 64, containerBufCap: 64,
		flushInterval: time.Hour, purgeInterval: time.Hour, retention: time.Hour,
		onSnapshot: func(collector.Snapshot) { tick++ },
		onAlert: func(f, r []alert.State) {
			for _, a := range f {
				fired = append(fired, event{tick, a.Name})
			}
			for _, a := range r {
				resolved = append(resolved, event{tick, a.Name})
			}
		},
	})

	wantFired := []event{
		{13, "cpu_percent"},
		{19, "mount_missing:/mnt/nas"},
		{26, "mount_unstable:/media/usb0"},
		{28, "service_down:mqtt"},
		{30, "throttle"},
	}
	// mount_unstable isn't here: its 5-minute window outlasts the take, so the tape dismisses it.
	wantResolved := []event{
		{44, "mount_missing:/mnt/nas"},
		{46, "cpu_percent"},
		{46, "service_down:mqtt"},
		{48, "throttle"},
	}
	if !reflect.DeepEqual(fired, wantFired) {
		t.Errorf("fired = %v, want %v", fired, wantFired)
	}
	if !reflect.DeepEqual(resolved, wantResolved) {
		t.Errorf("resolved = %v, want %v", resolved, wantResolved)
	}
}

func TestDemoShowsEveryOptInPanelAndEndsWhereItStarts(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	for _, tick := range []int{0, demoEndTick} {
		s := demoSnapshot(tick, now)
		if len(s.Containers) != 4 || len(s.Mounts) != 2 || len(s.Services) != 3 {
			t.Errorf("tick %d: %d containers, %d mounts, %d services; want 4, 2, 3", tick, len(s.Containers), len(s.Mounts), len(s.Services))
		}
		for _, c := range s.Containers {
			if c.Status != "running" {
				t.Errorf("tick %d: container %s is %s", tick, c.Name, c.Status)
			}
		}
		if !s.Throttle.Available || s.Throttle.Status() != "OK" {
			t.Errorf("tick %d: throttle status %q, want available and OK", tick, s.Throttle.Status())
		}
	}
}
