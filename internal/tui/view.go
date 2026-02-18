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

	// Overlays take priority (full screen)
	if m.overlay == OverlayHelp {
		return m.renderWithOverlay(m.renderHelpOverlay())
	}
	if m.overlay == OverlayDetail {
		return m.renderWithOverlay(m.renderDetailOverlay())
	}

	// Render the current screen
	switch m.screen {
	case ScreenOverview:
		return m.renderOverview()
	case ScreenNodes:
		return m.renderNodesScreen()
	case ScreenDrains:
		return m.renderDrainsScreen()
	case ScreenPods:
		return m.renderPodsScreen()
	case ScreenEvents:
		return m.renderEventsScreen()
	default:
		return m.renderOverview()
	}
}

// renderWithOverlay renders an overlay centered on screen
func (m Model) renderWithOverlay(overlay string) string {
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
}

// placeContent places content in the available main area dimensions
func (m Model) placeContent(content string) string {
	w := m.mainWidth()
	if w > 0 && m.height > 0 {
		return lipgloss.Place(w, m.height, lipgloss.Left, lipgloss.Top, content)
	}
	return content
}
