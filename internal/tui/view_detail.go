package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sabirmohamed/kupgrade/pkg/types"
)

// renderDetailOverlay renders the full-screen detail overlay for node/pod/event
func (m Model) renderDetailOverlay() string {
	title := m.detailTitle()

	vpView := m.detailViewport.View()

	// Scroll indicator
	scrollPercent := ""
	if m.detailViewport.TotalLineCount() > m.detailViewport.VisibleLineCount() {
		scrollPercent = fmt.Sprintf(" %d%%", int(m.detailViewport.ScrollPercent()*100))
	}

	footer := footerDescStyle.Render("Esc close  j/k scroll" + scrollPercent)

	content := title + "\n" + vpView + "\n" + footer

	// Use most of the terminal width/height
	w := m.width - 6
	h := m.height - 4
	if w < 40 {
		w = 40
	}
	if h < 10 {
		h = 10
	}

	return overlayStyle.Width(w).Height(h).Render(content)
}

// detailTitle returns the styled title for the current detail type
func (m Model) detailTitle() string {
	switch m.detailType {
	case DetailNode:
		return overlayTitleStyle.Render("Node: " + m.detailKey)
	case DetailPod:
		return overlayTitleStyle.Render("Pod: " + m.detailKey)
	case DetailEvent:
		return overlayTitleStyle.Render("Event")
	default:
		return overlayTitleStyle.Render("Detail")
	}
}

// renderEventDetailContent renders event detail for the overlay
func (m Model) renderEventDetailContent() string {
	event, ok := m.findEventByKey(m.detailKey)
	if !ok {
		return "Event not found"
	}

	var b strings.Builder

	b.WriteString("EVENT\n")
	b.WriteString(fmt.Sprintf("  Type:          %s\n", string(event.Type)))
	b.WriteString(fmt.Sprintf("  Severity:      %s\n", string(event.Severity)))
	b.WriteString(fmt.Sprintf("  Timestamp:     %s\n", event.Timestamp.Format("15:04:05")))

	b.WriteString("\nMESSAGE\n")
	b.WriteString("  " + event.Message + "\n")

	b.WriteString("\nCONTEXT\n")
	if event.NodeName != "" {
		b.WriteString(fmt.Sprintf("  Node:          %s\n", event.NodeName))
	}
	if event.Namespace != "" {
		b.WriteString(fmt.Sprintf("  Namespace:     %s\n", event.Namespace))
	}
	if event.PodName != "" {
		b.WriteString(fmt.Sprintf("  Pod:           %s\n", event.PodName))
	}
	if event.Reason != "" {
		b.WriteString(fmt.Sprintf("  Reason:        %s\n", event.Reason))
	}

	return b.String()
}

// eventKey builds a composite key for an event to avoid stale-index issues.
func eventKey(e types.Event) string {
	return e.Timestamp.Format("15:04:05.000000") + ":" + e.Message
}

// findEventByKey searches the events for one matching the composite key.
func (m Model) findEventByKey(key string) (types.Event, bool) {
	for _, e := range m.events {
		if eventKey(e) == key {
			return e, true
		}
	}
	return types.Event{}, false
}

// openDetail opens the detail overlay for the given resource.
func (m *Model) openDetail(dt DetailType, key string) tea.Cmd {
	m.detailType = dt
	m.detailKey = key
	m.overlay = OverlayDetail
	m.resizeDetailViewport()

	if dt == DetailEvent {
		// K8s events use "namespace/eventName" key → kubectl describe
		// Internal events use "timestamp:message" key → in-memory summary
		if strings.Contains(key, "/") {
			m.detailViewport.SetContent("Loading...")
			m.detailViewport.GotoTop()
			return m.fetchDescribe()
		}
		m.detailViewport.SetContent(colorizeDescribe(m.renderEventDetailContent()))
		m.detailViewport.GotoTop()
		return nil
	}

	// Async: show loading state and fire describe command
	m.detailViewport.SetContent("Loading...")
	m.detailViewport.GotoTop()
	return m.fetchDescribe()
}

