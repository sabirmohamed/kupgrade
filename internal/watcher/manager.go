package watcher

import (
	"context"
	"fmt"
	"sync"

	"github.com/sabirmohamed/kupgrade/pkg/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

const eventBufferSize = 100

// Manager coordinates all watchers and emits events
type Manager struct {
	factory   informers.SharedInformerFactory
	namespace string

	eventCh chan types.Event
	wg      sync.WaitGroup

	nodeWatcher  *NodeWatcher
	podWatcher   *PodWatcher
	eventWatcher *EventWatcher
	migrations   MigrationTracker
	stages       StageComputer
}

// NewManager creates a new watcher manager
func NewManager(factory informers.SharedInformerFactory, namespace string, targetVersion string) *Manager {
	eventCh := make(chan types.Event, eventBufferSize)

	stages := NewStageComputer(targetVersion)
	migrations := NewMigrationTracker()

	m := &Manager{
		factory:    factory,
		namespace:  namespace,
		eventCh:    eventCh,
		stages:     stages,
		migrations: migrations,
	}

	// Create watchers
	m.podWatcher = NewPodWatcher(factory, namespace, m, stages, migrations)
	m.nodeWatcher = NewNodeWatcher(factory, m, stages, m.countPodsOnNode)
	m.eventWatcher = NewEventWatcher(factory, namespace, m)

	return m
}

// Start begins all watchers
func (m *Manager) Start(ctx context.Context) error {
	// Start the informer factory
	m.factory.Start(ctx.Done())

	// Wait for cache sync
	synced := m.factory.WaitForCacheSync(ctx.Done())
	for typ, ok := range synced {
		if !ok {
			return fmt.Errorf("cache sync failed for %v", typ)
		}
	}

	// Start watchers (they register handlers, already running via factory)
	if err := m.nodeWatcher.Start(ctx); err != nil {
		return fmt.Errorf("node watcher start failed: %w", err)
	}
	if err := m.podWatcher.Start(ctx); err != nil {
		return fmt.Errorf("pod watcher start failed: %w", err)
	}
	if err := m.eventWatcher.Start(ctx); err != nil {
		return fmt.Errorf("event watcher start failed: %w", err)
	}

	// Start migration cleanup goroutine
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.migrations.(*migrationTracker).runCleanup(ctx)
	}()

	return nil
}

// Events returns the event channel for TUI consumption
func (m *Manager) Events() <-chan types.Event {
	return m.eventCh
}

// Emit implements EventEmitter with ring buffer semantics (ADR-005)
func (m *Manager) Emit(event types.Event) {
	select {
	case m.eventCh <- event:
		// Sent successfully
	default:
		// Channel full - drop oldest, send new
		select {
		case <-m.eventCh: // Remove oldest
		default:
		}
		select {
		case m.eventCh <- event:
		default:
			// Still full after drop - very rare, skip
		}
	}
}

// Wait blocks until all goroutines finish
func (m *Manager) Wait() {
	m.wg.Wait()
}

// StageComputer returns the stage computer for external access
func (m *Manager) StageComputer() StageComputer {
	return m.stages
}

// HasSynced returns true if all informer caches have synced
func (m *Manager) HasSynced() bool {
	return m.nodeWatcher.informer.HasSynced() &&
		m.podWatcher.informer.HasSynced() &&
		m.eventWatcher.informer.HasSynced()
}

// GetNodeStates returns current state of all nodes
func (m *Manager) GetNodeStates() []types.NodeState {
	return m.nodeWatcher.GetNodeStates()
}

// countPodsOnNode counts pods assigned to a node from the pod informer store
func (m *Manager) countPodsOnNode(nodeName string) int {
	count := 0
	for _, obj := range m.podWatcher.informer.GetStore().List() {
		pod := obj.(*corev1.Pod)
		if pod.Spec.NodeName == nodeName {
			count++
		}
	}
	return count
}

// WaitForCacheSync waits for all caches to sync
func WaitForCacheSync(ctx context.Context, syncFuncs ...cache.InformerSynced) bool {
	return cache.WaitForCacheSync(ctx.Done(), syncFuncs...)
}
