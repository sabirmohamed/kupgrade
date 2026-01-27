package tui

import (
	"fmt"
	"strings"
)

// renderEventsScreen renders the full events log screen
func (m Model) renderEventsScreen() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")

	if len(m.events) == 0 {
		b.WriteString(footerDescStyle.Render("  Waiting for events..."))
	} else {
		for i, e := range m.events {
			cursor := "  "
			if i == m.listIndex {
				cursor = "► "
			}

			ts := timestampStyle.Render(e.Timestamp.Format("15:04:05"))
			icon := m.severityIcon(e.Severity)
			nodeName := e.NodeName
			if nodeName == "" {
				nodeName = "-"
			}

			line := fmt.Sprintf("%s%s %s %-15s %s",
				cursor, ts, icon, nodeName, e.Message)

			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(m.renderFooter())

	return m.placeContent(b.String())
}

// renderEventsPanel renders events in bottom panel (legacy layout)
func (m Model) renderEventsPanel(width int) string {
	title := panelTitleStyle.Render("• EVENTS")
	var lines []string
	lines = append(lines, title)

	maxMsgLen := width - eventPaddingTotal
	if maxMsgLen < eventMinMessageWidth {
		maxMsgLen = eventMinMessageWidth
	}

	if len(m.events) == 0 {
		lines = append(lines, footerDescStyle.Render("Waiting for events..."))
	} else {
		for _, e := range m.events {
			ts := timestampStyle.Render(e.Timestamp.Format("15:04:05"))
			icon := m.severityIcon(e.Severity)
			msg := e.Message
			if len(msg) > maxMsgLen {
				msg = msg[:maxMsgLen] + "..."
			}
			lines = append(lines, fmt.Sprintf("%s %s %s", ts, icon, msg))
		}
	}

	content := strings.Join(lines, "\n")
	return panelStyle.Width(width).Render(content)
}
