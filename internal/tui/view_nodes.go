package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/sabirmohamed/kupgrade/pkg/types"
)

// renderNodesScreen renders the full node details screen
func (m Model) renderNodesScreen() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")

	if len(m.nodes) == 0 {
		b.WriteString(footerDescStyle.Render("  No nodes discovered"))
	} else {
		b.WriteString(m.renderNodesTable())
	}

	b.WriteString("\n")
	b.WriteString(m.renderFooter())

	return m.placeContent(b.String())
}

// renderNodesTable renders the nodes table using lipgloss/table with per-cell coloring
func (m Model) renderNodesTable() string {
	nodes := m.sortedNodeNames()

	rows := make([][]string, len(nodes))
	for i, name := range nodes {
		node := m.nodes[name]

		conditions := "Ready"
		if !node.Ready {
			conditions = "NotReady"
		} else if len(node.Conditions) > 0 {
			conditions = strings.Join(node.Conditions, ",")
		}

		taints := "-"
		if len(node.Taints) > 0 {
			taints = strings.Join(node.Taints, ",")
		} else if !node.Schedulable {
			taints = "NoSchedule"
		}

		age := node.Age
		if age == "" {
			age = "-"
		}

		rows[i] = []string{
			name,
			node.Version,
			string(node.Stage),
			age,
			conditions,
			taints,
		}
	}

	visibleRows := m.height - 10
	if visibleRows < 5 {
		visibleRows = 5
	}
	scrollOffset := calcScrollOffset(m.listIndex, visibleRows, len(nodes))

	tableWidth := m.width - 2
	if tableWidth < 80 {
		tableWidth = 80
	}

	t := table.New().
		Headers("NAME", "VERSION", "STAGE", "AGE", "CONDITIONS", "TAINTS").
		Rows(rows...).
		Width(tableWidth).
		Height(visibleRows).
		Offset(scrollOffset).
		Border(lipgloss.RoundedBorder()).
		BorderColumn(false).
		BorderRow(false).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderHeader(true).
		BorderStyle(tableBorderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			style := lipgloss.NewStyle().Padding(0, 1)
			if row == table.HeaderRow {
				style = style.Foreground(colorTextMuted).Bold(true)
				if col == 3 { // AGE
					style = style.Align(lipgloss.Right)
				}
				return style
			}

			actualIdx := row

			// Alternating row backgrounds
			if actualIdx%2 == 0 {
				style = style.Background(colorBg)
			} else {
				style = style.Background(colorBgAlt)
			}

			// Selected row highlight
			if actualIdx == m.listIndex {
				style = style.Background(colorSelected).Foreground(colorTextBold)
			}

			// Right-align AGE column
			if col == 3 {
				style = style.Align(lipgloss.Right)
			}

			if actualIdx >= len(nodes) {
				return style
			}

			// Color STAGE column
			if col == 2 {
				node := m.nodes[nodes[actualIdx]]
				if sc, ok := stageColors[string(node.Stage)]; ok {
					style = style.Foreground(sc)
				}
			}

			// Color CONDITIONS column
			if col == 4 {
				node := m.nodes[nodes[actualIdx]]
				if !node.Ready {
					style = style.Foreground(colorError)
				}
			}

			return style
		})

	rendered := t.String()

	// Show scroll indicator
	if len(nodes) > visibleRows {
		pos := fmt.Sprintf(" %d/%d", m.listIndex+1, len(nodes))
		rendered += "\n" + footerDescStyle.Render(pos)
	}

	return rendered
}

// renderNodeColumns renders old column-based layout (kept for overview kanban)
func (m Model) renderNodeColumns() string {
	stages := types.AllStages()
	columns := make([]string, len(stages))
	cardWidth := m.nodeCardWidth()

	for i, stage := range stages {
		name := string(stage)
		count := len(m.nodesByStage[stage])

		var header string
		if i == m.selectedStage {
			header = stageStyleSelected(name).Render(name)
		} else {
			header = stageStyle(name).Render(name)
		}

		var columnParts []string
		columnParts = append(columnParts, centerText(header, cardWidth))
		columnParts = append(columnParts, centerText(fmt.Sprintf("%d", count), cardWidth))
		columnParts = append(columnParts, "")

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

// renderEmptyStage renders placeholder for empty stage column
func (m Model) renderEmptyStage(cardWidth int) string {
	content := nodePodStyle.Render("(empty)")
	return nodeCardNormal.Width(cardWidth).Render(content)
}

// renderNodeCard renders a single node card
func (m Model) renderNodeCard(node types.NodeState, selected bool, cardWidth int) string {
	var b strings.Builder

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
		b.WriteString(m.spinner.View() + " reimaging...")
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
