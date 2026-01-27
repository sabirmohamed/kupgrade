package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderBottomPanels renders legacy bottom panel layout
func (m Model) renderBottomPanels() string {
	var panels []string

	blockersWidth, migrationsWidth, eventsWidth := m.panelWidths()

	if len(m.blockers) > 0 {
		panels = append(panels, m.renderBlockersPanel(blockersWidth))
	}

	panels = append(panels, m.renderMigrationsPanel(migrationsWidth))
	panels = append(panels, m.renderEventsPanel(eventsWidth))

	return lipgloss.JoinHorizontal(lipgloss.Top, panels...)
}

// renderMigrationsPanel renders migrations in bottom panel
func (m Model) renderMigrationsPanel(width int) string {
	title := panelTitleStyle.Render("↹ RESCHEDULED")
	var lines []string
	lines = append(lines, title)

	if len(m.migrations) == 0 {
		lines = append(lines, footerDescStyle.Render("No pod moves yet"))
	} else {
		for _, mig := range m.migrations {
			icon := migrateIcon
			if mig.Complete {
				icon = checkIcon
			}
			line := fmt.Sprintf("%s %s/%s → %s", icon, mig.Namespace, mig.NewPod, mig.ToNode)
			lines = append(lines, line)
		}
	}

	content := strings.Join(lines, "\n")
	return panelStyle.Width(width).MarginRight(2).Render(content)
}
