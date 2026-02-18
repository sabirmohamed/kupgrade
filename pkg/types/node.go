package types

import "time"

// NodeStage represents the upgrade stage of a node
type NodeStage string

const (
	StageReady       NodeStage = "READY"
	StageCordoned    NodeStage = "CORDONED"
	StageDraining    NodeStage = "DRAINING"
	StageQuarantined NodeStage = "QUARANTINED"
	StageReimaging   NodeStage = "REIMAGING"
	StageComplete    NodeStage = "COMPLETE"
)

// NodeState holds the current state of a node relevant to upgrades
type NodeState struct {
	Name              string
	Stage             NodeStage
	Version           string
	Ready             bool
	Schedulable       bool
	PodCount          int  // Total pods on node (for display)
	EvictablePodCount int  // Non-DaemonSet pods (for drain progress)
	Deleted           bool // true when node was deleted
	SurgeNode         bool // true for AKS surge nodes (excluded from progress)

	// Phase 2 fields
	InitialPodCount int // Evictable pods when drain started
	DrainProgress   int
	Blocked         bool
	BlockerReason   string
	DrainStartTime  time.Time // When drain started (for elapsed display)
	WaitingPods     []string  // Pods that can't be evicted (PDB blocked)

	// Pool information (from node labels)
	Pool       string // Pool/nodegroup name (AKS agentpool, EKS nodegroup, GKE nodepool)
	PoolMode   string // Pool mode (System/User for AKS)
	ProviderID string // node.Spec.ProviderID (for platform detection)

	// Enhanced node details for NODES screen
	Conditions []string // Non-ready conditions (MemoryPressure, DiskPressure, etc.)
	Taints     []string // Active taints (NoSchedule, NoExecute, etc.)
	Age        string   // Human-readable age (e.g., "5d", "2h")

	// Resource metrics (from metrics-server, 0 = unavailable)
	CPUPercent int // CPU usage as percentage of allocatable (0-100)
	MemPercent int // Memory usage as percentage of allocatable (0-100)
}

// AllStages returns all stages in pipeline order
func AllStages() []NodeStage {
	return []NodeStage{
		StageReady,
		StageCordoned,
		StageDraining,
		StageQuarantined,
		StageReimaging,
		StageComplete,
	}
}
