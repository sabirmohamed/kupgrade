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
	podStateBufferSize  = 200
	blockerBufferSize   = 50
)

// Compile-time interface checks
var _ EventEmitter = (*Manager)(nil)

// Manager coordinates all watchers and emits events
type Manager struct {
	factory   informers.SharedInformerFactory
	namespace string

	eventCh     chan types.Event
	nodeStateCh chan types.NodeState
	podStateCh  chan types.PodState
	blockerCh   chan types.Blocker
	wg          sync.WaitGroup

	nodeWatcher  *NodeWatcher
	podWatcher   *PodWatcher
	eventWatcher *EventWatcher
	pdbWatcher   *PDBWatcher
	migrations   MigrationTracker
	stages       StageComputer
	pdbMu        sync.Mutex // Guards CheckPDBBlockers against concurrent informer callbacks
}

// NewManager creates a new watcher manager
func NewManager(factory informers.SharedInformerFactory, namespace string, targetVersion string) *Manager {
	eventCh := make(chan types.Event, eventBufferSize)
	nodeStateCh := make(chan types.NodeState, nodeStateBufferSize)
	podStateCh := make(chan types.PodState, podStateBufferSize)
	blockerCh := make(chan types.Blocker, blockerBufferSize)

	stages := NewStageComputer(targetVersion)
	migrations := NewMigrationTracker()

	m := &Manager{
		factory:     factory,
		namespace:   namespace,
		eventCh:     eventCh,
		nodeStateCh: nodeStateCh,
		podStateCh:  podStateCh,
		blockerCh:   blockerCh,
		stages:      stages,
		migrations:  migrations,
	}

	// Create watchers
	m.podWatcher = NewPodWatcher(factory, namespace, m, stages, migrations)
	m.nodeWatcher = NewNodeWatcher(factory, m, stages, m.countPodsOnNode, m.countEvictablePodsOnNode)
	m.eventWatcher = NewEventWatcher(factory, namespace, m)
	m.pdbWatcher = NewPDBWatcher(factory, namespace, m)

	// Wire PDB and node stage changes to trigger blocker re-evaluation
	m.pdbWatcher.SetOnChange(m.CheckPDBBlockers)
	m.nodeWatcher.onStageChangeFunc = m.CheckPDBBlockers

	// Wire surge event callback from EventWatcher to NodeWatcher
	m.eventWatcher.surgeCallback = m.handleSurgeEvent

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
	if err := m.pdbWatcher.Start(ctx); err != nil {
		return fmt.Errorf("watcher: %w", err)
	}

	// Start migration cleanup goroutine
	if tracker, ok := m.migrations.(*migrationTracker); ok {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			tracker.runCleanup(ctx)
		}()
	}

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

// PodStateUpdates returns channel for pod state changes
func (m *Manager) PodStateUpdates() <-chan types.PodState {
	return m.podStateCh
}

