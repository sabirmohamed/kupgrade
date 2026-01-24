package tui

import "github.com/sabirmohamed/kupgrade/pkg/types"

// EventMsg wraps an event for the TUI
type EventMsg struct {
	Event types.Event
}

// ErrorMsg indicates an error occurred
type ErrorMsg struct {
	Err         error
	Recoverable bool
}

// TickMsg triggers periodic updates
type TickMsg struct{}
