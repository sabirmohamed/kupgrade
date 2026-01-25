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

// EventEmitter sends events, node state, and pod state updates.
type EventEmitter interface {
	// Emit sends an event. MUST NOT block.
	Emit(event types.Event)

	// EmitNodeState sends a node state update. MUST NOT block.
	EmitNodeState(state types.NodeState)

	// EmitPodState sends a pod state update. MUST NOT block.
	EmitPodState(state types.PodState)

	// EmitBlocker sends a blocker update. MUST NOT block.
	EmitBlocker(blocker types.Blocker)

	// RefreshNodeState triggers a node state refresh (e.g., when pods change).
	RefreshNodeState(nodeName string)
}

// StageComputer determines node upgrade stage from observable state.
type StageComputer interface {
	// ComputeStage returns current stage for a node.
	ComputeStage(node *corev1.Node) types.NodeStage

	// UpdatePodCount updates the pod count for a node.
	UpdatePodCount(nodeName string, delta int)

	// SetTargetVersion sets the upgrade target.
	SetTargetVersion(version string)

	// TargetVersion returns the current target version.
	TargetVersion() string

	// PodCount returns the pod count for a node.
	PodCount(nodeName string) int
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
