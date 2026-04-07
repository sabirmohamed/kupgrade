package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/sabirmohamed/kupgrade/pkg/types"
)

// renderHeader renders header with screen name for sub-screens
// Format: ⎈ kupgrade · Nodes  {cluster} | CP v1.33.6 ✓  Nodes v1.32.9 → v1.33.6  [████░░] 45%    ▸ 23m 41s
func (m Model) renderHeader() string {
	title := "⎈ kupgrade"
	if screenName := m.screenName(); screenName != "" {
		title = fmt.Sprintf("⎈ kupgrade · %s", screenName)
	}
	titleDisplay := headerStyle.Render(title)

	context := contextStyle.Render(m.contextName())

	current := m.currentVersion()
	target := m.targetVersion()
	upgradeDetected := current != "" && target != "" && versionCore(current) != versionCore(target)

	versionPart := m.renderCPVersionDisplay(upgradeDetected)

	var left string
	if upgradeDetected {
		progress := m.progress.ViewAs(float64(m.progressPercent()) / 100.0)
		percent := fmt.Sprintf("%d%%", m.progressPercent())
		left = fmt.Sprintf("%s  %s | %s  %s %s", titleDisplay, context, versionPart, progress, percent)
	} else {
		left = fmt.Sprintf("%s  %s | %s", titleDisplay, context, versionPart)
	}

	// Right side: elapsed timer (only during upgrade)
	var right string
	if upgradeDetected {
		if elapsed := m.elapsedDisplay(); elapsed != "" {
			right = footerDescStyle.Render("▸ " + elapsed)
		}
	}

	if right != "" {
		spacing := m.mainWidth() - lipgloss.Width(left) - lipgloss.Width(right) - 2
		if spacing < 2 {
			spacing = 2
		}
		return left + strings.Repeat(" ", spacing) + right
	}

	return left
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

// renderStatusBar renders the segmented status bar.
// [STATUS] Upgrading ... [3/11] ● Live [⎈ kupgrade]
func (m Model) renderStatusBar(width int) string {
	// Determine state
	upgradeComplete := m.progressPercent() == 100 && m.totalNodes() > 0 && m.completedNodes() > 0
	current := m.currentVersion()
	target := m.targetVersion()
	versionMismatch := current != "" && target != "" && versionCore(current) != versionCore(target)
	upgradeActive := m.isUpgradeActive() || versionMismatch || m.isCPAhead()

	// STATUS badge
	var statusBadge, statusText string
	if upgradeComplete {
		statusBadge = sbStatusCompleteStyle.Render("STATUS")
		statusText = sbTextStyle.Render("Complete")
	} else if upgradeActive {
		statusBadge = sbStatusStyle.Render("STATUS")
		statusText = sbTextStyle.Render("Upgrading")
	} else {
		statusBadge = sbStatusWatchingStyle.Render("STATUS")
		statusText = sbTextStyle.Render("Watching")
	}

	// Right segments
	countBadge := sbCountStyle.Render(fmt.Sprintf("%d/%d", m.completedNodes(), m.totalNodes()))

	liveDot := lipgloss.NewStyle().Foreground(colorSuccess).Render("●")
	liveText := lipgloss.NewStyle().Foreground(colorText).Render(" Live")
	clockText := lipgloss.NewStyle().Foreground(colorTextMuted).Render(" " + m.currentTime.Format("15:04:05"))
	liveBadge := sbLiveStyle.Render(liveDot + liveText + clockText)

	brandBadge := sbBrandStyle.Render("⎈ kupgrade")

	left := statusBadge + statusText
	right := countBadge + liveBadge + brandBadge

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	filler := sbFillStyle.Render(strings.Repeat(" ", gap))

	return left + filler + right
}

// renderKeyHints renders the centered key hint row.
// [0] Dashboard  [1] Nodes  [2] Drains  [3] Pods  [4] Events  [?] Help  [q] Quit
func (m Model) renderKeyHints(width int) string {
	hints := []struct {
		key   string
		label string
	}{
		{"0", "Dashboard"},
		{"1", "Nodes"},
		{"2", "Drains"},
		{"3", "Pods"},
		{"4", "Events"},
		{"?", "Help"},
		{"q", "Quit"},
	}

	var parts []string
	for _, h := range hints {
		badge := keyBadgeStyle.Render(h.key)
		label := keyLabelStyle.Render(h.label)
		parts = append(parts, badge+" "+label)
	}

	row := strings.Join(parts, "  ")
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, row)
}

// progressBar renders a colored progress bar: green filled, muted empty.
// width = total block characters, filledCount = how many blocks are filled.
func progressBar(width, filledCount int) string {
	if filledCount > width {
		filledCount = width
	}
	if filledCount < 0 {
		filledCount = 0
	}
	empty := width - filledCount
	bar := lipgloss.NewStyle().Foreground(colorSuccess).Render(strings.Repeat("█", filledCount))
	bar += lipgloss.NewStyle().Foreground(colorTextMuted).Render(strings.Repeat("░", empty))
	return bar
}

