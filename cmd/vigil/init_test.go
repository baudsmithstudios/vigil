package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"vigil/internal/config"
	"vigil/internal/metric"
)

func TestPromptDefault(t *testing.T) {
	// Empty input → returns default
	scanner := bufio.NewScanner(strings.NewReader("\n"))
	got := prompt(scanner, "DB path", "/data/vigil.db")
	if got != "/data/vigil.db" {
		t.Errorf("expected default, got %q", got)
	}

	// Custom input → returns trimmed input
	scanner = bufio.NewScanner(strings.NewReader("  /custom/path.db  \n"))
	got = prompt(scanner, "DB path", "/data/vigil.db")
	if got != "/custom/path.db" {
		t.Errorf("expected /custom/path.db, got %q", got)
	}
}

func TestPromptThreshold(t *testing.T) {
	// Empty input → returns default
	scanner := bufio.NewScanner(strings.NewReader("\n"))
	got := promptThreshold(scanner, "CPU", 90.0)
	if got != 90.0 {
		t.Errorf("expected 90.0, got %f", got)
	}

	// Valid input
	scanner = bufio.NewScanner(strings.NewReader("85\n"))
	got = promptThreshold(scanner, "CPU", 90.0)
	if got != 85.0 {
		t.Errorf("expected 85.0, got %f", got)
	}
}

func TestBuildConfigTOML(t *testing.T) {
	cfg := initConfig{
		dbPath:    "/data/vigil.db",
		interval:  "2s",
		retention: "12h",
		alerts: []config.Alert{
			{Metric: metric.CPUPercent, Threshold: 90.0, Above: true, Message: "CPU usage above 90%"},
		},
	}
	toml := buildConfigTOML(cfg)
	if !strings.Contains(toml, `db_path = "/data/vigil.db"`) {
		t.Error("expected db_path in output")
	}
	if !strings.Contains(toml, `metric = "cpu_percent"`) {
		t.Error("expected cpu_percent alert in output")
	}
}

func TestBuildConfigTOML_Docker(t *testing.T) {
	cfg := initConfig{
		dbPath:       "/data/vigil.db",
		interval:     "2s",
		retention:    "12h",
		dockerSocket: "/var/run/docker.sock",
	}
	toml := buildConfigTOML(cfg)
	if !strings.Contains(toml, `socket = "/var/run/docker.sock"`) {
		t.Error("expected docker socket in output")
	}
}

func TestBuildConfigTOML_Notifications(t *testing.T) {
	cfg := initConfig{
		dbPath:         "/data/vigil.db",
		interval:       "2s",
		retention:      "12h",
		discordWebhook: "https://discord.com/api/webhooks/123/abc",
		ntfyTopic:      "vigil-alerts",
		ntfyServer:     "https://ntfy.example.com",
		quietHours:     []string{"02:00-06:00"},
	}
	toml := buildConfigTOML(cfg)
	if !strings.Contains(toml, `discord_webhook = "https://discord.com/api/webhooks/123/abc"`) {
		t.Error("expected discord webhook in output")
	}
	if !strings.Contains(toml, `ntfy_topic = "vigil-alerts"`) {
		t.Error("expected ntfy topic in output")
	}
	if !strings.Contains(toml, `ntfy_server = "https://ntfy.example.com"`) {
		t.Error("expected ntfy server in output")
	}
	if !strings.Contains(toml, `quiet_hours = ["02:00-06:00"]`) {
		t.Error("expected quiet hours in output")
	}
}

func TestMaskSecrets_RedactsNtfyTopic(t *testing.T) {
	input := `[notifications]
ntfy_topic = "private-topic"
ntfy_server = "https://ntfy.example.com"`
	got := maskSecrets(input)
	if strings.Contains(got, "private-topic") {
		t.Fatalf("expected ntfy topic to be redacted, got %q", got)
	}
	if !strings.Contains(got, `ntfy_server = "https://ntfy.example.com"`) {
		t.Fatalf("expected ntfy server to remain visible, got %q", got)
	}
}

