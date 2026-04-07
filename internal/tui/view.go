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
	placed := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay,
		lipgloss.WithWhitespaceBackground(colorBg))
	return fillLinesBg(placed, m.width, colorBg)
}

// placeContentWithFooter renders content with footer pinned to the terminal bottom.
// Main content is placed in the available space above the footer.
func (m Model) placeContentWithFooter(main, footer string) string {
	footerHeight := lipgloss.Height(footer)
	mainAreaHeight := m.height - footerHeight
	if mainAreaHeight < 1 {
		mainAreaHeight = 1
	}

	w := m.mainWidth()
	placed := lipgloss.Place(w, mainAreaHeight, lipgloss.Left, lipgloss.Top, main,
		lipgloss.WithWhitespaceBackground(colorBg))

	content := placed + "\n" + footer
	return fillLinesBg(content, m.width, colorBg)
}
