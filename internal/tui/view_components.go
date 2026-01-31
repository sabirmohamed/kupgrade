package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sabirmohamed/kupgrade/pkg/types"
)

// renderCompactHeader renders compact header for overview screen
func (m Model) renderCompactHeader() string {
	title := "★ kupgrade"
	titleDisplay := headerStyle.Render(title)

	context := contextStyle.Render(m.contextName())

	version := m.serverVersion()
	if m.targetVersion() != "" && m.targetVersion() != m.serverVersion() {
		version = fmt.Sprintf("%s → %s", m.serverVersion(), m.targetVersion())
	}
	versionDisplay := versionStyle.Render(version)

	// Progress bar with percentage
	progress := m.progress.ViewAs(float64(m.progressPercent()) / 100.0)
	percent := fmt.Sprintf("%3d%%", m.progressPercent())

	timeDisplay := timeStyle.Render(m.currentTime.Format("15:04:05"))

	return fmt.Sprintf("%s  %s  %s  %s %s  %s",
		titleDisplay, context, versionDisplay, progress, percent, timeDisplay)
}

// renderHeader renders header with screen name for sub-screens
func (m Model) renderHeader() string {
	title := "⎈ kupgrade"
	if screenName := m.screenName(); screenName != "" {
		title = fmt.Sprintf("⎈ kupgrade › %s", screenName)
	}
	titleDisplay := headerStyle.Render(title)

	context := contextStyle.Render(m.contextName())

	version := m.serverVersion()
	if m.targetVersion() != "" && m.targetVersion() != m.serverVersion() {
		version = fmt.Sprintf("%s→%s", m.serverVersion(), m.targetVersion())
	}
	versionDisplay := versionStyle.Render(version)

	progress := m.progress.ViewAs(float64(m.progressPercent()) / 100.0)
	percent := fmt.Sprintf("%d%%", m.progressPercent())

	timeDisplay := timeStyle.Render(m.currentTime.Format("15:04:05"))

	return fmt.Sprintf("%s  %s | %s | %s %s | %s",
		titleDisplay, context, versionDisplay, progress, percent, timeDisplay)
}

// renderPipelineRow renders compact stage counts with arrows
func (m Model) renderPipelineRow() string {
	stages := types.AllStages()
	var parts []string

	for i, stage := range stages {
		count := len(m.nodesByStage[stage])
		name := string(stage)

		var stageStr string
		if i == m.selectedStage {
			stageStr = stageStyleSelected(name).Render(name)
		} else {
			stageStr = stageStyle(name).Render(name)
		}

		countStr := fmt.Sprintf("%d", count)
		if count > 0 {
			countStr = stageStyle(name).Render(countStr)
		} else {
			countStr = footerDescStyle.Render(countStr)
		}

		parts = append(parts, fmt.Sprintf("%s\n%s", centerText(stageStr, 12), centerText(countStr, 12)))

		if i < len(stages)-1 {
			parts = append(parts, footerDescStyle.Render("  —  "))
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Center, parts...)
}

// renderFooter renders screen navigation footer with screen hints + key help
func (m Model) renderFooter() string {
	screenHints := []struct {
		key  string
		desc string
	}{
		{"0", "overview"},
		{"1", "nodes"},
		{"2", "drains"},
		{"3", "pods"},
		{"4", "blockers"},
		{"5", "events"},
	}

	var parts []string
	for _, h := range screenHints {
		parts = append(parts, footerKeyStyle.Render(h.key)+" "+footerDescStyle.Render(h.desc))
	}

	// Append key help from bubbles
	parts = append(parts,
		footerKeyStyle.Render("?")+" "+footerDescStyle.Render("help"),
		footerKeyStyle.Render("q")+" "+footerDescStyle.Render("quit"),
	)

	return footerStyle.Render(strings.Join(parts, "  "))
}

// severityIcon returns styled icon for event severity
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

// Helper functions

// sortedNodeNames returns all node names sorted alphabetically
func (m Model) sortedNodeNames() []string {
	names := make([]string, 0, len(m.nodes))
	for name := range m.nodes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// truncateString truncates string to maxLen with ellipsis
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// Layout calculation helpers

// mainWidth returns the width available for the main content area.
func (m Model) mainWidth() int {
	return m.width
}

// getFilteredPodList returns pods filtered to upgrading nodes (or all if none upgrading)
func (m *Model) getFilteredPodList() []types.PodState {
	upgradeNodes := make(map[string]bool)
	for _, name := range m.nodesByStage[types.StageCordoned] {
		upgradeNodes[name] = true
	}
	for _, name := range m.nodesByStage[types.StageDraining] {
		upgradeNodes[name] = true
	}
	for _, name := range m.nodesByStage[types.StageUpgrading] {
		upgradeNodes[name] = true
	}

	var podList []types.PodState
	showAll := len(upgradeNodes) == 0
	for _, pod := range m.pods {
		if showAll || upgradeNodes[pod.NodeName] {
			podList = append(podList, pod)
		}
	}

	sort.Slice(podList, func(i, j int) bool {
		if podList[i].NodeName != podList[j].NodeName {
			return podList[i].NodeName < podList[j].NodeName
		}
		if podList[i].Namespace != podList[j].Namespace {
			return podList[i].Namespace < podList[j].Namespace
		}
		return podList[i].Name < podList[j].Name
	})

	return podList
}

// getDrainNodes returns sorted list of nodes in drain pipeline
func (m *Model) getDrainNodes() []string {
	var drainNodes []string
	drainNodes = append(drainNodes, m.nodesByStage[types.StageCordoned]...)
	drainNodes = append(drainNodes, m.nodesByStage[types.StageDraining]...)
	sort.Strings(drainNodes)
	return drainNodes
}
