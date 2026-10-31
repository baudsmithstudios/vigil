package alert

import (
	"testing"
	"time"

	"vigil/internal/collector"
)

func TestMountHandler_NoAlertWhenMounted(t *testing.T) {
	h := NewMountHandler()
	mounts := []collector.MountStatus{{Path: "/mnt/data", Mounted: true}}
	fired, resolved := h.Evaluate(mounts)
	if len(fired) != 0 || len(resolved) != 0 {
		t.Errorf("expected no alerts, got fired=%d resolved=%d", len(fired), len(resolved))
	}
}

func TestMountHandler_DebounceBeforeFiring(t *testing.T) {
	h := NewMountHandler()
	mounts := []collector.MountStatus{{Path: "/mnt/data", Mounted: false}}

	for i := 0; i < 2; i++ {
		fired, _ := h.Evaluate(mounts)
		if len(fired) != 0 {
			t.Errorf("tick %d: expected no fire during debounce, got %d", i+1, len(fired))
		}
	}

	fired, _ := h.Evaluate(mounts)
	if len(fired) != 1 {
		t.Fatalf("expected 1 alert after debounce, got %d", len(fired))
	}
	if fired[0].Name != "mount_missing:/mnt/data" {
		t.Errorf("unexpected alert name: %q", fired[0].Name)
	}
}

func TestMountHandler_DebounceCancelledOnReappearance(t *testing.T) {
	h := NewMountHandler()
	h.Evaluate([]collector.MountStatus{{Path: "/mnt/data", Mounted: false}})
	fired, _ := h.Evaluate([]collector.MountStatus{{Path: "/mnt/data", Mounted: true}})
	if len(fired) != 0 {
		t.Error("expected no fire after debounce cancelled")
	}
	for i := 0; i < 2; i++ {
		fired, _ = h.Evaluate([]collector.MountStatus{{Path: "/mnt/data", Mounted: false}})
		if len(fired) != 0 {
			t.Errorf("tick %d: expected debounce restart, not fire", i+1)
		}
	}
}

func TestMountHandler_ResolvesOnReappearance(t *testing.T) {
	h := NewMountHandler()
	mounts := []collector.MountStatus{{Path: "/mnt/data", Mounted: false}}
	for i := 0; i < 3; i++ {
		h.Evaluate(mounts)
	}
	_, resolved := h.Evaluate([]collector.MountStatus{{Path: "/mnt/data", Mounted: true}})
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved, got %d", len(resolved))
	}
}

func TestMountHandler_DoesNotReFireWhileFiring(t *testing.T) {
	h := NewMountHandler()
	mounts := []collector.MountStatus{{Path: "/mnt/data", Mounted: false}}
	for i := 0; i < 3; i++ {
		h.Evaluate(mounts)
	}
	fired, _ := h.Evaluate(mounts)
	if len(fired) != 0 {
		t.Error("expected no re-fire while already firing")
	}
}

func TestMountHandler_FlapDetection(t *testing.T) {
	h := NewMountHandler()
	for flap := 1; flap <= 3; flap++ {
		h.Evaluate([]collector.MountStatus{{Path: "/mnt/data", Mounted: false}})
		fired, _ := h.Evaluate([]collector.MountStatus{{Path: "/mnt/data", Mounted: true}})
		if unstable := len(fired) == 1 && fired[0].Name == "mount_unstable:/mnt/data"; unstable != (flap == 3) {
			t.Errorf("flap %d: fired %+v", flap, fired)
		}
	}
}

func TestMountHandler_UnstableResolvesWhenFlapWindowExpires(t *testing.T) {
	h := NewMountHandler()
	for flap := 0; flap < 3; flap++ {
		h.Evaluate([]collector.MountStatus{{Path: "/mnt/data", Mounted: false}})
		h.Evaluate([]collector.MountStatus{{Path: "/mnt/data", Mounted: true}})
	}
	if !h.IsUnstable("/mnt/data") {
		t.Fatal("setup: expected the mount to be unstable")
	}

	h.states["/mnt/data"].flapStart = time.Now().Add(-h.flapWindow - time.Second)
	_, resolved := h.Evaluate([]collector.MountStatus{{Path: "/mnt/data", Mounted: true}})
	if len(resolved) != 1 || resolved[0].Name != "mount_unstable:/mnt/data" {
		t.Fatalf("expected unstable to resolve, got %+v", resolved)
	}
	if h.IsUnstable("/mnt/data") {
		t.Error("expected the mount to be stable again")
	}
}

func TestMountHandler_Dismiss(t *testing.T) {
	h := NewMountHandler()
	mounts := []collector.MountStatus{{Path: "/mnt/data", Mounted: false}}
	for i := 0; i < 3; i++ {
		h.Evaluate(mounts)
	}
	h.Dismiss("mount_missing:/mnt/data")
	var fired []State
	for i := 0; i < 3; i++ {
		f, _ := h.Evaluate(mounts)
		fired = append(fired, f...)
	}
	if len(fired) != 1 {
		t.Errorf("expected 1 re-fire after dismiss, got %d", len(fired))
	}
}

func TestMountHandler_Restore(t *testing.T) {
	h := NewMountHandler()
	h.Restore([]State{{Name: "mount_missing:/mnt/data"}})
	fired, _ := h.Evaluate([]collector.MountStatus{{Path: "/mnt/data", Mounted: false}})
	if len(fired) != 0 {
		t.Errorf("expected no re-fire after restore, got %d", len(fired))
	}
	_, resolved := h.Evaluate([]collector.MountStatus{{Path: "/mnt/data", Mounted: true}})
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved after restore, got %d", len(resolved))
	}
}
