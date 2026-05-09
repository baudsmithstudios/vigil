package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"vigil/internal/alert"
	"vigil/internal/checker"
	"vigil/internal/collector"
)

// renderDashboard lays out all sections in a panel grid using the layout engine.
// Panel render functions produce content strings; selectLayout chooses the row
// grouping based on terminal width; renderRow handles width distribution and
// empty-panel filtering.
func renderDashboard(
	snap collector.Snapshot,
	activeAlerts []alert.State,
	alertCursor int,
	muted bool,
	width int,
) string {
	rows := selectLayout(width)

	// Determine which optional panels have content so we can compute
	// accurate widths after filtering (renderRow drops empty panels).
	hasContainers := len(snap.Containers) > 0
	hasServices := len(snap.Services) > 0
	hasMounts := len(snap.Mounts) > 0
	hasContent := map[string]bool{
		"cpu": true, "mem": true, "disk": true, "net": true, "alerts": true,
		"ctr": hasContainers, "svc": hasServices, "mnt": hasMounts,
	}

	// Compute per-panel inner widths using only panels that have content,
	// matching the filtering that renderRow performs.
	innerWidths := make(map[string]int)
	for _, row := range rows {
		var active []panelSpec
		for _, p := range row {
			if len(p.stack) > 0 {
				for _, sp := range p.stack {
					if hasContent[sp.name] {
						active = append(active, p)
						break
					}
				}
			} else if hasContent[p.name] {
				active = append(active, p)
			}
		}
		weights := make([]int, len(active))
		for i, p := range active {
			weights[i] = p.weight
		}
		widths := distributeWidths(width, weights)
		for i, p := range active {
			if len(p.stack) > 0 {
				for _, sp := range p.stack {
					innerWidths[sp.name] = widths[i] - 2
				}
			} else {
				innerWidths[p.name] = widths[i] - 2
			}
		}
	}

	contents := map[string]string{
		"cpu":    renderCPUContent(snap.CPU, innerWidths["cpu"]),
		"mem":    renderMemContent(snap.Memory, innerWidths["mem"]),
		"disk":   renderDiskContent(snap.Disks, snap.DiskIO, innerWidths["disk"]),
		"net":    renderNetContent(snap.Network, innerWidths["net"]),
		"alerts": renderAlertContent(activeAlerts, alertCursor, muted),
	}
	if hasContainers {
		contents["ctr"] = renderContainerContent(snap.Containers)
	}
	if hasServices {
		contents["svc"] = renderServiceContent(snap.Services)
	}
	if hasMounts {
		contents["mnt"] = renderMountContent(snap.Mounts, innerWidths["mnt"])
	}

	// Override alerts title when muted.
	titles := map[string]string{}
	if muted {
		titles["alerts"] = "Alerts [MUTED]"
	}

	var parts []string
	for _, row := range rows {
		rendered := renderRow(row, contents, titles, width)
		if rendered != "" {
			parts = append(parts, rendered)
		}
	}
	body := lipgloss.JoinVertical(lipgloss.Left, parts...)

	return renderHeader(width, snap, len(activeAlerts)) + "\n" + body + "\n"
}

