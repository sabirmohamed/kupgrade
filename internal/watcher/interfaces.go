package watcher

import (
	"context"
	"time"

	"github.com/sabirmohamed/kupgrade/pkg/types"
	corev1 "k8s.io/api/core/v1"
)

// Watcher observes a Kubernetes resource type and emits events.
type Watcher interface {
	// Start begins watching. Returns when ctx is cancelled.
	// MUST call WaitForCacheSync before returning from initialization.
	// MUST NOT block the caller - run watch loop in goroutine.
	Start(ctx context.Context) error
}

// EventEmitter sends events to a channel.
type EventEmitter interface {
	// Emit sends an event. MUST NOT block.
	// If channel is full, oldest event MAY be dropped.
	Emit(event types.Event)
}

// StageComputer determines node upgrade stage from observable state.
type StageComputer interface {
	// ComputeStage returns current stage for a node.
	// MUST be safe for concurrent calls.
	ComputeStage(node *corev1.Node) types.NodeStage

	// UpdatePodCount updates the pod count for a node.
	// Called by PodWatcher on add/delete.
	UpdatePodCount(nodeName string, delta int)

	// SetTargetVersion sets the upgrade target.
	// If not called, implementation SHOULD auto-detect.
	SetTargetVersion(version string)

	// GetTargetVersion returns the current target version.
	GetTargetVersion() string
}

// MigrationTracker correlates pod deletes with creates to detect migrations.
type MigrationTracker interface {
	// OnPodDelete records a potential migration source.
	// Returns immediately; correlation happens on OnPodAdd.
	OnPodDelete(pod *corev1.Pod)

	// OnPodAdd checks for migration correlation.
	// Returns Migration if this pod completes a migration, nil otherwise.
	OnPodAdd(pod *corev1.Pod) *types.Migration

	// Cleanup removes stale pending migrations.
	// SHOULD be called periodically (e.g., every 30s).
	Cleanup(maxAge time.Duration)
}
