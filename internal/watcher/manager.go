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

const (
	eventBufferSize     = 100
	nodeStateBufferSize = 50
)

// Compile-time interface check
var _ EventEmitter = (*Manager)(nil)

// Manager coordinates all watchers and emits events
type Manager struct {
	factory   informers.SharedInformerFactory
	namespace string

	eventCh     chan types.Event
	nodeStateCh chan types.NodeState
	wg          sync.WaitGroup

	nodeWatcher  *NodeWatcher
	podWatcher   *PodWatcher
	eventWatcher *EventWatcher
	migrations   MigrationTracker
	stages       StageComputer
}

// NewManager creates a new watcher manager
func NewManager(factory informers.SharedInformerFactory, namespace string, targetVersion string) *Manager {
	eventCh := make(chan types.Event, eventBufferSize)
	nodeStateCh := make(chan types.NodeState, nodeStateBufferSize)

	stages := NewStageComputer(targetVersion)
	migrations := NewMigrationTracker()

	m := &Manager{
		factory:     factory,
		namespace:   namespace,
		eventCh:     eventCh,
		nodeStateCh: nodeStateCh,
		stages:      stages,
		migrations:  migrations,
	}

	// Create watchers
	m.podWatcher = NewPodWatcher(factory, namespace, m, stages, migrations)
	m.nodeWatcher = NewNodeWatcher(factory, m, stages, m.countPodsOnNode)
	m.eventWatcher = NewEventWatcher(factory, namespace, m)

	return m
}

// Start begins all watchers
func (m *Manager) Start(ctx context.Context) error {
	m.factory.Start(ctx.Done())

	synced := m.factory.WaitForCacheSync(ctx.Done())
	for typ, ok := range synced {
		if !ok {
			return fmt.Errorf("watcher: cache sync failed for %v", typ)
		}
	}

	if err := m.nodeWatcher.Start(ctx); err != nil {
		return fmt.Errorf("watcher: %w", err)
	}
	if err := m.podWatcher.Start(ctx); err != nil {
		return fmt.Errorf("watcher: %w", err)
	}
	if err := m.eventWatcher.Start(ctx); err != nil {
		return fmt.Errorf("watcher: %w", err)
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

// NodeStateUpdates returns channel for node state changes
func (m *Manager) NodeStateUpdates() <-chan types.NodeState {
	return m.nodeStateCh
}

// Emit sends an event (ring buffer semantics - drops oldest if full)
func (m *Manager) Emit(event types.Event) {
	select {
	case m.eventCh <- event:
	default:
		select {
		case <-m.eventCh:
		default:
		}
		m.eventCh <- event
	}
}

// EmitNodeState sends a node state update (ring buffer semantics)
func (m *Manager) EmitNodeState(state types.NodeState) {
	select {
	case m.nodeStateCh <- state:
	default:
		select {
		case <-m.nodeStateCh:
		default:
		}
		m.nodeStateCh <- state
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

// InitialNodeStates returns current state of all nodes (for initial TUI load)
func (m *Manager) InitialNodeStates() []types.NodeState {
	return m.nodeWatcher.buildNodeStates()
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
