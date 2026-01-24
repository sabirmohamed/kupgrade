package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Color palette
var (
	colorBg        = lipgloss.Color("#0a0a0f")
	colorBgAlt     = lipgloss.Color("#12121a")
	colorBorder    = lipgloss.Color("#1a1a2e")
	colorText      = lipgloss.Color("#e0e0e0")
	colorTextMuted = lipgloss.Color("#666666")

	colorReady     = lipgloss.Color("#888888")
	colorCordoned  = lipgloss.Color("#FFAA00")
	colorDraining  = lipgloss.Color("#FF6B35")
	colorUpgrading = lipgloss.Color("#00D4FF")
	colorComplete  = lipgloss.Color("#00FF9F")

	colorError   = lipgloss.Color("#FF0055")
	colorWarning = lipgloss.Color("#FFAA00")
	colorSuccess = lipgloss.Color("#00FF9F")
	colorInfo    = lipgloss.Color("#00D4FF")
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

// Node card styles
var (
	nodeCardWidth = 20

	nodeCardBase = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1).
			Width(nodeCardWidth)

	nodeCardNormal = nodeCardBase.Copy().
			BorderForeground(colorBorder)

	nodeCardSelected = nodeCardBase.Copy().
				Border(lipgloss.DoubleBorder()).
				BorderForeground(colorText)

	nodeCardBlocked = nodeCardBase.Copy().
			BorderForeground(colorError)

	nodeCardComplete = nodeCardBase.Copy().
				BorderForeground(colorComplete)

	nodeNameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorText)

	nodePodStyle = lipgloss.NewStyle().
			Foreground(colorTextMuted)

	nodeVersionStyle = lipgloss.NewStyle().
				Foreground(colorTextMuted)

	progressBarFull  = "█"
	progressBarEmpty = "░"
)

// Panel styles
var (
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

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
			Foreground(colorTextMuted)

	eventTypeStyle = lipgloss.NewStyle().
			Width(14)

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
			Foreground(lipgloss.Color("#141414"))
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

// Layout helpers
func centerText(s string, width int) string {
	return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(s)
}
