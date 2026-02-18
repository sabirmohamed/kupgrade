package snapshot

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sabirmohamed/kupgrade/pkg/types"
)

// unhealthyStatuses are pod phases/reasons that indicate a problem.
// Includes Init: prefixed variants from podPhase() in collector.go.
var unhealthyStatuses = map[string]bool{
	"CrashLoopBackOff":                true,
	"Error":                           true,
	"Failed":                          true,
	"Pending":                         true,
	"ImagePullBackOff":                true,
	"ErrImagePull":                    true,
	"OOMKilled":                       true,
	"CreateContainerConfigError":      true,
	"Init:CrashLoopBackOff":           true,
	"Init:ImagePullBackOff":           true,
	"Init:ErrImagePull":               true,
	"Init:Error":                      true,
	"Init:CreateContainerConfigError": true,
}

// determineCategory returns the diff category based on before/after health states.
func determineCategory(beforeHealthy, afterHealthy bool) DiffCategory {
	switch {
	case beforeHealthy && afterHealthy:
		return CategoryUnchanged
	case beforeHealthy && !afterHealthy:
		return CategoryNewIssue
	case !beforeHealthy && !afterHealthy:
		return CategoryPreExisting
	default: // !beforeHealthy && afterHealthy
		return CategoryResolved
	}
}

// isHealthy returns true if a workload has sufficient ready replicas
// and no pods in known-bad states. Workloads where all pods have
// Succeeded (Jobs, CronJobs, PodTemplates, etc.) are treated as healthy.
func isHealthy(workload *types.WorkloadSnapshot) bool {
	// Completed workloads: all pods Succeeded, none in bad states.
	// Covers Jobs, CronJobs, PodTemplates, and any workload with only
	// Succeeded pods (e.g., AKS eraser image collectors).
	if workload.PodStatuses["Succeeded"] > 0 {
		hasOnlySucceeded := true
		for status := range workload.PodStatuses {
			if status != "Succeeded" {
				hasOnlySucceeded = false
			}
			if unhealthyStatuses[status] {
				return false
			}
		}
		if hasOnlySucceeded {
			return true
		}
	}

	if workload.DesiredReplicas > 0 && workload.ReadyReplicas < workload.DesiredReplicas {
		return false
	}
	for status := range workload.PodStatuses {
		if unhealthyStatuses[status] {
			return false
		}
	}
	return true
}

// workloadKey builds the map key for a workload: namespace/Kind/name.
func workloadKey(workload *types.WorkloadSnapshot) string {
	return fmt.Sprintf("%s/%s/%s", workload.Namespace, workload.Kind, workload.Name)
}

