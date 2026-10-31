package collector

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

const throttlePath = "/sys/devices/platform/soc/soc:firmware/get_throttled"

type ThrottleSnapshot struct {
	Raw                    uint32
	UnderVoltage           bool
	FreqCapped             bool
	Throttled              bool
	SoftTempLimit          bool
	UnderVoltageSinceBoot  bool
	FreqCappedSinceBoot    bool
	ThrottledSinceBoot     bool
	SoftTempLimitSinceBoot bool
	Available              bool
}

func (t ThrottleSnapshot) ActiveNow() bool {
	return t.UnderVoltage || t.FreqCapped || t.Throttled || t.SoftTempLimit
}

func (t ThrottleSnapshot) SinceBoot() bool {
	return t.UnderVoltageSinceBoot || t.FreqCappedSinceBoot ||
		t.ThrottledSinceBoot || t.SoftTempLimitSinceBoot
}

func (t ThrottleSnapshot) Status() string {
	if t.ActiveNow() {
		return "THROTTLED"
	}
	if t.SinceBoot() {
		return "WARN"
	}
	return "OK"
}

func (t ThrottleSnapshot) ActiveMessage() string {
	var parts []string
	if t.UnderVoltage {
		parts = append(parts, "under-voltage detected")
	}
	if t.FreqCapped {
		parts = append(parts, "frequency capped")
	}
	if t.Throttled {
		parts = append(parts, "CPU throttled")
	}
	if t.SoftTempLimit {
		parts = append(parts, "soft temperature limit")
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("%s — check power supply", strings.Join(parts, ", "))
}

func decodeThrottleBitmask(raw uint32) ThrottleSnapshot {
	return ThrottleSnapshot{
		Raw:                    raw,
		UnderVoltage:           raw&(1<<0) != 0,
		FreqCapped:             raw&(1<<1) != 0,
		Throttled:              raw&(1<<2) != 0,
		SoftTempLimit:          raw&(1<<3) != 0,
		UnderVoltageSinceBoot:  raw&(1<<16) != 0,
		FreqCappedSinceBoot:    raw&(1<<17) != 0,
		ThrottledSinceBoot:     raw&(1<<18) != 0,
		SoftTempLimitSinceBoot: raw&(1<<19) != 0,
		Available:              true,
	}
}

var throttleWarnOnce sync.Once

func readThrottleValue() (uint32, bool) {
	data, err := os.ReadFile(throttlePath)
	if err == nil {
		s := strings.TrimSpace(string(data))
		val, err := strconv.ParseUint(s, 0, 32)
		if err == nil {
			return uint32(val), true
		}
		throttleWarnOnce.Do(func() {
			fmt.Fprintf(os.Stderr, "vigil: parse throttle value %q: %v\n", s, err)
		})
		return 0, false
	}
	if !os.IsNotExist(err) {
		throttleWarnOnce.Do(func() {
			fmt.Fprintf(os.Stderr, "vigil: read throttle state: %v\n", err)
		})
	}

	out, err := exec.Command("vcgencmd", "get_throttled").Output()
	if err != nil {
		throttleWarnOnce.Do(func() {
			fmt.Fprintf(os.Stderr, "vigil: vcgencmd get_throttled: %v\n", err)
		})
		return 0, false
	}
	// Output format: "throttled=0x50005\n"
	s := strings.TrimSpace(string(out))
	s = strings.TrimPrefix(s, "throttled=")
	val, err := strconv.ParseUint(s, 0, 32)
	if err != nil {
		throttleWarnOnce.Do(func() {
			fmt.Fprintf(os.Stderr, "vigil: parse vcgencmd throttle value %q: %v\n", s, err)
		})
		return 0, false
	}
	return uint32(val), true
}

func collectThrottle() ThrottleSnapshot {
	val, ok := readThrottleValue()
	if !ok {
		return ThrottleSnapshot{}
	}
	return decodeThrottleBitmask(val)
}
