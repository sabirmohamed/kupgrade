package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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

// findEventByKey searches the filtered events for one matching the composite key.
func (m Model) findEventByKey(key string) (types.Event, bool) {
	for _, e := range m.filteredEvents() {
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
		m.detailViewport.SetContent(m.renderEventDetailContent())
		m.detailViewport.GotoTop()
		return nil
	}

	// Async: show loading state and fire describe command
	m.detailViewport.SetContent("Loading...")
	m.detailViewport.GotoTop()
	return m.fetchDescribe()
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
