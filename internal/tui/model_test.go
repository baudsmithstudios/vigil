package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"vigil/internal/alert"
	"vigil/internal/checker"
	"vigil/internal/collector"
)

func TestModel_AlertFireAndResolve(t *testing.T) {
	m := New("dark", nil, nil)

	result, _ := m.Update(AlertMsg{Fired: []alert.State{{Name: "cpu_percent", Message: "CPU high"}}})
	m = result.(Model)
	if len(m.activeAlerts) != 1 {
		t.Fatalf("expected 1 active alert, got %d", len(m.activeAlerts))
	}

	// Resolved alerts stay in the list but are marked resolved.
	result, _ = m.Update(AlertMsg{Resolved: []alert.State{{Name: "cpu_percent"}}})
	m = result.(Model)
	if len(m.activeAlerts) != 1 {
		t.Fatalf("expected 1 alert still present after resolve, got %d", len(m.activeAlerts))
	}
	if !m.activeAlerts[0].Resolved {
		t.Error("expected alert to be marked resolved")
	}
	if m.activeAlerts[0].ResolvedAt.IsZero() {
		t.Error("expected ResolvedAt to be set")
	}
}

func TestModel_AlertCursorClampsOnDismiss(t *testing.T) {
	m := New("dark", nil, nil)

	// Fire 3 alerts.
	result, _ := m.Update(AlertMsg{Fired: []alert.State{
		{Name: "a", Message: "a"},
		{Name: "b", Message: "b"},
		{Name: "c", Message: "c"},
	}})
	m = result.(Model)

	// Move cursor to last alert (index 2) and dismiss it.
	m.alertCursor = 2
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = result.(Model)
	if m.alertCursor >= len(m.activeAlerts) {
		t.Errorf("alertCursor %d out of bounds for %d alerts", m.alertCursor, len(m.activeAlerts))
	}
}

func TestModel_DismissResolvedAlert(t *testing.T) {
	m := New("dark", nil, nil)

	// Fire and resolve an alert.
	result, _ := m.Update(AlertMsg{Fired: []alert.State{{Name: "cpu_iowait", Message: "iowait high"}}})
	m = result.(Model)
	result, _ = m.Update(AlertMsg{Resolved: []alert.State{{Name: "cpu_iowait"}}})
	m = result.(Model)
	if len(m.activeAlerts) != 1 {
		t.Fatalf("expected 1 resolved alert, got %d", len(m.activeAlerts))
	}

	// Dismiss the resolved alert.
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = result.(Model)
	if len(m.activeAlerts) != 0 {
		t.Errorf("expected 0 alerts after dismissing resolved alert, got %d", len(m.activeAlerts))
	}
}

func TestModel_QuitOnQ(t *testing.T) {
	m := New("dark", nil, nil)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("expected a quit command")
	}
}

func TestModel_HelpToggle(t *testing.T) {
	m := New("dark", nil, nil)

	// ? opens help
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = result.(Model)
	if !m.showHelp {
		t.Error("expected showHelp=true after pressing ?")
	}

	// ? closes help
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = result.(Model)
	if m.showHelp {
		t.Error("expected showHelp=false after pressing ? again")
	}
}

func TestModel_HelpDismissEsc(t *testing.T) {
	m := New("dark", nil, nil)
	m.showHelp = true

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = result.(Model)
	if m.showHelp {
		t.Error("expected showHelp=false after pressing Esc")
	}
}

func TestModel_HelpSwallowsKeys(t *testing.T) {
	m := New("dark", nil, nil)
	m.showHelp = true
	m.activeAlerts = []alert.State{{Name: "test", Message: "test"}}

	// j should not move alert cursor
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = result.(Model)
	if m.alertCursor != 0 {
		t.Error("expected j to be swallowed when help is shown")
	}

	// d should not dismiss alert
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = result.(Model)
	if len(m.activeAlerts) != 1 {
		t.Error("expected d to be swallowed when help is shown")
	}
}

func TestRenderDiskContent_UsesParentDeviceIO(t *testing.T) {
	content := renderDiskContent(
		[]collector.DiskSnapshot{
			{MountPoint: "/", Device: "mmcblk0p2", UsedBytes: 1024, TotalBytes: 2048, Percent: 50},
		},
		[]collector.DiskIOSnapshot{
			{Device: "mmcblk0", ReadRate: 4096, WriteRate: 2048, UtilPercent: 77, LatencyMs: 4.2},
		},
		60,
	)

	if !strings.Contains(content, " util   77%\n await   4.2ms") {
		t.Fatalf("expected parent-device util/await fallback on separate lines, got:\n%s", content)
	}
}

func TestModel_QuitWorksWithHelp(t *testing.T) {
	m := New("dark", nil, nil)
	m.showHelp = true

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("expected quit command even when help is shown")
	}
}

func TestModel_HelpViewRenders(t *testing.T) {
	m := New("dark", nil, nil)

	// Simulate terminal resize
	result, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = result.(Model)

	// Toggle help on
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = result.(Model)

	output := m.View()
	for _, want := range []string{"Help", "Toggle help", "Global", "Dashboard", "Press ? or Esc to close"} {
		if !strings.Contains(output, want) {
			t.Errorf("help overlay missing %q", want)
		}
	}
}

func TestModel_ServicesPanelRendered(t *testing.T) {
	m := New("dark", nil, nil)
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = result.(Model)

	result, _ = m.Update(SnapshotMsg(collector.Snapshot{
		Services: []checker.ServiceStatus{
			{Name: "web", CheckType: "http", Up: true, StatusCode: 200, Latency: 23 * time.Millisecond, CheckedAt: time.Now()},
			{Name: "mqtt", CheckType: "tcp", Up: false, Error: "refused", CheckedAt: time.Now()},
		},
	}))
	m = result.(Model)

	output := m.View()
	if !strings.Contains(output, "Services") {
		t.Error("expected Services panel in output")
	}
	if !strings.Contains(output, "web") {
		t.Error("expected 'web' service in output")
	}
	if !strings.Contains(output, "mqtt") {
		t.Error("expected 'mqtt' service in output")
	}
	if !strings.Contains(output, "UP") {
		t.Error("expected UP status in output")
	}
	if !strings.Contains(output, "DOWN") {
		t.Error("expected DOWN status in output")
	}
}
