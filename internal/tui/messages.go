package tui

import "github.com/sabirmohamed/kupgrade/pkg/types"

// EventMsg wraps an event for the TUI
type EventMsg struct {
	Event types.Event
}

// NodeUpdateMsg indicates node state changed
type NodeUpdateMsg struct {
	Node types.NodeState
}

// PodUpdateMsg indicates pod state changed
type PodUpdateMsg struct {
	Pod types.PodState
}

// BlockerMsg indicates a blocker state changed
type BlockerMsg struct {
	Blocker types.Blocker
}

// ErrorMsg indicates an error occurred
type ErrorMsg struct {
	Err         error
	Recoverable bool
}

// TickMsg triggers periodic updates
type TickMsg struct{}

// SpinnerMsg triggers spinner animation
type SpinnerMsg struct{}
