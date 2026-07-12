package collector

import "testing"

func TestShouldCollectFS(t *testing.T) {
	for _, fs := range []string{"ext4", "xfs", "btrfs", "vfat"} {
		if !ShouldCollectFS(fs) {
			t.Errorf("expected ShouldCollectFS(%q) = true", fs)
		}
	}

	// "TMPFS" covers the case-insensitive match.
	for _, fs := range []string{"tmpfs", "TMPFS", "overlay", "proc", "squashfs"} {
		if ShouldCollectFS(fs) {
			t.Errorf("expected ShouldCollectFS(%q) = false", fs)
		}
	}
}

func TestShouldCollectInterface(t *testing.T) {
	for _, iface := range []string{"eth0", "wlan0", "enp3s0"} {
		if !shouldCollectInterface(iface) {
			t.Errorf("expected shouldCollectInterface(%q) = true", iface)
		}
	}

	// One name per excluded prefix, plus loopback.
	for _, iface := range []string{"lo", "docker0", "veth3f8a2b", "br-9f3d2a1c", "virbr0", "dummy0", "tunl0"} {
		if shouldCollectInterface(iface) {
			t.Errorf("expected shouldCollectInterface(%q) = false", iface)
		}
	}
}
