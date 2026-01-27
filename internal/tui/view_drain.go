package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sabirmohamed/kupgrade/pkg/types"
)

// renderDrainsScreen renders the drain progress screen
func (m Model) renderDrainsScreen() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")

	// Collect nodes in drain pipeline
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

			// Show blocker detail for selected blocked node
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
