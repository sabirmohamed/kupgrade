package tui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	// Header styles
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39"))

	contextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	versionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220"))

	// Event styles
	timestampStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("242"))

	eventTypeStyle = lipgloss.NewStyle().
			Width(14)

	// Severity icons and colors
	infoIcon    = "•"
	warningIcon = "⚠"
	errorIcon   = "✖"

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	migrationIcon  = "↹"
	migrationStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("141"))

	// Footer
	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			MarginTop(1)
)
