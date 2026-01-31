package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sabirmohamed/kupgrade/pkg/types"
)

// AggregatedEvent represents multiple similar events collapsed into one
type AggregatedEvent struct {
	Reason      string
	Severity    types.Severity
	Timestamp   time.Time   // Most recent timestamp
	Count       int         // Number of occurrences
	Resources   []string    // Affected resources (pods, nodes)
	NodeName    string      // Node if all events are for same node
	SampleEvent types.Event // Representative event for details
}

// aggregationWindow defines how close events must be to aggregate (30 seconds)
const aggregationWindow = 30 * time.Second

// aggregateEvents collapses similar events into aggregated groups
func aggregateEvents(events []types.Event) []AggregatedEvent {
	if len(events) == 0 {
		return nil
	}

	// Group by reason
	groups := make(map[string]*AggregatedEvent)

	for _, e := range events {
		reason := extractReason(e.Message)
		if reason == "" {
			reason = string(e.Type)
		}

		key := reason

		if existing, ok := groups[key]; ok {
			// Add to existing group
			existing.Count++
			if e.Timestamp.After(existing.Timestamp) {
				existing.Timestamp = e.Timestamp
			}

			// Track affected resource
			resource := extractResource(e)
			if resource != "" && !containsString(existing.Resources, resource) {
				existing.Resources = append(existing.Resources, resource)
			}

			// Update severity to worst case
			if severityRank(e.Severity) > severityRank(existing.Severity) {
				existing.Severity = e.Severity
			}

			// Clear node if events span multiple nodes
			if existing.NodeName != "" && existing.NodeName != e.NodeName {
				existing.NodeName = ""
			}
		} else {
			// Create new group
			resource := extractResource(e)
			resources := []string{}
			if resource != "" {
				resources = append(resources, resource)
			}

			groups[key] = &AggregatedEvent{
				Reason:      reason,
				Severity:    e.Severity,
				Timestamp:   e.Timestamp,
				Count:       1,
				Resources:   resources,
				NodeName:    e.NodeName,
				SampleEvent: e,
			}
		}
	}

	// Convert to slice and sort by timestamp (most recent first)
	result := make([]AggregatedEvent, 0, len(groups))
	for _, ag := range groups {
		result = append(result, *ag)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.After(result[j].Timestamp)
	})

	return result
}

// extractReason extracts the reason from event message like "[BackOff] pod..."
func extractReason(msg string) string {
	if len(msg) > 0 && msg[0] == '[' {
		end := strings.Index(msg, "]")
		if end > 1 {
			return msg[1:end]
		}
	}
	return ""
}

// extractResource extracts the resource name from event
func extractResource(e types.Event) string {
	if e.PodName != "" && e.PodName != "-" {
		// Shorten pod name if it has a hash suffix
		name := e.PodName
		if idx := strings.LastIndex(name, "-"); idx > 0 {
			suffix := name[idx+1:]
			// If suffix looks like a hash (5+ chars, alphanumeric)
			if len(suffix) >= 5 && isAlphanumeric(suffix) {
				// Get deployment/statefulset name
				prefix := name[:idx]
				if idx2 := strings.LastIndex(prefix, "-"); idx2 > 0 {
					suffix2 := prefix[idx2+1:]
					if len(suffix2) >= 5 && isAlphanumeric(suffix2) {
						return prefix[:idx2]
					}
				}
				return prefix
			}
		}
		return name
	}
	if e.NodeName != "" && e.NodeName != "-" {
		return e.NodeName
	}
	return ""
}

// isAlphanumeric checks if string is alphanumeric
func isAlphanumeric(s string) bool {
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// containsString checks if slice contains string
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// severityRank returns numeric rank for severity comparison
func severityRank(s types.Severity) int {
	switch s {
	case types.SeverityError:
		return 3
	case types.SeverityWarning:
		return 2
	case types.SeverityInfo:
		return 1
	default:
		return 0
	}
}

// formatAggregatedEvent formats an aggregated event for display
func (a AggregatedEvent) Format(icon string) string {
	if a.Count == 1 {
		// Single event - show original message
		return a.SampleEvent.Message
	}

	// Multiple events - show aggregated format
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[%s] ×%d ", a.Reason, a.Count))

	// Group resources by type
	if len(a.Resources) > 0 {
		if len(a.Resources) <= 3 {
			sb.WriteString(strings.Join(a.Resources, ", "))
		} else {
			sb.WriteString(fmt.Sprintf("%s, %s +%d more",
				a.Resources[0], a.Resources[1], len(a.Resources)-2))
		}
	}

	return sb.String()
}

// formatResourceList formats the expanded resource list
func (a AggregatedEvent) FormatResourceList() string {
	if len(a.Resources) == 0 {
		return ""
	}
	return "→ " + strings.Join(a.Resources, ", ")
}
