package tui

import "github.com/charmbracelet/lipgloss"

// Theme defines the semantic color roles for the dashboard.
type Theme struct {
	Base      lipgloss.Color
	Surface   lipgloss.Color
	Border    lipgloss.Color
	Divider   lipgloss.Color
	Text      lipgloss.Color
	Dim       lipgloss.Color
	Accent    lipgloss.Color
	SubAccent lipgloss.Color
	Ok        lipgloss.Color
	Warn      lipgloss.Color
	Critical  lipgloss.Color
}

var DarkTheme = Theme{
	Base:      "232",
	Surface:   "233",
	Border:    "72",
	Divider:   "23",
	Text:      "252",
	Dim:       "245",
	Accent:    "36",
	SubAccent: "29",
	Ok:        "36",
	Warn:      "178",
	Critical:  "196",
}

var LightTheme = Theme{
	Base:      "231",
	Surface:   "254",
	Border:    "29",
	Divider:   "72",
	Text:      "234",
	Dim:       "242",
	Accent:    "23",
	SubAccent: "29",
	Ok:        "23",
	Warn:      "130",
	Critical:  "124",
}

// ResolveTheme returns the theme for the given config setting.
// For "auto", it queries the terminal background color; in non-terminal
// environments (e.g. tests) it defaults to dark.
func ResolveTheme(setting string) Theme {
	switch setting {
	case "light":
		return LightTheme
	case "dark":
		return DarkTheme
	default:
		// "auto" or empty: query terminal, default to dark.
		if lipgloss.HasDarkBackground() {
			return DarkTheme
		}
		return LightTheme
	}
}
