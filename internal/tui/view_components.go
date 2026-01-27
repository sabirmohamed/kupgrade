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
	progress := m.renderProgressBar(headerProgressBarWidth)
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

	progress := m.renderProgressBar(headerProgressBarWidth)
	percent := fmt.Sprintf("%d%%", m.progressPercent())

	timeDisplay := timeStyle.Render(m.currentTime.Format("15:04:05"))

	return fmt.Sprintf("%s  %s | %s | %s %s | %s",
		titleDisplay, context, versionDisplay, progress, percent, timeDisplay)
}

// renderProgressBar renders a progress bar of given width
func (m Model) renderProgressBar(width int) string {
	percent := m.progressPercent()
	filled := (percent * width) / 100
	empty := width - filled

	bar := strings.Repeat(progressBarFull, filled) + strings.Repeat(progressBarEmpty, empty)
	return progressStyle.Render(bar)
}

// renderSmallProgressBar renders a small progress bar for node cards
func (m Model) renderSmallProgressBar(percent int) string {
	width := 12
	filled := (percent * width) / 100
	empty := width - filled
	bar := strings.Repeat(progressBarFull, filled) + strings.Repeat(progressBarEmpty, empty)
	return fmt.Sprintf("%s %d%%", bar, percent)
}

// renderPipelineRow renders compact stage counts with arrows
func (m Model) renderPipelineRow() string {
	stages := types.AllStages()
	var parts []string

	for i, stage := range stages {
		count := len(m.nodesByStage[stage])
		name := string(stage)

		// Highlight selected stage
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

		// Add arrow between stages (not after last)
		if i < len(stages)-1 {
			parts = append(parts, footerDescStyle.Render("  —  "))
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Center, parts...)
}

// renderFooter renders screen navigation footer
func (m Model) renderFooter() string {
	hints := []struct {
		key  string
		desc string
	}{
		{"0", "overview"},
		{"1", "nodes"},
		{"2", "drains"},
		{"3", "pods"},
		{"4", "blockers"},
		{"5", "events"},
		{"?", "help"},
		{"q", "quit"},
	}

	var parts []string
	for _, h := range hints {
		parts = append(parts, footerKeyStyle.Render(h.key)+" "+footerDescStyle.Render(h.desc))
	}

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

// min returns the smaller of two ints
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Layout calculation helpers

// nodeCardWidth calculates card width based on terminal width
func (m Model) nodeCardWidth() int {
	if m.width <= 0 {
		return nodeCardMinWidth + 4
	}
	gapTotal := (stageCount - 1) * nodeCardGapWidth
	available := m.width - gapTotal
	cardWidth := available / stageCount
	if cardWidth < nodeCardMinWidth {
		cardWidth = nodeCardMinWidth
	}
	if cardWidth > nodeCardMaxWidth {
		cardWidth = nodeCardMaxWidth
	}
	return cardWidth
}

// panelWidths calculates widths for bottom panels
func (m Model) panelWidths() (blockers, migrations, events int) {
	if m.width <= 0 {
		return 30, 30, 50
	}

	available := m.width - 8

	if len(m.blockers) > 0 {
		blockers = available * 25 / 100
		migrations = available * 30 / 100
		events = available - blockers - migrations
	} else {
		blockers = 0
		migrations = available * 35 / 100
		events = available - migrations
	}

	if migrations < 25 {
		migrations = 25
	}
	if events < 40 {
		events = 40
	}

	return blockers, migrations, events
}
