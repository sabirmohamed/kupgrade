package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// View renders the current TUI state
func (m Model) View() string {
	if m.fatalError != nil {
		return fmt.Sprintf("Error: %v\n", m.fatalError)
	}

	// Render the current screen
	var content string
	switch m.screen {
	case ScreenOverview:
		content = m.renderOverview()
	case ScreenNodes:
		content = m.renderNodesScreen()
	case ScreenDrains:
		content = m.renderDrainsScreen()
	case ScreenPods:
		content = m.renderPodsScreen()
	case ScreenBlockers:
		content = m.renderBlockersScreen()
	case ScreenEvents:
		content = m.renderEventsScreen()
	case ScreenStats:
		content = m.renderStatsScreen()
	default:
		content = m.renderOverview()
	}

	// Render overlay on top if active
	switch m.overlay {
	case OverlayHelp:
		return m.renderWithOverlay(m.renderHelpOverlay())
	case OverlayNodeDetail:
		return m.renderWithOverlay(m.renderNodeDetailOverlay())
	default:
		return content
	}
}

// renderWithOverlay renders an overlay centered on screen
func (m Model) renderWithOverlay(overlay string) string {
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
}

// placeContent places content in the terminal dimensions
func (m Model) placeContent(content string) string {
	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, content)
	}
	return content
}
