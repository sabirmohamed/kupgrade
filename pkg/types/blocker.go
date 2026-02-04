package types

import "time"

type BlockerType string

const (
	BlockerPDB       BlockerType = "PDB"
	BlockerPV        BlockerType = "PV"
	BlockerDaemonSet BlockerType = "DaemonSet"
)

// BlockerTier distinguishes informational PDB risks from active drain blockers.
type BlockerTier int

const (
	// BlockerTierRisk indicates a PDB with DisruptionsAllowed == 0 but no matched
	// pods on a draining node. Informational warning (yellow).
	BlockerTierRisk BlockerTier = iota
	// BlockerTierActive indicates a PDB actively blocking a drain — its selector
	// matches pods on a node in DRAINING stage. Urgent (red).
	BlockerTierActive
)

type Blocker struct {
	Type      BlockerType
	Tier      BlockerTier // Risk (informational) vs Active (blocking a drain)
	Name      string
	Namespace string // PDB namespace
	Detail    string
	NodeName  string    // Node being blocked (required for accurate display)
	PodName   string    // Pod that can't be evicted
	StartTime time.Time // When blocking started (for duration display)
	Cleared   bool      // true when blocker is resolved
}
