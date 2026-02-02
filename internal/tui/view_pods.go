package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/sabirmohamed/kupgrade/pkg/types"
)

// renderPodsScreen renders the pod list screen
func (m Model) renderPodsScreen() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")

	upgradeActive := len(m.nodesByStage[types.StageCordoned])+
		len(m.nodesByStage[types.StageDraining])+
		len(m.nodesByStage[types.StageUpgrading]) > 0

	stageFiltered := m.getFilteredPodList()
	totalFiltered := len(stageFiltered)

	var podList []types.PodState
	if query := m.podSearchInput.Value(); query != "" {
		podList = fuzzyFilterPods(stageFiltered, query)
	} else {
		podList = stageFiltered
	}

	if len(podList) == 0 {
		if m.podSearchInput.Value() != "" {
			b.WriteString(m.renderPodSearchBar(totalFiltered, len(podList)))
			b.WriteString("\n")
			b.WriteString(footerDescStyle.Render("  No matches"))
		} else if !upgradeActive {
			b.WriteString(footerDescStyle.Render("  No pods found"))
		} else {
			b.WriteString(footerDescStyle.Render(fmt.Sprintf("  No pods on %s", m.podFilterLabel())))
			b.WriteString("\n")
			b.WriteString(footerDescStyle.Render("  Press 'a' to cycle filter: disrupting → rescheduled → all"))
		}
	} else {
		total := len(podList)
		filterNote := ""
		if upgradeActive {
			filterNote = fmt.Sprintf(" (%s)", m.podFilterLabel())
		}
		toggleHint := ""
		if upgradeActive {
			toggleHint = "  " + footerDescStyle.Render("a cycle filter")
		}

		searchBar := ""
		if m.podSearchActive || m.podSearchInput.Value() != "" {
			searchBar = "  " + m.renderPodSearchBar(totalFiltered, total)
		}

		b.WriteString(fmt.Sprintf("  pods(%d)%s%s%s\n", total, filterNote, toggleHint, searchBar))
		b.WriteString(m.renderPodsTable(podList))
	}

	b.WriteString("\n")
	b.WriteString(m.renderFooter())

	return m.placeContent(b.String())
}

// nodeGroupIndex maps each pod to its node group for separator rendering.
// Returns a parallel slice of booleans: true if this pod is the first in a new node group.
func nodeGroupStarts(podList []types.PodState) []bool {
	starts := make([]bool, len(podList))
	for i, pod := range podList {
		if i == 0 || pod.NodeName != podList[i-1].NodeName {
			starts[i] = true
		}
	}
	return starts
}

// renderPodsTable renders the pods table using lipgloss/table with per-cell coloring
func (m Model) renderPodsTable(podList []types.PodState) string {
	// Skip node group separators when fuzzy search is active (results are ranked by score)
	searchActive := m.podSearchInput.Value() != ""
	var groupStarts []bool
	if !searchActive {
		groupStarts = nodeGroupStarts(podList)
	}

	rows := make([][]string, len(podList))
	for i, pod := range podList {
		rows[i] = buildPodRow(pod)
	}

	visibleRows := m.height - 10
	if visibleRows < 5 {
		visibleRows = 5
	}
	// Cap table height to actual row count to prevent lipgloss stretching rows
	if len(podList) < visibleRows {
		visibleRows = len(podList)
	}
	scrollOffset := calcScrollOffset(m.listIndex, visibleRows, len(podList))

	tableWidth := m.mainWidth() - 2
	if tableWidth < 120 {
		tableWidth = 120
	}

	t := table.New().
		Headers("NAMESPACE", "NAME", "READY", "STATUS", "RESTARTS", "PROBE", "OWNER", "NODE", "AGE").
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
				// Right-align numeric header columns
				switch col {
				case 2, 4, 8: // READY, RESTARTS, AGE
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

			// Node group separator: top border on first pod of new node group
			// (only when not in fuzzy search mode)
			if groupStarts != nil && actualIdx < len(groupStarts) && groupStarts[actualIdx] && actualIdx > 0 {
				style = style.BorderTop(true).
					BorderStyle(lipgloss.NormalBorder()).
					BorderForeground(colorBorderDim)
			}

			// Selected row highlight
			if actualIdx == m.listIndex {
				style = style.Background(colorSelected).Foreground(colorTextBold)
			}

			// Right-align numeric columns
			switch col {
			case 2, 4, 8: // READY, RESTARTS, AGE
				style = style.Align(lipgloss.Right)
			}

			if actualIdx >= len(podList) {
				return style
			}
			pod := podList[actualIdx]

			// Per-cell coloring
			switch col {
			case 2: // READY
				style = style.Foreground(readyColor(pod))
			case 3: // STATUS
				style = style.Foreground(statusColor(pod.Phase))
			case 4: // RESTARTS
				style = style.Foreground(restartColor(pod.Restarts))
			case 5: // PROBE
				style = style.Foreground(probeColor(pod))
			case 6: // OWNER
				if pod.OwnerKind == "DaemonSet" {
					style = style.Foreground(colorYellow)
				}
			case 7: // NODE
				if pod.NodeName != "" {
					if _, exists := m.nodes[pod.NodeName]; !exists {
						style = style.Foreground(colorError)
					}
				}
			}

			return style
		})

	rendered := t.String()

	var hint string
	if m.podSearchActive {
		hint = " ↑↓ navigate  Enter commit  Esc cancel"
	} else if m.podSearchInput.Value() != "" {
		// Filter committed — normal nav mode
		if len(podList) > visibleRows {
			hint = fmt.Sprintf(" %d/%d  •  d describe  •  / search  Esc clear", m.listIndex+1, len(podList))
		} else {
			hint = " d describe  •  / search  Esc clear"
		}
	} else {
		if len(podList) > visibleRows {
			hint = fmt.Sprintf(" %d/%d  •  d describe  •  / search", m.listIndex+1, len(podList))
		} else {
			hint = " d describe  •  / search"
		}
	}
	if len(podList) > 0 {
		rendered += "\n" + footerDescStyle.Render(hint)
	}

	return rendered
}

