package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/sabirmohamed/kupgrade/pkg/types"
)

// Event table column indices
const (
	evColTime    = 0
	evColSev     = 1
	evColReason  = 2
	evColTarget  = 3
	evColMessage = 4
)

// renderEventsScreen renders the full events screen
func (m Model) renderEventsScreen() string {
	w := m.mainWidth()

	counts := m.stageCounts()
	tabBar := m.renderTabBar(counts)

	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")

	events := m.sortedEvents()
	aggregated := aggregateEvents(events)

	if len(aggregated) == 0 {
		b.WriteString(footerDescStyle.Render("  Waiting for events..."))
	} else {
		b.WriteString(m.renderEventsTable(aggregated, events))
	}

	panelBody := b.String()

	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorSelected).
		Padding(0, 1).
		Width(w - 2).
		Render(panelBody)

	// Status bar + key hints (pinned to bottom)
	statusBar := m.renderStatusBar(w)
	keyHints := m.renderKeyHints(w)
	footer := lipgloss.JoinVertical(lipgloss.Left, statusBar, keyHints)

	return m.placeContentWithFooter(lipgloss.JoinVertical(lipgloss.Left, tabBar, panel), footer)
}

// renderEventsTable renders the aggregated events as a lipgloss/table.
// m.listIndex is a visual row index (spanning both group headers and expanded sub-rows).
func (m Model) renderEventsTable(aggregated []AggregatedEvent, allEvents []types.Event) string {
	colWidths := computeColumnWidths(m.tableWidth(), []columnLayout{
		{Fixed: 10}, // TIME
		{Fixed: 5},  // SEV
		{Fixed: 20}, // REASON
		{Weight: 3}, // TARGET — flexible, ~37% of remaining
		{Weight: 5}, // MESSAGE — flexible, ~63% of remaining
	})

	// Dynamic message truncation based on available column width (minus cell padding)
	msgMaxLen := colWidths[evColMessage] - 4
	if msgMaxLen < 40 {
		msgMaxLen = 40
	}

	var rows [][]string
	var rowSeverities []types.Severity // parallel slice for styling
	var rowIsExpanded []bool           // parallel slice: is this an expanded sub-row?

	for _, ag := range aggregated {
		rows = append(rows, buildEventRow(ag, msgMaxLen))
		rowSeverities = append(rowSeverities, ag.Severity)
		rowIsExpanded = append(rowIsExpanded, false)

		// If this group is expanded, add individual event rows
		if ag.Count > 1 && m.expandedGroup == ag.Reason {
			for _, e := range allEvents {
				if eventAggregationKey(e) != ag.Reason {
					continue
				}
				rows = append(rows, buildExpandedEventRow(e, msgMaxLen))
				rowSeverities = append(rowSeverities, e.Severity)
				rowIsExpanded = append(rowIsExpanded, true)
			}
		}
	}

	// m.listIndex is the visual row index directly
	selectedVisualRow := m.listIndex
	if selectedVisualRow >= len(rows) {
		selectedVisualRow = len(rows) - 1
	}

	totalRows := len(rows)
	// Overhead: tabBar(1) + panel borders(2) + header(1) + blank(1) +
	// table header+border(2) + hint(1) + statusBar(1) + keyHints(1) + buffer(2) = 12
	visibleRows := m.height - 12
	if visibleRows < 5 {
		visibleRows = 5
	}

	t := table.New().
		Headers("TIME", "SEV", "REASON", "TARGET", "MESSAGE").
		Rows(rows...).
		Width(m.tableWidth()).
		Border(lipgloss.RoundedBorder()).
		BorderColumn(false).
		BorderRow(false).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderHeader(true).
		BorderStyle(tableBorderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := m.eventCellStyle(row, col, selectedVisualRow, rowSeverities, rowIsExpanded)
			if col < len(colWidths) {
				s = s.Width(colWidths[col])
			}
			return s
		})

	// Always constrain height for consistent layout (prevents header/footer flicker)
	scrollOffset := calcScrollOffset(selectedVisualRow, visibleRows, totalRows)
	t = t.Height(visibleRows).Offset(scrollOffset)

	rendered := t.String()

	// Footer hint
	var hint string
	if totalRows > visibleRows {
		hint = fmt.Sprintf(" %d/%d  •  d describe  •  e expand group", m.listIndex+1, totalRows)
	} else {
		hint = fmt.Sprintf(" %d events  •  d describe  •  e expand group", len(allEvents))
	}
	rendered += "\n" + footerDescStyle.Render(hint)

	return rendered
}

