package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sabirmohamed/kupgrade/pkg/types"
)

const maxEvents = 50

// Model is the Bubble Tea model for kupgrade TUI
type Model struct {
	ctx           context.Context
	eventCh       <-chan types.Event
	events        []types.Event
	contextName   string
	serverVersion string
	targetVersion string
	eventCount    int
	fatalError    error
	width         int
	height        int
}

// New creates a new TUI model
func New(ctx context.Context, eventCh <-chan types.Event, contextName, serverVersion, targetVersion string) Model {
	return Model{
		ctx:           ctx,
		eventCh:       eventCh,
		events:        make([]types.Event, 0, maxEvents),
		contextName:   contextName,
		serverVersion: serverVersion,
		targetVersion: targetVersion,
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		waitForEvent(m.eventCh),
		tick(),
	)
}

// waitForEvent waits for the next event from the channel
func waitForEvent(eventCh <-chan types.Event) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-eventCh
		if !ok {
			return nil
		}
		return EventMsg{Event: event}
	}
}

// tick sends periodic tick messages for time updates
func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return TickMsg{}
	})
}
