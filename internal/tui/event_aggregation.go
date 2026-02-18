package tui

import (
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

// aggregateEvents collapses similar events into aggregated groups.
// Groups by Event.Reason field (populated by watcher), falling back to EventType.
func aggregateEvents(events []types.Event) []AggregatedEvent {
	if len(events) == 0 {
		return nil
	}

	groups := make(map[string]*AggregatedEvent)
	order := make([]string, 0) // preserve insertion order

	for _, e := range events {
		key := eventAggregationKey(e)

		if existing, ok := groups[key]; ok {
			existing.Count++
			if e.Timestamp.After(existing.Timestamp) {
				existing.Timestamp = e.Timestamp
			}

			resource := extractResource(e)
			if resource != "" && !containsString(existing.Resources, resource) {
				existing.Resources = append(existing.Resources, resource)
			}

			if severityRank(e.Severity) > severityRank(existing.Severity) {
				existing.Severity = e.Severity
			}

			if existing.NodeName != "" && existing.NodeName != e.NodeName {
				existing.NodeName = ""
			}
		} else {
			resource := extractResource(e)
			resources := []string{}
			if resource != "" {
				resources = append(resources, resource)
			}

			groups[key] = &AggregatedEvent{
				Reason:      key,
				Severity:    e.Severity,
				Timestamp:   e.Timestamp,
				Count:       1,
				Resources:   resources,
				NodeName:    e.NodeName,
				SampleEvent: e,
			}
			order = append(order, key)
		}
	}

	// Sort by severity (worst first), then by timestamp (newest first)
	result := make([]AggregatedEvent, 0, len(groups))
	for _, key := range order {
		result = append(result, *groups[key])
	}

	sort.SliceStable(result, func(i, j int) bool {
		ri := severityRank(result[i].Severity)
		rj := severityRank(result[j].Severity)
		if ri != rj {
			return ri > rj
		}
		return result[i].Timestamp.After(result[j].Timestamp)
	})

	return result
}

// eventAggregationKey returns the grouping key for an event.
// Uses Event.Reason (populated by watcher for K8s events), then bracket-parsed reason,
// then falls back to EventType string.
func eventAggregationKey(e types.Event) string {
	if e.Reason != "" {
		return e.Reason
	}
	if reason := extractReason(e.Message); reason != "" {
		return reason
	}
	return string(e.Type)
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
		name := e.PodName
		if idx := strings.LastIndex(name, "-"); idx > 0 {
			suffix := name[idx+1:]
			if len(suffix) >= 5 && isAlphanumeric(suffix) {
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

// isAlphanumeric checks if string is alphanumeric (lowercase + digits only)
func isAlphanumeric(s string) bool {
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
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
