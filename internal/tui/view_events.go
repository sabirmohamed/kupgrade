package tui

import (
	"fmt"
	"strings"

	"github.com/sabirmohamed/kupgrade/pkg/types"
)

// renderEventsScreen renders the full events log screen
func (m Model) renderEventsScreen() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	b.WriteString(m.renderEventFilterBar())
	b.WriteString("\n\n")

	events := m.filteredEvents()

	if len(events) == 0 {
		b.WriteString(m.renderEmptyEventsMessage())
	} else if m.eventAggregated {
		b.WriteString(m.renderAggregatedEventsList(events))
	} else {
		b.WriteString(m.renderRawEventsList(events))
	}

	b.WriteString("\n")
	b.WriteString(m.renderEventsFooter())

	return m.placeContent(b.String())
}

// renderEmptyEventsMessage returns the message to display when no events match the filter
func (m Model) renderEmptyEventsMessage() string {
	if m.eventFilter == EventFilterAll {
		return footerDescStyle.Render("  Waiting for events...")
	}
	return footerDescStyle.Render(fmt.Sprintf("  No %s events", strings.ToLower(m.eventFilterName())))
}

// renderAggregatedEventsList renders events grouped by reason
func (m Model) renderAggregatedEventsList(events []types.Event) string {
	var b strings.Builder
	aggregated := aggregateEvents(events)

	for i, ag := range aggregated {
		b.WriteString(m.renderAggregatedEventRow(i, ag))
		b.WriteString(m.renderExpandedEventsOrHint(i, ag, events))
	}

	return b.String()
}

// renderAggregatedEventRow renders a single aggregated event row
func (m Model) renderAggregatedEventRow(index int, ag AggregatedEvent) string {
	cursor := "  "
	if index == m.listIndex {
		cursor = "► "
	}

	ts := timestampStyle.Render(ag.Timestamp.Format("15:04:05"))
	icon := m.severityIcon(ag.Severity)

	expandIcon := "▸"
	if m.expandedGroup == ag.Reason {
		expandIcon = "▾"
	}

	var line string
	if ag.Count > 1 {
		line = fmt.Sprintf("%s%s %s %s %s", cursor, ts, icon, expandIcon, ag.Format(icon))
	} else {
		line = fmt.Sprintf("%s%s %s   %s", cursor, ts, icon, ag.Format(icon))
	}

	return line + "\n"
}

// renderExpandedEventsOrHint renders expanded event details or the expand hint
func (m Model) renderExpandedEventsOrHint(index int, ag AggregatedEvent, events []types.Event) string {
	if ag.Count <= 1 {
		return ""
	}

	if m.expandedGroup == ag.Reason {
		return m.renderExpandedEvents(ag.Reason, events)
	}

	if index == m.listIndex {
		return footerDescStyle.Render("       (press 'e' to expand)") + "\n"
	}

	return ""
}

// renderExpandedEvents renders the individual events within an expanded group
func (m Model) renderExpandedEvents(reason string, events []types.Event) string {
	var b strings.Builder

	msgWidth := m.mainWidth() - 45
	if msgWidth < 40 {
		msgWidth = 40
	}

	for _, e := range events {
		if extractReason(e.Message) != reason {
			continue
		}

		subTs := timestampStyle.Render(e.Timestamp.Format("15:04:05"))
		subIcon := m.severityIcon(e.Severity)
		nodeName := e.NodeName
		if nodeName == "" {
			nodeName = "-"
		}

		subLine := fmt.Sprintf("       %s %s %-12s %s",
			subTs, subIcon, nodeName, truncateMessage(e.Message, msgWidth))
		b.WriteString(footerDescStyle.Render(subLine))
		b.WriteString("\n")
	}

	return b.String()
}

