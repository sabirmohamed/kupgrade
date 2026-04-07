package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Layout constants
const (
	// Progress bars
	headerProgressBarWidth = 8  // compact bar in header
	dialogProgressBarWidth = 24 // wider bar in pill dialog
	dialogWidth            = 74 // centered pill dialog width

	// Stage pill dark foreground
	pillDarkFg = "#1a1b26"
)

// Color palette - Omarchy Tokyo Night theme
// Source: https://github.com/omarchy/themes/tokyo-night
var (
	// Backgrounds (layered depth)
	// colorBg is applied to all text styles so kupgrade's background is consistent
	// across terminals (Ghostty, Terminal.app, iTerm, Warp). This follows the LFK
	// pattern: force an opaque background on every element so the terminal's own
	// background never shows through.
	colorBg        = lipgloss.Color("#000000") // Main background — pure black for maximum contrast
	colorSelected  = lipgloss.Color("#414868") // Selected item background
	colorBorderDim = lipgloss.Color("#32344a") // Subtle borders

	// Text hierarchy
	colorText      = lipgloss.Color("#a9b1d6") // Primary text
	colorTextBold  = lipgloss.Color("#c0caf5") // Emphasized text
	colorTextMuted = lipgloss.Color("#787c99") // Secondary text
	colorTextDim   = lipgloss.Color("#565f89") // Tertiary/inactive text

	// ANSI color slots (terminal-safe)
	colorCyan = lipgloss.Color("#449dab") // color6

	// Bright variants
	colorBrightRed    = lipgloss.Color("#ff7a93") // color9
	colorBrightYellow = lipgloss.Color("#ff9e64") // color11 (orange)

	// Accent colors (Tokyo Night extended)
	colorPurple = lipgloss.Color("#9d7cd8") // pool names

	// Semantic: Stage colors
	colorReady     = lipgloss.Color("#787c99") // Muted grey - waiting
	colorCordoned  = lipgloss.Color("#e0af68") // Yellow - warning/paused
	colorDraining  = lipgloss.Color("#ff9e64") // Orange - active attention
	colorReimaging = lipgloss.Color("#7dcfff") // Bright cyan - in progress
	colorComplete  = lipgloss.Color("#9ece6a") // Green - done

	// Semantic: Status colors
	colorError   = lipgloss.Color("#f7768e") // Red - errors
	colorWarning = lipgloss.Color("#e0af68") // Yellow - warnings
	colorSuccess = lipgloss.Color("#9ece6a") // Green - success
	colorInfo    = lipgloss.Color("#7aa2f7") // Blue - info
)

// Header styles — all text styles include .Background(colorBg) so kupgrade's
// background is opaque and consistent across all terminals.
var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorInfo).
			Background(colorBg)

	contextStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorBg)

	versionStyle = lipgloss.NewStyle().
			Foreground(colorWarning).
			Background(colorBg)

	versionCompleteStyle = lipgloss.NewStyle().
				Foreground(colorComplete).
				Background(colorBg)
)

// Panel styles
var (
	panelTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorText).
		Background(colorBg)
)

// Event styles
var (
	infoIcon    = "•"
	warningIcon = "⚠"
	errorIcon   = "✖"
	migrateIcon = "↹"
	checkIcon   = "✓"

	warningStyle = lipgloss.NewStyle().
			Foreground(colorWarning).
			Background(colorBg)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorError).
			Background(colorBg)

	eventCountStyle = lipgloss.NewStyle().
			Foreground(colorTextBold).Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(colorSuccess).
			Background(colorBg)
)

// Footer styles
var (
	footerKeyStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Bold(true).
			Background(colorBg)

	footerDescStyle = lipgloss.NewStyle().
			Foreground(colorTextMuted).
			Background(colorBg)
)

// Overlay styles
var (
	overlayStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorInfo).
			BorderBackground(colorBg).
			Background(colorBg).
			Padding(1, 2)

	overlayTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorInfo).
				Background(colorBg).
				MarginBottom(1)
)

// List/table styles
var (
	// lipgloss/table border style
	tableBorderStyle = lipgloss.NewStyle().
		Foreground(colorBorderDim).
		Background(colorBg)
)

// stageForegroundColors maps stage names to foreground-only colors for table cells.
// Use these instead of stagePillStyles inside tables to preserve alternating row backgrounds.
var stageForegroundColors = map[string]lipgloss.Color{
	"READY":       colorReady,
	"CORDONED":    colorCordoned,
	"DRAINING":    colorDraining,
	"QUARANTINED": colorError,
	"REIMAGING":   colorReimaging,
	"COMPLETE":    colorComplete,
	"SURGE":       lipgloss.Color("#bb9af7"),
}

// Stage pill styles — high-contrast colored background with bold text.
// Light backgrounds (yellow, orange, green, cyan) use dark foreground for readability.
// Dark backgrounds (grey/ready) use white foreground.
var stagePillStyles = map[string]lipgloss.Style{
	"READY":       lipgloss.NewStyle().Background(colorReady).Foreground(lipgloss.Color("#ffffff")).Bold(true).Padding(0, 1),
	"CORDONED":    lipgloss.NewStyle().Background(colorCordoned).Foreground(lipgloss.Color(pillDarkFg)).Bold(true).Padding(0, 1),
	"DRAINING":    lipgloss.NewStyle().Background(colorDraining).Foreground(lipgloss.Color(pillDarkFg)).Bold(true).Padding(0, 1),
	"QUARANTINED": lipgloss.NewStyle().Background(colorError).Foreground(lipgloss.Color("#ffffff")).Bold(true).Padding(0, 1),
	"REIMAGING":   lipgloss.NewStyle().Background(colorReimaging).Foreground(lipgloss.Color(pillDarkFg)).Bold(true).Padding(0, 1),
	"COMPLETE":    lipgloss.NewStyle().Background(colorComplete).Foreground(lipgloss.Color(pillDarkFg)).Bold(true).Padding(0, 1),
	"SURGE":       lipgloss.NewStyle().Background(lipgloss.Color("#bb9af7")).Foreground(lipgloss.Color(pillDarkFg)).Bold(true).Padding(0, 1),
}

