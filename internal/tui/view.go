package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sabirmohamed/kupgrade/pkg/types"
)

func (m Model) View() string {
	if m.fatalError != nil {
		return fmt.Sprintf("Error: %v\n", m.fatalError)
	}

	// Render the current screen
	var content string
	switch m.screen {
	case ScreenOverview:
		content = m.renderOverview()
	case ScreenNodes:
		content = m.renderNodesScreen()
	case ScreenDrains:
		content = m.renderDrainsScreen()
	case ScreenPods:
		content = m.renderPodsScreen()
	case ScreenBlockers:
		content = m.renderBlockersScreen()
	case ScreenEvents:
		content = m.renderEventsScreen()
	case ScreenStats:
		content = m.renderStatsScreen()
	default:
		content = m.renderOverview()
	}

	// Render overlay on top if active
	switch m.overlay {
	case OverlayHelp:
		return m.renderWithOverlay(m.renderHelpOverlay())
	case OverlayNodeDetail:
		return m.renderWithOverlay(m.renderNodeDetailOverlay())
	default:
		return content
	}
}

func (m Model) renderOverview() string {
	var b strings.Builder

	// New layout: Header → Pipeline → Blockers → Node List → Events → Footer
	b.WriteString(m.renderCompactHeader())
	b.WriteString("\n")
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

	// Events at bottom (8 events minimum)
	b.WriteString(m.renderEventsSection())
	b.WriteString("\n")

	b.WriteString(m.renderFooter())

	content := b.String()

	// Fill terminal dimensions
	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, content)
	}
	return content
}

func (m Model) renderCompactHeader() string {
	// Compact header for new overview layout
	title := "★ kupgrade"
	titleDisplay := headerStyle.Render(title)

	context := contextStyle.Render(m.contextName())

	version := m.serverVersion()
	if m.targetVersion() != "" && m.targetVersion() != m.serverVersion() {
		version = fmt.Sprintf("%s → %s", m.serverVersion(), m.targetVersion())
	}
	versionDisplay := versionStyle.Render(version)

	// Progress bar with percentage
	progress := m.renderProgressBar(headerProgressBarWidth)
	percent := fmt.Sprintf("%3d%%", m.progressPercent())

	timeDisplay := timeStyle.Render(m.currentTime.Format("15:04:05"))

	// Build header line with separators
	return fmt.Sprintf("%s  %s  %s  %s %s  %s",
		titleDisplay, context, versionDisplay, progress, percent, timeDisplay)
}

func (m Model) renderHeader() string {
	// Build title with screen name
	title := "⎈ kupgrade"
	if screenName := m.screenName(); screenName != "" {
		title = fmt.Sprintf("⎈ kupgrade › %s", screenName)
	}
	titleDisplay := headerStyle.Render(title)

	context := contextStyle.Render(m.contextName())

	version := m.serverVersion()
	if m.targetVersion() != "" && m.targetVersion() != m.serverVersion() {
		version = fmt.Sprintf("%s→%s", m.serverVersion(), m.targetVersion())
	}
	versionDisplay := versionStyle.Render(version)

	progress := m.renderProgressBar(headerProgressBarWidth)
	percent := fmt.Sprintf("%d%%", m.progressPercent())

	timeDisplay := timeStyle.Render(m.currentTime.Format("15:04:05"))

	return fmt.Sprintf("%s  %s | %s | %s %s | %s",
		titleDisplay, context, versionDisplay, progress, percent, timeDisplay)
}

func (m Model) renderProgressBar(width int) string {
	percent := m.progressPercent()
	filled := (percent * width) / 100
	empty := width - filled

	bar := strings.Repeat(progressBarFull, filled) + strings.Repeat(progressBarEmpty, empty)
	return progressStyle.Render(bar)
}

func (m Model) renderMainContent() string {
	return m.renderNodeColumns()
}

