package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"vigil/internal/config"
	"vigil/internal/metric"
	"vigil/internal/notify"
	"vigil/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Init flow styles, set by initStyles().
var (
	initTitleStyle   lipgloss.Style
	initSectionStyle lipgloss.Style
	initDimStyle     lipgloss.Style
	initAccentStyle  lipgloss.Style
	initWarnStyle    lipgloss.Style
	initPreviewStyle lipgloss.Style
)

func initStyles() {
	theme := tui.ResolveTheme("auto")
	initTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.Accent)
	initSectionStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.Accent)
	initDimStyle = lipgloss.NewStyle().Foreground(theme.Dim)
	initAccentStyle = lipgloss.NewStyle().Foreground(theme.Accent)
	initWarnStyle = lipgloss.NewStyle().Foreground(theme.Warn)
	initPreviewStyle = lipgloss.NewStyle().Foreground(theme.Dim)
}

// detectDockerSocket returns the socket path if it exists, empty string otherwise.
func detectDockerSocket(path string) string {
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

// detectThermalZones returns paths matching the glob pattern for thermal zone temp files.
func detectThermalZones(globPattern string) []string {
	matches, _ := filepath.Glob(globPattern)
	return matches
}

// detectInDocker returns true if the marker file exists.
func detectInDocker(markerPath string) bool {
	_, err := os.Stat(markerPath)
	return err == nil
}

// initConfig holds values collected during the init flow.
type initConfig struct {
	dbPath         string
	interval       string
	retention      string
	theme          string
	dockerSocket   string
	discordWebhook string
	webhookURL     string
	quietHours     []string
	mounts         []config.MountCheck
	alerts         []config.Alert
	httpChecks     []config.HTTPCheck
	portChecks     []config.PortCheck
}

// prompt prints a prompt with a default value and reads a line of input.
// Empty input returns the default.
func prompt(scanner *bufio.Scanner, label, defaultVal string) string {
	fmt.Printf("%s [%s] %s: ", label, defaultVal, initDimStyle.Render("(enter to accept)"))
	if scanner.Scan() {
		if v := strings.TrimSpace(scanner.Text()); v != "" {
			return v
		}
	}
	return defaultVal
}

// promptThreshold prompts for a numeric threshold, returning the default on empty input.
func promptThreshold(scanner *bufio.Scanner, label string, defaultVal float64) float64 {
	fmt.Printf("%s alert threshold %% [%.0f]: ", label, defaultVal)
	if scanner.Scan() {
		if v := strings.TrimSpace(scanner.Text()); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return f
			}
			fmt.Printf("  invalid number, using default %.0f\n", defaultVal)
		}
	}
	return defaultVal
}

// parseToggleInput parses space-separated 1-based numbers into a set of 0-based indices.
// Numbers out of range [1, count] are silently ignored.
func parseToggleInput(input string, count int) map[int]bool {
	toggled := make(map[int]bool)
	for _, tok := range strings.Fields(input) {
		n, err := strconv.Atoi(tok)
		if err != nil || n < 1 || n > count {
			continue
		}
		toggled[n-1] = true
	}
	return toggled
}

// promptNotifications collects optional Discord/webhook URLs and quiet hours.
func promptNotifications(scanner *bufio.Scanner) (discord, webhook string, quietHours []string) {
	fmt.Print("Discord webhook URL (empty to skip): ")
	if scanner.Scan() {
		discord = strings.TrimSpace(scanner.Text())
	}
	if discord != "" {
		if err := notify.ValidateURL(discord); err != nil {
			fmt.Printf("  warning: %v\n", err)
			discord = ""
		}
	}

	fmt.Print("Generic webhook URL (empty to skip): ")
	if scanner.Scan() {
		webhook = strings.TrimSpace(scanner.Text())
	}
	if webhook != "" {
		if err := notify.ValidateURL(webhook); err != nil {
			fmt.Printf("  warning: %v\n", err)
			webhook = ""
		}
	}

	if discord != "" || webhook != "" {
		fmt.Print("Quiet hours (e.g. 02:00-06:00, empty to skip): ")
		if scanner.Scan() {
			if v := strings.TrimSpace(scanner.Text()); v != "" {
				if _, err := notify.ParseQuietHours([]string{v}); err != nil {
					fmt.Printf("  warning: %v — skipping quiet hours\n", err)
				} else {
					quietHours = []string{v}
				}
			}
		}
	}
	return
}

