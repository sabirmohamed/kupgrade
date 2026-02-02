package snapshot

import (
	"testing"
	"time"

	"github.com/sabirmohamed/kupgrade/pkg/types"
)

func newSnapshot(workloads []types.WorkloadSnapshot, nodes []types.NodeSnapshot) *types.Snapshot {
	return &types.Snapshot{
		SchemaVersion: types.SchemaVersion,
		Timestamp:     time.Now(),
		Context:       "test-context",
		ServerVersion: "v1.28.0",
		Nodes:         nodes,
		Workloads:     workloads,
	}
}

func healthyWorkload(namespace, kind, name string) types.WorkloadSnapshot {
	return types.WorkloadSnapshot{
		Namespace:       namespace,
		Kind:            kind,
		Name:            name,
		DesiredReplicas: 3,
		ReadyReplicas:   3,
		PodStatuses:     map[string]int{"Running": 3},
	}
}

func unhealthyWorkload(namespace, kind, name string) types.WorkloadSnapshot {
	return types.WorkloadSnapshot{
		Namespace:       namespace,
		Kind:            kind,
		Name:            name,
		DesiredReplicas: 3,
		ReadyReplicas:   1,
		PodStatuses:     map[string]int{"Running": 1, "CrashLoopBackOff": 2},
	}
}

func TestDiffCategoryNewIssue(t *testing.T) {
	before := newSnapshot([]types.WorkloadSnapshot{
		healthyWorkload("default", "Deployment", "web"),
	}, nil)
	after := newSnapshot([]types.WorkloadSnapshot{
		unhealthyWorkload("default", "Deployment", "web"),
	}, nil)

	report := Diff(before, after)

	if len(report.WorkloadDiffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(report.WorkloadDiffs))
	}
	if report.WorkloadDiffs[0].Category != CategoryNewIssue {
		t.Errorf("expected NEW_ISSUE, got %s", report.WorkloadDiffs[0].Category)
	}
	if !report.HasNewIssues {
		t.Error("expected HasNewIssues to be true")
	}
	if report.Summary.NewIssues != 1 {
		t.Errorf("expected summary.NewIssues=1, got %d", report.Summary.NewIssues)
	}
}

func TestDiffCategoryPreExisting(t *testing.T) {
	before := newSnapshot([]types.WorkloadSnapshot{
		unhealthyWorkload("default", "Deployment", "web"),
	}, nil)
	after := newSnapshot([]types.WorkloadSnapshot{
		unhealthyWorkload("default", "Deployment", "web"),
	}, nil)

	report := Diff(before, after)

	if len(report.WorkloadDiffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(report.WorkloadDiffs))
	}
	if report.WorkloadDiffs[0].Category != CategoryPreExisting {
		t.Errorf("expected PRE_EXISTING, got %s", report.WorkloadDiffs[0].Category)
	}
	if report.HasNewIssues {
		t.Error("expected HasNewIssues to be false")
	}
}

func TestDiffCategoryResolved(t *testing.T) {
	before := newSnapshot([]types.WorkloadSnapshot{
		unhealthyWorkload("default", "Deployment", "web"),
	}, nil)
	after := newSnapshot([]types.WorkloadSnapshot{
		healthyWorkload("default", "Deployment", "web"),
	}, nil)

	report := Diff(before, after)

	if len(report.WorkloadDiffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(report.WorkloadDiffs))
	}
	if report.WorkloadDiffs[0].Category != CategoryResolved {
		t.Errorf("expected RESOLVED, got %s", report.WorkloadDiffs[0].Category)
	}
}

func TestDiffCategoryUnchanged(t *testing.T) {
	before := newSnapshot([]types.WorkloadSnapshot{
		healthyWorkload("default", "Deployment", "web"),
	}, nil)
	after := newSnapshot([]types.WorkloadSnapshot{
		healthyWorkload("default", "Deployment", "web"),
	}, nil)

	report := Diff(before, after)

	if len(report.WorkloadDiffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(report.WorkloadDiffs))
	}
	if report.WorkloadDiffs[0].Category != CategoryUnchanged {
		t.Errorf("expected UNCHANGED, got %s", report.WorkloadDiffs[0].Category)
	}
}

func TestDiffCategoryRemoved(t *testing.T) {
	before := newSnapshot([]types.WorkloadSnapshot{
		healthyWorkload("default", "Deployment", "web"),
	}, nil)
	after := newSnapshot(nil, nil)

	report := Diff(before, after)

	if len(report.WorkloadDiffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(report.WorkloadDiffs))
	}
	if report.WorkloadDiffs[0].Category != CategoryRemoved {
		t.Errorf("expected REMOVED, got %s", report.WorkloadDiffs[0].Category)
	}
}

func TestDiffCategoryNewWorkloadUnhealthy(t *testing.T) {
	before := newSnapshot(nil, nil)
	after := newSnapshot([]types.WorkloadSnapshot{
		unhealthyWorkload("default", "Deployment", "new-app"),
	}, nil)

	report := Diff(before, after)

	if len(report.WorkloadDiffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(report.WorkloadDiffs))
	}
	if report.WorkloadDiffs[0].Category != CategoryNewWorkload {
		t.Errorf("expected NEW_WORKLOAD, got %s", report.WorkloadDiffs[0].Category)
	}
}