// BlockerUpdates returns channel for blocker changes
func (m *Manager) BlockerUpdates() <-chan types.Blocker {
	return m.blockerCh
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

// EmitNodeState sends a node state update (ring buffer semantics).
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

// EmitPodState sends a pod state update (ring buffer semantics)
func (m *Manager) EmitPodState(state types.PodState) {
	select {
	case m.podStateCh <- state:
	default:
		select {
		case <-m.podStateCh:
		default:
		}
		m.podStateCh <- state
	}
}

// EmitBlocker sends a blocker update (ring buffer semantics)
func (m *Manager) EmitBlocker(blocker types.Blocker) {
	select {
	case m.blockerCh <- blocker:
	default:
		select {
		case <-m.blockerCh:
		default:
		}
		m.blockerCh <- blocker
	}
}

// RefreshNodeState re-emits the current state of a node (called when pods change)
func (m *Manager) RefreshNodeState(nodeName string) {
	// Find the node in the informer store
	for _, obj := range m.nodeWatcher.informer.GetStore().List() {
		node, ok := obj.(*corev1.Node)
		if !ok {
			continue
		}
		if node.Name == nodeName {
			m.EmitNodeState(m.nodeWatcher.buildState(node))
			return
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
		m.eventWatcher.informer.HasSynced() &&
		m.pdbWatcher.informer.HasSynced()
}

// InitialNodeStates returns current state of all nodes (for initial TUI load)
func (m *Manager) InitialNodeStates() []types.NodeState {
	return m.nodeWatcher.buildNodeStates()
}

// InitialPodStates returns current state of all pods (for initial TUI load)
func (m *Manager) InitialPodStates() []types.PodState {
	return m.podWatcher.buildPodStates()
}

// InitialBlockers returns current blockers (for initial TUI load)
func (m *Manager) InitialBlockers() []types.Blocker {
	return m.pdbWatcher.BuildBlockers(m.nodesBlockableByPDB(), m.allPods())
}

// CheckPDBBlockers evaluates all PDBs against current cluster state and emits
// blocker updates. Sends a full replacement set: first clears all PDB blockers,
// then emits the current set. Called on node stage changes and PDB status updates.
// Serialized with pdbMu since informer callbacks run on separate goroutines.
func (m *Manager) CheckPDBBlockers() {
	m.pdbMu.Lock()
	defer m.pdbMu.Unlock()

	blockers := m.pdbWatcher.BuildBlockers(m.nodesBlockableByPDB(), m.allPods())

	// Signal TUI to replace all PDB blockers with the fresh set
	m.EmitBlocker(types.Blocker{
		Type:    types.BlockerPDB,
		Cleared: true,
	})
	for _, blocker := range blockers {
		m.EmitBlocker(blocker)
	}
}

// nodesBlockableByPDB returns names of all nodes in DRAINING or CORDONED stage.
// ComputeStage returns CORDONED for unschedulable nodes; the CORDONED→DRAINING
// correction based on evictable pod counts only happens in NodeWatcher.buildState.
// For PDB blocker evaluation, both stages are relevant: a cordoned node is about
// to be drained (or already is), so PDBs matching pods on it are active blockers.
func (m *Manager) nodesBlockableByPDB() []string {
	var nodes []string
	for _, obj := range m.nodeWatcher.informer.GetStore().List() {
		node, ok := obj.(*corev1.Node)
		if !ok {
			continue
		}
		stage := m.stages.ComputeStage(node)
		if stage == types.StageDraining || stage == types.StageCordoned {
			nodes = append(nodes, node.Name)
		}
	}
	return nodes
}

// allPods returns all pods from the informer cache.
func (m *Manager) allPods() []*corev1.Pod {
	objects := m.podWatcher.informer.GetStore().List()
	pods := make([]*corev1.Pod, 0, len(objects))
	for _, obj := range objects {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			continue
		}
		pods = append(pods, pod)
	}
	return pods
}

// countPodsOnNode counts all pods on a node (for display in overview/nodes).
func (m *Manager) countPodsOnNode(nodeName string) int {
	count := 0
	for _, obj := range m.podWatcher.informer.GetStore().List() {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			continue
		}
		if pod.Spec.NodeName == nodeName {
			count++
		}
	}
	return count
}

// countEvictablePodsOnNode counts non-DaemonSet pods on a node (for drain progress).
// DaemonSet pods are ignored by `kubectl drain --ignore-daemonsets`.
func (m *Manager) countEvictablePodsOnNode(nodeName string) int {
	count := 0
	for _, obj := range m.podWatcher.informer.GetStore().List() {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			continue
		}
		if pod.Spec.NodeName == nodeName && !isDaemonSetPod(pod) {
			count++
		}
	}
	return count
}

// isDaemonSetPod checks if pod is owned by a DaemonSet
func isDaemonSetPod(pod *corev1.Pod) bool {
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}

// handleSurgeEvent processes surge create/remove events from the EventWatcher.
// NOTE: We only act on "created" events. The "Removing surge node" event arrives
// BEFORE the actual node deletion, so we must NOT unmark here — otherwise onDelete
// sees wasSurge=false and creates a ghost. Cleanup happens naturally in onDelete.
func (m *Manager) handleSurgeEvent(nodeName, poolName string, created bool) {
	if created {
		m.nodeWatcher.MarkSurgeNode(nodeName, poolName)
	}
	// Don't unmark on "Removing" event — let onDelete handle cleanup
}

// IsSurgeNode returns true if the named node is a tracked surge node
func (m *Manager) IsSurgeNode(nodeName string) bool {
	return m.nodeWatcher.IsSurgeNode(nodeName)
}

// WaitForCacheSync waits for all caches to sync
func WaitForCacheSync(ctx context.Context, syncFuncs ...cache.InformerSynced) bool {
	return cache.WaitForCacheSync(ctx.Done(), syncFuncs...)
}