// promptHTTPCheck collects a single HTTP check. Returns ok=false if name is empty.
func promptHTTPCheck(scanner *bufio.Scanner) (config.HTTPCheck, bool) {
	var check config.HTTPCheck
	fmt.Print("  Check name: ")
	if scanner.Scan() {
		check.Name = strings.TrimSpace(scanner.Text())
	}
	if check.Name == "" {
		return check, false
	}
	fmt.Print("  URL: ")
	if scanner.Scan() {
		check.URL = strings.TrimSpace(scanner.Text())
	}
	if u, err := url.Parse(check.URL); err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		fmt.Println("  warning: invalid URL (must be http or https)")
		return check, false
	}
	fmt.Print("  Expected status [200]: ")
	if scanner.Scan() {
		if v := strings.TrimSpace(scanner.Text()); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				check.ExpectedStatus = n
			}
		}
	}
	if check.ExpectedStatus == 0 {
		check.ExpectedStatus = 200
	}
	return check, true
}

// promptPortCheck collects a single TCP check. Returns ok=false if name is empty.
func promptPortCheck(scanner *bufio.Scanner) (config.PortCheck, bool) {
	var check config.PortCheck
	fmt.Print("  Check name: ")
	if scanner.Scan() {
		check.Name = strings.TrimSpace(scanner.Text())
	}
	if check.Name == "" {
		return check, false
	}
	fmt.Print("  Host: ")
	if scanner.Scan() {
		check.Host = strings.TrimSpace(scanner.Text())
	}
	fmt.Print("  Port: ")
	if scanner.Scan() {
		if v := strings.TrimSpace(scanner.Text()); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				if n < 1 || n > 65535 {
					fmt.Println("  warning: port must be 1-65535")
					return check, false
				}
				check.Port = n
			}
		}
	}
	return check, true
}

