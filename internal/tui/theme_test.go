package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestResolveTheme_Dark(t *testing.T) {
	theme := ResolveTheme("dark")
	if theme.Accent != "36" {
		t.Errorf("expected dark accent ANSI 36, got %s", string(theme.Accent))
	}
}

func TestResolveTheme_Light(t *testing.T) {
	theme := ResolveTheme("light")
	if theme.Accent != "23" {
		t.Errorf("expected light accent ANSI 23, got %s", string(theme.Accent))
	}
}

func TestResolveTheme_AutoReturnsValidTheme(t *testing.T) {
	theme := ResolveTheme("auto")
	if theme.Accent == "" {
		t.Error("expected auto to resolve to a theme with non-empty Accent")
	}
	if theme.Accent != DarkTheme.Accent && theme.Accent != LightTheme.Accent {
		t.Errorf("auto resolved to unknown theme with accent %s", string(theme.Accent))
	}
}

func TestDarkTheme_HasAllRoles(t *testing.T) {
	// Verify no role is empty.
	colors := map[string]lipgloss.Color{
		"Base":      DarkTheme.Base,
		"Surface":   DarkTheme.Surface,
		"Border":    DarkTheme.Border,
		"Divider":   DarkTheme.Divider,
		"Text":      DarkTheme.Text,
		"Dim":       DarkTheme.Dim,
		"Accent":    DarkTheme.Accent,
		"SubAccent": DarkTheme.SubAccent,
		"Ok":        DarkTheme.Ok,
		"Warn":      DarkTheme.Warn,
		"Critical":  DarkTheme.Critical,
	}
	for role, code := range colors {
		if string(code) == "" {
			t.Errorf("DarkTheme.%s is empty", role)
		}
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

func TestLightTheme_HasAllRoles(t *testing.T) {
	colors := map[string]lipgloss.Color{
		"Base":      LightTheme.Base,
		"Surface":   LightTheme.Surface,
		"Border":    LightTheme.Border,
		"Divider":   LightTheme.Divider,
		"Text":      LightTheme.Text,
		"Dim":       LightTheme.Dim,
		"Accent":    LightTheme.Accent,
		"SubAccent": LightTheme.SubAccent,
		"Ok":        LightTheme.Ok,
		"Warn":      LightTheme.Warn,
		"Critical":  LightTheme.Critical,
	}
	for role, code := range colors {
		if string(code) == "" {
			t.Errorf("LightTheme.%s is empty", role)
		}
	}
}