// Diff compares two snapshots and produces a DiffReport.
func Diff(before, after *types.Snapshot) *DiffReport {
	report := &DiffReport{
		BeforeTimestamp: before.Timestamp,
		AfterTimestamp:  after.Timestamp,
		BeforeVersion:   before.ServerVersion,
		AfterVersion:    after.ServerVersion,
		BeforeContext:   before.Context,
	}

	// Build workload maps.
	beforeMap := make(map[string]*types.WorkloadSnapshot, len(before.Workloads))
	for i := range before.Workloads {
		w := &before.Workloads[i]
		beforeMap[workloadKey(w)] = w
	}

	afterMap := make(map[string]*types.WorkloadSnapshot, len(after.Workloads))
	for i := range after.Workloads {
		w := &after.Workloads[i]
		afterMap[workloadKey(w)] = w
	}

	// Compare before workloads against after.
	for key, beforeWorkload := range beforeMap {
		afterWorkload, exists := afterMap[key]
		if !exists {
			report.WorkloadDiffs = append(report.WorkloadDiffs, WorkloadDiff{
				Namespace: beforeWorkload.Namespace,
				Kind:      beforeWorkload.Kind,
				Name:      beforeWorkload.Name,
				Category:  CategoryRemoved,
				Before:    beforeWorkload,
			})
			continue
		}

		beforeHealthy := isHealthy(beforeWorkload)
		afterHealthy := isHealthy(afterWorkload)
		category := determineCategory(beforeHealthy, afterHealthy)

		report.WorkloadDiffs = append(report.WorkloadDiffs, WorkloadDiff{
			Namespace: beforeWorkload.Namespace,
			Kind:      beforeWorkload.Kind,
			Name:      beforeWorkload.Name,
			Category:  category,
			Before:    beforeWorkload,
			After:     afterWorkload,
		})
	}

	// Find new workloads (in after but not in before).
	for key, afterWorkload := range afterMap {
		if _, exists := beforeMap[key]; exists {
			continue
		}
		// Only include unhealthy new workloads — healthy new ones aren't interesting.
		if !isHealthy(afterWorkload) {
			report.WorkloadDiffs = append(report.WorkloadDiffs, WorkloadDiff{
				Namespace: afterWorkload.Namespace,
				Kind:      afterWorkload.Kind,
				Name:      afterWorkload.Name,
				Category:  CategoryNewWorkload,
				After:     afterWorkload,
			})
		}
	}

	// Sort workload diffs: NEW_ISSUE first, then by key.
	sort.Slice(report.WorkloadDiffs, func(i, j int) bool {
		if report.WorkloadDiffs[i].Category != report.WorkloadDiffs[j].Category {
			return categoryOrder(report.WorkloadDiffs[i].Category) < categoryOrder(report.WorkloadDiffs[j].Category)
		}
		return report.WorkloadDiffs[i].Key() < report.WorkloadDiffs[j].Key()
	})

	// Diff nodes.
	report.NodeDiffs = diffNodes(before.Nodes, after.Nodes)

	// Diff PDBs.
	report.PDBDiffs = diffPDBs(before.PDBs, after.PDBs)

	// Build summary.
	report.Summary = buildSummary(report.WorkloadDiffs, report.NodeDiffs)
	report.HasNewIssues = report.Summary.NewIssues > 0

	return report
}

// categoryOrder defines sort priority (lower = first in output).
func categoryOrder(category DiffCategory) int {
	switch category {
	case CategoryNewIssue:
		return 0
	case CategoryPreExisting:
		return 1
	case CategoryResolved:
		return 2
	case CategoryRemoved:
		return 3
	case CategoryNewWorkload:
		return 4
	case CategoryUnchanged:
		return 5
	default:
		return 6
	}
}

func diffNodes(before, after []types.NodeSnapshot) []NodeDiff {
	beforeMap := make(map[string]*types.NodeSnapshot, len(before))
	for i := range before {
		beforeMap[before[i].Name] = &before[i]
	}

	afterMap := make(map[string]*types.NodeSnapshot, len(after))
	for i := range after {
		afterMap[after[i].Name] = &after[i]
	}

	var diffs []NodeDiff

	// Check before nodes.
	for name, beforeNode := range beforeMap {
		afterNode, exists := afterMap[name]
		if !exists {
			diffs = append(diffs, NodeDiff{
				Name:          name,
				Category:      NodeRemoved,
				BeforeVersion: beforeNode.Version,
				BeforeReady:   toBoolPtr(beforeNode.Ready),
			})
			continue
		}

		// Check for changes.
		var changes []string
		if beforeNode.Version != afterNode.Version {
			changes = append(changes, fmt.Sprintf("version: %s -> %s", beforeNode.Version, afterNode.Version))
		}
		if beforeNode.Ready != afterNode.Ready {
			changes = append(changes, fmt.Sprintf("ready: %t -> %t", beforeNode.Ready, afterNode.Ready))
		}

		conditionChanges := diffConditions(beforeNode.Conditions, afterNode.Conditions)
		changes = append(changes, conditionChanges...)

		if len(changes) > 0 {
			diffs = append(diffs, NodeDiff{
				Name:             name,
				Category:         NodeChanged,
				BeforeVersion:    beforeNode.Version,
				AfterVersion:     afterNode.Version,
				BeforeReady:      toBoolPtr(beforeNode.Ready),
				AfterReady:       toBoolPtr(afterNode.Ready),
				ConditionChanges: changes,
			})
		}
	}

	// Check for new nodes.
	for name, afterNode := range afterMap {
		if _, exists := beforeMap[name]; exists {
			continue
		}
		diffs = append(diffs, NodeDiff{
			Name:         name,
			Category:     NodeAdded,
			AfterVersion: afterNode.Version,
			AfterReady:   toBoolPtr(afterNode.Ready),
		})
	}

	sort.Slice(diffs, func(i, j int) bool {
		return diffs[i].Name < diffs[j].Name
	})

	return diffs
}