// renderPipelineRow shows compact stage counts with dashes between
func (m Model) renderPipelineRow() string {
	stages := types.AllStages()
	var parts []string

	for i, stage := range stages {
		count := len(m.nodesByStage[stage])
		name := string(stage)

		// Highlight selected stage
		var stageStr string
		if i == m.selectedStage {
			stageStr = stageStyleSelected(name).Render(name)
		} else {
			stageStr = stageStyle(name).Render(name)
		}

		countStr := fmt.Sprintf("%d", count)
		if count > 0 {
			countStr = stageStyle(name).Render(countStr)
		} else {
			countStr = footerDescStyle.Render(countStr)
		}

		parts = append(parts, fmt.Sprintf("%s\n%s", centerText(stageStr, 12), centerText(countStr, 12)))

		// Add dash between stages (not after last)
		if i < len(stages)-1 {
			parts = append(parts, footerDescStyle.Render("  —  "))
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Center, parts...)
}

// renderBlockersSection shows blockers with left border accent
func (m Model) renderBlockersSection() string {
	if len(m.blockers) == 0 {
		return ""
	}

	var lines []string
	title := warningStyle.Render(fmt.Sprintf("⚠ BLOCKERS (%d)", len(m.blockers)))
	lines = append(lines, title)

	for _, blocker := range m.blockers {
		// Show namespace/name for namespaced resources
		name := blocker.Name
		if blocker.Namespace != "" {
			name = blocker.Namespace + "/" + blocker.Name
		}

		// Format: PDB namespace/name    constraint    → result
		nameStr := fmt.Sprintf("%s %s", blocker.Type, warningStyle.Render(name))

		// Parse detail to show constraint and result separately
		result := ""
		constraint := blocker.Detail
		if blocker.Detail != "" {
			result = errorStyle.Render("→ 0 evictions allowed")
		}

		line := fmt.Sprintf("%s    %s    %s", nameStr, footerDescStyle.Render(constraint), result)
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")

	// Left border only style
	return blockerPanelStyle.Render(content)
}

// renderDrainProgressSection shows drain progress for actively draining nodes
func (m Model) renderDrainProgressSection() string {
	// Find draining nodes
	drainingNodes := m.nodesByStage[types.StageDraining]
	if len(drainingNodes) == 0 {
		return ""
	}

	var b strings.Builder

	for _, nodeName := range drainingNodes {
		node := m.nodes[nodeName]

		// Title with spinner
		title := fmt.Sprintf("%s DRAINING: %s", m.spinner(), strings.ToUpper(nodeName))
		b.WriteString(warningStyle.Render(title))
		b.WriteString("\n")

		// Progress bar
		evicted := node.InitialPodCount - node.PodCount
		if evicted < 0 {
			evicted = 0
		}
		total := node.InitialPodCount
		if total == 0 {
			total = node.PodCount // fallback if initial not set
		}

		// Build progress bar (20 chars wide)
		barWidth := 20
		filled := (node.DrainProgress * barWidth) / 100
		if filled > barWidth {
			filled = barWidth
		}
		empty := barWidth - filled
		bar := strings.Repeat(progressBarFull, filled) + strings.Repeat(progressBarEmpty, empty)

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

		progressLine := fmt.Sprintf("%s  %d/%d pods evicted%s", progressStyle.Render(bar), evicted, total, footerDescStyle.Render(elapsed))
		b.WriteString(progressLine)
		b.WriteString("\n")

		// Show waiting pods if any
		if len(node.WaitingPods) > 0 {
			waiting := "Waiting on: " + strings.Join(node.WaitingPods, "  ")
			if len(waiting) > m.width-4 {
				waiting = waiting[:m.width-7] + "..."
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
	// Reserve: header(1) + pipeline(3) + blockers(if any) + events(10) + footer(3)
	blockerLines := 0
	if len(m.blockers) > 0 {
		blockerLines = len(m.blockers) + 3 // title + border
	}
	reservedLines := 1 + 3 + blockerLines + 12 + 3
	visibleRows := m.height - reservedLines
	if visibleRows < 3 {
		visibleRows = 3
	}
	if visibleRows > 15 {
		visibleRows = 15 // cap to leave room for events
	}

	// Get all nodes sorted by stage priority, then name
	allNodes := m.getSortedNodeList()

	// Calculate scroll offset
	scrollOffset := 0
	if m.listIndex >= visibleRows {
		scrollOffset = m.listIndex - visibleRows + 1
	}

	// Show node count with selected stage in title
	stageName := m.stageAtIndex(m.selectedStage)
	stageCount := len(m.nodesByStage[stageName])
	total := len(allNodes)

	titleStr := fmt.Sprintf("NODES: %s (%d)", stageName, stageCount)
	if stageCount == total {
		titleStr = fmt.Sprintf("NODES: %s (%d)", stageName, total)
	}

	// Right-aligned hints
	hints := footerDescStyle.Render("↑↓ navigate • ⏎ details")

	// Calculate spacing for right alignment
	titleLen := len(fmt.Sprintf("NODES: %s (%d)", stageName, stageCount))
	hintsLen := 20 // approximate
	spacing := m.width - titleLen - hintsLen - 4
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
	if m.width > 100 {
		nameWidth = 35
	}
	if m.width > 120 {
		nameWidth = 40
	}

	// Row width for selection box
	rowWidth := m.width - 6
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

		// Truncate name if needed
		displayName := nodeName
		if len(displayName) > nameWidth {
			displayName = displayName[len(displayName)-nameWidth:]
		}

		// Pod count
		pods := fmt.Sprintf("%d pods", node.PodCount)

		// Version
		version := node.Version
		if version == "" {
			version = "unknown"
		}

		// Build line content (no stage column - it's in title)
		lineContent := fmt.Sprintf("    %-*s    %8s    %-10s", nameWidth, displayName, pods, version)

		if i == m.listIndex {
			// Selected row: cursor + highlighted box
			cursor := "► "
			// Use rounded border for selected row
			selectedLine := nodeListSelectedStyle.Width(rowWidth).Render(lineContent)
			b.WriteString(cursor)
			b.WriteString(selectedLine)
		} else {
			// Normal row
			b.WriteString("  ")
			b.WriteString(lineContent)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// renderEventsSection shows latest events
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

	// Calculate max message length
	maxMsgLen := m.width - 25 // timestamp + icon + padding
	if maxMsgLen < 30 {
		maxMsgLen = 30
	}

	// Show most recent events first
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

func (m Model) renderNodeColumns() string {
	stages := types.AllStages()
	columns := make([]string, len(stages))
	cardWidth := m.nodeCardWidth()

	for i, stage := range stages {
		name := string(stage)
		count := len(m.nodesByStage[stage])

		// Stage header
		var header string
		if i == m.selectedStage {
			header = stageStyleSelected(name).Render(name)
		} else {
			header = stageStyle(name).Render(name)
		}

		// Build column: header + count + nodes
		var columnParts []string
		columnParts = append(columnParts, centerText(header, cardWidth))
		columnParts = append(columnParts, centerText(fmt.Sprintf("%d", count), cardWidth))
		columnParts = append(columnParts, "") // spacer

		// Node cards
		nodes := m.nodesByStage[stage]
		if len(nodes) == 0 {
			columnParts = append(columnParts, m.renderEmptyStage(cardWidth))
		} else {
			for j, nodeName := range nodes {
				node := m.nodes[nodeName]
				isSelected := i == m.selectedStage && j == m.selectedNode
				columnParts = append(columnParts, m.renderNodeCard(node, isSelected, cardWidth))
			}
		}

		columns[i] = lipgloss.JoinVertical(lipgloss.Left, columnParts...)
	}

	var parts []string
	for i, col := range columns {
		if i > 0 {
			parts = append(parts, "  ")
		}
		parts = append(parts, col)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (m Model) renderEmptyStage(cardWidth int) string {
	content := nodePodStyle.Render("(empty)")
	return nodeCardNormal.Width(cardWidth).Render(content)
}

func (m Model) renderNodeCard(node types.NodeState, selected bool, cardWidth int) string {
	var b strings.Builder

	// Truncate name to fit card width (accounting for padding/border)
	maxNameLen := cardWidth - 4
	if maxNameLen < 8 {
		maxNameLen = 8
	}
	name := node.Name
	if len(name) > maxNameLen {
		name = name[len(name)-maxNameLen:]
	}
	b.WriteString(nodeNameStyle.Render(name))
	b.WriteString("\n")

	if node.Stage == types.StageDraining && node.DrainProgress > 0 {
		b.WriteString(fmt.Sprintf("%d pods remaining\n", node.PodCount))
		b.WriteString(m.renderSmallProgressBar(node.DrainProgress))
	} else if node.Stage == types.StageUpgrading {
		b.WriteString(m.spinner() + " reimaging...")
	} else {
		b.WriteString(nodePodStyle.Render(fmt.Sprintf("%d pods", node.PodCount)))
	}
	b.WriteString("\n")

	version := node.Version
	if version == "" {
		version = "unknown"
	}
	if node.Stage == types.StageComplete {
		version += " " + checkIcon
	}
	b.WriteString(nodeVersionStyle.Render(version))

	if node.Blocked {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render("⚠ " + node.BlockerReason))
	}

	content := b.String()

	var style lipgloss.Style
	switch {
	case selected:
		style = nodeCardSelected.Width(cardWidth)
	case node.Blocked:
		style = nodeCardBlocked.Width(cardWidth)
	case node.Stage == types.StageComplete:
		style = nodeCardComplete.Width(cardWidth)
	default:
		style = nodeCardNormal.Width(cardWidth)
	}

	return style.Render(content)
}

func (m Model) renderSmallProgressBar(percent int) string {
	width := 12
	filled := (percent * width) / 100
	empty := width - filled
	bar := strings.Repeat(progressBarFull, filled) + strings.Repeat(progressBarEmpty, empty)
	return fmt.Sprintf("%s %d%%", bar, percent)
}

func (m Model) renderBottomPanels() string {
	var panels []string

	// Calculate panel widths based on terminal width
	blockersWidth, migrationsWidth, eventsWidth := m.panelWidths()

	if len(m.blockers) > 0 {
		panels = append(panels, m.renderBlockersPanel(blockersWidth))
	}

	panels = append(panels, m.renderMigrationsPanel(migrationsWidth))
	panels = append(panels, m.renderEventsPanel(eventsWidth))

	return lipgloss.JoinHorizontal(lipgloss.Top, panels...)
}

func (m Model) renderBlockersPanel(width int) string {
	title := panelTitleError.Render(fmt.Sprintf("⚠ BLOCKERS (%d)", len(m.blockers)))
	var lines []string
	lines = append(lines, title)

	for _, blocker := range m.blockers {
		// Show namespace/name for namespaced resources
		name := blocker.Name
		if blocker.Namespace != "" {
			name = blocker.Namespace + "/" + blocker.Name
		}
		line := fmt.Sprintf("%s: %s", blocker.Type, name)
		lines = append(lines, errorStyle.Render(line))

		// Show detail on next line with indent
		if blocker.Detail != "" {
			detail := "  └─ " + blocker.Detail
			lines = append(lines, warningStyle.Render(truncateString(detail, width-4)))
		}
	}

	content := strings.Join(lines, "\n")
	return panelStyle.Width(width).MarginRight(2).Render(content)
}

func (m Model) renderMigrationsPanel(width int) string {
	title := panelTitleStyle.Render("↹ RESCHEDULED")
	var lines []string
	lines = append(lines, title)

	if len(m.migrations) == 0 {
		lines = append(lines, footerDescStyle.Render("No pod moves yet"))
	} else {
		for _, mig := range m.migrations {
			icon := migrateIcon
			if mig.Complete {
				icon = checkIcon
			}
			line := fmt.Sprintf("%s %s/%s → %s", icon, mig.Namespace, mig.NewPod, mig.ToNode)
			lines = append(lines, line)
		}
	}

	content := strings.Join(lines, "\n")
	return panelStyle.Width(width).MarginRight(2).Render(content)
}

func (m Model) renderEventsPanel(width int) string {
	title := panelTitleStyle.Render("• EVENTS")
	var lines []string
	lines = append(lines, title)

	// Calculate max message length based on panel width
	maxMsgLen := width - eventPaddingTotal
	if maxMsgLen < eventMinMessageWidth {
		maxMsgLen = eventMinMessageWidth
	}

	if len(m.events) == 0 {
		lines = append(lines, footerDescStyle.Render("Waiting for events..."))
	} else {
		for _, e := range m.events {
			ts := timestampStyle.Render(e.Timestamp.Format("15:04:05"))
			icon := m.severityIcon(e.Severity)
			msg := e.Message
			if len(msg) > maxMsgLen {
				msg = msg[:maxMsgLen] + "..."
			}
			lines = append(lines, fmt.Sprintf("%s %s %s", ts, icon, msg))
		}
	}

	content := strings.Join(lines, "\n")
	return panelStyle.Width(width).Render(content)
}

func (m Model) severityIcon(s types.Severity) string {
	switch s {
	case types.SeverityWarning:
		return warningStyle.Render(warningIcon)
	case types.SeverityError:
		return errorStyle.Render(errorIcon)
	default:
		return infoStyle.Render(infoIcon)
	}
}

func (m Model) renderFooter() string {
	// Single row: screen navigation
	hints := []struct {
		key  string
		desc string
	}{
		{"0", "overview"},
		{"1", "nodes"},
		{"2", "drains"},
		{"3", "pods"},
		{"4", "blockers"},
		{"5", "events"},
		{"?", "help"},
		{"q", "quit"},
	}

	var parts []string
	for _, h := range hints {
		parts = append(parts, footerKeyStyle.Render(h.key)+" "+footerDescStyle.Render(h.desc))
	}

	return footerStyle.Render(strings.Join(parts, "  "))
}

func (m Model) renderWithOverlay(overlay string) string {
	bg := m.renderOverview()
	_ = bg
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
}

func (m Model) renderHelpOverlay() string {
	title := overlayTitleStyle.Render("Keyboard Shortcuts")

	help := []string{
		title,
		"",
		footerKeyStyle.Render("←/h") + "     Previous stage",
		footerKeyStyle.Render("→/l") + "     Next stage",
		footerKeyStyle.Render("↑/k") + "     Previous node",
		footerKeyStyle.Render("↓/j") + "     Next node",
		footerKeyStyle.Render("enter") + "   Node details",
		footerKeyStyle.Render("?") + "       Toggle help",
		footerKeyStyle.Render("esc") + "     Close overlay",
		footerKeyStyle.Render("q") + "       Quit",
	}

	content := strings.Join(help, "\n")
	return overlayStyle.Render(content)
}

func (m Model) renderNodeDetailOverlay() string {
	node, ok := m.selectedNodeState()
	if !ok {
		return overlayStyle.Render("No node selected")
	}

	title := overlayTitleStyle.Render("Node: " + node.Name)

	lines := []string{
		title,
		"",
		fmt.Sprintf("Stage:       %s", stageStyle(string(node.Stage)).Render(string(node.Stage))),
		fmt.Sprintf("Version:     %s", node.Version),
		fmt.Sprintf("Ready:       %v", node.Ready),
		fmt.Sprintf("Schedulable: %v", node.Schedulable),
		fmt.Sprintf("Pod Count:   %d", node.PodCount),
	}

	if node.Blocked {
		lines = append(lines, "")
		lines = append(lines, errorStyle.Render("⚠ BLOCKED: "+node.BlockerReason))
	}

	lines = append(lines, "")
	lines = append(lines, footerDescStyle.Render("Press ESC or Enter to close"))

	content := strings.Join(lines, "\n")
	return overlayStyle.Render(content)
}

// Screen placeholder renderers (E1-E7 will implement these)

func (m Model) renderNodesScreen() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")

	// Calculate name column width based on terminal width
	// Reserve space for: cursor(2) + version(12) + stage(12) + age(8) + conditions(12) + taints(20) + gaps(6)
	fixedWidth := 2 + 12 + 12 + 8 + 12 + 20 + 6
	nameWidth := m.width - fixedWidth
	if nameWidth < 20 {
		nameWidth = 20
	}
	if nameWidth > 50 {
		nameWidth = 50
	}

	// Table header
	header := fmt.Sprintf("  %-*s %-12s %-12s %-8s %-12s %-20s",
		nameWidth, "NAME", "VERSION", "STAGE", "AGE", "CONDITIONS", "TAINTS")
	b.WriteString(panelTitleStyle.Render(header))
	b.WriteString("\n")

	// Node list
	nodes := m.sortedNodeNames()
	for i, name := range nodes {
		node := m.nodes[name]
		cursor := "  "
		if i == m.listIndex {
			cursor = "► "
		}

		// Format conditions - show "Ready" if no issues, otherwise list problems
		conditions := "Ready"
		if !node.Ready {
			conditions = "NotReady"
		} else if len(node.Conditions) > 0 {
			conditions = strings.Join(node.Conditions, ",")
		}

		// Format taints - show "-" if none, otherwise list effects
		taints := "-"
		if len(node.Taints) > 0 {
			taints = strings.Join(node.Taints, ",")
		} else if !node.Schedulable {
			taints = "NoSchedule"
		}

		// Format age - use actual age from node if available
		age := node.Age
		if age == "" {
			age = "-"
		}

		// Show full name if it fits, otherwise truncate
		displayName := name
		if len(name) > nameWidth {
			displayName = truncateString(name, nameWidth)
		}

		line := fmt.Sprintf("%s%-*s %-12s %-12s %-8s %-12s %-20s",
			cursor,
			nameWidth,
			displayName,
			node.Version,
			node.Stage,
			age,
			truncateString(conditions, 12),
			truncateString(taints, 20),
		)

		if i == m.listIndex {
			b.WriteString(nodeNameStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(m.renderFooter())

	return m.placeContent(b.String())
}

func (m Model) renderDrainsScreen() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")

	// Collect nodes that are draining or cordoned (in drain pipeline)
	var drainNodes []string
	for _, name := range m.nodesByStage[types.StageCordoned] {
		drainNodes = append(drainNodes, name)
	}
	for _, name := range m.nodesByStage[types.StageDraining] {
		drainNodes = append(drainNodes, name)
	}
	sort.Strings(drainNodes)

	if len(drainNodes) == 0 {
		b.WriteString(footerDescStyle.Render("  No nodes currently draining or cordoned\n"))
		b.WriteString(footerDescStyle.Render("  Nodes will appear here when cordoned for upgrade"))
	} else {
		// Table header
		header := fmt.Sprintf("  %-40s %-12s %-14s %-10s %s",
			"NODE", "STAGE", "PROGRESS", "PODS", "STATUS")
		b.WriteString(panelTitleStyle.Render(header))
		b.WriteString("\n")

		for i, name := range drainNodes {
			node := m.nodes[name]
			cursor := "  "
			if i == m.listIndex {
				cursor = "► "
			}

			// Progress bar
			progress := m.renderSmallProgressBar(node.DrainProgress)

			// Status
			status := "-"
			statusStyle := footerDescStyle
			if node.Blocked {
				status = node.BlockerReason
				if status == "" {
					status = "blocked"
				}
				statusStyle = errorStyle
			} else if node.Stage == types.StageDraining {
				status = "evicting..."
				statusStyle = warningStyle
			} else if node.Stage == types.StageCordoned {
				status = "waiting"
				statusStyle = footerDescStyle
			}

			// Pods remaining
			pods := fmt.Sprintf("%d", node.PodCount)

			line := fmt.Sprintf("%s%-40s %-12s %s  %-10s ",
				cursor,
				truncateString(name, 40),
				node.Stage,
				progress,
				pods,
			)

			if i == m.listIndex {
				b.WriteString(nodeNameStyle.Render(line))
				b.WriteString(statusStyle.Render(truncateString(status, 30)))
			} else {
				b.WriteString(line)
				b.WriteString(statusStyle.Render(truncateString(status, 30)))
			}
			b.WriteString("\n")

			// Show blocker detail for selected node if blocked
			if i == m.listIndex && node.Blocked && node.BlockerReason != "" {
				b.WriteString(errorStyle.Render(fmt.Sprintf("      └─ %s", node.BlockerReason)))
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(m.renderFooter())

	return m.placeContent(b.String())
}

func (m Model) renderPodsScreen() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")

	// Get nodes in upgrade pipeline (CORDONED, DRAINING, UPGRADING)
	upgradeNodes := make(map[string]bool)
	for _, name := range m.nodesByStage[types.StageCordoned] {
		upgradeNodes[name] = true
	}
	for _, name := range m.nodesByStage[types.StageDraining] {
		upgradeNodes[name] = true
	}
	for _, name := range m.nodesByStage[types.StageUpgrading] {
		upgradeNodes[name] = true
	}

	// Collect pods on upgrade nodes (or all if no nodes upgrading)
	var podList []types.PodState
	showAll := len(upgradeNodes) == 0
	for _, pod := range m.pods {
		if showAll || upgradeNodes[pod.NodeName] {
			podList = append(podList, pod)
		}
	}

	// Sort by node first (group by node), then namespace, then name
	sort.Slice(podList, func(i, j int) bool {
		if podList[i].NodeName != podList[j].NodeName {
			return podList[i].NodeName < podList[j].NodeName
		}
		if podList[i].Namespace != podList[j].Namespace {
			return podList[i].Namespace < podList[j].Namespace
		}
		return podList[i].Name < podList[j].Name
	})

	if len(podList) == 0 {
		if showAll {
			b.WriteString(footerDescStyle.Render("  No pods found"))
		} else {
			b.WriteString(footerDescStyle.Render("  No pods on upgrading nodes"))
			b.WriteString("\n")
			b.WriteString(footerDescStyle.Render("  (showing pods on CORDONED/DRAINING/UPGRADING nodes only)"))
		}
	} else {
		// Calculate responsive column widths based on terminal width
		// Minimum: 120 chars, expand columns proportionally for wider terminals
		availWidth := m.width - 4 // margins
		if availWidth < 120 {
			availWidth = 120
		}

		// Fixed columns: READY(5), STATUS(16), RESTARTS(10), PROBES(5), OWNER(12), AGE(5) = ~57
		// Variable columns: NAMESPACE, NAME, NODE share remaining space
		fixedWidth := 57
		varWidth := availWidth - fixedWidth
		nsWidth := varWidth * 15 / 100  // 15%
		nameWidth := varWidth * 40 / 100 // 40%
		nodeWidth := varWidth * 45 / 100 // 45%

		// Minimum widths
		if nsWidth < 12 {
			nsWidth = 12
		}
		if nameWidth < 30 {
			nameWidth = 30
		}
		if nodeWidth < 25 {
			nodeWidth = 25
		}

		// Maximum widths (prevent excessive spacing on wide terminals)
		if nsWidth > 15 {
			nsWidth = 15
		}
		if nameWidth > 55 {
			nameWidth = 55 // longest pod names are ~50-55 chars
		}
		if nodeWidth > 40 {
			nodeWidth = 40
		}

		// Calculate visible rows
		visibleRows := m.height - 10
		if visibleRows < 5 {
			visibleRows = 5
		}

		// Calculate scroll offset to keep cursor visible
		scrollOffset := 0
		if m.listIndex >= visibleRows {
			scrollOffset = m.listIndex - visibleRows + 1
		}

		// Show count and scroll position
		total := len(podList)
		filterNote := ""
		if !showAll {
			filterNote = " (upgrading nodes)"
		}
		scrollInfo := ""
		if total > visibleRows {
			scrollInfo = fmt.Sprintf(" [%d-%d of %d]", scrollOffset+1, min(scrollOffset+visibleRows, total), total)
		}
		b.WriteString(fmt.Sprintf("  pods(%d)%s%s\n", total, filterNote, scrollInfo))

		// Table header with separator
		headerFmt := fmt.Sprintf("  %%-%ds %%-%ds %%5s %%-16s %%-10s %%-5s %%-12s %%-%ds %%5s",
			nsWidth, nameWidth, nodeWidth)
		header := fmt.Sprintf(headerFmt,
			"NAMESPACE", "NAME", "READY", "STATUS", "RESTARTS", "PROBE", "OWNER", "NODE", "AGE")
		b.WriteString(panelTitleStyle.Render(header))
		b.WriteString("\n")

		// Separator line
		sepLen := nsWidth + nameWidth + nodeWidth + 50
		if sepLen > m.width-2 {
			sepLen = m.width - 2
		}
		b.WriteString(footerDescStyle.Render("  " + strings.Repeat("─", sepLen)))
		b.WriteString("\n")

		endIdx := scrollOffset + visibleRows
		if endIdx > len(podList) {
			endIdx = len(podList)
		}

		prevNode := ""
		for i := scrollOffset; i < endIdx; i++ {
			pod := podList[i]

			// Add visual separator between nodes (group by node)
			if pod.NodeName != prevNode && prevNode != "" && i > scrollOffset {
				b.WriteString(footerDescStyle.Render("  " + strings.Repeat("·", sepLen/2)))
				b.WriteString("\n")
			}
			prevNode = pod.NodeName

			cursor := "  "
			if i == m.listIndex {
				cursor = "► "
			}

			// Namespace
			namespace := truncateString(pod.Namespace, nsWidth)

			// Pod name
			name := truncateString(pod.Name, nameWidth)

			// Ready containers (1/1 format)
			readyStr := fmt.Sprintf("%d/%d", pod.ReadyContainers, pod.TotalContainers)
			readyStyle := successStyle
			if pod.ReadyContainers < pod.TotalContainers {
				readyStyle = warningStyle
			}
			if pod.ReadyContainers == 0 && pod.TotalContainers > 0 {
				readyStyle = errorStyle
			}

			// Status with color (16 chars to fit CrashLoopBackOff)
			status := truncateString(pod.Phase, 16)
			statusStyle := successStyle
			switch {
			case pod.Phase == "Running":
				statusStyle = successStyle
			case pod.Phase == "Pending":
				statusStyle = warningStyle
			case pod.Phase == "Succeeded" || pod.Phase == "Completed":
				statusStyle = footerDescStyle
			case pod.Phase == "CrashLoopBackOff" || pod.Phase == "ImagePullBackOff" ||
				pod.Phase == "ErrImagePull" || pod.Phase == "Error" ||
				pod.Phase == "Failed" || pod.Phase == "Unknown" ||
				pod.Phase == "Terminating" || pod.Phase == "OOMKilled" ||
				strings.HasPrefix(pod.Phase, "Init:"):
				statusStyle = errorStyle
			}

			// Restarts with time since last restart (like kubectl)
			// Format: "23 4m" (count + time since last restart)
			var restartStr string
			restartStyle := footerDescStyle
			if pod.Restarts == 0 {
				restartStr = "0"
			} else if pod.LastRestartAge != "" {
				restartStr = fmt.Sprintf("%d %s", pod.Restarts, pod.LastRestartAge)
			} else {
				restartStr = fmt.Sprintf("%d", pod.Restarts)
			}

			if pod.Restarts > 5 {
				restartStyle = errorStyle
			} else if pod.Restarts > 0 {
				restartStyle = warningStyle
			}

			// Probes - R✓ L✓ format (Readiness first, then Liveness)
			// · = not configured, ✓ = passing (green), ✗ = failing (red)
			var rProbe, lProbe string
			var rStyle, lStyle lipgloss.Style

			// Readiness probe
			if pod.HasReadiness {
				if pod.ReadinessOK {
					rProbe = "R✓"
					rStyle = successStyle
				} else {
					rProbe = "R✗"
					rStyle = errorStyle
				}
			} else {
				rProbe = "··"
				rStyle = footerDescStyle
			}

			// Liveness probe
			if pod.HasLiveness {
				if pod.LivenessOK {
					lProbe = "L✓"
					lStyle = successStyle
				} else {
					lProbe = "L✗"
					lStyle = errorStyle
				}
			} else {
				lProbe = "··"
				lStyle = footerDescStyle
			}

			// Owner kind (important for upgrades - DaemonSet can't evict)
			owner := truncateString(pod.OwnerKind, 12)
			if owner == "" {
				owner = "<none>"
			}
			ownerStyle := footerDescStyle
			if pod.OwnerKind == "DaemonSet" {
				ownerStyle = warningStyle // DaemonSets can't be evicted
			}

			// Node name
			nodeName := truncateString(pod.NodeName, nodeWidth)
			if nodeName == "" {
				nodeName = "<pending>"
			}

			// Build line with proper formatting
			lineFmt := fmt.Sprintf("%%s%%-%ds %%-%ds ", nsWidth, nameWidth)
			line := fmt.Sprintf(lineFmt, cursor, namespace, name)
			if i == m.listIndex {
				b.WriteString(nodeNameStyle.Render(line))
			} else {
				b.WriteString(line)
			}

			b.WriteString(readyStyle.Render(fmt.Sprintf("%5s ", readyStr)))
			b.WriteString(statusStyle.Render(fmt.Sprintf("%-16s ", status)))
			b.WriteString(restartStyle.Render(fmt.Sprintf("%-10s ", restartStr)))
			b.WriteString(rStyle.Render(rProbe))
			b.WriteString(" ")
			b.WriteString(lStyle.Render(lProbe))
			b.WriteString(" ")
			b.WriteString(ownerStyle.Render(fmt.Sprintf("%-12s ", owner)))
			b.WriteString(footerDescStyle.Render(fmt.Sprintf("%-*s ", nodeWidth, nodeName)))
			b.WriteString(footerDescStyle.Render(fmt.Sprintf("%5s", pod.Age)))
			b.WriteString("\n")
		}

		// Scroll indicator at bottom if more items
		if total > visibleRows {
			b.WriteString("\n")
			if scrollOffset > 0 {
				b.WriteString(footerDescStyle.Render("  ↑ more above"))
			}
			if endIdx < total {
				if scrollOffset > 0 {
					b.WriteString(footerDescStyle.Render("  |  "))
				} else {
					b.WriteString(footerDescStyle.Render("  "))
				}
				b.WriteString(footerDescStyle.Render("↓ more below"))
			}
		}
	}

	b.WriteString("\n\n")
	b.WriteString(m.renderPodsFooter())

	return m.placeContent(b.String())
}

func (m Model) renderPodsFooter() string {
	// Use the common two-row footer
	return m.renderFooter()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (m Model) renderBlockersScreen() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")

	if len(m.blockers) == 0 {
		b.WriteString(successStyle.Render("  No blockers detected"))
	} else {
		for i, blocker := range m.blockers {
			cursor := "  "
			if i == m.listIndex {
				cursor = "► "
			}

			// Show namespace/name for namespaced resources
			name := blocker.Name
			if blocker.Namespace != "" {
				name = blocker.Namespace + "/" + blocker.Name
			}

			// First line: type and name
			line1 := fmt.Sprintf("%s%s: %s", cursor, blocker.Type, name)

			// Second line: detail (why it's blocking)
			line2 := fmt.Sprintf("    └─ %s", blocker.Detail)

			if i == m.listIndex {
				b.WriteString(errorStyle.Render(line1))
				b.WriteString("\n")
				b.WriteString(warningStyle.Render(line2))
			} else {
				b.WriteString(warningStyle.Render(line1))
				b.WriteString("\n")
				b.WriteString(footerDescStyle.Render(line2))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(m.renderFooter())

	return m.placeContent(b.String())
}

func (m Model) renderEventsScreen() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")

	if len(m.events) == 0 {
		b.WriteString(footerDescStyle.Render("  Waiting for events..."))
	} else {
		for i, e := range m.events {
			cursor := "  "
			if i == m.listIndex {
				cursor = "► "
			}

			ts := timestampStyle.Render(e.Timestamp.Format("15:04:05"))
			icon := m.severityIcon(e.Severity)
			nodeName := e.NodeName
			if nodeName == "" {
				nodeName = "-"
			}

			line := fmt.Sprintf("%s%s %s %-15s %s",
				cursor, ts, icon, nodeName, e.Message)

			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(m.renderFooter())

	return m.placeContent(b.String())
}

func (m Model) renderStatsScreen() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")

	// Progress section
	b.WriteString(panelTitleStyle.Render("  PROGRESS"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  ├─ Nodes Complete:    %d / %d  (%d%%)\n",
		m.completedNodes(), m.totalNodes(), m.progressPercent()))
	b.WriteString(fmt.Sprintf("  ├─ Nodes In Progress: %d\n",
		len(m.nodesByStage[types.StageCordoned])+
			len(m.nodesByStage[types.StageDraining])+
			len(m.nodesByStage[types.StageUpgrading])))
	b.WriteString(fmt.Sprintf("  └─ Nodes Remaining:   %d\n",
		len(m.nodesByStage[types.StageReady])))

	b.WriteString("\n")

	// Stage breakdown
	b.WriteString(panelTitleStyle.Render("  BY STAGE"))
	b.WriteString("\n")
	for _, stage := range types.AllStages() {
		count := len(m.nodesByStage[stage])
		b.WriteString(fmt.Sprintf("  ├─ %-12s %d\n", stage, count))
	}

	b.WriteString("\n")
	b.WriteString(footerDescStyle.Render("  Timing & velocity metrics will be added in E7"))
	b.WriteString("\n\n")
	b.WriteString(m.renderFooter())

	return m.placeContent(b.String())
}

// Helper functions for screens

func (m Model) sortedNodeNames() []string {
	names := make([]string, 0, len(m.nodes))
	for name := range m.nodes {
		names = append(names, name)
	}
	// Sort alphabetically for stable ordering
	sort.Strings(names)
	return names
}

func (m Model) placeContent(content string) string {
	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, content)
	}
	return content
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// Layout calculation helpers

// nodeCardWidth calculates card width based on terminal width
func (m Model) nodeCardWidth() int {
	if m.width <= 0 {
		return nodeCardMinWidth + 4 // default
	}
	// stageCount stages, nodeCardGapWidth between each
	gapTotal := (stageCount - 1) * nodeCardGapWidth
	available := m.width - gapTotal
	cardWidth := available / stageCount
	if cardWidth < nodeCardMinWidth {
		cardWidth = nodeCardMinWidth
	}
	if cardWidth > nodeCardMaxWidth {
		cardWidth = nodeCardMaxWidth
	}
	return cardWidth
}

// panelWidths calculates widths for bottom panels
func (m Model) panelWidths() (blockers, migrations, events int) {
	if m.width <= 0 {
		return 30, 30, 50 // defaults
	}

	available := m.width - 8 // margins between panels

	if len(m.blockers) > 0 {
		// Three panels: blockers 25%, migrations 30%, events 45%
		blockers = available * 25 / 100
		migrations = available * 30 / 100
		events = available - blockers - migrations
	} else {
		// Two panels: migrations 35%, events 65%
		blockers = 0
		migrations = available * 35 / 100
		events = available - migrations
	}

	// Ensure minimums
	if migrations < 25 {
		migrations = 25
	}
	if events < 40 {
		events = 40
	}

	return blockers, migrations, events
}