// renderRawEventsList renders events in raw (non-aggregated) format
func (m Model) renderRawEventsList(events []types.Event) string {
	var b strings.Builder

	for i, e := range events {
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

	return b.String()
}

// renderEventFilterBar renders the filter toggle bar
func (m Model) renderEventFilterBar() string {
	upgradeStyle := footerDescStyle
	warningsStyle := footerDescStyle
	allStyle := footerDescStyle

	// Highlight the active filter
	switch m.eventFilter {
	case EventFilterUpgrade:
		upgradeStyle = footerKeyStyle
	case EventFilterWarnings:
		warningsStyle = footerKeyStyle
	case EventFilterAll:
		allStyle = footerKeyStyle
	}

	// Aggregation indicator
	aggLabel := "Raw"
	aggStyle := footerDescStyle
	if m.eventAggregated {
		aggLabel = "Grouped"
		aggStyle = footerKeyStyle
	}

	if m.eventAggregated {
		return fmt.Sprintf("  %s %s  %s %s  %s %s  │  %s %s  %s expand",
			footerKeyStyle.Render("[u]"), upgradeStyle.Render("Upgrade"),
			footerKeyStyle.Render("[w]"), warningsStyle.Render("Warnings"),
			footerKeyStyle.Render("[a]"), allStyle.Render("All"),
			footerKeyStyle.Render("[g]"), aggStyle.Render(aggLabel),
			footerKeyStyle.Render("[e]"))
	}
	return fmt.Sprintf("  %s %s  %s %s  %s %s  │  %s %s",
		footerKeyStyle.Render("[u]"), upgradeStyle.Render("Upgrade"),
		footerKeyStyle.Render("[w]"), warningsStyle.Render("Warnings"),
		footerKeyStyle.Render("[a]"), allStyle.Render("All"),
		footerKeyStyle.Render("[g]"), aggStyle.Render(aggLabel))
}

// renderEventsFooter renders footer with event-specific hints
func (m Model) renderEventsFooter() string {
	events := m.filteredEvents()
	var countInfo string
	if m.eventAggregated {
		aggregated := aggregateEvents(events)
		countInfo = fmt.Sprintf("%d groups from %d events", len(aggregated), len(events))
	} else {
		countInfo = fmt.Sprintf("Showing %d of %d events", len(events), len(m.events))
	}
	return footerDescStyle.Render("  " + countInfo + "  •  ↑↓ navigate  •  d describe  •  q back")
}

// truncateMessage smartly truncates a message, preserving important parts
// Keeps: [Reason] prefix, shortened resource name, error type at end
func truncateMessage(msg string, maxLen int) string {
	if len(msg) <= maxLen {
		return msg
	}

	// Try to find the error description after the last colon
	lastColon := strings.LastIndex(msg, ": ")
	if lastColon > 0 && lastColon < len(msg)-2 {
		prefix := msg[:lastColon]
		suffix := msg[lastColon+2:]

		// Shorten the prefix (resource name), keep the suffix (error)
		availableForPrefix := maxLen - len(suffix) - 5 // 5 for ": " and "..."
		if availableForPrefix > 20 && len(suffix) < maxLen/2 {
			// Shorten prefix, keep full suffix
			shortPrefix := shortenResourceName(prefix, availableForPrefix)
			return shortPrefix + ": " + suffix
		}
	}

	// Fallback: truncate end but try to end at word boundary
	if maxLen > 10 {
		truncated := msg[:maxLen-3]
		// Find last space to break at word
		if lastSpace := strings.LastIndex(truncated, " "); lastSpace > maxLen/2 {
			return truncated[:lastSpace] + "..."
		}
		return truncated + "..."
	}
	return msg[:maxLen-3] + "..."
}

// shortenResourceName shortens names with hashes like "app-845849966b-flnj7" to "app-...-flnj7"
func shortenResourceName(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}

	// Look for hash patterns (e.g., deployment-hash-podHash)
	parts := strings.Split(name, "-")
	if len(parts) >= 3 {
		// Keep first and last parts, shorten middle
		first := parts[0]
		last := parts[len(parts)-1]

		// If we have room, keep more context
		if len(first)+len(last)+5 <= maxLen {
			return first + "-...-" + last
		}
	}

	// Simple truncation with middle ellipsis
	if maxLen > 10 {
		half := (maxLen - 3) / 2
		return name[:half] + "..." + name[len(name)-half:]
	}
	return name[:maxLen-3] + "..."
}