// Describe output colorization styles
var (
	describeSectionStyle = lipgloss.NewStyle().Foreground(colorReimaging).Bold(true)   // cyan bold
	describeKeyStyle     = lipgloss.NewStyle().Foreground(colorInfo)                   // blue
	describeTrueStyle    = lipgloss.NewStyle().Foreground(colorSuccess)                // green
	describeFalseStyle   = lipgloss.NewStyle().Foreground(colorError)                  // red
	describeWarningStyle = lipgloss.NewStyle().Foreground(colorWarning)                // yellow
	describeDimStyle     = lipgloss.NewStyle().Foreground(colorTextDim)                // dim
	describeValueStyle   = lipgloss.NewStyle().Foreground(colorText)                   // default text
	describeNoneStyle    = lipgloss.NewStyle().Foreground(colorTextMuted).Italic(true) // <none>/<nil>
)

// colorizeDescribe applies syntax highlighting to kubectl describe output.
// Parses line-by-line: section headers → cyan bold, keys → blue, True/False → green/red.
func colorizeDescribe(text string) string {
	lines := strings.Split(text, "\n")
	result := make([]string, len(lines))

	inEventsTable := false

	for i, line := range lines {
		trimmed := strings.TrimRight(line, " ")

		// Blank lines
		if trimmed == "" {
			result[i] = ""
			inEventsTable = false
			continue
		}

		// Section header: no leading whitespace, ends with ":"
		// e.g. "Conditions:", "Volumes:", "Events:", "Containers:"
		if len(trimmed) > 1 && trimmed[0] != ' ' && trimmed[0] != '\t' && trimmed[len(trimmed)-1] == ':' {
			result[i] = describeSectionStyle.Render(trimmed)
			if trimmed == "Events:" {
				inEventsTable = true
			}
			continue
		}

		// Events table header line (underscores: "  ----  ------")
		if inEventsTable && isEventsTableDivider(trimmed) {
			result[i] = describeDimStyle.Render(trimmed)
			continue
		}

		// Events table rows: Warning lines get yellow treatment
		if inEventsTable && strings.Contains(trimmed, "Warning") {
			result[i] = describeWarningStyle.Render(trimmed)
			continue
		}

		// Key: Value line (leading whitespace, then Key: Value)
		if colonIdx := findKeyColon(trimmed); colonIdx > 0 {
			key := trimmed[:colonIdx+1]   // includes the ":"
			value := trimmed[colonIdx+1:] // everything after ":"

			coloredKey := describeKeyStyle.Render(key)
			coloredValue := colorizeValue(value)
			result[i] = coloredKey + coloredValue
			continue
		}

		// Default: plain text
		result[i] = describeValueStyle.Render(trimmed)
	}

	return strings.Join(result, "\n")
}

// findKeyColon finds the colon in a "Key: Value" pattern.
// Returns -1 if the line doesn't match the pattern.
// Handles indented lines like "  Name:         foo" and "  Reason:  Error".
func findKeyColon(line string) int {
	// Skip leading whitespace
	start := 0
	for start < len(line) && (line[start] == ' ' || line[start] == '\t') {
		start++
	}
	// Find first colon after a word
	for i := start; i < len(line); i++ {
		if line[i] == ':' {
			// Must have at least one word char before the colon
			if i > start {
				return i
			}
			return -1
		}
		// Key part: allow letters, digits, hyphens, dots, underscores, slashes
		c := line[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') &&
			c != '-' && c != '.' && c != '_' && c != '/' && c != ' ' {
			return -1
		}
	}
	return -1
}

// colorizeValue applies color to the value part of a key-value line.
func colorizeValue(value string) string {
	trimmed := strings.TrimSpace(value)

	switch trimmed {
	case "True":
		return strings.Replace(value, "True", describeTrueStyle.Render("True"), 1)
	case "False":
		return strings.Replace(value, "False", describeFalseStyle.Render("False"), 1)
	case "<none>", "<nil>", "<unset>":
		return strings.Replace(value, trimmed, describeNoneStyle.Render(trimmed), 1)
	}

	return describeValueStyle.Render(value)
}

// isEventsTableDivider returns true for the dashed separator line in the Events table.
func isEventsTableDivider(line string) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 3 {
		return false
	}
	for _, c := range trimmed {
		if c != '-' && c != ' ' {
			return false
		}
	}
	return true
}

// resizeDetailViewport sets viewport dimensions for the detail overlay
func (m *Model) resizeDetailViewport() {
	w := m.width - 10 // overlay border + padding
	h := m.height - 8 // title + footer + border + padding
	if w < 40 {
		w = 40
	}
	if h < 5 {
		h = 5
	}
	m.detailViewport.Width = w
	m.detailViewport.Height = h
}