func renderHeader(width int, snap collector.Snapshot, alertCount int) string {
	title := " " + appTitleStyle.Render("▪ VIGIL")
	help := hintKeyStyle.Render("?") + dimStyle.Render(" help") + " "
	pipe := dividerStyle.Render(" │ ")

	// Center section: load, temperature, uptime separated by pipes.
	var centerParts []string
	load := snap.Load
	centerParts = append(centerParts, dimStyle.Render("load ")+
		valueStyle.Render(fmt.Sprintf("%.2f", load.Load1))+
		dimStyle.Render(" ")+
		valueStyle.Render(fmt.Sprintf("%.2f", load.Load5))+
		dimStyle.Render(" ")+
		valueStyle.Render(fmt.Sprintf("%.2f", load.Load15)))
	if len(snap.Temperature) > 0 {
		t := snap.Temperature[0]
		centerParts = append(centerParts, renderTemp(t.Celsius)+unitStyle.Render("°C"))
	}
	if snap.UptimeSec > 0 {
		centerParts = append(centerParts, dimStyle.Render("up ")+valueStyle.Render(formatUptime(snap.UptimeSec)))
	}

	// Right section: throttle warning, alert count, then help anchored at far right.
	var rightParts []string
	if snap.Throttle.Available {
		switch snap.Throttle.Status() {
		case "OK":
			rightParts = append(rightParts, pwrOkStyle.Render("● PWR: OK"))
		case "WARN":
			rightParts = append(rightParts, pwrWarnStyle.Render("● PWR: WARN"))
		case "THROTTLED":
			rightParts = append(rightParts, pwrThrottledStyle.Render("● PWR: THROTTLED"))
		}
	}
	if alertCount > 0 {
		rightParts = append(rightParts, alertStyle.Render(fmt.Sprintf("⚠ %d alerts", alertCount)))
	}

	center := strings.Join(centerParts, pipe)
	rightInfo := strings.Join(rightParts, pipe)

	// Layout: title ... center ... rightInfo   ? help
	// Try full layout first.
	rightFull := rightInfo
	if rightFull != "" {
		rightFull += "   "
	}
	rightFull += help

	gap := width - lipgloss.Width(title) - lipgloss.Width(center) - lipgloss.Width(rightFull)
	if gap >= 2 {
		leftGap := gap / 2
		rightGap := gap - leftGap
		return headerStyle.Render(title + strings.Repeat(" ", leftGap) + center + strings.Repeat(" ", rightGap) + rightFull)
	}

	// Narrow: drop alerts and uptime from center, keep load + temp.
	shortCenter := centerParts[0] // just load
	if len(snap.Temperature) > 0 {
		shortCenter = strings.Join(centerParts[:2], pipe)
	}
	gap = width - lipgloss.Width(title) - lipgloss.Width(shortCenter) - lipgloss.Width(help)
	if gap >= 2 {
		leftGap := gap / 2
		rightGap := gap - leftGap
		return headerStyle.Render(title + strings.Repeat(" ", leftGap) + shortCenter + strings.Repeat(" ", rightGap) + help)
	}

	// Very narrow: just title and help.
	gap = width - lipgloss.Width(title) - lipgloss.Width(help)
	if gap < 1 {
		gap = 1
	}
	return headerStyle.Render(title + strings.Repeat(" ", gap) + help)
}

