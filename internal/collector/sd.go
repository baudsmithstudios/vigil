package collector

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const mmcErrStatsGlob = "/sys/kernel/debug/mmc*/err_stats"

type sdErrorCollector struct {
	prev     map[string]uint64
	warnOnce sync.Once
	glob     func(pattern string) ([]string, error)
	readFile func(name string) ([]byte, error)
}

func newSDErrorCollector() *sdErrorCollector {
	return &sdErrorCollector{
		prev:     make(map[string]uint64),
		glob:     filepath.Glob,
		readFile: os.ReadFile,
	}
}

// collect returns per-host deltas from MMC debugfs error counters.
// This is best-effort and may be unavailable on hardened deployments.
func (c *sdErrorCollector) collect() []SDErrorSnapshot {
	paths, err := c.glob(mmcErrStatsGlob)
	if err != nil {
		c.warnUnavailable(err)
		return nil
	}
	if len(paths) == 0 {
		c.warnUnavailable(fmt.Errorf("no files matched %s", mmcErrStatsGlob))
		return nil
	}

	var out []SDErrorSnapshot
	readAny := false
	for _, p := range paths {
		data, err := c.readFile(p)
		if err != nil {
			continue
		}
		total, err := parseMMCErrStats(string(data))
		if err != nil {
			continue
		}
		readAny = true
		host := filepath.Base(filepath.Dir(p))
		var delta uint64
		if prev, ok := c.prev[host]; ok && total >= prev {
			delta = total - prev
		}
		c.prev[host] = total
		out = append(out, SDErrorSnapshot{
			Host:  host,
			Delta: delta,
		})
	}
	if !readAny {
		c.warnUnavailable(fmt.Errorf("unable to read or parse any mmc err_stats files"))
	}
	return out
}

func (c *sdErrorCollector) warnUnavailable(err error) {
	c.warnOnce.Do(func() {
		log.Printf("sd error counters unavailable: %v", err)
	})
}

func parseMMCErrStats(data string) (uint64, error) {
	var total uint64
	var found bool

	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		i := strings.LastIndexByte(line, ':')
		if i < 0 {
			continue
		}
		n, err := strconv.ParseUint(strings.TrimSpace(line[i+1:]), 10, 64)
		if err != nil {
			continue
		}
		total += n
		found = true
	}
	if !found {
		return 0, fmt.Errorf("no counters found")
	}
	return total, nil
}
