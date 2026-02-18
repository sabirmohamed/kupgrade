package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/sabirmohamed/kupgrade/pkg/types"
)

// renderDrainsScreen renders the drain + blockers screen with sections
func (m Model) renderDrainsScreen() string {
	var b strings.Builder
	w := m.mainWidth()

	// Tab bar
	counts := m.stageCounts()
	tabBar := m.renderTabBar(counts)

	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")

	drainNodes := m.getDrainNodes()
	blockers := m.activeBlockers()

	// ── ACTIVE DRAINS ──────────────────────────────────────
	b.WriteString(renderSectionHeader("ACTIVE DRAINS", w))
	b.WriteString("\n")

	if len(drainNodes) == 0 {
		b.WriteString(footerDescStyle.Render("  No nodes currently draining or cordoned"))
		b.WriteString("\n")
	} else {
		for _, nodeName := range drainNodes {
			node := m.nodes[nodeName]
			b.WriteString(m.renderDrainLine(node))
			b.WriteString("\n")
		}
	}

	// ── BLOCKERS ──────────────────────────────────────────
	if len(blockers) > 0 {
		b.WriteString("\n")
		b.WriteString(renderSectionHeader(fmt.Sprintf("BLOCKERS (%d)", len(blockers)), w))
		b.WriteString("\n")

		for _, blocker := range blockers {
			b.WriteString(m.renderBlockerEntry(blocker, true))
			b.WriteString("\n")
		}
	}

	// ── RESCHEDULED ───────────────────────────────────────
	if len(m.migrations) > 0 {
		b.WriteString("\n")
		b.WriteString(m.renderMigrationsSection())
	}

	panelBody := b.String()

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

// renderDrainLine renders a single drain node as a section line
func (m Model) renderDrainLine(node types.NodeState) string {
	pill := renderStagePillInline(string(node.Stage))
	name := truncateString(node.Name, 40)

	// Progress info
	progressInfo := ""
	if node.Stage == types.StageDraining && node.InitialPodCount > 0 {
		evicted := node.InitialPodCount - node.EvictablePodCount
		if evicted < 0 {
			evicted = 0
		}
		progressInfo = fmt.Sprintf("  %d/%d evicted", evicted, node.InitialPodCount)
	} else if node.Stage == types.StageReimaging {
		progressInfo = "  rebooting..."
	} else if node.Stage == types.StageCordoned {
		progressInfo = "  waiting"
	}

	// Inline blocker
	blockerInfo := ""
	if node.Blocked && node.BlockerReason != "" {
		reason := truncateString(node.BlockerReason, 25)
		blockerInfo = "  " + errorStyle.Render("⚠ "+reason)
		if !node.DrainStartTime.IsZero() {
			dur := m.currentTime.Sub(node.DrainStartTime)
			blockerInfo += footerDescStyle.Render(fmt.Sprintf(" (%s)", formatDuration(dur)))
		}
	}

	return fmt.Sprintf("  %s  %s%s%s", pill, name, footerDescStyle.Render(progressInfo), blockerInfo)
}

// renderBlockerEntry renders a blocker line for the drains screen
func (m Model) renderBlockerEntry(blocker types.Blocker, isActive bool) string {
	name := blocker.Name
	if blocker.Namespace != "" {
		name = blocker.Namespace + "/" + blocker.Name
	}

	var style func(strs ...string) string
	if isActive {
		style = errorStyle.Render
	} else {
		style = warningStyle.Render
	}

	label := fmt.Sprintf("  %s  %s", blocker.Type, style(name))

	constraint := blocker.Detail
	if constraint == "" {
		constraint = "disruption budget exhausted"
	}

	nodeInfo := ""
	if blocker.NodeName != "" {
		nodeInfo = fmt.Sprintf("  blocking %s", blocker.NodeName)
	}

	durationInfo := ""
	if !blocker.StartTime.IsZero() {
		dur := m.currentTime.Sub(blocker.StartTime)
		durationInfo = "  " + style(formatDuration(dur))
	}

	return fmt.Sprintf("%s  %s%s%s", label, footerDescStyle.Render(constraint), nodeInfo, durationInfo)
}

// migrationTableMaxRows limits the migrations table height
const migrationTableMaxRows = 10

// renderMigrationsSection shows recent pod migrations during drains
func (m Model) renderMigrationsSection() string {
	var b strings.Builder
	w := m.mainWidth()

	b.WriteString(renderSectionHeader(fmt.Sprintf("RESCHEDULED (%d)", len(m.migrations)), w))
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

	tableWidth := w - 2
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
			s := lipgloss.NewStyle().Padding(0, 1)
			if row == table.HeaderRow {
				return s.Foreground(colorTextMuted).Bold(true)
			}
			switch col {
			case 0:
				s = s.Foreground(colorTextDim)
			case 1:
				if row < len(rows) && rows[row][1] == checkIcon {
					s = s.Foreground(colorComplete)
				} else {
					s = s.Foreground(colorCyan)
				}
			case 2:
				s = s.Foreground(colorText)
			case 3:
				s = s.Foreground(colorTextMuted)
			}
			return s
		})

	b.WriteString(t.String())

	if len(m.migrations) > migrationTableMaxRows {
		b.WriteString("\n")
		b.WriteString(footerDescStyle.Render(fmt.Sprintf("  ... %d more", len(m.migrations)-migrationTableMaxRows)))
	}

	return b.String()
}

// shortenNodeName truncates long node names while preserving the meaningful parts.
// For AKS names like "aks-agentpool-55576254-vmss000000", keeps prefix and vmss suffix.
func shortenNodeName(name string) string {
	const maxNodeNameLen = 40
	if len(name) <= maxNodeNameLen {
		return name
	}
	// Try to keep prefix (pool name) + suffix (vmss ID)
	// Find last dash-group that starts with "vmss" or similar
	parts := strings.Split(name, "-")
	if len(parts) >= 3 {
		// Keep first 2 parts + last 1 part, shorten middle
		prefix := parts[0] + "-" + parts[1]
		suffix := parts[len(parts)-1]
		shortened := prefix + "-...-" + suffix
		if len(shortened) <= maxNodeNameLen {
			return shortened
		}
	}
	return "..." + name[len(name)-maxNodeNameLen+3:]
}
