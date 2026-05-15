package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"vigil/internal/alert"
	"vigil/internal/collector"
	"vigil/internal/metric"
	"vigil/internal/notify"
)

type SnapshotMsg collector.Snapshot

type AlertMsg struct {
	Fired    []alert.State
	Resolved []alert.State
}

// DismissFunc is called when the user presses 'd' on a selected alert.
type DismissFunc func(name string)

// Model is the bubbletea root for the dashboard.
type Model struct {
	width    int
	height   int
	showHelp bool

	lastSnap     collector.Snapshot
	hasSnap      bool
	activeAlerts []alert.State
	alertCursor  int // selected alert index for dismissal
	onDismiss    DismissFunc
	mute         *notify.Mute // nil if no notifier configured
}

func New(themeSetting string, onDismiss DismissFunc, mute *notify.Mute) Model {
	buildStyles(ResolveTheme(themeSetting))
	return Model{
		onDismiss: onDismiss,
		mute:      mute,
	}
}

// SetAlerts seeds the active alert list (e.g. restored from DB on startup).
func (m *Model) SetAlerts(alerts []alert.State) {
	m.activeAlerts = alerts
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "?":
			m.showHelp = !m.showHelp
			return m, nil
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.showHelp {
				m.showHelp = false
				return m, nil
			}
		default:
			if m.showHelp {
				return m, nil
			}
			switch msg.String() {
			case "d":
				if len(m.activeAlerts) > 0 {
					// Throttle alerts auto-resolve only; skip manual dismiss.
					if m.activeAlerts[m.alertCursor].Name == metric.AlertThrottle {
						break
					}
					dismissed := m.activeAlerts[m.alertCursor]
					m.activeAlerts = append(m.activeAlerts[:m.alertCursor], m.activeAlerts[m.alertCursor+1:]...)
					if m.alertCursor >= len(m.activeAlerts) && m.alertCursor > 0 {
						m.alertCursor--
					}
					if m.onDismiss != nil {
						go m.onDismiss(dismissed.Name)
					}
				}
			case "j", "down":
				if len(m.activeAlerts) > 0 && m.alertCursor < len(m.activeAlerts)-1 {
					m.alertCursor++
				}
			case "k", "up":
				if m.alertCursor > 0 {
					m.alertCursor--
				}
			case "m":
				if m.mute != nil {
					m.mute.Toggle()
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case SnapshotMsg:
		snap := collector.Snapshot(msg)
		m.lastSnap = snap
		m.hasSnap = true

	case AlertMsg:
		for _, a := range msg.Fired {
			m.activeAlerts = append(m.activeAlerts, a)
		}
		if len(msg.Resolved) > 0 {
			resolvedNames := make(map[string]bool, len(msg.Resolved))
			for _, a := range msg.Resolved {
				resolvedNames[a.Name] = true
			}
			for i := range m.activeAlerts {
				if resolvedNames[m.activeAlerts[i].Name] {
					m.activeAlerts[i].Resolved = true
					m.activeAlerts[i].ResolvedAt = time.Now()
				}
			}
		}
	}

	return m, nil
}

func (m Model) View() string {
	if m.showHelp && m.width > 0 {
		return renderHelp(m.width, m.height)
	}
	if m.width == 0 || !m.hasSnap {
		return fmt.Sprintf("%s\n", dimStyle.Render("waiting for data…"))
	}
	muted := m.mute != nil && m.mute.IsMuted()
	dashboard := renderDashboard(m.lastSnap, m.activeAlerts, m.alertCursor, muted, m.width)

	// Truncate dashboard to fit terminal height, preserving header at top.
	if m.height > 0 {
		lines := strings.Split(dashboard, "\n")
		if len(lines) > m.height {
			lines = lines[:m.height]
		}
		dashboard = strings.Join(lines, "\n")
	}

	return dashboard
}
