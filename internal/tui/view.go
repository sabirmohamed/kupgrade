package tui

import (
	"fmt"
	"strings"

	"github.com/sabirmohamed/kupgrade/pkg/types"
)

// View renders the model
func (m Model) View() string {
	if m.fatalError != nil {
		return fmt.Sprintf("Error: %v\n", m.fatalError)
	}

	var b strings.Builder

	// Header
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")

	// Events
	b.WriteString(m.renderEvents())

	// Footer
	b.WriteString(footerStyle.Render("[q] quit"))
	b.WriteString("\n")

	return b.String()
}

func (m Model) renderHeader() string {
	title := headerStyle.Render("⎈ kupgrade watch")

	context := contextStyle.Render(m.contextName)

	version := m.serverVersion
	if m.targetVersion != "" && m.targetVersion != m.serverVersion {
		version = fmt.Sprintf("%s → %s", m.serverVersion, m.targetVersion)
	}
	versionDisplay := versionStyle.Render(version)

	eventCount := fmt.Sprintf("Events: %d", m.eventCount)

	return fmt.Sprintf("%s  %s | %s | %s", title, context, versionDisplay, eventCount)
}

func (m Model) renderEvents() string {
	if len(m.events) == 0 {
		return "Waiting for events...\n\n"
	}

	var b strings.Builder

	// Show events in reverse chronological order (newest first)
	// But render them oldest to newest for natural reading
	start := 0
	if len(m.events) > 20 {
		start = len(m.events) - 20
	}

	for i := start; i < len(m.events); i++ {
		b.WriteString(m.renderEvent(m.events[i]))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	return b.String()
}

func (m Model) renderEvent(e types.Event) string {
	// Timestamp
	ts := timestampStyle.Render(e.Timestamp.Format("15:04:05"))

	// Severity icon
	var icon string
	var iconStyled string
	switch e.Severity {
	case types.SeverityWarning:
		iconStyled = warningStyle.Render(warningIcon)
	case types.SeverityError:
		iconStyled = errorStyle.Render(errorIcon)
	default:
		if e.Type == types.EventMigration {
			iconStyled = migrationStyle.Render(migrationIcon)
		} else {
			iconStyled = infoStyle.Render(infoIcon)
		}
	}
	_ = icon // unused

	// Event type
	eventType := eventTypeStyle.Render(fmt.Sprintf("[%-14s]", e.Type))

	return fmt.Sprintf("%s %s %s %s", ts, iconStyled, eventType, e.Message)
}
