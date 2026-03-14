package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// panelSpec describes a dashboard panel's layout properties.
// When stack is set, the cell renders multiple panels stacked vertically
// instead of a single panel.
type panelSpec struct {
	name   string
	title  string
	weight int         // proportional width: 3=wide, 2=medium, 1=compact
	stack  []panelSpec // when set, renders stacked panels in this column
}

// hasContent reports whether this cell has renderable content.
// For stacked cells, returns true if any sub-panel has content.
func (p panelSpec) hasContent(contents map[string]string) bool {
	if len(p.stack) > 0 {
		for _, sp := range p.stack {
			if contents[sp.name] != "" {
				return true
			}
		}
		return false
	}
	return contents[p.name] != ""
}

// selectLayout returns the row configuration for the given terminal width.
// Panel order is fixed; only the grouping changes.
func selectLayout(width int) [][]panelSpec {
	cpu := panelSpec{name: "cpu", title: "CPU", weight: 3}
	mem := panelSpec{name: "mem", title: "Memory", weight: 3}
	disk := panelSpec{name: "disk", title: "Disk", weight: 3}
	net := panelSpec{name: "net", title: "Network", weight: 3}
	svc := panelSpec{name: "svc", title: "Services", weight: 1}
	ctr := panelSpec{name: "ctr", title: "Containers", weight: 1}
	mnt := panelSpec{name: "mnt", title: "Mounts", weight: 1}
	alerts := panelSpec{name: "alerts", title: "Alerts", weight: 3}

	statusStack := panelSpec{weight: 3, stack: []panelSpec{svc, ctr, mnt}}

	switch {
	case width >= 160:
		return [][]panelSpec{
			{cpu, mem},
			{disk, net},
			{alerts, statusStack},
		}
	case width >= 100:
		return [][]panelSpec{
			{cpu},
			{mem, net},
			{disk, alerts},
			{svc, ctr, mnt},
		}
	default:
		return [][]panelSpec{
			{cpu},
			{mem},
			{disk},
			{net},
			{alerts},
			{svc, ctr, mnt},
		}
	}
}

// distributeWidths allocates terminal width among panels in a row
// proportional to their weights, after subtracting border+padding overhead
// (4 chars per panel: 2 border + 2 padding).
func distributeWidths(termWidth int, weights []int) []int {
	n := len(weights)
	available := termWidth - (n * 4)
	if available < n {
		available = n
	}

	totalWeight := 0
	for _, w := range weights {
		totalWeight += w
	}

	widths := make([]int, n)
	assigned := 0
	for i, w := range weights {
		widths[i] = available * w / totalWeight
		assigned += widths[i]
	}
	// Distribute remainder to first panel.
	widths[0] += available - assigned

	return widths
}

// renderRow renders a single row of panels, distributing width by weight.
// Panels whose content is empty are filtered out before width distribution.
// Stacked cells render multiple panels vertically within their column.
func renderRow(row []panelSpec, contents map[string]string, titles map[string]string, termWidth int) string {
	var active []panelSpec
	for _, p := range row {
		if p.hasContent(contents) {
			active = append(active, p)
		}
	}
	if len(active) == 0 {
		return ""
	}

	weights := make([]int, len(active))
	for i, p := range active {
		weights[i] = p.weight
	}
	widths := distributeWidths(termWidth, weights)

	// Render each cell and track its visual height.
	type cell struct {
		rendered string
		height   int // visual line count
		isStack  bool
	}
	cells := make([]cell, len(active))
	maxHeight := 0

	for i, p := range active {
		if len(p.stack) > 0 {
			var parts []string
			for _, sp := range p.stack {
				if c := contents[sp.name]; c != "" {
					t := sp.title
					if override, ok := titles[sp.name]; ok {
						t = override
					}
					parts = append(parts, renderPanel(t, c, widths[i]))
				}
			}
			r := strings.Join(parts, "\n")
			h := strings.Count(r, "\n") + 1
			cells[i] = cell{rendered: r, height: h, isStack: true}
		} else {
			c := contents[p.name]
			// Panel height = content split parts + 2 (top + bottom border).
			contentParts := strings.Count(c, "\n") + 1
			h := contentParts + 2
			cells[i] = cell{height: h}
		}
		if cells[i].height > maxHeight {
			maxHeight = cells[i].height
		}
	}

	// Render single panels with content padded to match the tallest cell.
	panels := make([]string, len(active))
	for i, p := range active {
		if cells[i].isStack {
			panels[i] = cells[i].rendered
		} else {
			c := contents[p.name]
			// Pad content so the rendered panel's total height matches maxHeight.
			targetParts := maxHeight - 2
			currentParts := strings.Count(c, "\n") + 1
			if targetParts > currentParts {
				c += strings.Repeat("\n", targetParts-currentParts)
			}
			t := p.title
			if override, ok := titles[p.name]; ok {
				t = override
			}
			panels[i] = renderPanel(t, c, widths[i])
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, panels...)
}

// renderPanel renders content inside a bordered box with the title embedded
// in the top border line: ╭─ Title ──────────╮
func renderPanel(title, content string, width int) string {
	border := lipgloss.RoundedBorder()
	borderFg := panelStyle.GetBorderTopForeground()

	styledTitle := " " + sectionTitleStyle.Render(title) + " "
	titleW := lipgloss.Width(styledTitle)

	// Top border: ╭─ Title ─...─╮
	// Inner width = width (content area), total = width + 2 (border chars)
	topFill := width - titleW - 1 // -1 for the ╭ prefix dash
	if topFill < 0 {
		topFill = 0
	}
	top := lipgloss.NewStyle().Foreground(borderFg).Render(
		border.TopLeft+border.Top) +
		styledTitle +
		lipgloss.NewStyle().Foreground(borderFg).Render(
			strings.Repeat(border.Top, topFill)+border.TopRight)

	// Content with side borders and padding.
	side := lipgloss.NewStyle().Foreground(borderFg).Render(border.Left)
	sideR := lipgloss.NewStyle().Foreground(borderFg).Render(border.Right)
	pad := " " // 1 char padding on each side
	innerW := width - 2 // subtract padding

	var body strings.Builder
	for _, line := range strings.Split(content, "\n") {
		// Truncate or pad each line to fill the inner width.
		lineW := lipgloss.Width(line)
		if lineW > innerW {
			line = line[:innerW]
			lineW = innerW
		}
		fill := ""
		if innerW > lineW {
			fill = strings.Repeat(" ", innerW-lineW)
		}
		body.WriteString(side + pad + line + fill + pad + sideR + "\n")
	}

	// Bottom border: ╰─...─╯
	bottom := lipgloss.NewStyle().Foreground(borderFg).Render(
		border.BottomLeft + strings.Repeat(border.Bottom, width) + border.BottomRight)

	return top + "\n" + body.String() + bottom
}
