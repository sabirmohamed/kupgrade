package tui

import (
	"testing"
	"time"

	"github.com/sabirmohamed/kupgrade/pkg/types"
)

func TestIsAlphanumeric(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"abc123", true},
		{"abcdef", true},
		{"012345", true},
		{"", true},
		{"ABC", false},     // uppercase not accepted (hash detection only needs lowercase)
		{"abc-123", false}, // hyphen
		{"abc 123", false}, // space
		{"abc_123", false}, // underscore
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isAlphanumeric(tt.input); got != tt.want {
				t.Errorf("isAlphanumeric(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestAggregateEvents_Empty(t *testing.T) {
	result := aggregateEvents(nil)
	if result != nil {
		t.Errorf("aggregateEvents(nil) = %v, want nil", result)
	}
}

func TestAggregateEvents_SingleEvent(t *testing.T) {
	events := []types.Event{
		{Type: types.EventPodEvicted, Severity: types.SeverityWarning, Message: "[Evicted] pod-abc", Timestamp: time.Now()},
	}
	result := aggregateEvents(events)
	if len(result) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result))
	}
	if result[0].Count != 1 {
		t.Errorf("expected count 1, got %d", result[0].Count)
	}
	if result[0].Reason != "Evicted" {
		t.Errorf("expected reason 'Evicted', got %q", result[0].Reason)
	}
}

func TestAggregateEvents_GroupsBySameReason(t *testing.T) {
	now := time.Now()
	events := []types.Event{
		{Type: types.EventPodEvicted, Severity: types.SeverityWarning, Message: "[BackOff] pod-1", PodName: "pod-1", Timestamp: now},
		{Type: types.EventPodEvicted, Severity: types.SeverityWarning, Message: "[BackOff] pod-2", PodName: "pod-2", Timestamp: now.Add(time.Second)},
		{Type: types.EventPodEvicted, Severity: types.SeverityError, Message: "[BackOff] pod-3", PodName: "pod-3", Timestamp: now.Add(2 * time.Second)},
	}
	result := aggregateEvents(events)
	if len(result) != 1 {
		t.Fatalf("expected 1 group for same reason, got %d", len(result))
	}
	if result[0].Count != 3 {
		t.Errorf("expected count 3, got %d", result[0].Count)
	}
	// Severity should be worst case (error)
	if result[0].Severity != types.SeverityError {
		t.Errorf("expected severity error, got %s", result[0].Severity)
	}
}

func TestExtractReason(t *testing.T) {
	tests := []struct {
		msg  string
		want string
	}{
		{"[BackOff] restarting container", "BackOff"},
		{"[Evicted] pod-abc from node-1", "Evicted"},
		{"no bracket message", ""},
		{"[Incomplete", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			if got := extractReason(tt.msg); got != tt.want {
				t.Errorf("extractReason(%q) = %q, want %q", tt.msg, got, tt.want)
			}
		})
	}
}

func TestSeverityRank(t *testing.T) {
	if severityRank(types.SeverityError) <= severityRank(types.SeverityWarning) {
		t.Error("error should rank higher than warning")
	}
	if severityRank(types.SeverityWarning) <= severityRank(types.SeverityInfo) {
		t.Error("warning should rank higher than info")
	}
}
