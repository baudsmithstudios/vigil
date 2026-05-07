package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateDockerSocket(t *testing.T) {
	tests := []struct {
		name    string
		socket  string
		wantErr bool
	}{
		{"empty is valid", "", false},
		{"standard path", "/var/run/docker.sock", false},
		{"path traversal", "/var/run/../etc/shadow", true},
		{"relative path", "docker.sock", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Docker.Socket = tt.socket
			err := cfg.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	cfg, err := Load("/nonexistent/config.toml")
	if err != nil {
		t.Fatalf("expected defaults on missing file, got error: %v", err)
	}
	if cfg.DBPath != "/data/vigil.db" {
		t.Errorf("expected default db_path, got %s", cfg.DBPath)
	}
}

func TestValidateMountChecks(t *testing.T) {
	tests := []struct {
		name    string
		checks  []MountCheck
		wantErr bool
	}{
		{"empty is valid", nil, false},
		{"valid path", []MountCheck{{Path: "/mnt/data"}}, false},
		{"valid with label", []MountCheck{{Path: "/mnt/data", Label: "NAS"}}, false},
		{"relative path", []MountCheck{{Path: "mnt/data"}}, true},
		{"path traversal", []MountCheck{{Path: "/mnt/../etc/shadow"}}, true},
		{"empty path", []MountCheck{{Path: ""}}, true},
		{"duplicate paths", []MountCheck{{Path: "/mnt/a"}, {Path: "/mnt/a"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.MountChecks = tt.checks
			err := cfg.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadMountChecks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[[mount_checks]]
path = "/mnt/data"
label = "NAS Storage"

[[mount_checks]]
path = "/media/usb0"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.MountChecks) != 2 {
		t.Fatalf("expected 2 mount_checks, got %d", len(cfg.MountChecks))
	}
	if cfg.MountChecks[0].Path != "/mnt/data" || cfg.MountChecks[0].Label != "NAS Storage" {
		t.Errorf("unexpected first mount_check: %+v", cfg.MountChecks[0])
	}
	if cfg.MountChecks[1].Path != "/media/usb0" || cfg.MountChecks[1].Label != "" {
		t.Errorf("unexpected second mount_check: %+v", cfg.MountChecks[1])
	}
}

func TestLoadNotificationsNtfyDefaultServer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[notifications]
ntfy_topic = "vigil-alerts"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Notifications.NtfyTopic != "vigil-alerts" {
		t.Errorf("expected ntfy topic, got %q", cfg.Notifications.NtfyTopic)
	}
	if cfg.Notifications.NtfyServer != "https://ntfy.sh" {
		t.Errorf("expected default ntfy server, got %q", cfg.Notifications.NtfyServer)
	}
}

func TestLoadNotificationsNtfyCustomServer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[notifications]
ntfy_topic = "vigil-alerts"
ntfy_server = "https://ntfy.example.com"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Notifications.NtfyServer != "https://ntfy.example.com" {
		t.Errorf("expected custom ntfy server, got %q", cfg.Notifications.NtfyServer)
	}
}

func TestValidateHTTPChecks(t *testing.T) {
	tests := []struct {
		name    string
		checks  []HTTPCheck
		wantErr bool
	}{
		{"empty is valid", nil, false},
		{"valid http", []HTTPCheck{{Name: "web", URL: "http://example.com"}}, false},
		{"valid https", []HTTPCheck{{Name: "secure", URL: "https://example.com"}}, false},
		{"missing name", []HTTPCheck{{URL: "http://example.com"}}, true},
		{"missing url", []HTTPCheck{{Name: "web"}}, true},
		{"invalid scheme", []HTTPCheck{{Name: "ftp", URL: "ftp://example.com"}}, true},
		{"not a url", []HTTPCheck{{Name: "bad", URL: "not a url"}}, true},
		{"duplicate names", []HTTPCheck{{Name: "web", URL: "http://a.com"}, {Name: "web", URL: "http://b.com"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Services.Interval = TestDuration(30 * time.Second)
			cfg.HTTPChecks = tt.checks
			err := cfg.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePortChecks(t *testing.T) {
	tests := []struct {
		name    string
		checks  []PortCheck
		wantErr bool
	}{
		{"empty is valid", nil, false},
		{"valid", []PortCheck{{Name: "ssh", Host: "192.168.1.1", Port: 22}}, false},
		{"missing name", []PortCheck{{Host: "192.168.1.1", Port: 22}}, true},
		{"missing host", []PortCheck{{Name: "ssh", Port: 22}}, true},
		{"port zero", []PortCheck{{Name: "ssh", Host: "192.168.1.1", Port: 0}}, true},
		{"port too high", []PortCheck{{Name: "ssh", Host: "192.168.1.1", Port: 65536}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Services.Interval = TestDuration(30 * time.Second)
			cfg.PortChecks = tt.checks
			err := cfg.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateDuplicateNamesAcrossCheckTypes(t *testing.T) {
	cfg := Defaults()
	cfg.Services.Interval = TestDuration(30 * time.Second)
	cfg.HTTPChecks = []HTTPCheck{{Name: "myservice", URL: "http://example.com"}}
	cfg.PortChecks = []PortCheck{{Name: "myservice", Host: "192.168.1.1", Port: 22}}
	err := cfg.validate()
	if err == nil {
		t.Error("expected error for duplicate name across check types, got nil")
	}
}

func TestValidateServicesInterval(t *testing.T) {
	cfg := Defaults()
	cfg.Services.Interval = TestDuration(0)
	cfg.HTTPChecks = []HTTPCheck{{Name: "web", URL: "http://example.com"}}
	err := cfg.validate()
	if err == nil {
		t.Error("expected error for zero interval with checks configured, got nil")
	}
}

func TestLoadHTTPAndPortChecks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[services]
interval = "15s"
failures_before_alert = 3

[[http_checks]]
name = "website"
url = "https://example.com"
expected_status = 200

[[port_checks]]
name = "ssh"
host = "192.168.1.1"
port = 22
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Services.Interval.Duration != 15*time.Second {
		t.Errorf("expected 15s interval, got %s", cfg.Services.Interval.Duration)
	}
	if cfg.Services.FailuresBeforeAlert != 3 {
		t.Errorf("expected failures_before_alert 3, got %d", cfg.Services.FailuresBeforeAlert)
	}
	if len(cfg.HTTPChecks) != 1 {
		t.Fatalf("expected 1 http_check, got %d", len(cfg.HTTPChecks))
	}
	if cfg.HTTPChecks[0].Name != "website" || cfg.HTTPChecks[0].URL != "https://example.com" || cfg.HTTPChecks[0].ExpectedStatus != 200 {
		t.Errorf("unexpected http_check: %+v", cfg.HTTPChecks[0])
	}
	if len(cfg.PortChecks) != 1 {
		t.Fatalf("expected 1 port_check, got %d", len(cfg.PortChecks))
	}
	if cfg.PortChecks[0].Name != "ssh" || cfg.PortChecks[0].Host != "192.168.1.1" || cfg.PortChecks[0].Port != 22 {
		t.Errorf("unexpected port_check: %+v", cfg.PortChecks[0])
	}
}

func TestLoadHTTPCheckDefaultExpectedStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[services]
interval = "10s"

[[http_checks]]
name = "web"
url = "http://example.com"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPChecks[0].ExpectedStatus != 0 {
		t.Errorf("expected default expected_status 0, got %d", cfg.HTTPChecks[0].ExpectedStatus)
	}
}

func TestValidateTheme(t *testing.T) {
	tests := []struct {
		name    string
		theme   string
		wantErr bool
	}{
		{"auto is valid", "auto", false},
		{"dark is valid", "dark", false},
		{"light is valid", "light", false},
		{"empty defaults to auto", "", false},
		{"invalid theme", "neon", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Theme = tt.theme
			err := cfg.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateDuplicateAlertMetrics(t *testing.T) {
	cfg := Defaults()
	cfg.Alerts = []Alert{
		{Metric: "cpu_percent", Threshold: 90, Above: true, Message: "cpu high"},
		{Metric: "cpu_percent", Threshold: 95, Above: true, Message: "cpu very high"},
	}

	err := cfg.validate()
	if err == nil {
		t.Fatal("expected error for duplicate alert metrics, got nil")
	}
}

func TestLoadValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `db_path = "/tmp/test.db"
interval = "5s"
retention = "24h"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DBPath != "/tmp/test.db" {
		t.Errorf("expected /tmp/test.db, got %s", cfg.DBPath)
	}
}
