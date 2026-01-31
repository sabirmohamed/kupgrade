package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Layout constants (T2: extracted from magic numbers)
const (
	// Header
	headerProgressBarWidth = 10

	// Node cards
	nodeCardMinWidth = 16
	nodeCardMaxWidth = 30
	nodeCardGapWidth = 2
	stageCount       = 5

	// Events panel
	eventTypeWidth        = 14
	eventTimestampWidth   = 8
	eventIconWidth        = 2
	eventPaddingTotal     = eventTimestampWidth + eventIconWidth + 4 + 4 // spacing + border/padding
	eventMinMessageWidth  = 20

)

// Color palette - Omarchy Tokyo Night theme
// Source: https://github.com/omarchy/themes/tokyo-night
var (
	// Backgrounds (layered depth)
	colorBg        = lipgloss.Color("#1a1b26") // Main background
	colorBgAlt     = lipgloss.Color("#16161e") // Alternate panel background
	colorBgHover   = lipgloss.Color("#1f2335") // Hover state
	colorSelected  = lipgloss.Color("#414868") // Selected item background
	colorBorder    = lipgloss.Color("#565f89") // Border/divider lines
	colorBorderDim = lipgloss.Color("#32344a") // Subtle borders

	// Text hierarchy
	colorText      = lipgloss.Color("#a9b1d6") // Primary text
	colorTextBold  = lipgloss.Color("#c0caf5") // Emphasized text
	colorTextMuted = lipgloss.Color("#787c99") // Secondary text
	colorTextDim   = lipgloss.Color("#565f89") // Tertiary/inactive text

	// ANSI color slots (terminal-safe)
	colorBlack   = lipgloss.Color("#32344a") // color0
	colorRed     = lipgloss.Color("#f7768e") // color1
	colorGreen   = lipgloss.Color("#9ece6a") // color2
	colorYellow  = lipgloss.Color("#e0af68") // color3
	colorBlue    = lipgloss.Color("#7aa2f7") // color4
	colorMagenta = lipgloss.Color("#ad8ee6") // color5
	colorCyan    = lipgloss.Color("#449dab") // color6
	colorWhite   = lipgloss.Color("#787c99") // color7

	// Bright variants
	colorBrightRed     = lipgloss.Color("#ff7a93") // color9
	colorBrightGreen   = lipgloss.Color("#b9f27c") // color10
	colorBrightYellow  = lipgloss.Color("#ff9e64") // color11 (orange)
	colorBrightBlue    = lipgloss.Color("#7da6ff") // color12
	colorBrightMagenta = lipgloss.Color("#bb9af7") // color13
	colorBrightCyan    = lipgloss.Color("#0db9d7") // color14
	colorBrightWhite   = lipgloss.Color("#acb0d0") // color15

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
	colorAccent  = lipgloss.Color("#7dcfff") // Cyan - highlights/active
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

	progressStyle = lipgloss.NewStyle().
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
	nodeCardBase = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1)

	nodeCardNormal = nodeCardBase.Copy().
			BorderForeground(colorBorder)

	nodeCardSelected = nodeCardBase.Copy().
				Border(lipgloss.DoubleBorder()).
				BorderForeground(colorAccent).
				Background(colorSelected)

	nodeCardBlocked = nodeCardBase.Copy().
			BorderForeground(colorError)

	nodeCardComplete = nodeCardBase.Copy().
				BorderForeground(colorComplete)

	nodeNameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorText)

	// Selected row in node list - subtle rounded box
	nodeListSelectedStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorBorder).
				Padding(0, 1)

	nodePodStyle = lipgloss.NewStyle().
			Foreground(colorTextMuted)

	nodeVersionStyle = lipgloss.NewStyle().
				Foreground(colorTextMuted)

)

// Panel styles
var (
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	// Left border only for blockers section
	blockerPanelStyle = lipgloss.NewStyle().
				Border(lipgloss.Border{Left: "│"}).
				BorderForeground(colorWarning).
				PaddingLeft(1)

	panelTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorText)

	panelTitleError = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorError)
)

// Event styles
var (
	timestampStyle = lipgloss.NewStyle().
			Foreground(colorTextDim)

	eventTypeStyle = lipgloss.NewStyle().
			Width(eventTypeWidth)

	infoIcon    = "•"
	warningIcon = "⚠"
	errorIcon   = "✖"
	migrateIcon = "↹"
	spinnerIcon = "◌"
	checkIcon   = "✓"

	infoStyle = lipgloss.NewStyle().
			Foreground(colorInfo)

	warningStyle = lipgloss.NewStyle().
			Foreground(colorWarning)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorError)

	successStyle = lipgloss.NewStyle().
			Foreground(colorSuccess)

	migrationStyle = lipgloss.NewStyle().
			Foreground(colorBrightCyan)
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

// Stage arrow
var stageArrow = "━━▶"

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
	// Selected row in lists
	selectedRowStyle = lipgloss.NewStyle().
				Background(colorSelected).
				Foreground(colorTextBold)

	// Header row for tables
	tableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorTextMuted).
				BorderBottom(true).
				BorderForeground(colorBorderDim).
				BorderStyle(lipgloss.NormalBorder())

	// Alternating row backgrounds for readability
	rowEvenStyle = lipgloss.NewStyle().
			Background(colorBg)

	rowOddStyle = lipgloss.NewStyle().
			Background(colorBgAlt)

	// lipgloss/table border style
	tableBorderStyle = lipgloss.NewStyle().
				Foreground(colorBorderDim)
)

// Layout helpers
func centerText(s string, width int) string {
	return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(s)
}
