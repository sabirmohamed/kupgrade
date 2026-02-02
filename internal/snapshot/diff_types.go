package snapshot

import (
	"time"

	"github.com/sabirmohamed/kupgrade/pkg/types"
)

// DiffCategory classifies a workload's change between snapshots.
type DiffCategory string

const (
	// CategoryNewIssue means the workload was healthy before but unhealthy after.
	CategoryNewIssue DiffCategory = "NEW_ISSUE"
	// CategoryPreExisting means the workload was unhealthy before and after.
	CategoryPreExisting DiffCategory = "PRE_EXISTING"
	// CategoryResolved means the workload was unhealthy before but healthy after.
	CategoryResolved DiffCategory = "RESOLVED"
	// CategoryUnchanged means the workload was healthy before and after.
	CategoryUnchanged DiffCategory = "UNCHANGED"
	// CategoryRemoved means the workload existed before but is gone.
	CategoryRemoved DiffCategory = "REMOVED"
	// CategoryNewWorkload means the workload is new and unhealthy.
	CategoryNewWorkload DiffCategory = "NEW_WORKLOAD"
)

// NodeDiffCategory classifies a node's change between snapshots.
type NodeDiffCategory string

const (
	NodeAdded   NodeDiffCategory = "ADDED"
	NodeRemoved NodeDiffCategory = "REMOVED"
	NodeChanged NodeDiffCategory = "CHANGED"
)

// WorkloadDiff captures the before/after state for a single workload.
type WorkloadDiff struct {
	Namespace string                  `json:"namespace"`
	Kind      string                  `json:"kind"`
	Name      string                  `json:"name"`
	Category  DiffCategory            `json:"category"`
	Before    *types.WorkloadSnapshot `json:"before,omitempty"`
	After     *types.WorkloadSnapshot `json:"after,omitempty"`
}

// Key returns the workload's unique identifier: namespace/Kind/name.
func (d *WorkloadDiff) Key() string {
	return d.Namespace + "/" + d.Kind + "/" + d.Name
}

// NodeDiff captures the before/after state for a single node.
type NodeDiff struct {
	Name             string           `json:"name"`
	Category         NodeDiffCategory `json:"category"`
	BeforeVersion    string           `json:"beforeVersion,omitempty"`
	AfterVersion     string           `json:"afterVersion,omitempty"`
	BeforeReady      *bool            `json:"beforeReady,omitempty"`
	AfterReady       *bool            `json:"afterReady,omitempty"`
	ConditionChanges []string         `json:"conditionChanges,omitempty"`
}

// DiffSummary holds counts per category.
type DiffSummary struct {
	NewIssues    int `json:"newIssues"`
	PreExisting  int `json:"preExisting"`
	Resolved     int `json:"resolved"`
	Unchanged    int `json:"unchanged"`
	Removed      int `json:"removed"`
	NewWorkloads int `json:"newWorkloads"`
	NodesAdded   int `json:"nodesAdded"`
	NodesRemoved int `json:"nodesRemoved"`
	NodesChanged int `json:"nodesChanged"`
}

// DiffReport is the complete comparison between two snapshots.
type DiffReport struct {
	Summary         DiffSummary    `json:"summary"`
	WorkloadDiffs   []WorkloadDiff `json:"workloadDiffs"`
	NodeDiffs       []NodeDiff     `json:"nodeDiffs"`
	BeforeTimestamp time.Time      `json:"beforeTimestamp"`
	AfterTimestamp  time.Time      `json:"afterTimestamp"`
	BeforeVersion   string         `json:"beforeVersion"`
	AfterVersion    string         `json:"afterVersion"`
	BeforeContext   string         `json:"beforeContext"`
	HasNewIssues    bool           `json:"hasNewIssues"`
}
