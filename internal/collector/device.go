package collector

import "strings"

// ParentBlockDevice returns the parent block device for common Linux partition
// naming schemes (e.g. sda1->sda, mmcblk0p2->mmcblk0, nvme0n1p2->nvme0n1).
func ParentBlockDevice(name string) string {
	if name == "" {
		return ""
	}
	if strings.HasPrefix(name, "nvme") || strings.HasPrefix(name, "mmcblk") {
		if i := strings.LastIndex(name, "p"); i > 0 && i < len(name)-1 && isDigits(name[i+1:]) {
			return name[:i]
		}
		return ""
	}
	i := len(name) - 1
	for i >= 0 && name[i] >= '0' && name[i] <= '9' {
		i--
	}
	if i < len(name)-1 && i >= 0 {
		return name[:i+1]
	}
	return ""
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