// Tab bar styles
var (
	tabActiveStyle   = lipgloss.NewStyle().Bold(true).Background(colorSelected).Foreground(lipgloss.Color("#c0caf5")).Padding(0, 1)
	tabInactiveStyle = lipgloss.NewStyle().Foreground(colorTextMuted).Background(colorBg)
	tabSepStyle      = lipgloss.NewStyle().Foreground(colorTextDim).Background(colorBg)
)

// Screen tab definitions
type screenTab struct {
	icon   string
	label  string
	screen Screen
}

var screenTabs = []screenTab{
	{"★", "Dashboard", ScreenOverview},
	{"●", "Nodes", ScreenNodes},
	{"⇌", "Drains", ScreenDrains},
	{"⫼", "Pods", ScreenPods},
	{"●", "Events", ScreenEvents},
}

// renderStagePill renders a stage name with count as a colored pill: [DRAINING 2]
// Always uses the stage color — zero-count pills are NOT dimmed.
func renderStagePill(stage string, count int) string {
	label := fmt.Sprintf("%s %d", stage, count)
	style, ok := stagePillStyles[stage]
	if !ok {
		return label
	}
	return style.Render(label)
}

// renderStagePillInline renders just a stage name as a colored pill (no count)
func renderStagePillInline(stage string) string {
	style, ok := stagePillStyles[stage]
	if !ok {
		return stage
	}
	return style.Render(stage)
}

// renderTabBar renders the top tab bar with screen icons and counts.
// Active screen is highlighted, inactive screens are muted.
func (m Model) renderTabBar(stageCounts map[string]int) string {
	sep := tabSepStyle.Render(" │ ")
	var parts []string

	for _, tab := range screenTabs {
		label := fmt.Sprintf("%s %s", tab.icon, tab.label)

		// Add count for relevant screens
		switch tab.screen {
		case ScreenNodes:
			label += fmt.Sprintf(" (%d)", m.totalNodes())
		case ScreenDrains:
			drainCount := stageCounts["CORDONED"] + stageCounts["DRAINING"]
			if drainCount > 0 {
				label += fmt.Sprintf(" (%d)", drainCount)
			}
		case ScreenEvents:
			if len(m.events) > 0 {
				label += fmt.Sprintf(" (%d)", len(m.events))
			}
		}

		if tab.screen == m.screen {
			parts = append(parts, tabActiveStyle.Render(label))
		} else {
			parts = append(parts, tabInactiveStyle.Render(label))
		}
	}

	return strings.Join(parts, sep)
}

// Status bar styles
var (
	sbStatusStyle = lipgloss.NewStyle().
			Background(colorError).
			Foreground(colorBg).
			Bold(true).
			Padding(0, 1)

	sbStatusWatchingStyle = lipgloss.NewStyle().
				Background(colorInfo).
				Foreground(colorBg).
				Bold(true).
				Padding(0, 1)

	sbStatusCompleteStyle = lipgloss.NewStyle().
				Background(colorSuccess).
				Foreground(colorBg).
				Bold(true).
				Padding(0, 1)

	sbTextStyle = lipgloss.NewStyle().
			Background(colorSelected).
			Foreground(colorText).
			Padding(0, 1)

	sbFillStyle = lipgloss.NewStyle().
			Background(colorSelected)

	sbCountStyle = lipgloss.NewStyle().
			Background(colorPurple).
			Foreground(lipgloss.Color("#ffffff")).
			Bold(true).
			Padding(0, 1)

	sbLiveStyle = lipgloss.NewStyle().
			Background(colorSelected).
			Padding(0, 1)

	sbBrandStyle = lipgloss.NewStyle().
			Background(colorInfo).
			Foreground(lipgloss.Color("#ffffff")).
			Bold(true).
			Padding(0, 1)
)

// Key hint styles
var (
	keyBadgeStyle = lipgloss.NewStyle().
			Background(colorSelected).
			Foreground(colorTextBold).
			Bold(true).
			Padding(0, 1)

	keyLabelStyle = lipgloss.NewStyle().
			Foreground(colorTextDim).
			Background(colorBg)
)

// Dialog styles
var (
	dialogBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPurple).
		BorderBackground(colorBg).
		Background(colorBg).
		Padding(0, 2).
		Width(dialogWidth)
)

// Info card styles
var (
	cardTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorTextBold).
		Background(colorBg)
)

// renderSectionHeader renders: ── TITLE ───────────────────
func renderSectionHeader(title string, width int) string {
	prefix := "── "
	suffix := " "
	remaining := width - len(prefix) - len(title) - len(suffix)
	if remaining < 4 {
		remaining = 4
	}
	line := prefix + title + suffix + strings.Repeat("─", remaining)
	return panelTitleStyle.Render(line)
}
