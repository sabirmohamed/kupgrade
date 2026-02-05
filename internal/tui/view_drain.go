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

	// Migrations section (only if migrations exist)
	if len(m.migrations) > 0 {
		b.WriteString("\n")
		b.WriteString(m.renderMigrationsSection())
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

	tableWidth := m.mainWidth() - 2
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
			return m.drainCellStyle(row, col, drainNodes)
		})

	rendered := t.String()

	if len(drainNodes) > visibleRows {
		pos := fmt.Sprintf(" %d/%d  •  d describe", m.listIndex+1, len(drainNodes))
		rendered += "\n" + footerDescStyle.Render(pos)
	} else if len(drainNodes) > 0 {
		rendered += "\n" + footerDescStyle.Render(" d describe")
	}

	return rendered
}

// drainCellStyle returns the lipgloss style for a cell in the drains table.
// Extracted from renderDrainsTable to reduce cyclomatic complexity.
func (m Model) drainCellStyle(row, col int, drainNodes []string) lipgloss.Style {
	style := lipgloss.NewStyle().Padding(0, 1)

	// Header row styling
	if row == table.HeaderRow {
		style = style.Foreground(colorTextMuted).Bold(true)
		if col == 3 { // PODS
			style = style.Align(lipgloss.Right)
		}
		return style
	}

	// Row background (alternating)
	style = m.drainRowBackground(style, row)

	// Right-align PODS column
	if col == 3 {
		style = style.Align(lipgloss.Right)
	}

	// Bounds check before accessing node data
	if row >= len(drainNodes) {
		return style
	}

	// Per-column coloring based on node state
	node := m.nodes[drainNodes[row]]
	return m.drainColumnColor(style, col, node)
}

// drainRowBackground applies alternating background and selected row highlight.
func (m Model) drainRowBackground(style lipgloss.Style, row int) lipgloss.Style {
	if row%2 == 0 {
		style = style.Background(colorBg)
	} else {
		style = style.Background(colorBgAlt)
	}

	// Selected row highlight overrides alternating background
	if row == m.listIndex {
		style = style.Background(colorSelected).Foreground(colorTextBold)
	}

	return style
}

// drainColumnColor applies per-column foreground colors based on node state.
func (m Model) drainColumnColor(style lipgloss.Style, col int, node types.NodeState) lipgloss.Style {
	switch col {
	case 1: // STAGE
		if stageColor, ok := stageColors[string(node.Stage)]; ok {
			style = style.Foreground(stageColor)
		}
	case 4: // STATUS
		style = m.drainStatusColor(style, node)
	}
	return style
}

// drainStatusColor returns the style for the STATUS column based on node state.
func (m Model) drainStatusColor(style lipgloss.Style, node types.NodeState) lipgloss.Style {
	if node.Blocked {
		return style.Foreground(colorError) // red for blocked
	}
	if node.Stage == types.StageDraining {
		return style.Foreground(colorCordoned) // yellow for evicting
	}
	return style.Foreground(colorTextMuted)
}

// buildDrainRow builds a table row for a drain node (plain text, coloring via StyleFunc)
func buildDrainRow(node types.NodeState) []string {
	progressBar := plainProgressBar(node.DrainProgress, 10)

	// REIMAGING nodes have completed drain — show 100% progress
	if node.Stage == types.StageReimaging {
		progressBar = plainProgressBar(100, 10)
	}

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
	} else if node.Stage == types.StageReimaging {
		status = "rebooting..."
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

// migrationTableMaxRows limits the migrations table height
const migrationTableMaxRows = 10

// renderMigrationsSection shows recent pod migrations during drains
func (m Model) renderMigrationsSection() string {
	var b strings.Builder

	title := panelTitleStyle.Render(fmt.Sprintf("%s RESCHEDULED (%d)", migrateIcon, len(m.migrations)))
	b.WriteString(title)
	b.WriteString("\n")

	if len(m.migrations) == 0 {
		b.WriteString(footerDescStyle.Render("  No pod moves yet"))
		return b.String()
	}

	// Build rows from most recent migrations (newest first)
	maxDisplay := migrationTableMaxRows
	if len(m.migrations) < maxDisplay {
		maxDisplay = len(m.migrations)
	}

	rows := make([][]string, 0, maxDisplay)
	for i := len(m.migrations) - 1; i >= len(m.migrations)-maxDisplay; i-- {
		mig := m.migrations[i]
		status := migrateIcon
		if mig.Complete {
			status = checkIcon
		}
		podName := mig.Namespace + "/" + mig.NewPod
		nodeName := shortenNodeName(mig.ToNode)
		rows = append(rows, []string{
			mig.Timestamp.Format("15:04:05"),
			status,
			podName,
			nodeName,
		})
	}

	tableWidth := m.mainWidth() - 2
	if tableWidth < 60 {
		tableWidth = 60
	}

	t := table.New().
		Headers("TIME", "", "POD", "DESTINATION").
		Rows(rows...).
		Width(tableWidth).
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
				return style.Foreground(colorTextMuted).Bold(true)
			}
			// Alternating row backgrounds
			if row%2 == 0 {
				style = style.Background(colorBg)
			} else {
				style = style.Background(colorBgAlt)
			}
			switch col {
			case 0: // TIME
				style = style.Foreground(colorTextDim)
			case 1: // STATUS icon
				if row < len(rows) && rows[row][1] == checkIcon {
					style = style.Foreground(colorComplete)
				} else {
					style = style.Foreground(colorCyan)
				}
			case 2: // POD
				style = style.Foreground(colorText)
			case 3: // DESTINATION
				style = style.Foreground(colorTextMuted)
			}
			return style
		})

	b.WriteString(t.String())

	if len(m.migrations) > migrationTableMaxRows {
		b.WriteString("\n")
		b.WriteString(footerDescStyle.Render(fmt.Sprintf("  ... %d more", len(m.migrations)-migrationTableMaxRows)))
	}

	return b.String()
}

// shortenNodeName truncates long node names to show the meaningful suffix.
// e.g., "gke-testbed-gke-default-pool-fa2ce801-l2k0" → "...fa2ce801-l2k0"
func shortenNodeName(name string) string {
	const maxNodeNameLen = 24
	if len(name) <= maxNodeNameLen {
		return name
	}
	return "..." + name[len(name)-maxNodeNameLen+3:]
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
