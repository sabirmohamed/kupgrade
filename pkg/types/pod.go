package types

// PodState holds the current state of a pod relevant to upgrades
type PodState struct {
	Name            string
	Namespace       string
	NodeName        string
	Ready           bool
	ReadyContainers int // e.g., 1 in "1/2"
	TotalContainers int // e.g., 2 in "1/2"
	Phase           string
	Restarts        int
	LastRestartAge  string // e.g., "4m", "8h" - empty if no restarts
	Age             string
	HasLiveness     bool // true if pod has liveness probe configured
	HasReadiness    bool // true if pod has readiness probe configured
	LivenessOK      bool // true if liveness probe is passing (only valid if HasLiveness)
	ReadinessOK     bool // true if readiness probe is passing (only valid if HasReadiness)
	OwnerKind       string // Deployment, DaemonSet, StatefulSet, etc.
	OwnerRef        string // Controller UID for migration tracking
	Deleted         bool   // true when pod was deleted
}