func TestBuildConfigTOML_ServiceChecks(t *testing.T) {
	cfg := initConfig{
		dbPath:    "/data/vigil.db",
		interval:  "2s",
		retention: "12h",
		httpChecks: []config.HTTPCheck{
			{Name: "web", URL: "https://example.com", ExpectedStatus: 200},
		},
		portChecks: []config.PortCheck{
			{Name: "ssh", Host: "192.0.2.1", Port: 22},
		},
	}
	toml := buildConfigTOML(cfg)
	if !strings.Contains(toml, `name = "web"`) {
		t.Error("expected http check in output")
	}
	if !strings.Contains(toml, `port = 22`) {
		t.Error("expected port check in output")
	}
	if !strings.Contains(toml, `[services]`) {
		t.Error("expected services section in output")
	}
}

func TestParseToggleInput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		count    int
		expected map[int]bool
	}{
		{"single", "1", 4, map[int]bool{0: true}},
		{"multiple", "1 3", 4, map[int]bool{0: true, 2: true}},
		{"out of range ignored", "1 5", 4, map[int]bool{0: true}},
		{"empty confirms", "", 4, map[int]bool{}},
		{"non-numeric ignored", "1 abc 3", 4, map[int]bool{0: true, 2: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseToggleInput(tt.input, tt.count)
			for k, v := range tt.expected {
				if got[k] != v {
					t.Errorf("index %d: expected %v, got %v", k, v, got[k])
				}
			}
		})
	}
}

func TestPromptNotifications(t *testing.T) {
	// Skip discord, provide webhook, provide quiet hours
	input := "\nhttps://hooks.example.com/alert\n\n02:00-06:00\n"
	scanner := bufio.NewScanner(strings.NewReader(input))
	discord, webhook, ntfyTopic, ntfyServer, qh := promptNotifications(scanner)
	if discord != "" {
		t.Errorf("expected empty discord, got %q", discord)
	}
	if webhook != "https://hooks.example.com/alert" {
		t.Errorf("expected webhook URL, got %q", webhook)
	}
	if ntfyTopic != "" || ntfyServer != "" {
		t.Errorf("expected empty ntfy config, got topic=%q server=%q", ntfyTopic, ntfyServer)
	}
	if len(qh) != 1 || qh[0] != "02:00-06:00" {
		t.Errorf("expected quiet hours, got %v", qh)
	}
}

func TestPromptNotifications_Ntfy(t *testing.T) {
	input := "\n\nvigil-alerts\nhttps://ntfy.example.com\n02:00-06:00\n"
	scanner := bufio.NewScanner(strings.NewReader(input))
	discord, webhook, ntfyTopic, ntfyServer, qh := promptNotifications(scanner)
	if discord != "" || webhook != "" {
		t.Errorf("expected empty webhooks, got discord=%q webhook=%q", discord, webhook)
	}
	if ntfyTopic != "vigil-alerts" {
		t.Errorf("expected ntfy topic, got %q", ntfyTopic)
	}
	if ntfyServer != "https://ntfy.example.com" {
		t.Errorf("expected ntfy server, got %q", ntfyServer)
	}
	if len(qh) != 1 || qh[0] != "02:00-06:00" {
		t.Errorf("expected quiet hours, got %v", qh)
	}
}

func TestPromptNotifications_InvalidNtfyTopic(t *testing.T) {
	input := "\n\nvigil alerts\n"
	scanner := bufio.NewScanner(strings.NewReader(input))
	discord, webhook, ntfyTopic, ntfyServer, qh := promptNotifications(scanner)
	if discord != "" || webhook != "" || ntfyTopic != "" || ntfyServer != "" || len(qh) != 0 {
		t.Errorf("expected all empty, got discord=%q webhook=%q ntfyTopic=%q ntfyServer=%q qh=%v", discord, webhook, ntfyTopic, ntfyServer, qh)
	}
}

func TestPromptNotifications_NoWebhooks(t *testing.T) {
	// Skip notification destinations, so quiet hours are not prompted.
	input := "\n\n\n\n"
	scanner := bufio.NewScanner(strings.NewReader(input))
	discord, webhook, ntfyTopic, ntfyServer, qh := promptNotifications(scanner)
	if discord != "" || webhook != "" || ntfyTopic != "" || ntfyServer != "" || len(qh) != 0 {
		t.Errorf("expected all empty, got discord=%q webhook=%q ntfyTopic=%q ntfyServer=%q qh=%v", discord, webhook, ntfyTopic, ntfyServer, qh)
	}
}

