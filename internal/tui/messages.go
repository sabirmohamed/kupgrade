package tui

import (
	"github.com/sabirmohamed/kupgrade/internal/kube"
	"github.com/sabirmohamed/kupgrade/pkg/types"
)

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

// DescribeMsg carries the result of an async kubectl-describe call
type DescribeMsg struct {
	Content string
	Err     error
}

// TickMsg triggers periodic updates
type TickMsg struct{}

// NodeMetricsMsg carries node CPU/memory metrics from the metrics-server
type NodeMetricsMsg map[string]kube.NodeMetrics

// metricsRefreshMsg triggers the next metrics fetch cycle
type metricsRefreshMsg struct{}

// cpVersionMsg carries the polled control plane version
type cpVersionMsg struct {
	Version string
}

// cpVersionCheckMsg triggers the next CP version poll
type cpVersionCheckMsg struct{}