func TestDiffNewWorkloadHealthyIgnored(t *testing.T) {
	before := newSnapshot(nil, nil)
	after := newSnapshot([]types.WorkloadSnapshot{
		healthyWorkload("default", "Deployment", "new-app"),
	}, nil)

	report := Diff(before, after)

	if len(report.WorkloadDiffs) != 0 {
		t.Errorf("expected 0 diffs for healthy new workload, got %d", len(report.WorkloadDiffs))
	}
}

func TestIsHealthyBasic(t *testing.T) {
	tests := []struct {
		name     string
		workload types.WorkloadSnapshot
		expected bool
	}{
		{
			name:     "all running",
			workload: healthyWorkload("ns", "Deployment", "app"),
			expected: true,
		},
		{
			name:     "crash loop",
			workload: unhealthyWorkload("ns", "Deployment", "app"),
			expected: false,
		},
		{
			name: "scaled to zero",
			workload: types.WorkloadSnapshot{
				DesiredReplicas: 0,
				ReadyReplicas:   0,
				PodStatuses:     map[string]int{},
			},
			expected: true,
		},
		{
			name: "init crash loop",
			workload: types.WorkloadSnapshot{
				DesiredReplicas: 1,
				ReadyReplicas:   0,
				PodStatuses:     map[string]int{"Init:CrashLoopBackOff": 1},
			},
			expected: false,
		},
		{
			name: "init image pull backoff",
			workload: types.WorkloadSnapshot{
				DesiredReplicas: 1,
				ReadyReplicas:   0,
				PodStatuses:     map[string]int{"Init:ImagePullBackOff": 1},
			},
			expected: false,
		},
		{
			name: "OOMKilled",
			workload: types.WorkloadSnapshot{
				DesiredReplicas: 2,
				ReadyReplicas:   1,
				PodStatuses:     map[string]int{"Running": 1, "OOMKilled": 1},
			},
			expected: false,
		},
		{
			name: "CreateContainerConfigError",
			workload: types.WorkloadSnapshot{
				DesiredReplicas: 1,
				ReadyReplicas:   0,
				PodStatuses:     map[string]int{"CreateContainerConfigError": 1},
			},
			expected: false,
		},
		{
			name: "succeeded job is healthy",
			workload: types.WorkloadSnapshot{
				Kind:            "Job",
				DesiredReplicas: 1,
				ReadyReplicas:   0,
				PodStatuses:     map[string]int{"Succeeded": 1},
			},
			expected: true,
		},
		{
			name: "succeeded cronjob is healthy",
			workload: types.WorkloadSnapshot{
				Kind:            "CronJob",
				DesiredReplicas: 1,
				ReadyReplicas:   0,
				PodStatuses:     map[string]int{"Succeeded": 1},
			},
			expected: true,
		},
		{
			name: "failed job with succeeded pods is unhealthy",
			workload: types.WorkloadSnapshot{
				Kind:            "Job",
				DesiredReplicas: 1,
				ReadyReplicas:   0,
				PodStatuses:     map[string]int{"Succeeded": 1, "Error": 1},
			},
			expected: false,
		},
		{
			name: "podtemplate with all succeeded is healthy",
			workload: types.WorkloadSnapshot{
				Kind:            "PodTemplate",
				DesiredReplicas: 9,
				ReadyReplicas:   0,
				PodStatuses:     map[string]int{"Succeeded": 9},
			},
			expected: true,
		},
		{
			name: "workload with succeeded and running is not auto-healthy",
			workload: types.WorkloadSnapshot{
				Kind:            "Deployment",
				DesiredReplicas: 3,
				ReadyReplicas:   2,
				PodStatuses:     map[string]int{"Succeeded": 1, "Running": 2},
			},
			expected: false,
		},
		{
			name: "pending pod despite ready count match",
			workload: types.WorkloadSnapshot{
				DesiredReplicas: 2,
				ReadyReplicas:   2,
				PodStatuses:     map[string]int{"Running": 2, "Pending": 1},
			},
			expected: false,
		},
		{
			name: "failed pod alongside running",
			workload: types.WorkloadSnapshot{
				DesiredReplicas: 2,
				ReadyReplicas:   2,
				PodStatuses:     map[string]int{"Running": 2, "Failed": 1},
			},
			expected: false,
		},
		{
			name: "all failed",
			workload: types.WorkloadSnapshot{
				DesiredReplicas: 2,
				ReadyReplicas:   0,
				PodStatuses:     map[string]int{"Failed": 2},
			},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isHealthy(&tc.workload)
			if result != tc.expected {
				t.Errorf("isHealthy() = %t, want %t", result, tc.expected)
			}
		})
	}
}

