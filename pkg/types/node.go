package types

import "time"

// NodeStage represents the upgrade stage of a node
type NodeStage string

const (
	StageReady     NodeStage = "READY"
	StageCordoned  NodeStage = "CORDONED"
	StageDraining  NodeStage = "DRAINING"
	StageUpgrading NodeStage = "UPGRADING"
	StageComplete  NodeStage = "COMPLETE"
)

// NodeState holds the current state of a node relevant to upgrades
type NodeState struct {
	Name        string
	Stage       NodeStage
	Version     string
	Ready       bool
	Schedulable bool
	PodCount    int
	Deleted     bool // true when node was deleted

	// Phase 2 fields
	InitialPodCount int
	DrainProgress   int
	Blocked         bool
	BlockerReason   string
	DrainStartTime  time.Time // When drain started (for elapsed display)
	WaitingPods     []string  // Pods that can't be evicted (PDB blocked)

	// Enhanced node details for NODES screen
	Conditions []string // Non-ready conditions (MemoryPressure, DiskPressure, etc.)
	Taints     []string // Active taints (NoSchedule, NoExecute, etc.)
	Age        string   // Human-readable age (e.g., "5d", "2h")
}

// AllStages returns all stages in pipeline order
func AllStages() []NodeStage {
	return []NodeStage{
		StageReady,
		StageCordoned,
		StageDraining,
		StageUpgrading,
		StageComplete,
	}
}
