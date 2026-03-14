package collector

import "strings"

// virtualFS lists filesystem types that represent kernel/container internals
// rather than real storage. Mirrors the exclusion list used by Prometheus
// node_exporter.
var virtualFS = map[string]bool{
	"autofs": true, "bpf": true, "cgroup": true, "cgroup2": true,
	"configfs": true, "debugfs": true, "devpts": true, "devtmpfs": true,
	"fusectl": true, "hugetlbfs": true, "mqueue": true, "overlay": true,
	"proc": true, "pstore": true, "ramfs": true, "securityfs": true,
	"squashfs": true, "sysfs": true, "tmpfs": true, "tracefs": true,
}

// virtualIfacePrefixes lists network interface name prefixes that indicate
// virtual/container interfaces rather than physical hardware.
var virtualIfacePrefixes = []string{
	"docker", "veth", "br-", "virbr", "dummy", "tunl",
}

// ShouldCollectFS returns true if the filesystem type represents real storage
// worth monitoring.
func ShouldCollectFS(fstype string) bool {
	return !virtualFS[strings.ToLower(fstype)]
}

// shouldCollectInterface returns true if the interface is a physical or
// meaningful network interface. Loopback and known virtual interfaces are
// excluded.
func shouldCollectInterface(name string) bool {
	if name == "lo" {
		return false
	}
	for _, prefix := range virtualIfacePrefixes {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	return true
}
