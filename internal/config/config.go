package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"vigil/internal/metric"
)

// Alert defines a threshold-based alert rule.
type Alert struct {
	Metric         string  `toml:"metric"`
	Threshold      float64 `toml:"threshold"`
	Above          bool    `toml:"above"`           // true = fire when value > threshold
	Message        string  `toml:"message"`
	DeltaThreshold float64 `toml:"delta_threshold"` // fire if |change| >= this in one tick (0 = disabled)
	SustainedTicks int     `toml:"sustained_ticks"` // consecutive ticks above threshold before firing (0 = immediate)
}

// Notifications configures outbound alert delivery.
type Notifications struct {
	DiscordWebhook string   `toml:"discord_webhook"` // Discord webhook URL
	WebhookURL     string   `toml:"webhook_url"`     // Generic webhook URL (POST JSON)
	QuietHours     []string `toml:"quiet_hours"`     // e.g. ["02:00-06:00", "12:00-13:00"]
}

// Docker configures Docker container monitoring.
type Docker struct {
	Socket string `toml:"socket"` // Docker socket path (empty = disabled)
}

// MountCheck defines an expected mount point to watch.
type MountCheck struct {
	Path  string `toml:"path"`
	Label string `toml:"label"`
}

// Services configures service health check behavior.
type Services struct {
	Interval            duration `toml:"interval"`
	FailuresBeforeAlert int      `toml:"failures_before_alert"`
}

// HTTPCheck defines an HTTP endpoint health check.
type HTTPCheck struct {
	Name           string `toml:"name"`
	URL            string `toml:"url"`
	ExpectedStatus int    `toml:"expected_status"`
}

// PortCheck defines a TCP port health check.
type PortCheck struct {
	Name string `toml:"name"`
	Host string `toml:"host"`
	Port int    `toml:"port"`
}

// Config holds all vigil configuration.
type Config struct {
	DBPath        string        `toml:"db_path"`
	Interval      duration      `toml:"interval"`
	Retention     duration      `toml:"retention"`
	Alerts        []Alert       `toml:"alerts"`
	Notifications Notifications `toml:"notifications"`
	Docker        Docker        `toml:"docker"`
	MountChecks   []MountCheck  `toml:"mount_checks"`
	Services      Services      `toml:"services"`
	HTTPChecks    []HTTPCheck   `toml:"http_checks"`
	PortChecks    []PortCheck   `toml:"port_checks"`
	Theme         string        `toml:"theme"`
}

// duration wraps time.Duration for TOML unmarshalling.
type duration struct {
	time.Duration
}

// TestDuration creates a duration value for use in tests.
func TestDuration(d time.Duration) duration {
	return duration{d}
}

func (d *duration) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	return err
}

// Defaults returns a Config pre-populated with sane defaults.
func Defaults() Config {
	return Config{
		DBPath:    "/data/vigil.db",
		Interval:  duration{2 * time.Second},
		Retention: duration{12 * time.Hour},
		Theme:     "auto",
		Services: Services{
			Interval:            duration{30 * time.Second},
			FailuresBeforeAlert: 2,
		},
		Alerts: []Alert{
			{Metric: metric.CPUPercent, Threshold: 90.0, Above: true, Message: "CPU usage above 90%"},
			{Metric: metric.MemPercent, Threshold: 90.0, Above: true, Message: "Memory usage above 90%"},
			{Metric: metric.DiskPercent, Threshold: 90.0, Above: true, Message: "Disk usage above 90%"},
		},
	}
}

// Load reads a TOML config file, falling back to defaults for missing fields.
func Load(path string) (Config, error) {
	cfg := Defaults()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // file not found → use defaults
		}
		return cfg, err
	}
	defer f.Close()

	if _, err := toml.NewDecoder(f).Decode(&cfg); err != nil {
		return cfg, err
	}
	if err := cfg.validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if c.Interval.Duration <= 0 {
		return fmt.Errorf("interval must be positive, got %s", c.Interval.Duration)
	}
	if c.Retention.Duration <= 0 {
		return fmt.Errorf("retention must be positive, got %s", c.Retention.Duration)
	}
	if s := c.Docker.Socket; s != "" {
		if !filepath.IsAbs(s) {
			return fmt.Errorf("docker socket must be an absolute path, got %q", s)
		}
		if filepath.Clean(s) != s {
			return fmt.Errorf("docker socket path contains traversal components: %q", s)
		}
	}
	seen := make(map[string]bool, len(c.MountChecks))
	for _, mc := range c.MountChecks {
		if mc.Path == "" {
			return fmt.Errorf("mount_checks: path must not be empty")
		}
		if !filepath.IsAbs(mc.Path) {
			return fmt.Errorf("mount_checks: path must be absolute, got %q", mc.Path)
		}
		if filepath.Clean(mc.Path) != mc.Path {
			return fmt.Errorf("mount_checks: path contains traversal components: %q", mc.Path)
		}
		if seen[mc.Path] {
			return fmt.Errorf("mount_checks: duplicate path %q", mc.Path)
		}
		seen[mc.Path] = true
	}
	hasChecks := len(c.HTTPChecks) > 0 || len(c.PortChecks) > 0
	if hasChecks {
		if c.Services.Interval.Duration <= 0 {
			return fmt.Errorf("services.interval must be positive when checks are configured")
		}
		if c.Services.FailuresBeforeAlert < 1 {
			return fmt.Errorf("services.failures_before_alert must be >= 1 when checks are configured")
		}
	}
	checkNames := make(map[string]bool, len(c.HTTPChecks)+len(c.PortChecks))
	for _, hc := range c.HTTPChecks {
		if hc.Name == "" {
			return fmt.Errorf("http_checks: name must not be empty")
		}
		if hc.URL == "" {
			return fmt.Errorf("http_checks: url must not be empty")
		}
		u, err := url.Parse(hc.URL)
		if err != nil || u.Host == "" {
			return fmt.Errorf("http_checks: invalid url %q", hc.URL)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("http_checks: url scheme must be http or https, got %q", u.Scheme)
		}
		if u.Scheme == "http" {
			fmt.Fprintf(os.Stderr, "vigil: warning: http_check %q uses plaintext HTTP — consider HTTPS\n", hc.Name)
		}
		if checkNames[hc.Name] {
			return fmt.Errorf("http_checks: duplicate name %q", hc.Name)
		}
		checkNames[hc.Name] = true
	}
	for _, pc := range c.PortChecks {
		if pc.Name == "" {
			return fmt.Errorf("port_checks: name must not be empty")
		}
		if pc.Host == "" {
			return fmt.Errorf("port_checks: host must not be empty")
		}
		if pc.Port < 1 || pc.Port > 65535 {
			return fmt.Errorf("port_checks: port must be 1-65535, got %d", pc.Port)
		}
		if checkNames[pc.Name] {
			return fmt.Errorf("duplicate check name %q", pc.Name)
		}
		checkNames[pc.Name] = true
	}
	switch c.Theme {
	case "", "auto", "dark", "light":
		// valid
	default:
		return fmt.Errorf("theme must be auto, dark, or light, got %q", c.Theme)
	}
	return nil
}