func TestDiffHPAScaling(t *testing.T) {
	// HPA scales from 3 to 10 replicas — both healthy, should be UNCHANGED.
	before := newSnapshot([]types.WorkloadSnapshot{
		{
			Namespace: "default", Kind: "Deployment", Name: "api",
			DesiredReplicas: 3, ReadyReplicas: 3,
			PodStatuses: map[string]int{"Running": 3},
		},
	}, nil)
	after := newSnapshot([]types.WorkloadSnapshot{
		{
			Namespace: "default", Kind: "Deployment", Name: "api",
			DesiredReplicas: 10, ReadyReplicas: 10,
			PodStatuses: map[string]int{"Running": 10},
		},
	}, nil)

	report := Diff(before, after)
	if report.WorkloadDiffs[0].Category != CategoryUnchanged {
		t.Errorf("HPA scaling should be UNCHANGED, got %s", report.WorkloadDiffs[0].Category)
	}
}

func TestDiffNodeAddedRemovedChanged(t *testing.T) {
	before := newSnapshot(nil, []types.NodeSnapshot{
		{Name: "node-1", Version: "v1.28.0", Ready: true},
		{Name: "node-2", Version: "v1.28.0", Ready: true},
	})
	after := newSnapshot(nil, []types.NodeSnapshot{
		{Name: "node-1", Version: "v1.29.0", Ready: true},
		{Name: "node-3", Version: "v1.29.0", Ready: true},
	})

	report := Diff(before, after)

	if len(report.NodeDiffs) != 3 {
		t.Fatalf("expected 3 node diffs, got %d", len(report.NodeDiffs))
	}

	// Sort is by name, so: node-1 (changed), node-2 (removed), node-3 (added).
	nodeMap := make(map[string]NodeDiff)
	for _, diff := range report.NodeDiffs {
		nodeMap[diff.Name] = diff
	}

	if nodeMap["node-1"].Category != NodeChanged {
		t.Errorf("node-1: expected CHANGED, got %s", nodeMap["node-1"].Category)
	}
	if nodeMap["node-2"].Category != NodeRemoved {
		t.Errorf("node-2: expected REMOVED, got %s", nodeMap["node-2"].Category)
	}
	if nodeMap["node-3"].Category != NodeAdded {
		t.Errorf("node-3: expected ADDED, got %s", nodeMap["node-3"].Category)
	}

	if report.Summary.NodesChanged != 1 {
		t.Errorf("expected NodesChanged=1, got %d", report.Summary.NodesChanged)
	}
	if report.Summary.NodesRemoved != 1 {
		t.Errorf("expected NodesRemoved=1, got %d", report.Summary.NodesRemoved)
	}
	if report.Summary.NodesAdded != 1 {
		t.Errorf("expected NodesAdded=1, got %d", report.Summary.NodesAdded)
	}
}

func TestDiffNodeConditionChanges(t *testing.T) {
	before := newSnapshot(nil, []types.NodeSnapshot{
		{Name: "node-1", Version: "v1.28.0", Ready: true, Conditions: []string{"MemoryPressure"}},
	})
	after := newSnapshot(nil, []types.NodeSnapshot{
		{Name: "node-1", Version: "v1.28.0", Ready: true, Conditions: []string{"DiskPressure"}},
	})

	report := Diff(before, after)

	if len(report.NodeDiffs) != 1 {
		t.Fatalf("expected 1 node diff, got %d", len(report.NodeDiffs))
	}

	changes := report.NodeDiffs[0].ConditionChanges
	if len(changes) != 2 {
		t.Fatalf("expected 2 condition changes, got %d: %v", len(changes), changes)
	}
}

func TestDiffRenamedWorkload(t *testing.T) {
	// Renamed workload shows as REMOVED (old) + potentially NEW_WORKLOAD (new if unhealthy).
	before := newSnapshot([]types.WorkloadSnapshot{
		healthyWorkload("default", "Deployment", "old-name"),
	}, nil)
	after := newSnapshot([]types.WorkloadSnapshot{
		unhealthyWorkload("default", "Deployment", "new-name"),
	}, nil)

	report := Diff(before, after)

	categoryMap := make(map[DiffCategory]int)
	for _, diff := range report.WorkloadDiffs {
		categoryMap[diff.Category]++
	}

	if categoryMap[CategoryRemoved] != 1 {
		t.Errorf("expected 1 REMOVED, got %d", categoryMap[CategoryRemoved])
	}
	if categoryMap[CategoryNewWorkload] != 1 {
		t.Errorf("expected 1 NEW_WORKLOAD, got %d", categoryMap[CategoryNewWorkload])
	}
}

func TestPodStatusSummary(t *testing.T) {
	result := PodStatusSummary(map[string]int{"Running": 2, "CrashLoopBackOff": 1})
	if result != "CrashLoopBackOff:1 Running:2" {
		t.Errorf("unexpected summary: %s", result)
	}

	empty := PodStatusSummary(map[string]int{})
	if empty != "no pods" {
		t.Errorf("expected 'no pods', got %s", empty)
	}
}
