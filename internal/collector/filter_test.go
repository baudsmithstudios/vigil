package collector

import "testing"

func TestShouldCollectFS(t *testing.T) {
	real := []string{"ext4", "xfs", "btrfs", "vfat", "ntfs", "exfat", "f2fs", "zfs"}
	for _, fs := range real {
		if !ShouldCollectFS(fs) {
			t.Errorf("expected ShouldCollectFS(%q) = true", fs)
		}
	}

	virtual := []string{
		"tmpfs", "devtmpfs", "overlay", "squashfs",
		"cgroup", "cgroup2", "proc", "sysfs",
		"debugfs", "tracefs", "devpts", "hugetlbfs",
		"mqueue", "configfs", "ramfs", "bpf",
		"fusectl", "pstore", "securityfs", "autofs",
	}
	for _, fs := range virtual {
		if ShouldCollectFS(fs) {
			t.Errorf("expected ShouldCollectFS(%q) = false", fs)
		}
	}
}

func TestShouldCollectInterface(t *testing.T) {
	real := []string{"eth0", "wlan0", "enp3s0", "ens18", "wlp2s0", "end0"}
	for _, iface := range real {
		if !shouldCollectInterface(iface) {
			t.Errorf("expected shouldCollectInterface(%q) = true", iface)
		}
	}

	virtual := []string{
		"lo",
		"docker0", "docker_gwbridge",
		"veth3f8a2b", "vethabc123",
		"br-9f3d2a1c", "br-custom",
		"virbr0", "virbr0-nic",
		"dummy0",
		"tunl0",
	}
	for _, iface := range virtual {
		if shouldCollectInterface(iface) {
			t.Errorf("expected shouldCollectInterface(%q) = false", iface)
		}
	}
}
