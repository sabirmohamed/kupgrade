package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sabirmohamed/kupgrade/pkg/types"
)

func (m Model) View() string {
	if m.fatalError != nil {
		return fmt.Sprintf("Error: %v\n", m.fatalError)
	}

	switch m.viewMode {
	case ViewHelp:
		return m.renderWithOverlay(m.renderHelpOverlay())
	case ViewNodeDetail:
		return m.renderWithOverlay(m.renderNodeDetailOverlay())
	default:
		return m.renderOverview()
	}
}

func (m Model) renderOverview() string {
	var b strings.Builder

	b.WriteString(m.renderHeader())
	b.WriteString("\n")
	b.WriteString(m.renderModeSelector())
	b.WriteString("\n\n")
	b.WriteString(m.renderStageFlow())
	b.WriteString("\n\n")
	b.WriteString(m.renderMainContent())
	b.WriteString("\n\n")
	b.WriteString(m.renderFooter())

	return b.String()
}

func (m Model) renderHeader() string {
	title := headerStyle.Render("⎈ kupgrade watch")
	context := contextStyle.Render(m.contextName)

	version := m.serverVersion
	if m.targetVersion != "" && m.targetVersion != m.serverVersion {
		version = fmt.Sprintf("%s→%s", m.serverVersion, m.targetVersion)
	}
	versionDisplay := versionStyle.Render(version)

	progress := m.renderProgressBar(10)
	percent := fmt.Sprintf("%d%%", m.progressPercent())

	timeDisplay := timeStyle.Render(m.currentTime.Format("15:04:05"))
	eventCount := fmt.Sprintf("Events: %d", m.eventCount)

	return fmt.Sprintf("%s  %s | %s | %s %s | %s | %s",
		title, context, versionDisplay, progress, percent, eventCount, timeDisplay)
}

func (m Model) renderProgressBar(width int) string {
	percent := m.progressPercent()
	filled := (percent * width) / 100
	empty := width - filled

	bar := strings.Repeat(progressBarFull, filled) + strings.Repeat(progressBarEmpty, empty)
	return progressStyle.Render(bar)
}

func (m Model) renderModeSelector() string {
	modes := []string{"DRAIN", "CORDON", "SCHEDULE"}
	var parts []string

	for i, mode := range modes {
		key := fmt.Sprintf("[%d]", i+1)
		if DrainMode(i) == m.drainMode {
			parts = append(parts, footerKeyStyle.Render(key+" "+mode))
		} else {
			parts = append(parts, footerDescStyle.Render(key+" "+mode))
		}
	}

	return "undrainableNodeBehavior: " + strings.Join(parts, "  ")
}

func (m Model) renderStageFlow() string {
	stages := types.AllStages()
	var headers []string
	var counts []string

	for i, stage := range stages {
		name := string(stage)
		count := len(m.nodesByStage[stage])

		var header string
		if i == m.selectedStage {
			header = stageStyleSelected(name).Render(name)
		} else {
			header = stageStyle(name).Render(name)
		}
		headers = append(headers, centerText(header, 12))
		counts = append(counts, centerText(fmt.Sprintf("%d", count), 12))
	}

	var headerRow, countRow strings.Builder

	for i := range stages {
		if i > 0 {
			headerRow.WriteString("  " + stageArrow + "  ")
			countRow.WriteString("       ")
		}
		headerRow.WriteString(headers[i])
		countRow.WriteString(counts[i])
	}

	return headerRow.String() + "\n" + countRow.String()
}

func (m Model) renderMainContent() string {
	leftPanel := m.renderNodeCards()
	rightPanel := m.renderRightPanel()

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, "  ", rightPanel)
}

func (m Model) renderNodeCards() string {
	stages := types.AllStages()
	columns := make([]string, len(stages))

	for i, stage := range stages {
		nodes := m.nodesByStage[stage]
		var cards []string

		if len(nodes) == 0 {
			cards = append(cards, m.renderEmptyStage())
		} else {
			for j, nodeName := range nodes {
				node := m.nodes[nodeName]
				isSelected := i == m.selectedStage && j == m.selectedNode
				cards = append(cards, m.renderNodeCard(node, isSelected))
			}
		}

		columns[i] = lipgloss.JoinVertical(lipgloss.Left, cards...)
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

func (m Model) renderEmptyStage() string {
	content := nodePodStyle.Render("(empty)")
	return nodeCardNormal.Render(content)
}

func (m Model) renderNodeCard(node types.NodeState, selected bool) string {
	var b strings.Builder

	name := node.Name
	if len(name) > 16 {
		name = name[len(name)-16:]
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
		style = nodeCardSelected
	case node.Blocked:
		style = nodeCardBlocked
	case node.Stage == types.StageComplete:
		style = nodeCardComplete
	default:
		style = nodeCardNormal
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

func (m Model) renderRightPanel() string {
	var panels []string

	if len(m.blockers) > 0 {
		panels = append(panels, m.renderBlockersPanel())
	}

	panels = append(panels, m.renderMigrationsPanel())
	panels = append(panels, m.renderEventsPanel())

	return lipgloss.JoinVertical(lipgloss.Left, panels...)
}

func (m Model) renderBlockersPanel() string {
	title := panelTitleError.Render(fmt.Sprintf("⚠ BLOCKERS (%d)", len(m.blockers)))
	var lines []string
	lines = append(lines, title)

	for _, blocker := range m.blockers {
		line := fmt.Sprintf("%s: %s", blocker.Type, blocker.Name)
		if blocker.Detail != "" {
			line += " - " + blocker.Detail
		}
		lines = append(lines, errorStyle.Render(line))
	}

	content := strings.Join(lines, "\n")
	return panelStyle.Width(40).Render(content)
}

func (m Model) renderMigrationsPanel() string {
	title := panelTitleStyle.Render("↹ MIGRATIONS")
	var lines []string
	lines = append(lines, title)

	if len(m.migrations) == 0 {
		lines = append(lines, footerDescStyle.Render("No migrations yet"))
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
	return panelStyle.Width(40).Render(content)
}

func (m Model) renderEventsPanel() string {
	title := panelTitleStyle.Render("• EVENTS")
	var lines []string
	lines = append(lines, title)

	if len(m.events) == 0 {
		lines = append(lines, footerDescStyle.Render("Waiting for events..."))
	} else {
		for _, e := range m.events {
			ts := timestampStyle.Render(e.Timestamp.Format("15:04:05"))
			icon := m.severityIcon(e.Severity)
			msg := e.Message
			if len(msg) > 35 {
				msg = msg[:35] + "..."
			}
			lines = append(lines, fmt.Sprintf("%s %s %s", ts, icon, msg))
		}
	}

	content := strings.Join(lines, "\n")
	return panelStyle.Width(40).Render(content)
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
	keys := []struct {
		key  string
		desc string
	}{
		{"←→/hl", "stages"},
		{"↑↓/jk", "nodes"},
		{"enter", "details"},
		{"?", "help"},
		{"q", "quit"},
	}

	var parts []string
	for _, k := range keys {
		parts = append(parts, footerKeyStyle.Render(k.key)+" "+footerDescStyle.Render(k.desc))
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
		footerKeyStyle.Render("1/2/3") + "   Switch drain mode",
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