// maskSecrets redacts webhook URLs in TOML output for safe terminal display.
func maskSecrets(toml string) string {
	var out strings.Builder
	for _, line := range strings.Split(toml, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "discord_webhook") || strings.HasPrefix(trimmed, "webhook_url") {
			if eq := strings.Index(line, "="); eq >= 0 {
				out.WriteString(line[:eq+1])
				out.WriteString(` "***REDACTED***"`)
				out.WriteByte('\n')
				continue
			}
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return strings.TrimRight(out.String(), "\n")
}

// buildConfigTOML renders the collected config as a TOML string.
func buildConfigTOML(cfg initConfig) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("db_path = %q\n", cfg.dbPath))
	b.WriteString(fmt.Sprintf("interval = %q\n", cfg.interval))
	b.WriteString(fmt.Sprintf("retention = %q\n", cfg.retention))
	b.WriteString(fmt.Sprintf("theme = %q\n", cfg.theme))

	if cfg.dockerSocket != "" {
		b.WriteString("\n[docker]\n")
		b.WriteString(fmt.Sprintf("socket = %q\n", cfg.dockerSocket))
	} else {
		b.WriteString("\n# [docker]\n")
		b.WriteString("# socket = \"/var/run/docker.sock\"\n")
	}

	if cfg.discordWebhook != "" || cfg.webhookURL != "" || len(cfg.quietHours) > 0 {
		b.WriteString("\n[notifications]\n")
		if cfg.discordWebhook != "" {
			b.WriteString(fmt.Sprintf("discord_webhook = %q\n", cfg.discordWebhook))
		}
		if cfg.webhookURL != "" {
			b.WriteString(fmt.Sprintf("webhook_url = %q\n", cfg.webhookURL))
		}
		if len(cfg.quietHours) > 0 {
			quoted := make([]string, len(cfg.quietHours))
			for i, qh := range cfg.quietHours {
				quoted[i] = fmt.Sprintf("%q", qh)
			}
			b.WriteString(fmt.Sprintf("quiet_hours = [%s]\n", strings.Join(quoted, ", ")))
		}
	}

	for _, a := range cfg.alerts {
		b.WriteString("\n[[alerts]]\n")
		b.WriteString(fmt.Sprintf("metric = %q\n", a.Metric))
		b.WriteString(fmt.Sprintf("threshold = %.1f\n", a.Threshold))
		b.WriteString(fmt.Sprintf("above = %t\n", a.Above))
		b.WriteString(fmt.Sprintf("message = %q\n", a.Message))
		if a.SustainedTicks > 0 {
			b.WriteString(fmt.Sprintf("sustained_ticks = %d\n", a.SustainedTicks))
		}
	}

	for _, m := range cfg.mounts {
		b.WriteString("\n[[mount_checks]]\n")
		b.WriteString(fmt.Sprintf("path = %q\n", m.Path))
		if m.Label != "" {
			b.WriteString(fmt.Sprintf("label = %q\n", m.Label))
		}
	}

	if len(cfg.httpChecks) > 0 || len(cfg.portChecks) > 0 {
		b.WriteString("\n[services]\n")
		b.WriteString("interval = \"30s\"\n")
		b.WriteString("failures_before_alert = 2\n")

		for _, hc := range cfg.httpChecks {
			b.WriteString("\n[[http_checks]]\n")
			b.WriteString(fmt.Sprintf("name = %q\n", hc.Name))
			b.WriteString(fmt.Sprintf("url = %q\n", hc.URL))
			if hc.ExpectedStatus != 0 {
				b.WriteString(fmt.Sprintf("expected_status = %d\n", hc.ExpectedStatus))
			}
		}

		for _, pc := range cfg.portChecks {
			b.WriteString("\n[[port_checks]]\n")
			b.WriteString(fmt.Sprintf("name = %q\n", pc.Name))
			b.WriteString(fmt.Sprintf("host = %q\n", pc.Host))
			b.WriteString(fmt.Sprintf("port = %d\n", pc.Port))
		}
	}

	return b.String()
}

// mountDiscoverer returns available mounts. Injected for testability.
type mountDiscoverer func() []discoveredMount

// initEnv holds detected environment features, injected for testability.
type initEnv struct {
	dockerSocket string   // non-empty if Docker socket found
	thermalZones []string // thermal zone temp file paths
	inDocker     bool     // true if running inside a container
}

const (
	defaultDockerSocket = "/var/run/docker.sock"
	defaultThermalGlob  = "/sys/class/thermal/thermal_zone*/temp"
	defaultDockerMarker = "/.dockerenv"
)

// detectEnv probes the host for Docker socket, thermal zones, and container markers.
func detectEnv() initEnv {
	return initEnv{
		dockerSocket: detectDockerSocket(defaultDockerSocket),
		thermalZones: detectThermalZones(defaultThermalGlob),
		inDocker:     detectInDocker(defaultDockerMarker),
	}
}

