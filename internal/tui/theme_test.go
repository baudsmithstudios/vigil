package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestResolveTheme_Named(t *testing.T) {
	for name, want := range map[string]Theme{"dark": DarkTheme, "light": LightTheme} {
		if got := ResolveTheme(name); got != want {
			t.Errorf("ResolveTheme(%s) = %+v, want %+v", name, got, want)
		}
	}
}

func TestResolveTheme_AutoReturnsValidTheme(t *testing.T) {
	theme := ResolveTheme("auto")
	if theme != DarkTheme && theme != LightTheme {
		t.Errorf("auto resolved to unknown theme: %+v", theme)
	}
}

func TestBuildStyles_SetsAccentOnAppTitle(t *testing.T) {
	// Lipgloss emits no color codes unless a profile is forced.
	lipgloss.DefaultRenderer().SetColorProfile(termenv.ANSI256)
	buildStyles(DarkTheme)
	rendered := appTitleStyle.Render("test")
	if !strings.Contains(rendered, "38;5;36") {
		t.Errorf("expected appTitleStyle to use ANSI 36, got %q", rendered)
	}
}
