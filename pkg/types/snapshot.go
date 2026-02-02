package types

import "time"

// SchemaVersion is the current snapshot schema version.
// Increment when the snapshot format changes in a breaking way.
const SchemaVersion = 1

// Snapshot captures the full cluster state at a point in time.
type Snapshot struct {
	SchemaVersion int                `json:"schemaVersion"`
	Timestamp     time.Time          `json:"timestamp"`
	Context       string             `json:"context"`
	ServerVersion string             `json:"serverVersion"`
	Nodes         []NodeSnapshot     `json:"nodes"`
	Workloads     []WorkloadSnapshot `json:"workloads"`
	PDBs          []PDBSnapshot      `json:"pdbs"`
}

// NodeSnapshot captures a node's state at snapshot time.
type NodeSnapshot struct {
	Name       string   `json:"name"`
	Version    string   `json:"version"`
	Ready      bool     `json:"ready"`
	Conditions []string `json:"conditions,omitempty"`
}

// WorkloadSnapshot captures a workload's state keyed by owner controller.
type WorkloadSnapshot struct {
	Namespace       string         `json:"namespace"`
	Kind            string         `json:"kind"`
	Name            string         `json:"name"`
	DesiredReplicas int            `json:"desiredReplicas"`
	ReadyReplicas   int            `json:"readyReplicas"`
	PodStatuses     map[string]int `json:"podStatuses"`
	TotalRestarts   int            `json:"totalRestarts"`
	BarePod         bool           `json:"barePod,omitempty"`
}

// PDBSnapshot captures a PodDisruptionBudget's state at snapshot time.
type PDBSnapshot struct {
	Name               string `json:"name"`
	Namespace          string `json:"namespace"`
	DisruptionsAllowed int32  `json:"disruptionsAllowed"`
	CurrentHealthy     int32  `json:"currentHealthy"`
	ExpectedPods       int32  `json:"expectedPods"`
}
