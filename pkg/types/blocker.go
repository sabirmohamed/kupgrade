package types

type BlockerType string

const (
	BlockerPDB       BlockerType = "PDB"
	BlockerPV        BlockerType = "PV"
	BlockerDaemonSet BlockerType = "DaemonSet"
)

type Blocker struct {
	Type     BlockerType
	Name     string
	Detail   string
	NodeName string
}
