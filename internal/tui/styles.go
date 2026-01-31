package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Layout constants (T2: extracted from magic numbers)
const (
	// Header
	headerProgressBarWidth = 10
)

// Color palette - Omarchy Tokyo Night theme
// Source: https://github.com/omarchy/themes/tokyo-night
var (
	// Backgrounds (layered depth)
	colorBg        = lipgloss.Color("#1a1b26") // Main background
	colorBgAlt     = lipgloss.Color("#16161e") // Alternate panel background
	colorSelected  = lipgloss.Color("#414868") // Selected item background
	colorBorder    = lipgloss.Color("#565f89") // Border/divider lines
	colorBorderDim = lipgloss.Color("#32344a") // Subtle borders

	// Text hierarchy
	colorText      = lipgloss.Color("#a9b1d6") // Primary text
	colorTextBold  = lipgloss.Color("#c0caf5") // Emphasized text
	colorTextMuted = lipgloss.Color("#787c99") // Secondary text
	colorTextDim   = lipgloss.Color("#565f89") // Tertiary/inactive text

	// ANSI color slots (terminal-safe)
	colorYellow = lipgloss.Color("#e0af68") // color3
	colorCyan   = lipgloss.Color("#449dab") // color6

	// Bright variants
	colorBrightRed    = lipgloss.Color("#ff7a93") // color9
	colorBrightYellow = lipgloss.Color("#ff9e64") // color11 (orange)

	// Semantic: Stage colors
	colorReady     = lipgloss.Color("#787c99") // Muted grey - waiting
	colorCordoned  = lipgloss.Color("#e0af68") // Yellow - warning/paused
	colorDraining  = lipgloss.Color("#ff9e64") // Orange - active attention
	colorUpgrading = lipgloss.Color("#7dcfff") // Bright cyan - in progress
	colorComplete  = lipgloss.Color("#9ece6a") // Green - done

	// Semantic: Status colors
	colorError   = lipgloss.Color("#f7768e") // Red - errors
	colorWarning = lipgloss.Color("#e0af68") // Yellow - warnings
	colorSuccess = lipgloss.Color("#9ece6a") // Green - success
	colorInfo    = lipgloss.Color("#7aa2f7") // Blue - info
)

// Header styles
var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorInfo)

	contextStyle = lipgloss.NewStyle().
			Foreground(colorText)

	versionStyle = lipgloss.NewStyle().
			Foreground(colorWarning)

	versionCompleteStyle = lipgloss.NewStyle().
				Foreground(colorComplete)

	timeStyle = lipgloss.NewStyle().
			Foreground(colorTextMuted)
)

// Stage styles
var stageColors = map[string]lipgloss.Color{
	"READY":     colorReady,
	"CORDONED":  colorCordoned,
	"DRAINING":  colorDraining,
	"UPGRADING": colorUpgrading,
	"COMPLETE":  colorComplete,
}

func stageStyle(stage string) lipgloss.Style {
	color, ok := stageColors[stage]
	if !ok {
		color = colorText
	}
	return lipgloss.NewStyle().Foreground(color).Bold(true)
}

func stageStyleSelected(stage string) lipgloss.Style {
	return stageStyle(stage).Underline(true)
}

// Node card styles (width set dynamically in view.go)
var (
	// Selected row in node list - subtle rounded box
	nodeListSelectedStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1)
)

// Panel styles
var (
	// Left border only for blockers section
	blockerPanelStyle = lipgloss.NewStyle().
				Border(lipgloss.Border{Left: "│"}).
				BorderForeground(colorWarning).
				PaddingLeft(1)

	panelTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorText)
)

// Event styles
var (
	timestampStyle = lipgloss.NewStyle().
			Foreground(colorTextDim)

	infoIcon    = "•"
	warningIcon = "⚠"
	errorIcon   = "✖"
	migrateIcon = "↹"
	checkIcon   = "✓"

	infoStyle = lipgloss.NewStyle().
			Foreground(colorInfo)

	warningStyle = lipgloss.NewStyle().
			Foreground(colorWarning)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorError)

	successStyle = lipgloss.NewStyle().
			Foreground(colorSuccess)
)

// Footer styles
var (
	footerStyle = lipgloss.NewStyle().
			Foreground(colorTextMuted)

	footerKeyStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Bold(true)

	footerDescStyle = lipgloss.NewStyle().
			Foreground(colorTextMuted)
)

// Overlay styles
var (
	overlayStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorInfo).
			Padding(1, 2)

	overlayTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorInfo).
				MarginBottom(1)
)

// List/table styles
var (
	// lipgloss/table border style
	tableBorderStyle = lipgloss.NewStyle().
		Foreground(colorBorderDim)
)

// Layout helpers
func centerText(s string, width int) string {
	return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(s)
}