func TestPromptHTTPCheck(t *testing.T) {
	input := "myapi\nhttps://api.example.com/health\n200\n"
	scanner := bufio.NewScanner(strings.NewReader(input))
	check, ok := promptHTTPCheck(scanner)
	if !ok {
		t.Fatal("expected ok")
	}
	if check.Name != "myapi" || check.URL != "https://api.example.com/health" || check.ExpectedStatus != 200 {
		t.Errorf("unexpected check: %+v", check)
	}
}

func TestPromptPortCheck(t *testing.T) {
	input := "ssh\n192.0.2.1\n22\n"
	scanner := bufio.NewScanner(strings.NewReader(input))
	check, ok := promptPortCheck(scanner)
	if !ok {
		t.Fatal("expected ok")
	}
	if check.Name != "ssh" || check.Host != "192.0.2.1" || check.Port != 22 {
		t.Errorf("unexpected check: %+v", check)
	}
}

func TestInitRefusesExistingConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}
	err := runInit(path, strings.NewReader(""), nil, initEnv{})
	if err == nil {
		t.Error("expected error for existing config")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInitWritesValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Simulate: accept default db path, accept default CPU/mem/disk thresholds,
	// no optional sections selected, confirm write.
	input := strings.Join([]string{
		"",  // db path (accept default)
		"",  // CPU threshold (accept default)
		"",  // mem threshold (accept default)
		"",  // disk threshold (accept default)
		"",  // optional sections (none selected, enter to confirm)
		"y", // confirm write
	}, "\n") + "\n"

	err := runInit(path, strings.NewReader(input), nil, initEnv{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("generated config is invalid: %v", err)
	}
	if len(cfg.Alerts) == 0 {
		t.Fatal("expected default alerts to be generated")
	}
}

func TestInitDeclinedWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	input := strings.Join([]string{
		"",  // db path
		"",  // CPU threshold
		"",  // mem threshold
		"",  // disk threshold
		"",  // optional sections
		"n", // decline write
	}, "\n") + "\n"

	err := runInit(path, strings.NewReader(input), nil, initEnv{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(path); err == nil {
		t.Error("config file should not have been written")
	}
}

func TestBuildConfigTOML_IncludesTheme(t *testing.T) {
	cfg := initConfig{
		dbPath:    "/data/vigil.db",
		interval:  "2s",
		retention: "12h",
		theme:     "auto",
	}
	toml := buildConfigTOML(cfg)
	if !strings.Contains(toml, `theme = "auto"`) {
		t.Error("expected theme in output")
	}
}

func TestInitDefaultThresholds_PiTuned(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Accept all defaults (empty input for each prompt), no optional sections, confirm write.
	input := strings.Join([]string{
		"",  // db path
		"",  // CPU threshold (accept default)
		"",  // mem threshold (accept default)
		"",  // disk threshold (accept default)
		"",  // optional sections (none)
		"y", // confirm
	}, "\n") + "\n"

	err := runInit(path, strings.NewReader(input), nil, initEnv{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("invalid config: %v", err)
	}

	// Check Pi-tuned defaults.
	thresholds := make(map[string]float64)
	for _, a := range cfg.Alerts {
		thresholds[a.Metric] = a.Threshold
	}
	if thresholds[metric.CPUPercent] != 85.0 {
		t.Errorf("expected CPU threshold 85, got %.0f", thresholds[metric.CPUPercent])
	}
	if thresholds[metric.MemPercent] != 90.0 {
		t.Errorf("expected memory threshold 90, got %.0f", thresholds[metric.MemPercent])
	}
	if thresholds[metric.DiskPercent] != 85.0 {
		t.Errorf("expected disk threshold 85, got %.0f", thresholds[metric.DiskPercent])
	}
}

func TestInitDefaultThresholds_TempPiTuned(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Simulate environment with thermal zones.
	zone := filepath.Join(dir, "thermal_zone0")
	os.MkdirAll(zone, 0755)
	os.WriteFile(filepath.Join(zone, "temp"), []byte("42000"), 0644)

	env := initEnv{
		thermalZones: []string{filepath.Join(zone, "temp")},
	}

	input := strings.Join([]string{
		"",  // db path
		"",  // CPU threshold
		"",  // mem threshold
		"",  // disk threshold
		"",  // temp threshold (accept default)
		"",  // optional sections
		"y", // confirm
	}, "\n") + "\n"

	err := runInit(path, strings.NewReader(input), nil, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("invalid config: %v", err)
	}

	for _, a := range cfg.Alerts {
		if a.Metric == "temp" && a.Threshold != 75.0 {
			t.Errorf("expected temp threshold 75, got %.0f", a.Threshold)
		}
	}
}

func TestInitDefaultAlerts_IncludeDiskIOMetrics(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	input := strings.Join([]string{
		"",  // db path
		"",  // CPU threshold
		"",  // mem threshold
		"",  // disk threshold
		"",  // optional sections
		"y", // confirm
	}, "\n") + "\n"

	err := runInit(path, strings.NewReader(input), nil, initEnv{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("invalid config: %v", err)
	}

	metrics := make(map[string]bool)
	for _, a := range cfg.Alerts {
		metrics[a.Metric] = true
	}
	if !metrics[metric.SwapPercent] {
		t.Error("expected swap_percent alert in defaults")
	}
	if !metrics[metric.SwapIn] {
		t.Error("expected swap_in alert in defaults")
	}
	if !metrics[metric.SwapOut] {
		t.Error("expected swap_out alert in defaults")
	}
	if !metrics[metric.CPUIowait] {
		t.Error("expected cpu_iowait alert in defaults")
	}
	if !metrics[metric.DiskUtil] {
		t.Error("expected disk_util alert in defaults")
	}
	if !metrics[metric.DiskLatency] {
		t.Error("expected disk_latency_ms alert in defaults")
	}
	if !metrics[metric.SDErrors] {
		t.Error("expected sd_errors alert in defaults")
	}
}

func TestInitDefaultAlerts_RelaxesSDLatencyThreshold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	input := strings.Join([]string{
		"",  // db path
		"",  // CPU
		"",  // mem
		"",  // disk
		"",  // optional sections
		"y", // confirm
	}, "\n") + "\n"

	env := initEnv{sdDevices: []string{"mmcblk0"}}
	if err := runInit(path, strings.NewReader(input), nil, env); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("invalid config: %v", err)
	}

	var generic, specific *config.Alert
	for i := range cfg.Alerts {
		a := &cfg.Alerts[i]
		switch a.Metric {
		case metric.DiskLatency:
			generic = a
		case metric.PrefixDiskLatency + "mmcblk0":
			specific = a
		}
	}
	if generic == nil {
		t.Fatal("expected generic disk_latency_ms alert to still be written")
	}
	if generic.Threshold != 50.0 {
		t.Errorf("generic disk_latency_ms threshold = %.0f, want 50", generic.Threshold)
	}
	if specific == nil {
		t.Fatal("expected disk_latency_ms:mmcblk0 override alert")
	}
	if specific.Threshold != 200.0 {
		t.Errorf("specific threshold = %.0f, want 200", specific.Threshold)
	}
	if specific.SustainedTicks != 5 {
		t.Errorf("specific sustained_ticks = %d, want 5", specific.SustainedTicks)
	}
}

func TestInitDefaultAlerts_NoSDOverrideWhenNoSDDetected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	input := strings.Join([]string{
		"",  // db path
		"",  // CPU
		"",  // mem
		"",  // disk
		"",  // optional sections
		"y", // confirm
	}, "\n") + "\n"

	if err := runInit(path, strings.NewReader(input), nil, initEnv{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("invalid config: %v", err)
	}
	for _, a := range cfg.Alerts {
		if strings.HasPrefix(a.Metric, metric.PrefixDiskLatency) {
			t.Errorf("did not expect any disk_latency_ms:* rules, got %q", a.Metric)
		}
	}
}

func TestInitGeneratedConfig_HasThemeAuto(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	input := strings.Join([]string{
		"",  // db path
		"",  // CPU
		"",  // mem
		"",  // disk
		"",  // optional sections
		"y", // confirm
	}, "\n") + "\n"

	err := runInit(path, strings.NewReader(input), nil, initEnv{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), `theme = "auto"`) {
		t.Error("expected theme = auto in generated config")
	}
}

func TestBuildConfigTOML_MountChecks(t *testing.T) {
	cfg := initConfig{
		dbPath:    "/data/vigil.db",
		interval:  "2s",
		retention: "12h",
		mounts: []config.MountCheck{
			{Path: "/mnt/data"},
			{Path: "/media/usb0"},
		},
	}
	toml := buildConfigTOML(cfg)
	if !strings.Contains(toml, `path = "/mnt/data"`) {
		t.Error("expected first mount in output")
	}
	if !strings.Contains(toml, `path = "/media/usb0"`) {
		t.Error("expected second mount in output")
	}
}
