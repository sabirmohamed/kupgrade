package types

import "time"

type BlockerType string

const (
	BlockerPDB       BlockerType = "PDB"
	BlockerPV        BlockerType = "PV"
	BlockerDaemonSet BlockerType = "DaemonSet"
)

type Blocker struct {
	Type      BlockerType
	Name      string
	Namespace string // PDB namespace
	Detail    string
	NodeName  string    // Node being blocked (required for accurate display)
	PodName   string    // Pod that can't be evicted
	StartTime time.Time // When blocking started (for duration display)
	Cleared   bool      // true when blocker is resolved
}
