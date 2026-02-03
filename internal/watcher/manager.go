package watcher

import (
	"context"
	"fmt"
	"sync"

	"github.com/sabirmohamed/kupgrade/pkg/types"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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
var _ BlockerDetector = (*Manager)(nil)

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

	// Wire up blocker detection (event watcher needs access to pod and PDB data)
	m.eventWatcher.SetBlockerDetector(m)

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
		node := obj.(*corev1.Node)
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
	return m.pdbWatcher.buildBlockers()
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

// WaitForCacheSync waits for all caches to sync
func WaitForCacheSync(ctx context.Context, syncFuncs ...cache.InformerSynced) bool {
	return cache.WaitForCacheSync(ctx.Done(), syncFuncs...)
}

// BlockerDetector implementation

// GetPodNode returns the node name for a pod, empty if not found.
func (m *Manager) GetPodNode(namespace, name string) string {
	key := namespace + "/" + name
	obj, exists, err := m.podWatcher.informer.GetStore().GetByKey(key)
	if err != nil || !exists {
		return ""
	}
	pod := obj.(*corev1.Pod)
	return pod.Spec.NodeName
}

// GetPod returns the pod object, nil if not found.
func (m *Manager) GetPod(namespace, name string) *corev1.Pod {
	key := namespace + "/" + name
	obj, exists, err := m.podWatcher.informer.GetStore().GetByKey(key)
	if err != nil || !exists {
		return nil
	}
	return obj.(*corev1.Pod)
}

// FindBlockingPDB finds a PDB that matches the given pod and has 0 disruption budget.
// Returns namespace, name, detail. Empty strings if no blocking PDB found.
func (m *Manager) FindBlockingPDB(pod *corev1.Pod) (namespace, name, detail string) {
	if pod == nil {
		return "", "", ""
	}

	podLabels := labels.Set(pod.Labels)

	// Iterate through all PDBs to find one that matches this pod
	for _, obj := range m.pdbWatcher.informer.GetStore().List() {
		pdb := obj.(*policyv1.PodDisruptionBudget)

		// PDB must be in same namespace as pod
		if pdb.Namespace != pod.Namespace {
			continue
		}

		// Check if PDB is blocking (0 disruptions allowed)
		if pdb.Status.DisruptionsAllowed > 0 {
			continue
		}

		// Check if PDB selector matches pod
		selector, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
		if err != nil {
			continue
		}

		if selector.Matches(podLabels) {
			// Found a blocking PDB that matches this pod
			detail := m.pdbWatcher.GetPDBDetail(pdb.Namespace, pdb.Name)
			return pdb.Namespace, pdb.Name, detail
		}
	}

	return "", "", ""
}
