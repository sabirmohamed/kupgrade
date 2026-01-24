package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Update handles messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case EventMsg:
		m.eventCount++
		m.events = append(m.events, msg.Event)

		// Keep only last maxEvents
		if len(m.events) > maxEvents {
			m.events = m.events[len(m.events)-maxEvents:]
		}

		return m, waitForEvent(m.eventCh)

	case ErrorMsg:
		if !msg.Recoverable {
			m.fatalError = msg.Err
			return m, tea.Quit
		}
		return m, nil

	case TickMsg:
		return m, tick()
	}

	return m, nil
}
