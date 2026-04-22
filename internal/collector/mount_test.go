package collector

import (
	"os"
	"path/filepath"
	"testing"

	"vigil/internal/config"
)

func TestParseProcMounts(t *testing.T) {
	content := `rootfs / rootfs rw 0 0
/dev/sda1 /mnt/data ext4 rw,relatime 0 0
/dev/sdb1 /media/usb0 vfat rw,relatime 0 0
tmpfs /tmp tmpfs rw 0 0
`
	mounts := parseProcMounts([]byte(content))
	if !mounts["/mnt/data"] {
		t.Error("expected /mnt/data to be in mount map")
	}
	if !mounts["/media/usb0"] {
		t.Error("expected /media/usb0 to be in mount map")
	}
	if !mounts["/tmp"] {
		t.Error("expected /tmp to be in mount map")
	}
}

func TestParseProcMounts_DecodesEscapedMountPaths(t *testing.T) {
	content := `/dev/sdc1 /media/My\040Drive ext4 rw,relatime 0 0
`
	mounts := parseProcMounts([]byte(content))
	if !mounts["/media/My Drive"] {
		t.Error("expected escaped mount path to decode to /media/My Drive")
	}
}

func TestMountCollector_NilWhenEmpty(t *testing.T) {
	mc := newMountCollector(nil, "/proc/mounts")
	result := mc.collect()
	if result != nil {
		t.Errorf("expected nil for empty config, got %v", result)
	}
}

func TestMountCollector_DetectsPresence(t *testing.T) {
	dir := t.TempDir()
	procMounts := filepath.Join(dir, "mounts")
	content := `/dev/sda1 /mnt/data ext4 rw 0 0
/dev/sdb1 /media/usb0 vfat rw 0 0
`
	if err := os.WriteFile(procMounts, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	checks := []config.MountCheck{
		{Path: "/mnt/data", Label: "NAS"},
		{Path: "/media/usb0", Label: "USB"},
		{Path: "/mnt/missing"},
	}
	mc := newMountCollector(checks, procMounts)
	result := mc.collect()

	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}
	if !result[0].Mounted {
		t.Error("expected /mnt/data to be mounted")
	}
	if result[0].Label != "NAS" {
		t.Errorf("expected label NAS, got %q", result[0].Label)
	}
	if !result[1].Mounted {
		t.Error("expected /media/usb0 to be mounted")
	}
	if result[2].Mounted {
		t.Error("expected /mnt/missing to not be mounted")
	}
}

func TestHostMountsPath_InContainer(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, ".dockerenv")
	if err := os.WriteFile(marker, nil, 0644); err != nil {
		t.Fatal(err)
	}
	got := hostMountsPath(marker)
	if got != "/proc/1/mounts" {
		t.Errorf("expected /proc/1/mounts, got %q", got)
	}
}

func TestHostMountsPath_OnHost(t *testing.T) {
	got := hostMountsPath("/nonexistent/.dockerenv")
	if got != "/proc/mounts" {
		t.Errorf("expected /proc/mounts, got %q", got)
	}
}

func TestMountCollector_HandlesUnreadableFile(t *testing.T) {
	checks := []config.MountCheck{
		{Path: "/mnt/data"},
	}
	mc := newMountCollector(checks, "/nonexistent/mounts")
	result := mc.collect()

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Mounted {
		t.Error("expected mount to report as not mounted on read error")
	}
}
