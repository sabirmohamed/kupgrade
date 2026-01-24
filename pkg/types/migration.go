package types

import "time"

// Migration represents a pod migration from one node to another
type Migration struct {
	Owner     string // Controller UID (ReplicaSet, Deployment, etc.)
	FromNode  string
	ToNode    string
	OldPod    string
	NewPod    string
	Namespace string
	Timestamp time.Time
	Complete  bool // true when new pod is Ready
}

// PendingMigration tracks a pod deletion that may result in a migration
type PendingMigration struct {
	OwnerRef  string
	FromNode  string
	PodName   string
	Namespace string
	Timestamp time.Time
}
