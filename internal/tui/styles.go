package tui

import "github.com/charmbracelet/lipgloss"

// Package-level style vars. Set by buildStyles() at startup.
var (
	appTitleStyle      lipgloss.Style
	headerStyle        lipgloss.Style
	sectionTitleStyle  lipgloss.Style
	labelStyle         lipgloss.Style
	valueStyle         lipgloss.Style
	unitStyle          lipgloss.Style
	dimStyle           lipgloss.Style
	gaugeHighStyle     lipgloss.Style
	gaugeMediumStyle   lipgloss.Style
	gaugeLowStyle      lipgloss.Style
	tempHighStyle      lipgloss.Style
	tempMediumStyle    lipgloss.Style
	tempLowStyle       lipgloss.Style
	alertStyle         lipgloss.Style
	alertResolvedStyle lipgloss.Style
	alertCursorStyle   lipgloss.Style

	containerRunningStyle lipgloss.Style
	containerStoppedStyle lipgloss.Style
	containerOtherStyle   lipgloss.Style

	mountOkStyle       lipgloss.Style
	mountMissingStyle  lipgloss.Style
	mountUnstableStyle lipgloss.Style

	serviceUpStyle   lipgloss.Style
	serviceDownStyle lipgloss.Style

	pwrOkStyle        lipgloss.Style
	pwrWarnStyle      lipgloss.Style
	pwrThrottledStyle lipgloss.Style

	// panelStyle wraps each dashboard section. Width(w) sets the content+padding
	// area; total terminal width = w + 2 (left/right border chars).
	panelStyle       lipgloss.Style
	helpBorderStyle  lipgloss.Style
	helpTitleStyle   lipgloss.Style
	helpKeyStyle     lipgloss.Style
	helpDescStyle    lipgloss.Style
	helpSectionStyle lipgloss.Style
	helpFooterStyle  lipgloss.Style
	hintKeyStyle     lipgloss.Style

	dividerStyle lipgloss.Style
)

func buildStyles(t Theme) {
	appTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(t.Accent)
	headerStyle = lipgloss.NewStyle().Background(t.Surface).Foreground(t.Dim)
	sectionTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(t.Accent)
	labelStyle = lipgloss.NewStyle().Foreground(t.Text).Width(14)
	valueStyle = lipgloss.NewStyle().Foreground(t.Text).Bold(true)
	unitStyle = lipgloss.NewStyle().Foreground(t.Dim)
	dimStyle = lipgloss.NewStyle().Foreground(t.Dim)
	gaugeHighStyle = lipgloss.NewStyle().Foreground(t.Critical)
	gaugeMediumStyle = lipgloss.NewStyle().Foreground(t.Warn)
	gaugeLowStyle = lipgloss.NewStyle().Foreground(t.Ok)

	tempHighStyle = lipgloss.NewStyle().Foreground(t.Critical).Bold(true)
	tempMediumStyle = lipgloss.NewStyle().Foreground(t.Warn)
	tempLowStyle = lipgloss.NewStyle().Foreground(t.Ok)

	alertStyle = lipgloss.NewStyle().Foreground(t.Critical).Bold(true)
	alertResolvedStyle = lipgloss.NewStyle().Foreground(t.Ok)
	alertCursorStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)

	containerRunningStyle = lipgloss.NewStyle().Foreground(t.Ok)
	containerStoppedStyle = lipgloss.NewStyle().Foreground(t.Critical)
	containerOtherStyle = lipgloss.NewStyle().Foreground(t.Warn)

	mountOkStyle = lipgloss.NewStyle().Foreground(t.Ok)
	mountMissingStyle = lipgloss.NewStyle().Foreground(t.Critical).Bold(true)
	mountUnstableStyle = lipgloss.NewStyle().Foreground(t.Warn).Bold(true)

	serviceUpStyle = lipgloss.NewStyle().Foreground(t.Ok)
	serviceDownStyle = lipgloss.NewStyle().Foreground(t.Critical).Bold(true)

	pwrOkStyle = lipgloss.NewStyle().Foreground(t.Ok)
	pwrWarnStyle = lipgloss.NewStyle().Foreground(t.Warn).Bold(true)
	pwrThrottledStyle = lipgloss.NewStyle().Foreground(t.Critical).Bold(true)

	panelStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Border).
		Padding(0, 1)

	helpBorderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Border).
		Padding(1, 2)

	helpTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(t.Accent)
	helpKeyStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Width(16)
	hintKeyStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	helpDescStyle = lipgloss.NewStyle().Foreground(t.Text)
	helpSectionStyle = lipgloss.NewStyle().Foreground(t.SubAccent).Bold(true)
	helpFooterStyle = lipgloss.NewStyle().Foreground(t.Dim)

	dividerStyle = lipgloss.NewStyle().Foreground(t.Divider)
}
