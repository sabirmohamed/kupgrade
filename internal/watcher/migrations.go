package watcher

import (
	"context"
	"sync"
	"time"

	"github.com/sabirmohamed/kupgrade/pkg/types"
	corev1 "k8s.io/api/core/v1"
)

const (
	migrationTTL    = 5 * time.Minute
	cleanupInterval = 30 * time.Second
)

// migrationTracker implements MigrationTracker using owner reference correlation
type migrationTracker struct {
	pending map[string]types.PendingMigration
	mu      sync.Mutex
}

// NewMigrationTracker creates a new migration tracker
func NewMigrationTracker() MigrationTracker {
	return &migrationTracker{
		pending: make(map[string]types.PendingMigration),
	}
}

// OnPodDelete records a potential migration source
func (t *migrationTracker) OnPodDelete(pod *corev1.Pod) {
	owner := getControllerOwner(pod)
	if owner == "" {
		return // Standalone pod, not a migration
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.pending[owner] = types.PendingMigration{
		OwnerRef:  owner,
		FromNode:  pod.Spec.NodeName,
		PodName:   pod.Name,
		Namespace: pod.Namespace,
		Timestamp: time.Now(),
	}
}

// OnPodAdd checks for migration correlation
func (t *migrationTracker) OnPodAdd(pod *corev1.Pod) *types.Migration {
	if pod.Spec.NodeName == "" {
		return nil // Not yet scheduled
	}

	owner := getControllerOwner(pod)
	if owner == "" {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	pending, ok := t.pending[owner]
	if !ok {
		return nil
	}

	// Found matching delete - this is a migration
	delete(t.pending, owner)

	// Only report if different node
	if pending.FromNode == pod.Spec.NodeName {
		return nil
	}

	return &types.Migration{
		Owner:     owner,
		FromNode:  pending.FromNode,
		ToNode:    pod.Spec.NodeName,
		OldPod:    pending.PodName,
		NewPod:    pod.Name,
		Namespace: pod.Namespace,
		Timestamp: time.Now(),
	}
}

// Cleanup removes stale pending migrations
func (t *migrationTracker) Cleanup(maxAge time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	for key, pending := range t.pending {
		if now.Sub(pending.Timestamp) > maxAge {
			delete(t.pending, key)
		}
	}
}

// runCleanup periodically cleans up stale migrations
func (t *migrationTracker) runCleanup(ctx context.Context) {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.Cleanup(migrationTTL)
		}
	}
}

// getControllerOwner returns the UID of the controlling owner reference
func getControllerOwner(pod *corev1.Pod) string {
	for _, ref := range pod.OwnerReferences {
		if ref.Controller != nil && *ref.Controller {
			return string(ref.UID)
		}
	}
	return ""
}
