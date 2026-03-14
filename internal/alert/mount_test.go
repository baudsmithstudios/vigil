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
	h.flapWindow = 5 * time.Minute
	h.flapThreshold = 3

	var allFired []State
	for flap := 0; flap < 3; flap++ {
		fired, _ := h.Evaluate([]collector.MountStatus{{Path: "/mnt/data", Mounted: false}})
		allFired = append(allFired, fired...)
		fired, _ = h.Evaluate([]collector.MountStatus{{Path: "/mnt/data", Mounted: true}})
		allFired = append(allFired, fired...)
	}

	var unstableFound bool
	for _, a := range allFired {
		if a.Name == "mount_unstable:/mnt/data" {
			unstableFound = true
		}
	}
	if !unstableFound {
		t.Error("expected unstable alert after 3 flaps")
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
