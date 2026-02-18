package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// renderNodesScreen renders the full node details screen
func (m Model) renderNodesScreen() string {
	w := m.mainWidth()

	// Tab bar
	counts := m.stageCounts()
	tabBar := m.renderTabBar(counts)

	// Build body
	var bodyParts []string
	bodyParts = append(bodyParts, m.renderHeader())
	bodyParts = append(bodyParts, "")

	if len(m.nodes) == 0 {
		bodyParts = append(bodyParts, footerDescStyle.Render("  No nodes discovered"))
	} else {
		bodyParts = append(bodyParts, m.renderNodesTable())
	}

	panelBody := lipgloss.JoinVertical(lipgloss.Left, bodyParts...)

	// Wrap in outer panel
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorSelected).
		Padding(0, 1).
		Width(w - 2).
		Render(panelBody)

	// Status bar + key hints
	statusBar := m.renderStatusBar(w)
	keyHints := m.renderKeyHints(w)

	content := lipgloss.JoinVertical(lipgloss.Left, tabBar, panel, statusBar, keyHints)
	return m.placeContent(content)
}

// buildNodeRow builds a single row for the nodes table
func (m Model) buildNodeRow(name string) []string {
	node := m.nodes[name]

	pool := node.Pool
	if pool == "" {
		pool = "-"
	}

	conditions := "Ready"
	if !node.Ready {
		conditions = "NotReady"
	} else if len(node.Conditions) > 0 {
		conditions = strings.Join(node.Conditions, ",")
	}

	age := node.Age
	if age == "" {
		age = "-"
	}

	stageStr := string(node.Stage)
	if node.SurgeNode {
		stageStr += " SURGE"
	} else if node.Deleted {
		stageStr += " replaced"
	}

	return []string{name, pool, node.Version, stageStr, age, conditions}
}

// nodeTableStyleFunc returns the style function for the nodes table
func (m Model) nodeTableStyleFunc(nodes []string) func(row, col int) lipgloss.Style {
	return func(row, col int) lipgloss.Style {
		style := lipgloss.NewStyle().Padding(0, 1)

		// Header row
		if row == table.HeaderRow {
			style = style.Foreground(colorTextMuted).Bold(true)
			if col == 4 { // AGE
				style = style.Align(lipgloss.Right)
			}
			return style
		}

		// Selected row highlight
		if row == m.listIndex {
			style = style.Background(colorSelected).Foreground(colorTextBold)
		}

		// Right-align AGE column
		if col == 4 {
			style = style.Align(lipgloss.Right)
		}

		if row >= len(nodes) {
			return style
		}

		// Color POOL column in purple
		if col == 1 {
			style = style.Foreground(colorPurple)
		}

		// Color STAGE column — foreground-only to preserve row background
		if col == 3 {
			node := m.nodes[nodes[row]]
			stageName := string(node.Stage)
			if fg, ok := stageForegroundColors[stageName]; ok {
				style = style.Foreground(fg).Bold(true)
			}
		}

		// Color CONDITIONS column
		if col == 5 && !m.nodes[nodes[row]].Ready {
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
	tableWidth := max(m.mainWidth()-8, 80) // panel border (2) + padding (2) + breathing room (2)

	t := table.New().
		Headers("NAME", "POOL", "VERSION", "STAGE", "AGE", "CONDITIONS").
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

	// Footer with scroll position and blocker info for selected node
	var footerParts []string
	if len(nodes) > visibleRows {
		footerParts = append(footerParts, fmt.Sprintf("%d/%d", m.listIndex+1, len(nodes)))
	}
	footerParts = append(footerParts, "d describe")

	// Show blocker info for selected node
	if m.listIndex < len(nodes) {
		node := m.nodes[nodes[m.listIndex]]
		if node.Blocked && node.BlockerReason != "" {
			footerParts = append(footerParts, errorStyle.Render("⚠ "+truncateString(node.BlockerReason, 40)))
		}
	}

	rendered += "\n" + footerDescStyle.Render(" "+strings.Join(footerParts, "  •  "))

	return rendered
}
