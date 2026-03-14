package collector

import (
	"os"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/host"
)

const piThermalZone = "/sys/class/thermal/thermal_zone0/temp"

func collectTemperature() []TempSnapshot {
	// Try gopsutil first — works on many Linux systems.
	sensors, err := host.SensorsTemperatures()
	if err == nil && len(sensors) > 0 {
		var out []TempSnapshot
		for _, s := range sensors {
			if s.Temperature > 0 {
				out = append(out, TempSnapshot{
					SensorKey: s.SensorKey,
					Celsius:   s.Temperature,
				})
			}
		}
		if len(out) > 0 {
			return out
		}
	}

	// Fallback: read Pi's thermal_zone0 directly.
	data, err := os.ReadFile(piThermalZone)
	if err != nil {
		return nil
	}
	raw, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
	if err != nil {
		return nil
	}
	return []TempSnapshot{{SensorKey: "cpu_thermal", Celsius: raw / 1000.0}}
}
