package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
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

// buildNodeRow builds a single row for the nodes table
func (m Model) buildNodeRow(name string) []string {
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

	stageStr := string(node.Stage)
	if node.SurgeNode {
		stageStr += " SURGE"
	}

	return []string{name, node.Version, stageStr, age, conditions, taints}
}

// nodeTableStyleFunc returns the style function for the nodes table
func (m Model) nodeTableStyleFunc(nodes []string) func(row, col int) lipgloss.Style {
	return func(row, col int) lipgloss.Style {
		style := lipgloss.NewStyle().Padding(0, 1)

		// Header row
		if row == table.HeaderRow {
			style = style.Foreground(colorTextMuted).Bold(true)
			if col == 3 { // AGE
				style = style.Align(lipgloss.Right)
			}
			return style
		}

		// Alternating row backgrounds
		if row%2 == 0 {
			style = style.Background(colorBg)
		} else {
			style = style.Background(colorBgAlt)
		}

		// Selected row highlight
		if row == m.listIndex {
			style = style.Background(colorSelected).Foreground(colorTextBold)
		}

		// Right-align AGE column
		if col == 3 {
			style = style.Align(lipgloss.Right)
		}

		if row >= len(nodes) {
			return style
		}

		// Color STAGE column
		if col == 2 {
			if sc, ok := stageColors[string(m.nodes[nodes[row]].Stage)]; ok {
				style = style.Foreground(sc)
			}
		}

		// Color CONDITIONS column
		if col == 4 && !m.nodes[nodes[row]].Ready {
			style = style.Foreground(colorError)
		}

		return style
	}
}

// renderNodesTable renders the nodes table using lipgloss/table with per-cell coloring
func (m Model) renderNodesTable() string {
	nodes := m.sortedNodeNames()

	rows := make([][]string, len(nodes))
	for i, name := range nodes {
		rows[i] = m.buildNodeRow(name)
	}

	visibleRows := max(m.height-10, 5)
	scrollOffset := calcScrollOffset(m.listIndex, visibleRows, len(nodes))
	tableWidth := max(m.mainWidth()-2, 80)

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
		StyleFunc(m.nodeTableStyleFunc(nodes))

	rendered := t.String()

	// Show scroll indicator and hint
	if len(nodes) > visibleRows {
		rendered += "\n" + footerDescStyle.Render(fmt.Sprintf(" %d/%d  •  d describe", m.listIndex+1, len(nodes)))
	} else if len(nodes) > 0 {
		rendered += "\n" + footerDescStyle.Render(" d describe")
	}

	return rendered
}
