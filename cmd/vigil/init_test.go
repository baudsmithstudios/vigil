package main

import (
	"bufio"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"vigil/internal/config"
	"vigil/internal/metric"
)

func answers(input string) *bufio.Scanner {
	return bufio.NewScanner(strings.NewReader(input))
}

func TestPrompt(t *testing.T) {
	if got := prompt(answers("\n"), "DB path", "/data/vigil.db"); got != "/data/vigil.db" {
		t.Errorf("expected default, got %q", got)
	}
	if got := prompt(answers("  /custom/path.db  \n"), "DB path", "/data/vigil.db"); got != "/custom/path.db" {
		t.Errorf("expected /custom/path.db, got %q", got)
	}
}

func TestPromptThreshold(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"\n", 90.0},
		{"85\n", 85.0},
		{"abc\n", 90.0},
	}
	for _, tt := range tests {
		if got := promptThreshold(answers(tt.input), "CPU", 90.0); got != tt.want {
			t.Errorf("input %q: got %f, want %f", tt.input, got, tt.want)
		}
	}
}

func TestBuildConfigTOML(t *testing.T) {
	tests := []struct {
		name string
		cfg  initConfig
		want []string
	}{
		{"core settings", initConfig{dbPath: "/data/vigil.db", theme: "auto"}, []string{`db_path = "/data/vigil.db"`, `theme = "auto"`}},
		{
			"alerts",
			initConfig{alerts: []config.Alert{{Metric: metric.CPUPercent, Threshold: 90.0, Above: true, Message: "CPU usage above 90%"}}},
			[]string{`metric = "cpu_percent"`},
		},
		{"docker", initConfig{dockerSocket: "/var/run/docker.sock"}, []string{`socket = "/var/run/docker.sock"`}},
		{
			"notifications",
			initConfig{
				discordWebhook: "https://discord.com/api/webhooks/123/abc",
				ntfyTopic:      "vigil-alerts",
				ntfyServer:     "https://ntfy.example.com",
				quietHours:     []string{"02:00-06:00"},
			},
			[]string{
				`discord_webhook = "https://discord.com/api/webhooks/123/abc"`,
				`ntfy_topic = "vigil-alerts"`,
				`ntfy_server = "https://ntfy.example.com"`,
				`quiet_hours = ["02:00-06:00"]`,
			},
		},
		{
			"service checks",
			initConfig{
				httpChecks: []config.HTTPCheck{{Name: "web", URL: "https://example.com", ExpectedStatus: 200}},
				portChecks: []config.PortCheck{{Name: "ssh", Host: "192.0.2.1", Port: 22}},
			},
			[]string{`[services]`, `name = "web"`, `port = 22`},
		},
		{
			"mounts",
			initConfig{mounts: []config.MountCheck{{Path: "/mnt/data"}, {Path: "/media/usb0"}}},
			[]string{`path = "/mnt/data"`, `path = "/media/usb0"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toml := buildConfigTOML(tt.cfg)
			for _, want := range tt.want {
				if !strings.Contains(toml, want) {
					t.Errorf("expected %s in:\n%s", want, toml)
				}
			}
		})
	}
}

func TestMaskSecrets(t *testing.T) {
	got := maskSecrets(`[notifications]
discord_webhook = "https://discord.com/api/webhooks/123/secret-a"
webhook_url = "https://hooks.example.com/secret-b"
ntfy_topic = "secret-c"
ntfy_server = "https://ntfy.example.com"`)
	if strings.Contains(got, "secret-") {
		t.Fatalf("expected secrets to be redacted, got %q", got)
	}
	if !strings.Contains(got, `ntfy_server = "https://ntfy.example.com"`) {
		t.Fatalf("expected ntfy server to remain visible, got %q", got)
	}
}

func TestParseToggleInput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		count    int
		expected []int
	}{
		{"multiple", "1 3", 4, []int{0, 2}},
		{"out of range ignored", "0 1 4 5", 4, []int{0, 3}},
		{"empty confirms", "", 4, nil},
		{"non-numeric ignored", "1 abc 3", 4, []int{0, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseToggleInput(tt.input, tt.count)
			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d entries, got %d", len(tt.expected), len(got))
			}
			for _, idx := range tt.expected {
				if _, ok := got[idx]; !ok {
					t.Errorf("missing index %d", idx)
				}
			}
		})
	}
}

func TestPromptNotifications(t *testing.T) {
	type result struct {
		discord, webhook, ntfyTopic, ntfyServer string
		quietHours                              []string
	}
	tests := []struct {
		name  string
		input string
		want  result
	}{
		{
			"webhook with quiet hours",
			"\nhttps://hooks.example.com/alert\n\n02:00-06:00\n",
			result{webhook: "https://hooks.example.com/alert", quietHours: []string{"02:00-06:00"}},
		},
		{
			"ntfy with quiet hours",
			"\n\nvigil-alerts\nhttps://ntfy.example.com\n02:00-06:00\n",
			result{ntfyTopic: "vigil-alerts", ntfyServer: "https://ntfy.example.com", quietHours: []string{"02:00-06:00"}},
		},
		{"invalid ntfy topic dropped", "\n\nvigil alerts\n", result{}},
		{"no destinations skips quiet hours", "\n\n\n02:00-06:00\n", result{}},
		{"invalid quiet hours dropped", "\nhttps://hooks.example.com/alert\n\nnever\n", result{webhook: "https://hooks.example.com/alert"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got result
			got.discord, got.webhook, got.ntfyTopic, got.ntfyServer, got.quietHours = promptNotifications(answers(tt.input))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPromptHTTPCheck(t *testing.T) {
	check, ok := promptHTTPCheck(answers("myapi\nhttps://api.example.com/health\n\n"))
	if !ok {
		t.Fatal("expected ok")
	}
	if check.Name != "myapi" || check.URL != "https://api.example.com/health" || check.ExpectedStatus != 200 {
		t.Errorf("unexpected check: %+v", check)
	}

	for _, input := range []string{"\n", "myapi\nftp://example.com\n", "myapi\nnot a url\n"} {
		if check, ok := promptHTTPCheck(answers(input)); ok {
			t.Errorf("input %q: accepted %+v", input, check)
		}
	}
}

func TestPromptPortCheck(t *testing.T) {
	check, ok := promptPortCheck(answers("ssh\n192.0.2.1\n22\n"))
	if !ok {
		t.Fatal("expected ok")
	}
	if check.Name != "ssh" || check.Host != "192.0.2.1" || check.Port != 22 {
		t.Errorf("unexpected check: %+v", check)
	}

	for _, input := range []string{"\n", "ssh\n\n22\n", "ssh\nhost\n\n", "ssh\nhost\nabc\n", "ssh\nhost\n0\n", "ssh\nhost\n65536\n"} {
		if check, ok := promptPortCheck(answers(input)); ok {
			t.Errorf("input %q: accepted %+v", input, check)
		}
	}
}

func TestInitRefusesExistingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}
	err := runInit(path, strings.NewReader(""), nil, initEnv{})
	if err == nil {
		t.Fatal("expected error for existing config")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("unexpected error: %v", err)
	}
}

// acceptDefaults presses enter at every prompt, then answers the write prompt.
func acceptDefaults(env initEnv, confirm string) *strings.Reader {
	blanks := 5
	if len(env.thermalZones) > 0 {
		blanks++
	}
	return strings.NewReader(strings.Repeat("\n", blanks) + confirm + "\n")
}

func runDefaultInit(t *testing.T, env initEnv) config.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := runInit(path, acceptDefaults(env, "y"), nil, env); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("generated config is invalid: %v", err)
	}
	return cfg
}

func TestInitDeclinedWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := runInit(path, acceptDefaults(initEnv{}, "n"), nil, initEnv{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("config file should not have been written")
	}
}

func TestInitDefaultThresholds_PiTuned(t *testing.T) {
	cfg := runDefaultInit(t, initEnv{thermalZones: []string{"thermal_zone0/temp"}})

	got := make(map[string]float64)
	for _, a := range cfg.Alerts {
		got[a.Metric] = a.Threshold
	}
	want := map[string]float64{metric.CPUPercent: 85, metric.MemPercent: 90, metric.DiskPercent: 85, "temp": 75}
	for name, threshold := range want {
		if got[name] != threshold {
			t.Errorf("%s threshold = %.0f, want %.0f", name, got[name], threshold)
		}
	}
}

func TestInitDefaultAlerts_IncludeDiskIOMetrics(t *testing.T) {
	cfg := runDefaultInit(t, initEnv{})

	found := make(map[string]bool)
	for _, a := range cfg.Alerts {
		found[a.Metric] = true
	}
	for _, name := range []string{metric.SwapPercent, metric.SwapIn, metric.SwapOut, metric.CPUIowait, metric.DiskUtil, metric.DiskLatency, metric.SDErrors} {
		if !found[name] {
			t.Errorf("expected %s alert in defaults", name)
		}
	}
}

func TestInitDefaultAlerts_RelaxesSDLatencyThreshold(t *testing.T) {
	cfg := runDefaultInit(t, initEnv{sdDevices: []string{"mmcblk0"}})

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
	cfg := runDefaultInit(t, initEnv{})
	for _, a := range cfg.Alerts {
		if strings.HasPrefix(a.Metric, metric.PrefixDiskLatency) {
			t.Errorf("did not expect any disk_latency_ms:* rules, got %q", a.Metric)
		}
	}
}
