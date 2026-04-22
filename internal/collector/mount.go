package collector

import (
	"bytes"
	"log"
	"os"
	"sync"
	"time"

	"vigil/internal/config"
)

const defaultDockerMarker = "/.dockerenv"

// HostMountsPath returns the path to read for host mount information.
// Inside a container with pid:host, /proc/1/mounts reflects the host's
// mount namespace. On the host, /proc/mounts is sufficient.
func HostMountsPath() string {
	return hostMountsPath(defaultDockerMarker)
}

func hostMountsPath(markerPath string) string {
	if _, err := os.Stat(markerPath); err == nil {
		return "/proc/1/mounts"
	}
	return "/proc/mounts"
}

type mountCollector struct {
	checks   []config.MountCheck
	path     string
	warnOnce sync.Once
	lastWarn time.Time
}

func newMountCollector(checks []config.MountCheck, procPath string) *mountCollector {
	if procPath == "" {
		procPath = HostMountsPath()
	}
	return &mountCollector{
		checks: checks,
		path:   procPath,
	}
}

func (m *mountCollector) collect() []MountStatus {
	if len(m.checks) == 0 {
		return nil
	}

	data, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			m.warnOnce.Do(func() {
				log.Printf("mount watchdog: %s not found, mount checking unavailable", m.path)
			})
		} else {
			now := time.Now()
			if m.lastWarn.IsZero() || now.Sub(m.lastWarn) >= time.Minute {
				log.Printf("mount watchdog: read %s: %v", m.path, err)
				m.lastWarn = now
			}
		}
		result := make([]MountStatus, len(m.checks))
		for i, c := range m.checks {
			result[i] = MountStatus{Path: c.Path, Label: c.Label, Mounted: false}
		}
		return result
	}

	mounts := parseProcMounts(data)
	result := make([]MountStatus, len(m.checks))
	for i, c := range m.checks {
		result[i] = MountStatus{
			Path:    c.Path,
			Label:   c.Label,
			Mounted: mounts[c.Path],
		}
	}
	return result
}

func parseProcMounts(data []byte) map[string]bool {
	mounts := make(map[string]bool)
	for _, line := range bytes.Split(data, []byte("\n")) {
		fields := bytes.Fields(line)
		if len(fields) >= 2 {
			mounts[decodeProcMountField(string(fields[1]))] = true
		}
	}
	return mounts
}

func decodeProcMountField(in string) string {
	out := make([]byte, 0, len(in))
	for i := 0; i < len(in); i++ {
		if in[i] != '\\' || i+3 >= len(in) {
			out = append(out, in[i])
			continue
		}
		a, b, c := in[i+1], in[i+2], in[i+3]
		if a < '0' || a > '7' || b < '0' || b > '7' || c < '0' || c > '7' {
			out = append(out, in[i])
			continue
		}
		decoded := (a-'0')*64 + (b-'0')*8 + (c - '0')
		out = append(out, decoded)
		i += 3
	}
	return string(out)
}