// runInitCmd is the entry point called from main().
func runInitCmd(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	configPath := fs.String("config", "config.toml", "path to config file to create")
	fs.Parse(args)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)
	go func() {
		<-sigs
		fmt.Println("\nAborted.")
		os.Exit(0)
	}()

	err := runInit(*configPath, os.Stdin, discoverMounts, detectEnv())
	signal.Stop(sigs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// runInit runs the interactive init flow. Reads input from r for testability.
// discover is the mount discovery function (pass nil in tests to skip).
func runInit(configPath string, r io.Reader, discover mountDiscoverer, env initEnv) error {
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("%s already exists — delete it or use a different path with -config", configPath)
	}

	scanner := bufio.NewScanner(r)
	initStyles()

	fmt.Println(initTitleStyle.Render("▪ vigil init") + initDimStyle.Render(" — interactive configuration"))
	fmt.Println()

	var cfg initConfig

	// --- Database path ---
	defaultDB := "./vigil.db"
	if env.inDocker {
		defaultDB = "/data/vigil.db"
		fmt.Println(initDimStyle.Render("  /data is mapped to your host volume in docker-compose.yml"))
	}
	cfg.dbPath = prompt(scanner, "Database path", defaultDB)
	if dir := filepath.Dir(cfg.dbPath); dir != "." {
		if _, err := os.Stat(dir); err != nil {
			fmt.Printf("  %s directory %s does not exist (may be created at runtime)\n",
				initWarnStyle.Render("note:"), dir)
		}
	}

	// --- Mount selection ---
	var mounts []discoveredMount
	if discover != nil {
		mounts = discover()
	}
	if len(mounts) > 0 && isTerminal(os.Stdout) {
		fmt.Println()
		pm := pickerModel{mounts: mounts}
		result, err := tea.NewProgram(pm).Run()
		if err != nil {
			return fmt.Errorf("mount picker: %w", err)
		}
		final := result.(pickerModel)
		if !final.cancelled {
			for _, m := range final.mounts {
				if m.Selected {
					cfg.mounts = append(cfg.mounts, config.MountCheck{Path: m.Path})
				}
			}
		}
	} else if len(mounts) > 0 {
		fmt.Println("\nMount selection requires an interactive terminal — skipping.")
		fmt.Println("Use 'vigil mounts' to configure mount checks later.")
	} else {
		fmt.Println("\nNo non-root mounts detected — skipping mount selection.")
	}

	// --- Alert thresholds ---
	fmt.Println("\n" + initSectionStyle.Render("Alert thresholds") + initDimStyle.Render(" (press enter to accept default)") + ":")
	cpuThresh := promptThreshold(scanner, "CPU", 85.0)
	memThresh := promptThreshold(scanner, "Memory", 90.0)
	diskThresh := promptThreshold(scanner, "Disk", 85.0)

	cfg.alerts = []config.Alert{
		{Metric: metric.CPUPercent, Threshold: cpuThresh, Above: true,
			Message: fmt.Sprintf("CPU usage above %.0f%%", cpuThresh)},
		{Metric: metric.MemPercent, Threshold: memThresh, Above: true,
			Message: fmt.Sprintf("Memory usage above %.0f%%", memThresh)},
		{Metric: metric.DiskPercent, Threshold: diskThresh, Above: true,
			Message: fmt.Sprintf("Disk usage above %.0f%%", diskThresh)},
		{Metric: metric.DiskUtil, Threshold: 90.0, Above: true, SustainedTicks: 3,
			Message: "Disk utilization above 90%"},
		{Metric: metric.DiskLatency, Threshold: 50.0, Above: true, SustainedTicks: 3,
			Message: "Disk latency above 50ms"},
		{Metric: metric.SwapPercent, Threshold: 10.0, Above: true,
			Message: "Swap usage above 10%"},
		{Metric: metric.SwapIn, Threshold: 1.0, Above: true, SustainedTicks: 3,
			Message: "Sustained swap-in activity"},
		{Metric: metric.SwapOut, Threshold: 1.0, Above: true, SustainedTicks: 3,
			Message: "Sustained swap-out activity"},
		{Metric: metric.CPUIowait, Threshold: 10.0, Above: true, SustainedTicks: 3,
			Message: "CPU iowait above 10%"},
		{Metric: metric.SDErrors, Threshold: 0.0, Above: true,
			Message: "SD card errors detected"},
	}

	if len(env.thermalZones) > 0 {
		tempThresh := promptThreshold(scanner, "Temperature (°C)", 75.0)
		cfg.alerts = append(cfg.alerts, config.Alert{
			Metric: "temp", Threshold: tempThresh, Above: true,
			Message: fmt.Sprintf("Temperature above %.0f°C", tempThresh),
		})
	}

	// --- Optional section menu ---
	type optionalSection struct {
		label   string
		enabled bool
	}
	var sections []optionalSection
	if env.dockerSocket != "" {
		sections = append(sections, optionalSection{label: "Docker container monitoring"})
	}
	sections = append(sections,
		optionalSection{label: "Notifications (Discord / webhook)"},
		optionalSection{label: "Service checks (HTTP / TCP)"},
		optionalSection{label: "Collection interval & retention"},
	)

	if len(sections) > 0 {
		fmt.Println("\n" + initSectionStyle.Render("Optional features") + initDimStyle.Render(" (enter numbers to toggle, enter to confirm)") + ":")
		for i, s := range sections {
			check := "[ ]"
			if s.enabled {
				check = "[x]"
			}
			fmt.Printf("  %d. %s %s\n", i+1, check, s.label)
		}
		if scanner.Scan() {
			toggled := parseToggleInput(scanner.Text(), len(sections))
			for idx, on := range toggled {
				if on {
					sections[idx].enabled = true
				}
			}
		}
	}

	// Walk through selected optional sections.
	sectionIndex := 0
	if env.dockerSocket != "" {
		if sections[sectionIndex].enabled {
			fmt.Println()
			cfg.dockerSocket = prompt(scanner, "Docker socket", env.dockerSocket)
			if _, err := os.Stat(cfg.dockerSocket); err != nil {
				fmt.Printf("  warning: %s not found\n", cfg.dockerSocket)
			}
		}
		sectionIndex++
	}

	if sections[sectionIndex].enabled {
		fmt.Println()
		cfg.discordWebhook, cfg.webhookURL, cfg.quietHours = promptNotifications(scanner)
	}
	sectionIndex++

	if sections[sectionIndex].enabled {
		fmt.Println("\nService checks:")
		for {
			fmt.Print("Add service check? (h)ttp / (t)cp / (d)one: ")
			if !scanner.Scan() {
				break
			}
			switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
			case "h":
				if check, ok := promptHTTPCheck(scanner); ok {
					cfg.httpChecks = append(cfg.httpChecks, check)
				}
			case "t":
				if check, ok := promptPortCheck(scanner); ok {
					cfg.portChecks = append(cfg.portChecks, check)
				}
			default:
				goto doneChecks
			}
		}
	}
doneChecks:
	sectionIndex++

	// Defaults for interval, retention, and theme.
	cfg.interval = "2s"
	cfg.retention = "12h"
	cfg.theme = "auto"
	if sections[sectionIndex].enabled {
		fmt.Println()
		cfg.interval = prompt(scanner, "Collection interval", "2s")
		cfg.retention = prompt(scanner, "Data retention", "12h")
	}

	// --- Preview and confirm ---
	tomlOutput := buildConfigTOML(cfg)
	divider := initDimStyle.Render("───")
	fmt.Println("\n" + divider + " " + initSectionStyle.Render("Generated config.toml") + " " + divider)
	fmt.Println(initPreviewStyle.Render(maskSecrets(tomlOutput)))
	fmt.Println(divider)

	fmt.Printf("Write to %s? [Y/n]: ", configPath)
	if scanner.Scan() {
		if v := strings.ToLower(strings.TrimSpace(scanner.Text())); v == "n" || v == "no" {
			fmt.Println("Not written.")
			return nil
		}
	}

	if err := os.WriteFile(configPath, []byte(tomlOutput), 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	if _, err := config.Load(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: config validation failed: %v\n", err)
	} else {
		fmt.Printf("Config written to %s\n", configPath)
	}

	return nil
}
