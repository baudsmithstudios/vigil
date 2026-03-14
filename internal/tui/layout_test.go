package tui

import (
	"strings"
	"testing"

	"vigil/internal/collector"
)

func TestDistributeWidths_EqualWeights(t *testing.T) {
	// 3 panels with weight 1 each, terminal width 66.
	// Available = 66 - (3 * 4) = 54. Each gets 54/3 = 18.
	widths := distributeWidths(66, []int{1, 1, 1})
	if len(widths) != 3 {
		t.Fatalf("expected 3 widths, got %d", len(widths))
	}
	for i, w := range widths {
		if w != 18 {
			t.Errorf("panel %d: expected width 18, got %d", i, w)
		}
	}
}

func TestDistributeWidths_MixedWeights(t *testing.T) {
	// wide(3) + compact(1) + compact(1), terminal width 164.
	// Available = 164 - (3 * 4) = 152. Weights sum = 5.
	// wide = 152*3/5 = 91, compact = 152*1/5 = 30.
	// 91 + 30 + 30 = 151, remainder 1 goes to first panel.
	widths := distributeWidths(164, []int{3, 1, 1})
	total := 0
	for _, w := range widths {
		total += w
	}
	if total != 152 {
		t.Errorf("expected total available 152, got %d", total)
	}
	if widths[0] < 90 {
		t.Errorf("wide panel should get ~91 cols, got %d", widths[0])
	}
}

func TestSelectLayout_Wide(t *testing.T) {
	rows := selectLayout(180)
	if len(rows) != 3 {
		t.Fatalf("wide layout should have 3 rows, got %d", len(rows))
	}
	for i, row := range rows {
		if len(row) != 2 {
			t.Errorf("row %d: expected 2 cells, got %d", i, len(row))
		}
	}
	// Row 3 right cell should be a stack of services, containers, mounts.
	lastRow := rows[2]
	if len(lastRow[1].stack) != 3 {
		t.Errorf("row 3 right cell should stack 3 panels, got %d", len(lastRow[1].stack))
	}
}

func TestSelectLayout_Medium(t *testing.T) {
	rows := selectLayout(120)
	// Medium layout should have single-panel CPU row, then 2-panel rows,
	// plus the optional 3-panel services row.
	if len(rows) < 3 {
		t.Error("medium layout should have at least 3 rows")
	}
	if len(rows[0]) != 1 || rows[0][0].name != "cpu" {
		t.Error("medium layout first row should be CPU alone")
	}
}

func TestSelectLayout_Narrow(t *testing.T) {
	rows := selectLayout(80)
	for _, row := range rows {
		if len(row) == 1 {
			continue
		}
		for _, p := range row {
			if p.weight == 3 {
				t.Errorf("narrow layout: wide panel %q should not share a row", p.name)
			}
		}
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

func TestRenderRow_SinglePanel(t *testing.T) {
	buildStyles(DarkTheme)
	contents := map[string]string{
		"cpu": "  42.7%\n",
	}
	row := []panelSpec{{name: "cpu", title: "CPU", weight: 3}}
	result := renderRow(row, contents, nil, 100)
	if !strings.Contains(result, "CPU") {
		t.Error("single-panel row should contain CPU")
	}
}

func TestRenderHeader_ContainsVigil(t *testing.T) {
	buildStyles(DarkTheme)
	header := renderHeader(120, collector.Snapshot{}, 0)
	if !strings.Contains(header, "VIGIL") {
		t.Error("header should contain VIGIL")
	}
}

func TestRenderHeader_ShowsLoadAverage(t *testing.T) {
	buildStyles(DarkTheme)
	snap := collector.Snapshot{
		Load: collector.LoadSnapshot{Load1: 1.23, Load5: 0.45, Load15: 0.67},
	}
	header := renderHeader(120, snap, 0)
	if !strings.Contains(header, "1.23") {
		t.Error("header should show load average")
	}
}

func TestRenderHeader_ShowsAlertCount(t *testing.T) {
	buildStyles(DarkTheme)
	header := renderHeader(120, collector.Snapshot{}, 3)
	if !strings.Contains(header, "3 alerts") {
		t.Error("header should show alert count")
	}
}

func TestRenderHeader_NarrowDropsAlertCount(t *testing.T) {
	buildStyles(DarkTheme)
	snap := collector.Snapshot{
		Throttle: collector.ThrottleSnapshot{Available: true},
	}
	header := renderHeader(35, snap, 2)
	if strings.Contains(header, "alerts") {
		t.Error("narrow header should drop alert count")
	}
	if !strings.Contains(header, "VIGIL") {
		t.Error("narrow header should still contain VIGIL")
	}
}

func TestRenderHeader_ShowsUptime(t *testing.T) {
	buildStyles(DarkTheme)
	snap := collector.Snapshot{UptimeSec: 3*86400 + 12*3600}
	header := renderHeader(160, snap, 0)
	if !strings.Contains(header, "3d 12h") {
		t.Error("header should show uptime")
	}
}

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		sec  uint64
		want string
	}{
		{0, "0m"},
		{300, "5m"},
		{3661, "1h 1m"},
		{90061, "1d 1h"},
	}
	for _, tt := range tests {
		got := formatUptime(tt.sec)
		if got != tt.want {
			t.Errorf("formatUptime(%d) = %q, want %q", tt.sec, got, tt.want)
		}
	}
}