// progressBarFromPercent renders a progress bar from a percentage (0-100).
func progressBarFromPercent(percent, width int) string {
	filled := (percent * width) / 100
	return progressBar(width, filled)
}

// resourceColor returns foreground color for CPU/MEM percentage thresholds.
func resourceColor(percent int) lipgloss.Color {
	switch {
	case percent > 70:
		return colorError // red
	case percent > 50:
		return colorWarning // yellow
	default:
		return colorTextMuted
	}
}

// isUpgradeActive returns true if any nodes are in active upgrade stages.
func (m Model) isUpgradeActive() bool {
	counts := m.stageCounts()
	return counts["CORDONED"]+counts["DRAINING"]+counts["REIMAGING"] > 0
}

// stageCountExcludingSurge returns the count of non-surge nodes in the given stage
func (m Model) stageCountExcludingSurge(stage types.NodeStage) int {
	count := 0
	for _, name := range m.nodesByStage[stage] {
		if node, ok := m.nodes[name]; ok && !node.SurgeNode {
			count++
		}
	}
	return count
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

// activeBlockers returns PDB blockers that are actively stalling a drain.
// Only blockers promoted to active tier (drain stalled 30+ seconds) are returned.
func (m Model) activeBlockers() []types.Blocker {
	var active []types.Blocker
	for _, b := range m.blockers {
		if b.Type == types.BlockerPDB && b.Tier == types.BlockerTierActive {
			active = append(active, b)
		}
	}
	return active
}

// formatDuration formats a duration as a human-readable string (e.g., "2m 14s")
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm %ds", m, s)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", h, m)
}

// Layout calculation helpers

// mainWidth returns the width available for the main content area.
func (m Model) mainWidth() int {
	return m.width
}

// tableWidth returns the width available for table content inside a bordered panel.
func (m Model) tableWidth() int {
	w := m.width - 8 // outer panel border (2) + padding (2) + breathing room (4)
	if w < 80 {
		w = 80
	}
	return w
}

// columnLayout describes a table column: either fixed width or flexible (shares remaining space).
type columnLayout struct {
	Fixed  int // >0 means exact width; 0 means flexible
	Weight int // relative weight among flexible columns (default 1)
}

// computeColumnWidths distributes tableWidth across columns.
// Fixed columns get their exact width; remaining space is split by weight among flexible columns.
// Each column width includes cell padding (2 chars).
func computeColumnWidths(totalWidth int, cols []columnLayout) []int {
	widths := make([]int, len(cols))
	remaining := totalWidth
	totalWeight := 0

	for i, c := range cols {
		if c.Fixed > 0 {
			widths[i] = c.Fixed
			remaining -= c.Fixed
		} else {
			w := c.Weight
			if w <= 0 {
				w = 1
			}
			totalWeight += w
		}
	}

	if remaining < 0 {
		remaining = 0
	}

	// Distribute remaining space by weight
	if totalWeight > 0 {
		for i, c := range cols {
			if c.Fixed > 0 {
				continue
			}
			w := c.Weight
			if w <= 0 {
				w = 1
			}
			widths[i] = remaining * w / totalWeight
		}
	}

	return widths
}

// getDrainNodes returns sorted list of nodes actively being drained (cordoned or draining).
// Reimaging nodes are excluded — they have already completed the drain phase.
func (m *Model) getDrainNodes() []string {
	var drainNodes []string
	drainNodes = append(drainNodes, m.nodesByStage[types.StageCordoned]...)
	drainNodes = append(drainNodes, m.nodesByStage[types.StageDraining]...)
	sort.Strings(drainNodes)
	return drainNodes
}

// clearPodSearch resets the pod search state
func (m *Model) clearPodSearch() {
	m.podSearchActive = false
	m.podSearchInput.SetValue("")
	m.podSearchInput.Blur()
}

// fillLinesBg ensures the background color is continuous across every line.
// It re-establishes the background after ANSI resets and pads each line to
// the given width. This prevents the terminal's own background from showing
// through gaps between styled segments.
func fillLinesBg(content string, width int, bg lipgloss.Color) string {
	sample := lipgloss.NewStyle().Background(bg).Render("X")
	idx := strings.Index(sample, "X")
	if idx <= 0 {
		return content
	}
	bgSeq := sample[:idx]

	fill := lipgloss.NewStyle().Background(bg)
	reset := "\x1b[0m"

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		line = bgSeq + strings.ReplaceAll(line, reset, reset+bgSeq)
		w := lipgloss.Width(line)
		if w < width {
			line += fill.Render(strings.Repeat(" ", width-w))
		}
		lines[i] = line + reset
	}
	return strings.Join(lines, "\n")
}
