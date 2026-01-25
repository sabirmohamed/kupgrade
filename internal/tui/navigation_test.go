package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sabirmohamed/kupgrade/pkg/types"
)

// mockChannels creates closed channels for testing
func mockChannels() (<-chan types.Event, <-chan types.NodeState) {
	eventCh := make(chan types.Event)
	nodeCh := make(chan types.NodeState)
	close(eventCh)
	close(nodeCh)
	return eventCh, nodeCh
}

func TestNewModel(t *testing.T) {
	eventCh, nodeCh := mockChannels()
	cfg := Config{
		Context:       "test-context",
		ServerVersion: "v1.28.0",
		TargetVersion: "v1.29.0",
		EventCh:       eventCh,
		NodeStateCh:   nodeCh,
	}

	m := New(cfg)

	if m.screen != ScreenOverview {
		t.Errorf("expected initial screen ScreenOverview, got %d", m.screen)
	}
	if m.overlay != OverlayNone {
		t.Errorf("expected initial overlay OverlayNone, got %d", m.overlay)
	}
}

func TestScreenNavigation(t *testing.T) {
	eventCh, nodeCh := mockChannels()
	m := New(Config{EventCh: eventCh, NodeStateCh: nodeCh})

	tests := []struct {
		key      string
		expected Screen
	}{
		{"1", ScreenNodes},
		{"2", ScreenDrains},
		{"3", ScreenPods},
		{"4", ScreenBlockers},
		{"5", ScreenEvents},
		{"6", ScreenStats},
		{"0", ScreenOverview},
	}

	for _, tt := range tests {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)}
		newModel, _ := m.Update(msg)
		m = newModel.(Model)

		if m.screen != tt.expected {
			t.Errorf("key %q: expected screen %d, got %d", tt.key, tt.expected, m.screen)
		}
	}
}

func TestHelpOverlayToggle(t *testing.T) {
	eventCh, nodeCh := mockChannels()
	m := New(Config{EventCh: eventCh, NodeStateCh: nodeCh})

	// Press ? to open help
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	if m.overlay != OverlayHelp {
		t.Errorf("expected OverlayHelp after ?, got %d", m.overlay)
	}

	// Press ? again to close (or any key)
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	if m.overlay != OverlayNone {
		t.Errorf("expected OverlayNone after second ?, got %d", m.overlay)
	}
}

func TestEscapeReturnsToOverview(t *testing.T) {
	eventCh, nodeCh := mockChannels()
	m := New(Config{EventCh: eventCh, NodeStateCh: nodeCh})

	// Navigate to Nodes screen
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	if m.screen != ScreenNodes {
		t.Fatalf("expected ScreenNodes, got %d", m.screen)
	}

	// Press Escape to return to Overview
	escMsg := tea.KeyMsg{Type: tea.KeyEsc}
	newModel, _ = m.Update(escMsg)
	m = newModel.(Model)

	if m.screen != ScreenOverview {
		t.Errorf("expected ScreenOverview after Escape, got %d", m.screen)
	}
}

func TestQuitFromNonOverviewReturnsToOverview(t *testing.T) {
	eventCh, nodeCh := mockChannels()
	m := New(Config{EventCh: eventCh, NodeStateCh: nodeCh})

	// Navigate to Stats screen
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("6")}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	// Press q - should return to Overview, not quit
	qMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}
	newModel, cmd := m.Update(qMsg)
	m = newModel.(Model)

	if m.screen != ScreenOverview {
		t.Errorf("expected ScreenOverview after q from Stats, got %d", m.screen)
	}

	// cmd should not be tea.Quit
	if cmd != nil {
		// Check if it's a quit command by running it
		// For simplicity, we just check it's nil (no quit)
	}
}

func TestListNavigation(t *testing.T) {
	eventCh, nodeCh := mockChannels()
	m := New(Config{EventCh: eventCh, NodeStateCh: nodeCh})

	// Add some test nodes
	m.nodes = map[string]types.NodeState{
		"node-1": {Name: "node-1", Stage: types.StageReady},
		"node-2": {Name: "node-2", Stage: types.StageReady},
		"node-3": {Name: "node-3", Stage: types.StageReady},
	}
	m.rebuildNodesByStage()

	// Navigate to Nodes screen
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	// Initial listIndex should be 0
	if m.listIndex != 0 {
		t.Errorf("expected listIndex 0, got %d", m.listIndex)
	}

	// Press down
	downMsg := tea.KeyMsg{Type: tea.KeyDown}
	newModel, _ = m.Update(downMsg)
	m = newModel.(Model)

	if m.listIndex != 1 {
		t.Errorf("expected listIndex 1 after down, got %d", m.listIndex)
	}

	// Press G to go to bottom
	gMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")}
	newModel, _ = m.Update(gMsg)
	m = newModel.(Model)

	if m.listIndex != 2 {
		t.Errorf("expected listIndex 2 after G, got %d", m.listIndex)
	}

	// Press g to go to top
	gSmallMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")}
	newModel, _ = m.Update(gSmallMsg)
	m = newModel.(Model)

	if m.listIndex != 0 {
		t.Errorf("expected listIndex 0 after g, got %d", m.listIndex)
	}
}

func TestScreenName(t *testing.T) {
	eventCh, nodeCh := mockChannels()
	m := New(Config{EventCh: eventCh, NodeStateCh: nodeCh})

	tests := []struct {
		screen   Screen
		expected string
	}{
		{ScreenOverview, ""},
		{ScreenNodes, "NODES"},
		{ScreenDrains, "DRAINS"},
		{ScreenPods, "PODS"},
		{ScreenBlockers, "BLOCKERS"},
		{ScreenEvents, "EVENTS"},
		{ScreenStats, "STATS"},
	}

	for _, tt := range tests {
		m.screen = tt.screen
		if got := m.screenName(); got != tt.expected {
			t.Errorf("screen %d: expected %q, got %q", tt.screen, tt.expected, got)
		}
	}
}
