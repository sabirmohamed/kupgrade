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

func TestEventAggregationKey(t *testing.T) {
	tests := []struct {
		name string
		e    types.Event
		want string
	}{
		{
			name: "uses Reason field first",
			e:    types.Event{Reason: "FailedScheduling", Message: "[FailedScheduling] pod-abc", Type: types.EventK8sWarning},
			want: "FailedScheduling",
		},
		{
			name: "falls back to bracket reason",
			e:    types.Event{Message: "[BackOff] pod-abc", Type: types.EventPodEvicted},
			want: "BackOff",
		},
		{
			name: "falls back to EventType",
			e:    types.Event{Message: "Node vmss000001 cordoned", Type: types.EventNodeCordon},
			want: "NODE_CORDON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := eventAggregationKey(tt.e)
			if got != tt.want {
				t.Errorf("eventAggregationKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAggregateEvents_SeveritySorted(t *testing.T) {
	now := time.Now()
	events := []types.Event{
		{Type: types.EventK8sNormal, Severity: types.SeverityInfo, Message: "[RegisteredNode] vmss000001", Reason: "RegisteredNode", Timestamp: now},
		{Type: types.EventK8sWarning, Severity: types.SeverityWarning, Message: "[FailedScheduling] pod-1", Reason: "FailedScheduling", Timestamp: now},
		{Type: types.EventPodFailed, Severity: types.SeverityError, Message: "Pod default/crash-pod failed", Timestamp: now},
	}

	result := aggregateEvents(events)
	if len(result) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(result))
	}

	// Errors should be first
	if result[0].Severity != types.SeverityError {
		t.Errorf("first group severity = %s, want error", result[0].Severity)
	}
	if result[1].Severity != types.SeverityWarning {
		t.Errorf("second group severity = %s, want warning", result[1].Severity)
	}
	if result[2].Severity != types.SeverityInfo {
		t.Errorf("third group severity = %s, want info", result[2].Severity)
	}
}

func TestAggregateEvents_GroupsByReasonField(t *testing.T) {
	now := time.Now()
	events := []types.Event{
		{Type: types.EventK8sWarning, Severity: types.SeverityWarning, Reason: "FailedScheduling",
			Message: "[FailedScheduling] pod-1: 0/9 nodes", PodName: "pod-1", Timestamp: now},
		{Type: types.EventK8sWarning, Severity: types.SeverityWarning, Reason: "FailedScheduling",
			Message: "[FailedScheduling] pod-2: 0/9 nodes", PodName: "pod-2", Timestamp: now.Add(time.Second)},
		{Type: types.EventK8sWarning, Severity: types.SeverityWarning, Reason: "Unhealthy",
			Message: "[Unhealthy] pod-3: probe failed", PodName: "pod-3", Timestamp: now},
	}

	result := aggregateEvents(events)
	if len(result) != 2 {
		t.Fatalf("expected 2 groups (FailedScheduling + Unhealthy), got %d", len(result))
	}

	// Find FailedScheduling group
	var fsGroup *AggregatedEvent
	for i := range result {
		if result[i].Reason == "FailedScheduling" {
			fsGroup = &result[i]
			break
		}
	}
	if fsGroup == nil {
		t.Fatal("FailedScheduling group not found")
	}
	if fsGroup.Count != 2 {
		t.Errorf("FailedScheduling count = %d, want 2", fsGroup.Count)
	}
}

// testUpgradeEvents returns a realistic set of events from an AKS upgrade
func testUpgradeEvents() []types.Event {
	now := time.Now()
	return []types.Event{
		// Error — only POD_FAILED is error severity (K8s events are Warning or Normal, never Error)
		{Type: types.EventPodFailed, Severity: types.SeverityError, Timestamp: now.Add(-45 * time.Second),
			Message: "Pod default/crash-pod failed on vmss000001", PodName: "crash-pod",
			Namespace: "default", NodeName: "vmss000001"},
		// Warnings — VolumeFailedDelete is a K8s Warning event, not Error
		{Type: types.EventK8sWarning, Severity: types.SeverityWarning, Timestamp: now.Add(-30 * time.Second),
			Message: "[VolumeFailedDelete] pvc-adc61c18: DeleteFileShare failed", Reason: "VolumeFailedDelete",
			PodName: "pvc-adc61c18", Namespace: "default"},
		// Warnings — 3 FailedScheduling
		{Type: types.EventK8sWarning, Severity: types.SeverityWarning, Timestamp: now.Add(-10 * time.Second),
			Message: "[FailedScheduling] retina-agent-djvjv: 0/9 nodes", Reason: "FailedScheduling",
			PodName: "retina-agent-djvjv", Namespace: "kube-system"},
		{Type: types.EventK8sWarning, Severity: types.SeverityWarning, Timestamp: now.Add(-11 * time.Second),
			Message: "[FailedScheduling] retina-agent-kgpwl: 0/9 nodes", Reason: "FailedScheduling",
			PodName: "retina-agent-kgpwl", Namespace: "kube-system"},
		{Type: types.EventK8sWarning, Severity: types.SeverityWarning, Timestamp: now.Add(-12 * time.Second),
			Message: "[FailedScheduling] podinfo-5d8f9b76b8: 0/9 nodes", Reason: "FailedScheduling",
			PodName: "podinfo-5d8f9b76b8", Namespace: "default"},
		// Warnings — 2 Unhealthy
		{Type: types.EventK8sWarning, Severity: types.SeverityWarning, Timestamp: now.Add(-20 * time.Second),
			Message: "[Unhealthy] dev-oauthpoc: probe failed", Reason: "Unhealthy",
			PodName: "dev-oauthpoc-7fb9dc9f8c", Namespace: "dev"},
		{Type: types.EventK8sWarning, Severity: types.SeverityWarning, Timestamp: now.Add(-21 * time.Second),
			Message: "[Unhealthy] dev-nsvendorscope: probe failed", Reason: "Unhealthy",
			PodName: "dev-nsvendorscope-bfd8bd69c", Namespace: "dev"},
		// Warnings — individual
		{Type: types.EventNodeCordon, Severity: types.SeverityWarning, Timestamp: now.Add(-60 * time.Second),
			Message: "Node vmss000001 cordoned", NodeName: "vmss000001"},
		{Type: types.EventPodEvicted, Severity: types.SeverityWarning, Timestamp: now.Add(-55 * time.Second),
			Message: "Pod default/web-abc evicted from vmss000001", PodName: "web-abc",
			Namespace: "default", NodeName: "vmss000001"},
		// Info — 2 RegisteredNode
		{Type: types.EventK8sNormal, Severity: types.SeverityInfo, Timestamp: now.Add(-5 * time.Second),
			Message: "[RegisteredNode] vmss000005: registered", Reason: "RegisteredNode", NodeName: "vmss000005"},
		{Type: types.EventK8sNormal, Severity: types.SeverityInfo, Timestamp: now.Add(-6 * time.Second),
			Message: "[RegisteredNode] vmss000006: registered", Reason: "RegisteredNode", NodeName: "vmss000006"},
		// Info — individual
		{Type: types.EventNodeReady, Severity: types.SeverityInfo, Timestamp: now.Add(-40 * time.Second),
			Message: "Node vmss000001 reimaged (v1.32.7 → v1.32.9)", NodeName: "vmss000001"},
		{Type: types.EventMigration, Severity: types.SeverityInfo, Timestamp: now.Add(-36 * time.Second),
			Message: "Pod default/web-abc migrated: vmss000001 → vmss000002", PodName: "web-abc",
			Namespace: "default", NodeName: "vmss000002"},
	}
}

func TestAggregateEvents_RealisticUpgrade(t *testing.T) {
	events := testUpgradeEvents()
	result := aggregateEvents(events)

	// Expect: errors first, then warnings, then info
	// Check overall severity ordering
	lastSeverity := severityRank(types.SeverityError) + 1
	for _, ag := range result {
		rank := severityRank(ag.Severity)
		if rank > lastSeverity {
			t.Errorf("severity ordering violated: %s after lower severity group", ag.Severity)
		}
		lastSeverity = rank
	}

	// Count groups by severity
	var errorGroups, warningGroups, infoGroups int
	for _, ag := range result {
		switch ag.Severity {
		case types.SeverityError:
			errorGroups++
		case types.SeverityWarning:
			warningGroups++
		case types.SeverityInfo:
			infoGroups++
		}
	}

	if errorGroups != 1 {
		t.Errorf("error groups = %d, want 1 (POD_FAILED only — K8s events are Warning, not Error)", errorGroups)
	}
	if warningGroups < 4 {
		t.Errorf("warning groups = %d, want at least 4 (VolumeFailedDelete, FailedScheduling, Unhealthy, NODE_CORDON+)", warningGroups)
	}
	if infoGroups < 2 {
		t.Errorf("info groups = %d, want at least 2 (RegisteredNode + NODE_READY+)", infoGroups)
	}

	// FailedScheduling should be grouped (3 events → 1 group)
	for _, ag := range result {
		if ag.Reason == "FailedScheduling" {
			if ag.Count != 3 {
				t.Errorf("FailedScheduling count = %d, want 3", ag.Count)
			}
			return
		}
	}
	t.Error("FailedScheduling group not found in aggregated results")
}

func TestSortedEvents(t *testing.T) {
	now := time.Now()
	m := newClassifyModel()
	m.events = []types.Event{
		{Severity: types.SeverityInfo, Message: "info event", Timestamp: now},
		{Severity: types.SeverityError, Message: "error event", Timestamp: now.Add(-10 * time.Second)},
		{Severity: types.SeverityWarning, Message: "warning event", Timestamp: now.Add(-5 * time.Second)},
	}

	sorted := m.sortedEvents()

	if sorted[0].Severity != types.SeverityError {
		t.Errorf("first event severity = %s, want error", sorted[0].Severity)
	}
	if sorted[1].Severity != types.SeverityWarning {
		t.Errorf("second event severity = %s, want warning", sorted[1].Severity)
	}
	if sorted[2].Severity != types.SeverityInfo {
		t.Errorf("third event severity = %s, want info", sorted[2].Severity)
	}
}

func TestExtractTarget(t *testing.T) {
	tests := []struct {
		name string
		ag   AggregatedEvent
		want string
	}{
		{
			name: "node event",
			ag:   AggregatedEvent{SampleEvent: types.Event{NodeName: "vmss000001"}},
			want: "vmss000001",
		},
		{
			name: "pod event",
			ag:   AggregatedEvent{SampleEvent: types.Event{PodName: "web-abc"}},
			want: "web-abc",
		},
		{
			name: "resource fallback",
			ag:   AggregatedEvent{Resources: []string{"pvc-abc"}},
			want: "pvc-abc",
		},
		{
			name: "no target",
			ag:   AggregatedEvent{},
			want: "-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTarget(tt.ag)
			if got != tt.want {
				t.Errorf("extractTarget() = %q, want %q", got, tt.want)
			}
		})
	}
}
