package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/sabirmohamed/kupgrade/pkg/types"
)

// renderDrainsScreen renders the drain progress screen
func (m Model) renderDrainsScreen() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")

	drainNodes := m.getDrainNodes()

	if len(drainNodes) == 0 {
		b.WriteString(footerDescStyle.Render("  No nodes currently draining or cordoned\n"))
		b.WriteString(footerDescStyle.Render("  Nodes will appear here when cordoned for upgrade"))
	} else {
		b.WriteString(m.renderDrainsTable(drainNodes))
	}

	b.WriteString("\n")
	b.WriteString(m.renderFooter())

	return m.placeContent(b.String())
}

// renderDrainsTable renders the drains table using lipgloss/table with per-cell coloring
func (m Model) renderDrainsTable(drainNodes []string) string {
	rows := make([][]string, len(drainNodes))
	for i, name := range drainNodes {
		node := m.nodes[name]
		rows[i] = buildDrainRow(node)
	}

	visibleRows := m.height - 10
	if visibleRows < 5 {
		visibleRows = 5
	}
	scrollOffset := calcScrollOffset(m.listIndex, visibleRows, len(drainNodes))

	tableWidth := m.width - 2
	if tableWidth < 80 {
		tableWidth = 80
	}

	t := table.New().
		Headers("NODE", "STAGE", "PROGRESS", "PODS", "STATUS").
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
				if col == 3 { // PODS
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

			// Right-align PODS column
			if col == 3 {
				style = style.Align(lipgloss.Right)
			}

			if actualIdx >= len(drainNodes) {
				return style
			}
			node := m.nodes[drainNodes[actualIdx]]

			switch col {
			case 1: // STAGE
				if sc, ok := stageColors[string(node.Stage)]; ok {
					style = style.Foreground(sc)
				}
			case 4: // STATUS
				if node.Blocked {
					style = style.Foreground(colorError) // red for blocked
				} else if node.Stage == types.StageDraining {
					style = style.Foreground(colorCordoned) // yellow for evicting
				} else {
					style = style.Foreground(colorTextMuted)
				}
			}

			return style
		})

	rendered := t.String()

	if len(drainNodes) > visibleRows {
		pos := fmt.Sprintf(" %d/%d", m.listIndex+1, len(drainNodes))
		rendered += "\n" + footerDescStyle.Render(pos)
	}

	return rendered
}

// buildDrainRow builds a table row for a drain node (plain text, coloring via StyleFunc)
func buildDrainRow(node types.NodeState) []string {
	progressBar := plainProgressBar(node.DrainProgress, 10)

	var status string
	if node.Blocked {
		status = node.BlockerReason
		if status == "" {
			status = "blocked"
		}
		status = truncateString(status, 30)
	} else if node.Stage == types.StageDraining {
		status = "evicting..."
	} else if node.Stage == types.StageCordoned {
		status = "waiting"
	} else {
		status = "-"
	}

	return []string{
		truncateString(node.Name, 40),
		string(node.Stage),
		progressBar,
		fmt.Sprintf("%d", node.PodCount),
		status,
	}
}

// plainProgressBar renders a text-only progress bar safe for use in table cells
func plainProgressBar(percent, width int) string {
	filled := (percent * width) / 100
	if filled > width {
		filled = width
	}
	empty := width - filled
	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	return fmt.Sprintf("%s %3d%%", bar, percent)
}