// buildEventRow builds a table row for an aggregated event.
// msgMaxLen is the dynamic message truncation length based on column width.
func buildEventRow(ag AggregatedEvent, msgMaxLen int) []string {
	ts := ag.Timestamp.Format("15:04:05")
	icon := severityIcon(ag.Severity)
	reason := ag.Reason
	target := extractTarget(ag)

	// Strip redundant [Reason] prefix since REASON column already shows it
	cleanMsg := truncateString(stripBracketPrefix(ag.SampleEvent.Message), msgMaxLen)

	var msg string
	if ag.Count > 1 {
		countStr := eventCountStyle.Render(fmt.Sprintf("×%d", ag.Count))
		msg = countStr + "  " + cleanMsg
	} else {
		msg = cleanMsg
	}

	return []string{ts, icon, reason, target, msg}
}

// buildExpandedEventRow builds a sub-row for an individual event within an expanded group.
func buildExpandedEventRow(e types.Event, msgMaxLen int) []string {
	ts := e.Timestamp.Format("15:04:05")
	target := e.NodeName
	if target == "" {
		target = e.PodName
	}
	if target == "" {
		target = "-"
	}
	// Strip redundant [Reason] prefix — REASON is shown in the group header
	msg := truncateString(stripBracketPrefix(e.Message), msgMaxLen)
	return []string{ts, " ", "", target, msg}
}

// severityIcon returns the icon string for a severity level (unstyled)
func severityIcon(s types.Severity) string {
	switch s {
	case types.SeverityError:
		return errorIcon
	case types.SeverityWarning:
		return warningIcon
	default:
		return infoIcon
	}
}

// extractTarget extracts the primary target name from an aggregated event.
// When a group spans multiple distinct targets, shows "(N targets)" instead of
// a single misleading sample name.
func extractTarget(ag AggregatedEvent) string {
	if len(ag.Resources) > 1 {
		// Use context-aware label: "nodes" for node events, "targets" otherwise
		label := "targets"
		if ag.SampleEvent.NodeName != "" && ag.SampleEvent.PodName == "" {
			label = "nodes"
		}
		return fmt.Sprintf("(%d %s)", len(ag.Resources), label)
	}
	e := ag.SampleEvent
	if e.NodeName != "" {
		return e.NodeName
	}
	if e.PodName != "" {
		return e.PodName
	}
	if len(ag.Resources) == 1 {
		return ag.Resources[0]
	}
	return "-"
}

