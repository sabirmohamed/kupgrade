package types

import "time"

// EventType represents the category of upgrade event
type EventType string

const (
	// Node events
	EventNodeCordon   EventType = "NODE_CORDON"
	EventNodeUncordon EventType = "NODE_UNCORDON"
	EventNodeReady    EventType = "NODE_READY"
	EventNodeNotReady EventType = "NODE_NOTREADY"
	EventNodeVersion  EventType = "NODE_VERSION"

	// Pod events
	EventPodEvicted   EventType = "POD_EVICTED"
	EventPodScheduled EventType = "POD_SCHEDULED"
	EventPodReady     EventType = "POD_READY"
	EventPodFailed    EventType = "POD_FAILED"
	EventPodDeleted   EventType = "POD_DELETED"

	// K8s events
	EventK8sWarning EventType = "K8S_WARNING"
	EventK8sError   EventType = "K8S_ERROR"
	EventK8sNormal  EventType = "K8S_NORMAL"

	// Migration
	EventMigration EventType = "MIGRATION"
)

// Severity indicates the importance level of an event
type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// Event represents a single upgrade-relevant occurrence
type Event struct {
	Type      EventType
	Severity  Severity
	Timestamp time.Time
	Message   string

	// Optional context
	NodeName  string
	PodName   string
	Namespace string
	Reason    string // K8s event reason
}

// SeverityForType returns the default severity for an event type
func SeverityForType(t EventType) Severity {
	switch t {
	case EventNodeCordon, EventNodeNotReady, EventPodEvicted, EventK8sWarning:
		return SeverityWarning
	case EventPodFailed, EventK8sError:
		return SeverityError
	default:
		return SeverityInfo
	}
}