// buildPodRow builds a table row for a pod (plain text, coloring via StyleFunc)
func buildPodRow(pod types.PodState) []string {
	namespace := truncateString(pod.Namespace, 15)
	name := truncateString(pod.Name, 55)

	readyStr := fmt.Sprintf("%d/%d", pod.ReadyContainers, pod.TotalContainers)
	status := truncateString(pod.Phase, 16)

	var restartStr string
	if pod.Restarts == 0 {
		restartStr = "0"
	} else if pod.LastRestartAge != "" {
		restartStr = fmt.Sprintf("%d %s", pod.Restarts, pod.LastRestartAge)
	} else {
		restartStr = fmt.Sprintf("%d", pod.Restarts)
	}

	var rProbe, lProbe string
	if pod.HasReadiness {
		if pod.ReadinessOK {
			rProbe = "R✓"
		} else {
			rProbe = "R✗"
		}
	} else {
		rProbe = "··"
	}
	if pod.HasLiveness {
		if pod.LivenessOK {
			lProbe = "L✓"
		} else {
			lProbe = "L✗"
		}
	} else {
		lProbe = "··"
	}
	probeStr := rProbe + lProbe

	owner := truncateString(pod.OwnerKind, 12)
	if owner == "" {
		owner = "<none>"
	}

	nodeName := truncateString(pod.NodeName, 40)
	if nodeName == "" {
		nodeName = "<pending>"
	}

	return []string{
		namespace,
		name,
		readyStr,
		status,
		restartStr,
		probeStr,
		owner,
		nodeName,
		pod.Age,
	}
}

// statusColor returns foreground color for pod phase
func statusColor(phase string) lipgloss.Color {
	switch phase {
	case "Running":
		return colorComplete // green
	case "Pending":
		return colorCordoned // yellow
	case "Completed", "Succeeded":
		return colorTextMuted
	case "CrashLoopBackOff", "Error", "Failed", "ImagePullBackOff", "ErrImagePull",
		"OOMKilled", "RunContainerError", "CreateContainerError":
		return colorError // red
	case "Terminating":
		return colorBrightYellow // orange
	case "Unknown":
		return colorBrightRed // bright red
	default:
		// Handle Init:* and PodInitializing prefixes
		if strings.HasPrefix(phase, "Init:") || phase == "PodInitializing" {
			return colorCyan // init state
		}
		return colorText
	}
}

// readyColor returns foreground color based on container readiness
func readyColor(pod types.PodState) lipgloss.Color {
	if pod.TotalContainers == 0 {
		return colorTextMuted
	}
	if pod.ReadyContainers == pod.TotalContainers {
		return colorComplete // green
	}
	if pod.ReadyContainers == 0 {
		return colorError // red
	}
	return colorCordoned // yellow - partial
}

// restartColor returns foreground color based on restart count
func restartColor(restarts int) lipgloss.Color {
	if restarts > 5 {
		return colorError // red
	}
	if restarts > 0 {
		return colorCordoned // yellow
	}
	return colorTextMuted
}

// probeColor returns foreground color based on probe status
func probeColor(pod types.PodState) lipgloss.Color {
	if !pod.HasReadiness && !pod.HasLiveness {
		return colorTextMuted
	}
	allOK := (!pod.HasReadiness || pod.ReadinessOK) && (!pod.HasLiveness || pod.LivenessOK)
	if allOK {
		return colorComplete // green
	}
	return colorError // red
}

// renderPodSearchBar renders the fuzzy search input bar
func (m Model) renderPodSearchBar(totalFiltered, matchCount int) string {
	if m.podSearchActive {
		return fmt.Sprintf("> %s  %d/%d",
			m.podSearchInput.View(), matchCount, totalFiltered)
	}
	// Filter is set but input is not focused
	return fmt.Sprintf("> %s  %d/%d",
		footerKeyStyle.Render(m.podSearchInput.Value()), matchCount, totalFiltered)
}
