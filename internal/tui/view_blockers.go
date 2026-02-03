package tui

import (
	"fmt"
	"strings"
	"time"
)

// renderBlockersScreen renders the blockers detail screen
func (m Model) renderBlockersScreen() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")

	if len(m.blockers) == 0 {
		b.WriteString(successStyle.Render("  No active blockers"))
		b.WriteString("\n")
		b.WriteString(footerDescStyle.Render("  Blockers appear when eviction is blocked by PDB"))
	} else {
		for i, blocker := range m.blockers {
			cursor := "  "
			if i == m.listIndex {
				cursor = "► "
			}

			name := blocker.Name
			if blocker.Namespace != "" {
				name = blocker.Namespace + "/" + blocker.Name
			}

			// Show duration if StartTime is set
			durationStr := ""
			if !blocker.StartTime.IsZero() {
				duration := m.currentTime.Sub(blocker.StartTime)
				durationStr = fmt.Sprintf(" (%s)", formatDuration(duration))
			}

			// Show node name if available
			nodeStr := ""
			if blocker.NodeName != "" {
				nodeStr = fmt.Sprintf(" on %s", blocker.NodeName)
			}

			line1 := fmt.Sprintf("%s%s: %s%s%s", cursor, blocker.Type, name, nodeStr, durationStr)
			line2 := fmt.Sprintf("    └─ %s", blocker.Detail)

			if i == m.listIndex {
				b.WriteString(errorStyle.Render(line1))
				b.WriteString("\n")
				b.WriteString(warningStyle.Render(line2))
			} else {
				b.WriteString(warningStyle.Render(line1))
				b.WriteString("\n")
				b.WriteString(footerDescStyle.Render(line2))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(m.renderFooter())

	return m.placeContent(b.String())
}

// formatDuration formats a duration as a human-readable string (e.g., "2m 14s")
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm %ds", m, s)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", h, m)
}