// eventCellStyle computes the style for a cell in the events table
func (m Model) eventCellStyle(row, col, selectedVisualRow int, rowSeverities []types.Severity, rowIsExpanded []bool) lipgloss.Style {
	style := lipgloss.NewStyle().Padding(0, 1)

	if row == table.HeaderRow {
		return style.Foreground(colorTextMuted).Bold(true)
	}

	if row >= len(rowSeverities) {
		return style
	}

	severity := rowSeverities[row]
	isExpanded := rowIsExpanded[row]

	// Selected row (using visual row index, not group index)
	if row == selectedVisualRow {
		style = style.Background(colorSelected).Foreground(colorTextBold)
		return style
	}

	// Expanded sub-rows are dimmed
	if isExpanded {
		style = style.Foreground(colorTextDim)
		if col == evColSev {
			return style
		}
		return style
	}

	// Column-specific styling
	switch col {
	case evColTime:
		style = style.Foreground(colorTextDim)
	case evColSev:
		switch severity {
		case types.SeverityError:
			style = style.Foreground(colorError)
		case types.SeverityWarning:
			style = style.Foreground(colorWarning)
		default:
			style = style.Foreground(colorInfo)
		}
	case evColReason:
		switch severity {
		case types.SeverityError:
			style = style.Foreground(colorError).Bold(true)
		case types.SeverityWarning:
			style = style.Foreground(colorWarning)
		default:
			style = style.Foreground(colorTextMuted)
		}
	case evColTarget:
		style = style.Foreground(colorTextBold)
	case evColMessage:
		style = style.Foreground(colorText)
	}

	return style
}

// eventVisualRowCount returns the total number of visual rows in the events table,
// including expanded sub-rows.
func (m Model) eventVisualRowCount() int {
	events := m.sortedEvents()
	aggregated := aggregateEvents(events)
	count := len(aggregated)
	for _, ag := range aggregated {
		if ag.Count > 1 && m.expandedGroup == ag.Reason {
			count += ag.Count
		}
	}
	return count
}

// eventGroupForVisualRow maps a visual row index to the aggregated group index.
// Returns the group index and whether the row is an expanded sub-row (not the header).
func (m Model) eventGroupForVisualRow(visualRow int, aggregated []AggregatedEvent, allEvents []types.Event) (groupIdx int, isSubRow bool) {
	currentRow := 0
	for i, ag := range aggregated {
		if currentRow == visualRow {
			return i, false
		}
		currentRow++
		if ag.Count > 1 && m.expandedGroup == ag.Reason {
			subCount := 0
			for _, e := range allEvents {
				if eventAggregationKey(e) == ag.Reason {
					subCount++
				}
			}
			if visualRow < currentRow+subCount {
				return i, true
			}
			currentRow += subCount
		}
	}
	// Fallback: last group
	if len(aggregated) > 0 {
		return len(aggregated) - 1, false
	}
	return 0, false
}

// eventAtVisualRow returns the specific event at a visual row position.
// For group header rows, returns the group's sample event.
// For expanded sub-rows, returns the individual event.
func (m Model) eventAtVisualRow(visualRow int, aggregated []AggregatedEvent, allEvents []types.Event) types.Event {
	currentRow := 0
	for _, ag := range aggregated {
		if currentRow == visualRow {
			return ag.SampleEvent
		}
		currentRow++
		if ag.Count > 1 && m.expandedGroup == ag.Reason {
			for _, e := range allEvents {
				if eventAggregationKey(e) == ag.Reason {
					if currentRow == visualRow {
						return e
					}
					currentRow++
				}
			}
		}
	}
	// Fallback
	if len(allEvents) > 0 {
		return allEvents[len(allEvents)-1]
	}
	return types.Event{}
}

// eventGroupHeaderRow returns the visual row index of a group header.
func eventGroupHeaderRow(groupIdx int, aggregated []AggregatedEvent, allEvents []types.Event, expandedGroup string) int {
	currentRow := 0
	for i, ag := range aggregated {
		if i == groupIdx {
			return currentRow
		}
		currentRow++
		if ag.Count > 1 && expandedGroup == ag.Reason {
			for _, e := range allEvents {
				if eventAggregationKey(e) == ag.Reason {
					currentRow++
				}
			}
		}
	}
	return currentRow
}

// stripBracketPrefix removes a leading "[Reason] " prefix from a message.
// e.g. "[Unhealthy] probe failed" → "probe failed"
func stripBracketPrefix(msg string) string {
	if len(msg) > 0 && msg[0] == '[' {
		end := strings.Index(msg, "] ")
		if end > 0 && end+2 < len(msg) {
			return msg[end+2:]
		}
	}
	return msg
}
