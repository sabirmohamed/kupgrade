package types

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
	NodeName  string
	Cleared   bool // true when blocker is resolved
}
