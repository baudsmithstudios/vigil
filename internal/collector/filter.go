package collector

import "strings"

// virtualFS mirrors Prometheus node_exporter's exclusion list.
var virtualFS = map[string]bool{
	"autofs": true, "bpf": true, "cgroup": true, "cgroup2": true,
	"configfs": true, "debugfs": true, "devpts": true, "devtmpfs": true,
	"fusectl": true, "hugetlbfs": true, "mqueue": true, "overlay": true,
	"proc": true, "pstore": true, "ramfs": true, "securityfs": true,
	"squashfs": true, "sysfs": true, "tmpfs": true, "tracefs": true,
}

var virtualIfacePrefixes = []string{
	"docker", "veth", "br-", "virbr", "dummy", "tunl",
}

func ShouldCollectFS(fstype string) bool {
	return !virtualFS[strings.ToLower(fstype)]
}

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
