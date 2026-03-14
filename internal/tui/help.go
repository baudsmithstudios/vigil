package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type helpEntry struct {
	key  string
	desc string
}

type helpSection struct {
	title   string
	entries []helpEntry
}

func renderHelp(width, height int) string {
	sections := []helpSection{
		{
			title: "Global",
			entries: []helpEntry{
				{"?", "Toggle help"},
				{"q / Ctrl+C", "Quit"},
				{"m", "Mute/unmute notifications"},
			},
		},
		{
			title: "Dashboard",
			entries: []helpEntry{
				{"j / ↓", "Next alert"},
				{"k / ↑", "Previous alert"},
				{"d", "Dismiss selected alert"},
			},
		},
	}

	var b strings.Builder
	b.WriteString(helpTitleStyle.Render("▪ Help"))
	b.WriteString("\n\n")

	for i, section := range sections {
		b.WriteString(helpSectionStyle.Render(section.title))
		b.WriteString("\n")
		for _, entry := range section.entries {
			b.WriteString(helpKeyStyle.Render(entry.key))
			b.WriteString(helpDescStyle.Render(entry.desc))
			b.WriteString("\n")
		}
		if i < len(sections)-1 {
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(helpFooterStyle.Render("Press ? or Esc to close"))

	modal := helpBorderStyle.Render(b.String())
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}
