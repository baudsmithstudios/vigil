package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type panelSpec struct {
	name   string
	title  string
	weight int
	stack  []panelSpec // when set, these render stacked in one column
}

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

// distributeWidths splits the width by weight, after reserving 4 chars per panel
// for border and padding.
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
	widths[0] += available - assigned

	return widths
}

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

	type cell struct {
		rendered string
		height   int
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
			// Content lines plus top and bottom border.
			contentParts := strings.Count(c, "\n") + 1
			h := contentParts + 2
			cells[i] = cell{height: h}
		}
		if cells[i].height > maxHeight {
			maxHeight = cells[i].height
		}
	}

	panels := make([]string, len(active))
	for i, p := range active {
		if cells[i].isStack {
			panels[i] = cells[i].rendered
		} else {
			c := contents[p.name]
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

// renderPanel embeds the title in the top border: ╭─ Title ──────────╮
// width excludes the two side borders.
func renderPanel(title, content string, width int) string {
	border := lipgloss.RoundedBorder()
	borderFg := panelStyle.GetBorderTopForeground()

	styledTitle := " " + sectionTitleStyle.Render(title) + " "
	titleW := lipgloss.Width(styledTitle)

	topFill := width - titleW - 1 // -1 for the ╭ prefix dash
	if topFill < 0 {
		topFill = 0
	}
	top := lipgloss.NewStyle().Foreground(borderFg).Render(
		border.TopLeft+border.Top) +
		styledTitle +
		lipgloss.NewStyle().Foreground(borderFg).Render(
			strings.Repeat(border.Top, topFill)+border.TopRight)

	side := lipgloss.NewStyle().Foreground(borderFg).Render(border.Left)
	sideR := lipgloss.NewStyle().Foreground(borderFg).Render(border.Right)
	pad := " "
	innerW := width - 2

	var body strings.Builder
	for _, line := range strings.Split(content, "\n") {
		lineW := lipgloss.Width(line)
		if lineW > innerW {
			line = lipgloss.NewStyle().MaxWidth(innerW).Render(line)
			lineW = lipgloss.Width(line)
		}
		fill := ""
		if innerW > lineW {
			fill = strings.Repeat(" ", innerW-lineW)
		}
		body.WriteString(side + pad + line + fill + pad + sideR + "\n")
	}

	bottom := lipgloss.NewStyle().Foreground(borderFg).Render(
		border.BottomLeft + strings.Repeat(border.Bottom, width) + border.BottomRight)

	return top + "\n" + body.String() + bottom
}
