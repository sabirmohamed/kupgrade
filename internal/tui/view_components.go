package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sabirmohamed/kupgrade/pkg/types"
	"github.com/sahilm/fuzzy"
)

// renderCompactHeader renders compact header for overview screen
func (m Model) renderCompactHeader() string {
	title := "★ kupgrade"
	titleDisplay := headerStyle.Render(title)

	context := contextStyle.Render(m.contextName())

	versionDisplay := m.renderVersionDisplay()

	// Progress bar with percentage
	progress := m.progress.ViewAs(float64(m.progressPercent()) / 100.0)
	percent := fmt.Sprintf("%3d%%", m.progressPercent())

	timeDisplay := timeStyle.Render(m.currentTime.Format("15:04:05"))

	return fmt.Sprintf("%s  %s | %s  %s %s | %s",
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

	versionDisplay := m.renderVersionDisplay()

	progress := m.progress.ViewAs(float64(m.progressPercent()) / 100.0)
	percent := fmt.Sprintf("%d%%", m.progressPercent())

	timeDisplay := timeStyle.Render(m.currentTime.Format("15:04:05"))

	return fmt.Sprintf("%s  %s | %s | %s %s | %s",
		titleDisplay, context, versionDisplay, progress, percent, timeDisplay)
}

// renderVersionDisplay renders the version indicator for headers.
// During upgrade (mixed versions): "v1.32.9 → v1.33.5" in warning color.
// After complete (all same version): "v1.33.5" in success color.
func (m Model) renderVersionDisplay() string {
	current := m.currentVersion()
	target := m.targetVersion()

	if current != "" && target != "" && versionCore(current) != versionCore(target) {
		// Upgrade in progress: show from → to
		return versionStyle.Render(fmt.Sprintf("%s → %s", current, target))
	}
	// All nodes at same version (or no upgrade detected)
	version := target
	if version == "" {
		version = current
	}
	if m.progressPercent() == 100 && m.totalNodes() > 0 {
		return versionCompleteStyle.Render(version)
	}
	return versionStyle.Render(version)
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

// getFilteredPodList returns pods filtered by the current pod filter mode.
// When no upgrade is active (no CORDONED/DRAINING/UPGRADING nodes), shows all pods.
func (m *Model) getFilteredPodList() []types.PodState {
	affectedNodes := make(map[string]bool)
	for _, name := range m.nodesByStage[types.StageCordoned] {
		affectedNodes[name] = true
	}
	for _, name := range m.nodesByStage[types.StageDraining] {
		affectedNodes[name] = true
	}
	for _, name := range m.nodesByStage[types.StageUpgrading] {
		affectedNodes[name] = true
	}

	settledNodes := make(map[string]bool)
	for _, name := range m.nodesByStage[types.StageComplete] {
		settledNodes[name] = true
	}

	upgradeActive := len(affectedNodes) > 0

	var podList []types.PodState
	for _, pod := range m.pods {
		if !upgradeActive {
			// No upgrade in progress: show all pods regardless of filter mode
			podList = append(podList, pod)
			continue
		}

		switch m.podFilterMode {
		case PodFilterDisrupting:
			if affectedNodes[pod.NodeName] {
				podList = append(podList, pod)
			}
		case PodFilterRescheduled:
			if settledNodes[pod.NodeName] {
				podList = append(podList, pod)
			}
		case PodFilterAll:
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
	drainNodes = append(drainNodes, m.nodesByStage[types.StageUpgrading]...)
	sort.Strings(drainNodes)
	return drainNodes
}

// getDisplayPodList returns pods after both stage filter and fuzzy search
func (m *Model) getDisplayPodList() []types.PodState {
	podList := m.getFilteredPodList()
	query := m.podSearchInput.Value()
	if query == "" {
		return podList
	}
	return fuzzyFilterPods(podList, query)
}

// fuzzyFilterPods filters pods using fuzzy matching against name, namespace, node, and status
func fuzzyFilterPods(pods []types.PodState, query string) []types.PodState {
	source := make(podSearchSource, len(pods))
	for i, pod := range pods {
		source[i] = pod.Namespace + "/" + pod.Name + " " + pod.NodeName + " " + pod.Phase
	}

	matches := fuzzy.FindFrom(query, source)
	result := make([]types.PodState, len(matches))
	for i, match := range matches {
		result[i] = pods[match.Index]
	}
	return result
}

// podSearchSource implements fuzzy.Source for pod searching
type podSearchSource []string

func (s podSearchSource) String(i int) string { return s[i] }
func (s podSearchSource) Len() int            { return len(s) }

// clearPodSearch resets the pod search state
func (m *Model) clearPodSearch() {
	m.podSearchActive = false
	m.podSearchInput.SetValue("")
	m.podSearchInput.Blur()
}
