package types

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
	Name          string
	Stage         NodeStage
	Version       string
	Ready         bool
	Schedulable   bool
	PodCount      int
	TargetVersion string

	// Phase 2 fields
	InitialPodCount int
	DrainProgress   int
	Blocked         bool
	BlockerReason   string
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