// formatUptime converts seconds to a compact human-readable duration.
func formatUptime(sec uint64) string {
	days := sec / 86400
	hours := (sec % 86400) / 3600
	minutes := (sec % 3600) / 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// gaugeWidth calculates the gauge bar width from the available inner width.
func gaugeWidth(innerW int) int {
	w := innerW - 22
	if w < 8 {
		w = 8
	}
	return w
}

func renderCPUContent(cpu collector.CPUSnapshot, innerW int) string {
	gaugeW := gaugeWidth(innerW)

	var sb strings.Builder

	if !cpu.Ready {
		sb.WriteString(labelStyle.Render(fmt.Sprintf("   %-6s", "total")))
		sb.WriteString(dimStyle.Render("  warming up…\n"))
	} else {
		sb.WriteString(labelStyle.Render(fmt.Sprintf("   %-6s", "total")))
		sb.WriteString(" ")
		sb.WriteString(renderGauge(cpu.PercentTotal, gaugeW))
		sb.WriteString(valueStyle.Render(fmt.Sprintf(" %5.1f", cpu.PercentTotal)))
		sb.WriteString(unitStyle.Render("%\n"))

		// Hide per-core rows on narrow panels — gaugeW/2 becomes too small to be useful.
		if innerW >= 40 {
			for i, p := range cpu.PercentPerCore {
				sb.WriteString(labelStyle.Render(fmt.Sprintf("  %-6s", fmt.Sprintf("core%d", i))))
				sb.WriteString(renderGauge(p, gaugeW))
				sb.WriteString(valueStyle.Render(fmt.Sprintf(" %5.1f", p)))
				sb.WriteString(unitStyle.Render("%\n"))
			}
		}

		// Adapt CPU breakdown line length to available width.
		// Full:    "  usr X%  sys X%  iowait X%  idle X%" (~52 chars)
		// Compact: "  u X%  s X%  iow X%  id X%"          (~43 chars)
		switch {
		case innerW >= 52:
			sb.WriteString(dimStyle.Render(fmt.Sprintf(
				"  usr %4.1f%%  sys %4.1f%%  iowait %4.1f%%  idle %4.1f%%\n",
				cpu.UserPercent, cpu.SystemPercent, cpu.IOWaitPercent, cpu.IdlePercent,
			)))
		case innerW >= 44:
			sb.WriteString(dimStyle.Render(fmt.Sprintf(
				"  u %4.1f%%  s %4.1f%%  iow %4.1f%%  id %4.1f%%\n",
				cpu.UserPercent, cpu.SystemPercent, cpu.IOWaitPercent, cpu.IdlePercent,
			)))
		}
	}
	return sb.String()
}

func renderMemContent(mem collector.MemSnapshot, innerW int) string {
	gaugeW := gaugeWidth(innerW)

	var sb strings.Builder

	sb.WriteString(labelStyle.Render("  used"))
	sb.WriteString(renderGauge(mem.Percent, gaugeW))
	sb.WriteString(valueStyle.Render(fmt.Sprintf(" %5.1f", mem.Percent)))
	sb.WriteString(unitStyle.Render("%\n"))
	sb.WriteString(dimStyle.Render(fmt.Sprintf("  %s / %s",
		formatBytes(mem.UsedBytes), formatBytes(mem.TotalBytes))))
	if mem.CachedBytes > 0 || mem.BuffersBytes > 0 {
		sb.WriteString(dimStyle.Render(fmt.Sprintf("  cache %s  buf %s",
			formatBytes(mem.CachedBytes), formatBytes(mem.BuffersBytes))))
	}
	sb.WriteString("\n")

	if mem.SwapTotalBytes > 0 {
		sb.WriteString(labelStyle.Render("  swap"))
		sb.WriteString(renderGauge(mem.SwapPercent, gaugeW))
		sb.WriteString(valueStyle.Render(fmt.Sprintf(" %5.1f", mem.SwapPercent)))
		sb.WriteString(unitStyle.Render("%\n"))
		sb.WriteString(dimStyle.Render(fmt.Sprintf("  %s / %s  in %s  out %s\n",
			formatBytes(mem.SwapUsedBytes), formatBytes(mem.SwapTotalBytes),
			formatRate(mem.SwapInRate), formatRate(mem.SwapOutRate))))
	}

	return sb.String()
}

func renderDiskContent(disks []collector.DiskSnapshot, io []collector.DiskIOSnapshot, innerW int) string {
	gaugeW := gaugeWidth(innerW)

	ioByDevice := make(map[string]collector.DiskIOSnapshot, len(io))
	for _, d := range io {
		ioByDevice[d.Device] = d
	}

	var sb strings.Builder

	if len(disks) == 0 {
		sb.WriteString(dimStyle.Render("  no disks detected\n"))
		return sb.String()
	}
	for _, d := range disks {
		sb.WriteString(labelStyle.Render(fmt.Sprintf("  %-12s", truncate(d.MountPoint, 12))))
		sb.WriteString(renderGauge(d.Percent, gaugeW))
		sb.WriteString(valueStyle.Render(fmt.Sprintf(" %5.1f", d.Percent)))
		sb.WriteString(unitStyle.Render("%\n"))
		ioSnap, ok := ioByDevice[d.Device]
		if !ok {
			if parent := collector.ParentBlockDevice(d.Device); parent != "" {
				ioSnap = ioByDevice[parent]
			}
		}
		sb.WriteString(dimStyle.Render(fmt.Sprintf("  %s / %s  r %s  w %s",
			formatBytes(d.UsedBytes), formatBytes(d.TotalBytes),
			formatRate(ioSnap.ReadRate), formatRate(ioSnap.WriteRate))))
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(fmt.Sprintf("   util %4.0f%%", ioSnap.UtilPercent)))
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(fmt.Sprintf("   await %5.1fms", ioSnap.LatencyMs)))
		sb.WriteString("\n")
	}
	return sb.String()
}

