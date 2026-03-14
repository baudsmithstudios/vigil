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

// ThrottleSnapshot holds decoded Pi throttle/voltage flags.
type ThrottleSnapshot struct {
	Raw                    uint32
	UnderVoltage           bool // bit 0
	FreqCapped             bool // bit 1
	Throttled              bool // bit 2
	SoftTempLimit          bool // bit 3
	UnderVoltageSinceBoot  bool // bit 16
	FreqCappedSinceBoot    bool // bit 17
	ThrottledSinceBoot     bool // bit 18
	SoftTempLimitSinceBoot bool // bit 19
	Available              bool // true if sysfs file was read successfully
}

// ActiveNow returns true if any active throttle flag (bits 0-3) is set.
func (t ThrottleSnapshot) ActiveNow() bool {
	return t.UnderVoltage || t.FreqCapped || t.Throttled || t.SoftTempLimit
}

// SinceBoot returns true if any historical flag (bits 16-19) is set.
func (t ThrottleSnapshot) SinceBoot() bool {
	return t.UnderVoltageSinceBoot || t.FreqCappedSinceBoot ||
		t.ThrottledSinceBoot || t.SoftTempLimitSinceBoot
}

// Status returns a three-state string: "THROTTLED", "WARN", or "OK".
func (t ThrottleSnapshot) Status() string {
	if t.ActiveNow() {
		return "THROTTLED"
	}
	if t.SinceBoot() {
		return "WARN"
	}
	return "OK"
}

// ActiveMessage returns a human-readable description of active flags.
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

// readThrottleValue tries the sysfs file first, then falls back to vcgencmd.
func readThrottleValue() (uint32, bool) {
	// Try sysfs first.
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

	// Fall back to vcgencmd.
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