func diffConditions(before, after []string) []string {
	beforeSet := make(map[string]bool, len(before))
	for _, condition := range before {
		beforeSet[condition] = true
	}

	afterSet := make(map[string]bool, len(after))
	for _, condition := range after {
		afterSet[condition] = true
	}

	var changes []string
	for condition := range afterSet {
		if !beforeSet[condition] {
			changes = append(changes, fmt.Sprintf("+%s", condition))
		}
	}
	for condition := range beforeSet {
		if !afterSet[condition] {
			changes = append(changes, fmt.Sprintf("-%s", condition))
		}
	}

	sort.Strings(changes)
	return changes
}

func diffPDBs(before, after []types.PDBSnapshot) []PDBDiff {
	afterMap := make(map[string]*types.PDBSnapshot, len(after))
	for i := range after {
		key := after[i].Namespace + "/" + after[i].Name
		afterMap[key] = &after[i]
	}

	beforeMap := make(map[string]*types.PDBSnapshot, len(before))
	for i := range before {
		key := before[i].Namespace + "/" + before[i].Name
		beforeMap[key] = &before[i]
	}

	var diffs []PDBDiff

	// Any PDB that currently will block drain gets reported — regardless
	// of whether it existed at snapshot time. The report must surface all
	// current blockers so the user knows what will stall their next upgrade.
	for key, afterPDB := range afterMap {
		if afterPDB.WillBlockDrain {
			beforePDB := beforeMap[key]
			diffs = append(diffs, PDBDiff{
				Name:      afterPDB.Name,
				Namespace: afterPDB.Namespace,
				Category:  PDBWillBlock,
				Before:    beforePDB,
				After:     afterPDB,
			})
		}
	}

	// Check before PDBs that were blocking but are now resolved
	for key, beforePDB := range beforeMap {
		if !beforePDB.WillBlockDrain {
			continue
		}
		afterPDB, exists := afterMap[key]
		if !exists || !afterPDB.WillBlockDrain {
			diffs = append(diffs, PDBDiff{
				Name:      beforePDB.Name,
				Namespace: beforePDB.Namespace,
				Category:  PDBResolved,
				Before:    beforePDB,
				After:     afterPDB,
			})
		}
	}

	sort.Slice(diffs, func(i, j int) bool {
		if diffs[i].Category != diffs[j].Category {
			return diffs[i].Category < diffs[j].Category
		}
		return diffs[i].Namespace+"/"+diffs[i].Name < diffs[j].Namespace+"/"+diffs[j].Name
	})

	return diffs
}

func buildSummary(workloadDiffs []WorkloadDiff, nodeDiffs []NodeDiff) DiffSummary {
	var summary DiffSummary
	for _, diff := range workloadDiffs {
		switch diff.Category {
		case CategoryNewIssue:
			summary.NewIssues++
		case CategoryPreExisting:
			summary.PreExisting++
		case CategoryResolved:
			summary.Resolved++
		case CategoryUnchanged:
			summary.Unchanged++
		case CategoryRemoved:
			summary.Removed++
		case CategoryNewWorkload:
			summary.NewWorkloads++
		}
	}
	for _, diff := range nodeDiffs {
		switch diff.Category {
		case NodeAdded:
			summary.NodesAdded++
		case NodeRemoved:
			summary.NodesRemoved++
		case NodeChanged:
			summary.NodesChanged++
		}
	}
	return summary
}

func toBoolPtr(b bool) *bool {
	return &b
}

// PodStatusSummary returns a human-readable summary of pod statuses.
func PodStatusSummary(statuses map[string]int) string {
	if len(statuses) == 0 {
		return "no pods"
	}

	var parts []string
	for status, count := range statuses {
		parts = append(parts, fmt.Sprintf("%s:%d", status, count))
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}
