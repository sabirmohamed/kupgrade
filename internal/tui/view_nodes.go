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

	tableWidth := m.mainWidth() - 2
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

	// Show scroll indicator and hint
	if len(nodes) > visibleRows {
		pos := fmt.Sprintf(" %d/%d  •  d describe", m.listIndex+1, len(nodes))
		rendered += "\n" + footerDescStyle.Render(pos)
	} else if len(nodes) > 0 {
		rendered += "\n" + footerDescStyle.Render(" d describe")
	}

	return rendered
}
