package types

// PodState holds the current state of a pod relevant to upgrades
type PodState struct {
	Name      string
	Namespace string
	NodeName  string
	Ready     bool
	Phase     string
	OwnerRef  string // Controller UID for migration tracking
}
