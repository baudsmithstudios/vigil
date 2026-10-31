package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"vigil/internal/alert"
	"vigil/internal/checker"
	"vigil/internal/collector"
	"vigil/internal/metric"
	"vigil/internal/notify"
)

func update(m Model, msg tea.Msg) Model {
	result, _ := m.Update(msg)
	return result.(Model)
}

func key(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func withAlerts(m Model, names ...string) Model {
	var fired []alert.State
	for _, name := range names {
		fired = append(fired, alert.State{Name: name, Message: name})
	}
	return update(m, AlertMsg{Fired: fired})
}

func TestModel_AlertFireAndResolve(t *testing.T) {
	m := withAlerts(New("dark", nil, nil), "cpu_percent")

	m = update(m, AlertMsg{Resolved: []alert.State{{Name: "cpu_percent"}}})
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

func TestModel_DismissReportsAlertName(t *testing.T) {
	dismissed := make(chan string, 1)
	m := withAlerts(New("dark", func(name string) { dismissed <- name }, nil), "a", "b")

	m = update(m, key("d"))
	select {
	case name := <-dismissed:
		if name != "a" {
			t.Errorf("dismissed %q, want a", name)
		}
	case <-time.After(time.Second):
		t.Fatal("onDismiss was not called")
	}
	if len(m.activeAlerts) != 1 || m.activeAlerts[0].Name != "b" {
		t.Errorf("expected only b left, got %+v", m.activeAlerts)
	}
}

func TestModel_AlertCursorClampsOnDismiss(t *testing.T) {
	m := withAlerts(New("dark", nil, nil), "a", "b", "c")
	m.alertCursor = 2

	m = update(m, key("d"))
	if m.alertCursor != 1 {
		t.Errorf("alertCursor = %d, want 1", m.alertCursor)
	}
}

func TestModel_AlertCursorStopsAtEnds(t *testing.T) {
	m := withAlerts(New("dark", nil, nil), "a", "b")

	m = update(m, key("k"))
	if m.alertCursor != 0 {
		t.Errorf("k at the top moved the cursor to %d", m.alertCursor)
	}
	m = update(update(m, key("j")), key("j"))
	if m.alertCursor != 1 {
		t.Errorf("j past the end moved the cursor to %d", m.alertCursor)
	}
}

func TestModel_DismissResolvedThrottleAlert(t *testing.T) {
	m := withAlerts(New("dark", nil, nil), metric.AlertThrottle)
	m = update(m, AlertMsg{Resolved: []alert.State{{Name: metric.AlertThrottle}}})

	m = update(m, key("d"))
	if len(m.activeAlerts) != 0 {
		t.Errorf("expected resolved throttle alert dismissed, got %d alerts", len(m.activeAlerts))
	}
}

func TestModel_ActiveThrottleAlertIsNotDismissable(t *testing.T) {
	m := withAlerts(New("dark", nil, nil), metric.AlertThrottle)

	m = update(m, key("d"))
	if len(m.activeAlerts) != 1 {
		t.Errorf("expected active throttle alert kept, got %d alerts", len(m.activeAlerts))
	}
}

func TestModel_MuteKeyTogglesMute(t *testing.T) {
	mute := &notify.Mute{}
	update(New("dark", nil, mute), key("m"))
	if !mute.IsMuted() {
		t.Error("expected m to mute")
	}

	update(New("dark", nil, nil), key("m"))
}

func TestModel_Quit(t *testing.T) {
	for _, showHelp := range []bool{false, true} {
		m := New("dark", nil, nil)
		m.showHelp = showHelp
		_, cmd := m.Update(key("q"))
		if cmd == nil {
			t.Fatalf("showHelp=%v: expected a quit command", showHelp)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("showHelp=%v: expected tea.QuitMsg", showHelp)
		}
	}
}

func TestModel_HelpToggle(t *testing.T) {
	m := update(New("dark", nil, nil), key("?"))
	if !m.showHelp {
		t.Error("expected showHelp=true after pressing ?")
	}

	m = update(m, key("?"))
	if m.showHelp {
		t.Error("expected showHelp=false after pressing ? again")
	}
}

func TestModel_HelpDismissEsc(t *testing.T) {
	m := New("dark", nil, nil)
	m.showHelp = true

	m = update(m, tea.KeyMsg{Type: tea.KeyEscape})
	if m.showHelp {
		t.Error("expected showHelp=false after pressing Esc")
	}
}

func TestModel_HelpSwallowsKeys(t *testing.T) {
	m := withAlerts(New("dark", nil, nil), "a", "b")
	m.showHelp = true

	m = update(m, key("j"))
	if m.alertCursor != 0 {
		t.Error("expected j to be swallowed when help is shown")
	}

	m = update(m, key("d"))
	if len(m.activeAlerts) != 2 {
		t.Error("expected d to be swallowed when help is shown")
	}
}

func TestModel_HelpViewRenders(t *testing.T) {
	m := update(New("dark", nil, nil), tea.WindowSizeMsg{Width: 80, Height: 24})
	m = update(m, key("?"))

	output := m.View()
	for _, want := range []string{"Help", "Toggle help", "Global", "Dashboard", "Press ? or Esc to close"} {
		if !strings.Contains(output, want) {
			t.Errorf("help overlay missing %q", want)
		}
	}
}

func TestModel_ViewFitsTerminalHeight(t *testing.T) {
	m := update(New("dark", nil, nil), tea.WindowSizeMsg{Width: 120, Height: 5})
	m = update(m, SnapshotMsg(collector.Snapshot{}))

	if got := strings.Count(m.View(), "\n") + 1; got != 5 {
		t.Errorf("view is %d lines, want 5", got)
	}
}

func TestModel_RendersEveryLayoutWithNoOptInPanels(t *testing.T) {
	for _, width := range []int{80, 120, 180} {
		m := update(New("dark", nil, nil), tea.WindowSizeMsg{Width: width, Height: 40})
		m = update(m, SnapshotMsg(collector.Snapshot{}))
		if !strings.Contains(m.View(), "VIGIL") {
			t.Errorf("width %d: expected the dashboard to render", width)
		}
	}
}

func TestModel_ServicesPanelRendered(t *testing.T) {
	m := update(New("dark", nil, nil), tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, SnapshotMsg(collector.Snapshot{
		Services: []checker.ServiceStatus{
			{Name: "web", CheckType: "http", Up: true, StatusCode: 200, Latency: 23 * time.Millisecond, CheckedAt: time.Now()},
			{Name: "mqtt", CheckType: "tcp", Up: false, Error: "refused", CheckedAt: time.Now()},
		},
	}))

	output := m.View()
	for _, want := range []string{"Services", "web", "mqtt", "UP", "DOWN"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output", want)
		}
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

	for _, want := range []string{"r 4.0K/s", "w 2.0K/s", "util 77%", "await 4.2ms"} {
		if !strings.Contains(content, want) {
			t.Errorf("expected disk content to contain %q, got:\n%s", want, content)
		}
	}
}