func renderNetContent(nets []collector.NetSnapshot, width int) string {
	var sb strings.Builder

	if len(nets) == 0 {
		sb.WriteString(dimStyle.Render("  no interfaces detected\n"))
		return sb.String()
	}
	for _, n := range nets {
		left := labelStyle.Render(fmt.Sprintf("  %-12s", truncate(n.Interface, 12)))
		left += dimStyle.Render("↑ ")
		left += valueStyle.Render(formatRate(n.SendRate))
		left += dimStyle.Render("  ↓ ")
		left += valueStyle.Render(formatRate(n.RecvRate))

		errDrop := fmt.Sprintf("err %.0f/s  drop %.0f/s", n.ErrRate, n.DropRate)
		var right string
		if n.ErrRate > 0 || n.DropRate > 0 {
			right = alertStyle.Render(errDrop)
		} else {
			right = dimStyle.Render(errDrop)
		}

		leftW := lipgloss.Width(left)
		rightW := lipgloss.Width(right)
		gap := width - leftW - rightW
		if gap < 1 {
			gap = 1
		}
		sb.WriteString(left + strings.Repeat(" ", gap) + right + "\n")
	}
	return sb.String()
}

func renderContainerContent(containers []collector.ContainerSnapshot) string {
	var sb strings.Builder

	if len(containers) == 0 {
		sb.WriteString(dimStyle.Render("  no containers\n"))
		return sb.String()
	}
	for _, c := range containers {
		sb.WriteString(labelStyle.Render(fmt.Sprintf("  %-12s", truncate(c.Name, 12))))

		// Color-code the status.
		switch c.Status {
		case "running":
			sb.WriteString(containerRunningStyle.Render("● run "))
		case "exited":
			sb.WriteString(containerStoppedStyle.Render("○ exit"))
		case "restarting":
			sb.WriteString(containerOtherStyle.Render("↻ rstr"))
		case "paused":
			sb.WriteString(containerOtherStyle.Render("‖ paus"))
		default:
			sb.WriteString(dimStyle.Render(fmt.Sprintf("%-6s", truncate(c.Status, 6))))
		}

		if c.Status == "running" {
			sb.WriteString(valueStyle.Render(fmt.Sprintf("  %5.1f", c.CPUPercent)))
			sb.WriteString(unitStyle.Render("% cpu"))
			sb.WriteString(unitStyle.Render("  mem "))
			sb.WriteString(valueStyle.Render(formatBytes(c.MemUsed)))
			if c.MemLimit > 0 {
				sb.WriteString(dimStyle.Render(fmt.Sprintf(" / %s", formatBytes(c.MemLimit))))
				sb.WriteString(dimStyle.Render(fmt.Sprintf("  %.1f%%", c.MemPercent)))
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func renderMountContent(mounts []collector.MountStatus, width int) string {
	var sb strings.Builder

	// Adapt path column width to available panel width.
	pathW := 20
	if width < 50 {
		pathW = width - 26 // leave room for label(14) + status(~12)
		if pathW < 6 {
			pathW = 6
		}
	}

	for _, m := range mounts {
		label := m.Label
		if label == "" {
			label = m.Path
		}
		left := labelStyle.Render(fmt.Sprintf("  %-12s", truncate(label, 12)))
		left += dimStyle.Render(fmt.Sprintf("%-*s", pathW, truncate(m.Path, pathW)))

		var right string
		switch {
		case !m.Mounted:
			right = mountMissingStyle.Render("✖ MISSING")
		case m.Unstable:
			right = mountUnstableStyle.Render("⚠ UNSTABLE")
		default:
			right = mountOkStyle.Render("● OK")
		}

		leftW := lipgloss.Width(left)
		rightW := lipgloss.Width(right)
		gap := width - leftW - rightW
		if gap < 1 {
			gap = 1
		}
		sb.WriteString(left + strings.Repeat(" ", gap) + right + "\n")
	}
	return sb.String()
}

func renderServiceContent(services []checker.ServiceStatus) string {
	var sb strings.Builder

	for _, s := range services {
		sb.WriteString(labelStyle.Render(fmt.Sprintf("  %-12s", truncate(s.Name, 12))))
		if s.Up {
			sb.WriteString(serviceUpStyle.Render("✓ UP  "))
			sb.WriteString(valueStyle.Render(fmt.Sprintf("%4dms", s.Latency.Milliseconds())))
		} else {
			sb.WriteString(serviceDownStyle.Render("✗ DOWN"))
			sb.WriteString(dimStyle.Render("     —"))
		}
		sb.WriteString(dimStyle.Render("  " + formatAge(s.CheckedAt)))
		sb.WriteString("\n")
	}
	return sb.String()
}

func formatAge(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

func renderAlertContent(alerts []alert.State, cursor int, muted bool) string {
	var sb strings.Builder
	if len(alerts) == 0 {
		sb.WriteString(alertResolvedStyle.Render("  ✓ all clear\n"))
	} else {
		for i, a := range alerts {
			if i == cursor {
				sb.WriteString(alertCursorStyle.Render("> "))
			} else {
				sb.WriteString("  ")
			}
			if a.Resolved {
				sb.WriteString(alertResolvedStyle.Render(fmt.Sprintf("✓ %s  %s  (resolved %s)",
					a.Name, a.Message, a.ResolvedAt.Format("15:04:05"))))
			} else {
				sb.WriteString(alertStyle.Render(fmt.Sprintf("⚠ %s  %s  (since %s)",
					a.Name, a.Message, a.FiredAt.Format("15:04:05"))))
			}
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// renderTemp returns the temperature value styled by heat level.
// Thresholds tuned for Raspberry Pi: normal <50°C, warm 50-70°C, hot ≥70°C.
func renderTemp(c float64) string {
	s := fmt.Sprintf("%.1f", c)
	switch {
	case c >= 70:
		return tempHighStyle.Render(s)
	case c >= 50:
		return tempMediumStyle.Render(s)
	default:
		return tempLowStyle.Render(s)
	}
}

// renderGauge draws a color-coded progress bar.
//
// The filled portion is colored green/yellow/red based on severity thresholds.
func renderGauge(percent float64, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	filled := int(percent / 100.0 * float64(width))

	fillStyle := gaugeLowStyle
	switch {
	case percent >= 90:
		fillStyle = gaugeHighStyle
	case percent >= 70:
		fillStyle = gaugeMediumStyle
	}

	var result strings.Builder
	if filled > 0 {
		result.WriteString(fillStyle.Render(strings.Repeat("|", filled)))
	}
	if remaining := width - filled; remaining > 0 {
		result.WriteString(dimStyle.Render(strings.Repeat(".", remaining)))
	}

	return result.String()
}

func formatBytes(b uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1fG", float64(b)/GB)
	case b >= MB:
		return fmt.Sprintf("%.1fM", float64(b)/MB)
	case b >= KB:
		return fmt.Sprintf("%.1fK", float64(b)/KB)
	default:
		return fmt.Sprintf("%dB", b)
	}
}

func formatRate(bps float64) string {
	return formatBytes(uint64(bps)) + "/s"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
