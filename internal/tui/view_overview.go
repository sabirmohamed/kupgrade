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

// renderNodeList shows unified node list with selected row highlight
func (m Model) renderNodeList() string {
	var b strings.Builder

	// Calculate visible height for node list
	blockerLines := 0
	if len(m.blockers) > 0 {
		activeBlockers, riskBlockers := m.splitBlockersByTier()
		// Each blocker takes 2 lines (name + detail), plus section headers
		blockerLines = len(m.blockers) * 2
		if len(activeBlockers) > 0 {
			blockerLines++ // "ACTIVE BLOCKERS" header
		}
		if len(riskBlockers) > 0 {
			blockerLines++ // "PDB RISKS" header
		}
		blockerLines += 2 // panel border padding
	}
	eventsLines := len(m.events)
	if eventsLines > 8 {
		eventsLines = 8
	}
	reservedLines := 1 + 3 + blockerLines + eventsLines + 4 + 3
	visibleRows := m.height - reservedLines
	if visibleRows < 3 {
		visibleRows = 3
	}
	if visibleRows > 15 {
		visibleRows = 15
	}

	// Get all nodes sorted by stage priority, then name
	allNodes := m.getSortedNodeList()

	// Calculate scroll offset
	scrollOffset := 0
	if m.listIndex >= visibleRows {
		scrollOffset = m.listIndex - visibleRows + 1
	}

	// Show node count with total in title
	total := len(allNodes)

	titleStr := fmt.Sprintf("NODES (%d)", total)

	hints := footerDescStyle.Render("↑↓ navigate • d describe")

	titleLen := len(titleStr)
	hintsLen := 20
	spacing := m.mainWidth() - titleLen - hintsLen - 4
	if spacing < 4 {
		spacing = 4
	}

	b.WriteString(panelTitleStyle.Render(titleStr))
	b.WriteString(strings.Repeat(" ", spacing))
	b.WriteString(hints)
	b.WriteString("\n")

	if len(allNodes) == 0 {
		b.WriteString(footerDescStyle.Render("  No nodes discovered"))
		return b.String()
	}

	// Calculate column widths
	nameWidth := 30
	if m.mainWidth() > 100 {
		nameWidth = 35
	}
	if m.mainWidth() > 120 {
		nameWidth = 40
	}

	rowWidth := m.mainWidth() - 6
	if rowWidth < 60 {
		rowWidth = 60
	}

	endIdx := scrollOffset + visibleRows
	if endIdx > len(allNodes) {
		endIdx = len(allNodes)
	}

	for i := scrollOffset; i < endIdx; i++ {
		nodeName := allNodes[i]
		node := m.nodes[nodeName]

		displayName := nodeName
		if len(displayName) > nameWidth {
			displayName = displayName[len(displayName)-nameWidth:]
		}

		pods := fmt.Sprintf("%d pods", node.PodCount)

		version := node.Version
		if version == "" {
			version = "unknown"
		}

		lineContent := fmt.Sprintf("    %-*s    %8s    %-10s", nameWidth, displayName, pods, version)

		if i == m.listIndex {
			cursor := "► "
			selectedLine := nodeListSelectedStyle.Width(rowWidth).Render(lineContent)
			b.WriteString(cursor)
			b.WriteString(selectedLine)
		} else {
			b.WriteString("  ")
			b.WriteString(lineContent)
		}
		b.WriteString("\n")
	}

	return b.String()
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
