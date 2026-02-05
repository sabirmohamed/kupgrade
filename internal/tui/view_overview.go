package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sabirmohamed/kupgrade/pkg/types"
)

// renderOverview renders the main overview screen
func (m Model) renderOverview() string {
	var b strings.Builder

	// Header → Pipeline → Blockers → Drain → Node List → Events → Footer
	b.WriteString(m.renderCompactHeader())
	b.WriteString("\n\n")
	b.WriteString(m.renderPipelineRow())
	b.WriteString("\n")

	// Blockers section (only if blockers exist)
	if len(m.blockers) > 0 {
		b.WriteString(m.renderBlockersSection())
		b.WriteString("\n")
	}

	// Drain progress section (only if node is draining)
	if drainSection := m.renderDrainProgressSection(); drainSection != "" {
		b.WriteString(drainSection)
		b.WriteString("\n")
	}

	// Node list with stage column
	b.WriteString(m.renderNodeList())
	b.WriteString("\n")

	// Events at bottom
	b.WriteString(m.renderEventsSection())
	b.WriteString("\n")

	b.WriteString(m.renderFooter())

	content := b.String()

	// Fill available main area dimensions
	w := m.mainWidth()
	if w > 0 && m.height > 0 {
		return lipgloss.Place(w, m.height, lipgloss.Left, lipgloss.Top, content)
	}
	return content
}

// renderBlockersSection shows blockers with left border accent.
// Active blockers (red) are shown first, then risks (yellow).
func (m Model) renderBlockersSection() string {
	if len(m.blockers) == 0 {
		return ""
	}

	activeBlockers, riskBlockers := m.splitBlockersByTier()

	var lines []string

	// Active blockers: red, with node name and duration
	if len(activeBlockers) > 0 {
		title := errorStyle.Render(fmt.Sprintf("%s ACTIVE BLOCKERS (%d)", errorIcon, len(activeBlockers)))
		lines = append(lines, title)

		for _, blocker := range activeBlockers {
			lines = append(lines, m.formatOverviewBlockerLine(blocker, true))
		}
	}

	// Risk blockers: yellow, informational
	if len(riskBlockers) > 0 {
		title := warningStyle.Render(fmt.Sprintf("%s PDB RISKS (%d)", warningIcon, len(riskBlockers)))
		lines = append(lines, title)

		for _, blocker := range riskBlockers {
			lines = append(lines, m.formatOverviewBlockerLine(blocker, false))
		}
	}

	content := strings.Join(lines, "\n")

	// Use red border for active blockers, yellow for risks only
	if len(activeBlockers) > 0 {
		return activeBlockerPanelStyle.Render(content)
	}
	return blockerPanelStyle.Render(content)
}

// formatOverviewBlockerLine formats a single blocker line for the overview section.
func (m Model) formatOverviewBlockerLine(blocker types.Blocker, isActive bool) string {
	name := blocker.Name
	if blocker.Namespace != "" {
		name = blocker.Namespace + "/" + blocker.Name
	}

	var nameStyle func(strs ...string) string
	if isActive {
		nameStyle = errorStyle.Render
	} else {
		nameStyle = warningStyle.Render
	}

	nameStr := fmt.Sprintf("%s %s", blocker.Type, nameStyle(name))

	nodeStr := ""
	if blocker.NodeName != "" {
		nodeStr = fmt.Sprintf(" blocking %s", blocker.NodeName)
	}

	durationStr := ""
	if !blocker.StartTime.IsZero() {
		duration := m.currentTime.Sub(blocker.StartTime)
		durationStr = fmt.Sprintf(" (%s)", formatDuration(duration))
	}

	constraint := blocker.Detail
	if constraint == "" {
		constraint = "disruption budget exhausted"
	}

	return fmt.Sprintf("%s    %s%s%s", nameStr, footerDescStyle.Render(constraint), nodeStr, errorStyle.Render(durationStr))
}

// renderDrainProgressSection shows drain progress for actively draining nodes
func (m Model) renderDrainProgressSection() string {
	drainingNodes := m.nodesByStage[types.StageDraining]
	if len(drainingNodes) == 0 {
		return ""
	}

	var b strings.Builder

	for _, nodeName := range drainingNodes {
		node := m.nodes[nodeName]

		// Title with spinner
		title := fmt.Sprintf("%s DRAINING: %s", m.spinner.View(), strings.ToUpper(nodeName))
		b.WriteString(warningStyle.Render(title))
		b.WriteString("\n")

		// Drain progress uses evictable pods (excludes DaemonSets)
		evicted := node.InitialPodCount - node.EvictablePodCount
		if evicted < 0 {
			evicted = 0
		}
		total := node.InitialPodCount
		if total == 0 {
			total = node.EvictablePodCount
		}

		// Progress bar
		bar := m.smallProg.ViewAs(float64(node.DrainProgress) / 100.0)

		// Elapsed time
		elapsed := ""
		if !node.DrainStartTime.IsZero() {
			dur := m.currentTime.Sub(node.DrainStartTime)
			mins := int(dur.Minutes())
			secs := int(dur.Seconds()) % 60
			if mins > 0 {
				elapsed = fmt.Sprintf("    Elapsed: %dm %ds", mins, secs)
			} else {
				elapsed = fmt.Sprintf("    Elapsed: %ds", secs)
			}
		}

		progressLine := fmt.Sprintf("%s  %d/%d pods evicted%s", bar, evicted, total, footerDescStyle.Render(elapsed))
		b.WriteString(progressLine)
		b.WriteString("\n")

		// Show waiting pods if any
		if len(node.WaitingPods) > 0 {
			waiting := "Waiting on: " + strings.Join(node.WaitingPods, "  ")
			if len(waiting) > m.mainWidth()-4 {
				waiting = waiting[:m.mainWidth()-7] + "..."
			}
			b.WriteString(footerDescStyle.Render(waiting))
			b.WriteString("\n")
		}

		// Show blocker if blocked
		if node.Blocked && node.BlockerReason != "" {
			blockerLine := fmt.Sprintf("Blocked: %s", node.BlockerReason)
			b.WriteString(errorStyle.Render(blockerLine))
			b.WriteString("\n")
		}
	}

	return b.String()
}

