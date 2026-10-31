package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"vigil/internal/checker"
	"vigil/internal/collector"
)

func TestDistributeWidths(t *testing.T) {
	tests := []struct {
		name    string
		term    int
		weights []int
		want    []int
	}{
		{"equal weights", 66, []int{1, 1, 1}, []int{18, 18, 18}},
		{"remainder goes to the first panel", 164, []int{3, 1, 1}, []int{92, 30, 30}},
		{"terminal narrower than the overhead", 5, []int{1, 1}, []int{1, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := distributeWidths(tt.term, tt.weights); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("distributeWidths(%d, %v) = %v, want %v", tt.term, tt.weights, got, tt.want)
			}
		})
	}
}

func TestSelectLayout_Breakpoints(t *testing.T) {
	tests := []struct {
		width         int
		rows          int
		firstRowCells int
	}{
		{160, 3, 2},
		{159, 4, 1},
		{100, 4, 1},
		{99, 6, 1},
	}
	for _, tt := range tests {
		rows := selectLayout(tt.width)
		if len(rows) != tt.rows || len(rows[0]) != tt.firstRowCells {
			t.Errorf("width %d: %d rows with %d cells first, want %d and %d", tt.width, len(rows), len(rows[0]), tt.rows, tt.firstRowCells)
		}
	}
}

func TestRenderDashboard_WideStacksPanelsWithContent(t *testing.T) {
	buildStyles(DarkTheme)
	snap := collector.Snapshot{
		Services:   []checker.ServiceStatus{{Name: "web", Up: true}},
		Containers: []collector.ContainerSnapshot{{Name: "hass", Status: "running"}},
	}
	output := renderDashboard(snap, nil, 0, false, 180)
	for _, want := range []string{"Services", "Containers"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %s panel in output", want)
		}
	}
	if strings.Contains(output, "Mounts") {
		t.Error("expected the empty Mounts panel to be dropped")
	}
}

func TestRenderRow_SkipsEmptyPanels(t *testing.T) {
	buildStyles(DarkTheme)
	contents := map[string]string{
		"cpu": "  test content\n",
		"ctr": "",
	}
	row := []panelSpec{{name: "cpu", title: "CPU", weight: 3}, {name: "ctr", title: "Containers", weight: 1}}
	result := renderRow(row, contents, nil, 120)
	if !strings.Contains(result, "CPU") {
		t.Error("rendered row should contain CPU panel")
	}
	if strings.Contains(result, "Containers") {
		t.Error("empty container panel should be excluded")
	}
}

func TestRenderPanel_TruncatesStyledLineByVisibleWidth(t *testing.T) {
	const width, innerW = 12, 10
	styled := "\x1b[31m" + strings.Repeat("x", 20) + "\x1b[0m"

	row := strings.Split(renderPanel("T", styled, width), "\n")[1]

	if got := strings.Count(row, "x"); got != innerW {
		t.Errorf("kept %d visible chars, want %d: %q", got, innerW, row)
	}
	if got := lipgloss.Width(row); got != width+2 {
		t.Errorf("row width = %d, want %d: %q", got, width+2, row)
	}
}

func TestLongNameKeepsGapBeforeNextColumn(t *testing.T) {
	const name = "homeassistant"

	containers := renderContainerContent([]collector.ContainerSnapshot{{Name: name, Status: "running"}})
	if !strings.Contains(containers, "… ●") {
		t.Errorf("no gap after truncated container name: %q", containers)
	}
	services := renderServiceContent([]checker.ServiceStatus{{Name: name, Up: true}})
	if !strings.Contains(services, "… ✓") {
		t.Errorf("no gap after truncated service name: %q", services)
	}
}

func TestRenderMountContent_MissingOutranksUnstable(t *testing.T) {
	buildStyles(DarkTheme)
	tests := []struct {
		mount collector.MountStatus
		want  string
	}{
		{collector.MountStatus{Path: "/a", Mounted: true}, "OK"},
		{collector.MountStatus{Path: "/a", Mounted: true, Unstable: true}, "UNSTABLE"},
		{collector.MountStatus{Path: "/a", Unstable: true}, "MISSING"},
	}
	for _, tt := range tests {
		if got := renderMountContent([]collector.MountStatus{tt.mount}, 60); !strings.Contains(got, tt.want) {
			t.Errorf("%+v rendered %q, want %s", tt.mount, got, tt.want)
		}
	}
}

func TestRenderHeader_DropsDetailAsWidthShrinks(t *testing.T) {
	buildStyles(DarkTheme)
	snap := collector.Snapshot{
		Load:        collector.LoadSnapshot{Load1: 1.23, Load5: 0.45, Load15: 0.67},
		Temperature: []collector.TempSnapshot{{Celsius: 47.5}},
		UptimeSec:   3*86400 + 12*3600,
		Throttle:    collector.ThrottleSnapshot{Available: true},
	}
	tests := []struct {
		name   string
		width  int
		want   []string
		absent []string
	}{
		{"full", 120, []string{"VIGIL", "1.23", "47.5", "3d 12h", "PWR: OK", "3 alerts"}, nil},
		{"load and temperature only", 46, []string{"VIGIL", "1.23", "47.5"}, []string{"3d 12h", "PWR", "alerts"}},
		{"title only", 40, []string{"VIGIL"}, []string{"1.23"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := renderHeader(tt.width, snap, 3)
			for _, want := range tt.want {
				if !strings.Contains(header, want) {
					t.Errorf("header missing %q: %q", want, header)
				}
			}
			for _, absent := range tt.absent {
				if strings.Contains(header, absent) {
					t.Errorf("header should drop %q: %q", absent, header)
				}
			}
		})
	}
}

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		sec  uint64
		want string
	}{
		{0, "0m"},
		{3599, "59m"},
		{3600, "1h 0m"},
		{86399, "23h 59m"},
		{86400, "1d 0h"},
	}
	for _, tt := range tests {
		got := formatUptime(tt.sec)
		if got != tt.want {
			t.Errorf("formatUptime(%d) = %q, want %q", tt.sec, got, tt.want)
		}
	}
}
