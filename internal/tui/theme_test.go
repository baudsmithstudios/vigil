package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestResolveTheme_Dark(t *testing.T) {
	if got := ResolveTheme("dark"); got != DarkTheme {
		t.Errorf("ResolveTheme(dark) = %+v, want DarkTheme", got)
	}
}

func TestResolveTheme_Light(t *testing.T) {
	if got := ResolveTheme("light"); got != LightTheme {
		t.Errorf("ResolveTheme(light) = %+v, want LightTheme", got)
	}
}

func TestResolveTheme_AutoReturnsValidTheme(t *testing.T) {
	theme := ResolveTheme("auto")
	if theme != DarkTheme && theme != LightTheme {
		t.Errorf("auto resolved to unknown theme: %+v", theme)
	}
}

func TestBuildStyles_SetsAccentOnAppTitle(t *testing.T) {
	// Force ANSI256 so lipgloss emits color codes regardless of terminal.
	lipgloss.DefaultRenderer().SetColorProfile(termenv.ANSI256)
	buildStyles(DarkTheme)
	rendered := appTitleStyle.Render("test")
	// ANSI 36 produces an escape sequence containing "38;5;36".
	if !strings.Contains(rendered, "38;5;36") {
		t.Errorf("expected appTitleStyle to use ANSI 36, got %q", rendered)
	}
}