// calcNodeListVisibleRows calculates visible rows for node list based on screen space
func (m Model) calcNodeListVisibleRows() int {
	blockerLines := m.calcBlockerLines()
	eventsLines := min(len(m.events), 8)
	reservedLines := 1 + 3 + blockerLines + eventsLines + 4 + 3
	visibleRows := m.height - reservedLines
	return clamp(visibleRows, 3, 15)
}

// calcBlockerLines calculates lines needed for blocker display
func (m Model) calcBlockerLines() int {
	if len(m.blockers) == 0 {
		return 0
	}
	activeBlockers, riskBlockers := m.splitBlockersByTier()
	lines := len(m.blockers) * 2 // Each blocker takes 2 lines
	if len(activeBlockers) > 0 {
		lines++ // "ACTIVE BLOCKERS" header
	}
	if len(riskBlockers) > 0 {
		lines++ // "PDB RISKS" header
	}
	return lines + 2 // panel border padding
}

// calcNameWidth returns name column width based on terminal width
func (m Model) calcNameWidth() int {
	switch {
	case m.mainWidth() > 120:
		return 40
	case m.mainWidth() > 100:
		return 35
	default:
		return 30
	}
}

// renderNodeList shows unified node list with selected row highlight
func (m Model) renderNodeList() string {
	var b strings.Builder

	visibleRows := m.calcNodeListVisibleRows()
	allNodes := m.getSortedNodeList()
	scrollOffset := calcScrollOffset(m.listIndex, visibleRows, len(allNodes))

	// Header with hints
	titleStr := fmt.Sprintf("NODES (%d)", len(allNodes))
	hints := footerDescStyle.Render("↑↓ navigate • d describe")
	spacing := max(m.mainWidth()-len(titleStr)-20-4, 4)

	b.WriteString(panelTitleStyle.Render(titleStr))
	b.WriteString(strings.Repeat(" ", spacing))
	b.WriteString(hints)
	b.WriteString("\n")

	if len(allNodes) == 0 {
		b.WriteString(footerDescStyle.Render("  No nodes discovered"))
		return b.String()
	}

	nameWidth := m.calcNameWidth()
	rowWidth := max(m.mainWidth()-6, 60)
	endIdx := min(scrollOffset+visibleRows, len(allNodes))

	for i := scrollOffset; i < endIdx; i++ {
		b.WriteString(m.renderNodeListRow(allNodes[i], i, nameWidth, rowWidth))
	}

	return b.String()
}

// renderNodeListRow renders a single row in the node list
func (m Model) renderNodeListRow(nodeName string, idx, nameWidth, rowWidth int) string {
	node := m.nodes[nodeName]

	displayName := nodeName
	if len(displayName) > nameWidth {
		displayName = displayName[len(displayName)-nameWidth:]
	}

	version := node.Version
	if version == "" {
		version = "unknown"
	}

	surgeLabel := ""
	if node.SurgeNode {
		surgeLabel = " SURGE"
	}

	lineContent := fmt.Sprintf("    %-*s    %8s    %-10s%s", nameWidth, displayName, fmt.Sprintf("%d pods", node.PodCount), version, surgeLabel)

	if idx == m.listIndex {
		return "► " + nodeListSelectedStyle.Width(rowWidth).Render(lineContent) + "\n"
	}
	return "  " + lineContent + "\n"
}

// renderEventsSection shows latest events for overview
func (m Model) renderEventsSection() string {
	var b strings.Builder

	title := panelTitleStyle.Render("● EVENTS")
	b.WriteString(title)
	b.WriteString("\n")

	eventsToShow := 8
	if len(m.events) < eventsToShow {
		eventsToShow = len(m.events)
	}

	if eventsToShow == 0 {
		b.WriteString(footerDescStyle.Render("  Waiting for events..."))
		return b.String()
	}

	maxMsgLen := m.mainWidth() - 25
	if maxMsgLen < 30 {
		maxMsgLen = 30
	}

	for i := 0; i < eventsToShow; i++ {
		e := m.events[i]

		ts := timestampStyle.Render(e.Timestamp.Format("15:04:05"))
		icon := m.severityIcon(e.Severity)

		msg := e.Message
		if len(msg) > maxMsgLen {
			msg = msg[:maxMsgLen-3] + "..."
		}

		b.WriteString(fmt.Sprintf("%s  %s %s\n", ts, icon, msg))
	}

	return b.String()
}
