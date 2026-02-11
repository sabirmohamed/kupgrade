package watcher

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sabirmohamed/kupgrade/pkg/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// DrainStallThreshold is how long a drain must be stalled before a PDB is
// considered an active blocker. Transient PDB blocks (normal pacing) resolve
// faster than this; persistent blocks (user intervention needed) exceed it.
const DrainStallThreshold = 30 * time.Second

// GhostTTL is how long a reimaging ghost node is kept before cleanup.
// If a node hasn't re-registered within this window, the ghost is orphaned.
const GhostTTL = 5 * time.Minute

// NodeLifecycle tracks per-node upgrade lifecycle state.
// All per-node state lives here — one map instead of seven.
type NodeLifecycle struct {
	// Identity
	IsSurge               bool // Deterministic: version-based or event-confirmed
	SurgeConfirmedByEvent bool // AKS Surge event arrived (secondary signal)

	// Version tracking
	PreUpgradeVersion string // Captured on first cordon (for ghost "from" version)
	LastKnownVersion  string // Updated on every onAdd/onUpdate

	// Drain tracking (replaces 4 independent maps)
	DrainStartTime        time.Time
	InitialEvictableCount int
	LastEvictableCount    int
	LastEvictionTime      time.Time

	// Stage lifecycle
	Completed   bool      // Latch: once true, stage cannot regress
	CompletedAt time.Time // When COMPLETE was first reached

	// Reimage tracking
	Reimaging        bool             // Node deleted during upgrade, awaiting re-register
	ReimageStartTime time.Time        // For TTL cleanup
	GhostState       *types.NodeState // Frozen state at delete time
	WasReimaged      bool             // Survives TTL cleanup — prevents false surge detection
}

// clearDrain resets all drain tracking fields.
func (lc *NodeLifecycle) clearDrain() {
	lc.DrainStartTime = time.Time{}
	lc.InitialEvictableCount = 0
	lc.LastEvictableCount = 0
	lc.LastEvictionTime = time.Time{}
}

// NodeWatcher watches node resources for upgrade-relevant changes
type NodeWatcher struct {
	informer            cache.SharedIndexInformer
	emitter             EventEmitter
	stages              StageComputer
	podCounter          func(nodeName string) int // Total pods (for display)
	evictablePodCounter func(nodeName string) int // Non-DaemonSet pods (for drain progress)
	onStageChangeFunc   func()                    // Called when any node's stage changes

	mu         sync.RWMutex              // Guards lifecycles map (single lock for all per-node state)
	lifecycles map[string]*NodeLifecycle // THE single map — replaces 7 independent maps
}

// NewNodeWatcher creates a new node watcher
func NewNodeWatcher(factory informers.SharedInformerFactory, emitter EventEmitter, stages StageComputer, podCounter func(string) int, evictablePodCounter func(string) int) *NodeWatcher {
	return &NodeWatcher{
		informer:            factory.Core().V1().Nodes().Informer(),
		emitter:             emitter,
		stages:              stages,
		podCounter:          podCounter,
		evictablePodCounter: evictablePodCounter,
		lifecycles:          make(map[string]*NodeLifecycle),
	}
}

// getOrCreateLifecycle returns the lifecycle for a node, creating it if needed.
// Caller MUST hold w.mu (write lock).
func (w *NodeWatcher) getOrCreateLifecycle(nodeName string) *NodeLifecycle {
	lc := w.lifecycles[nodeName]
	if lc == nil {
		lc = &NodeLifecycle{}
		w.lifecycles[nodeName] = lc
	}
	return lc
}

// applyLifecycleFlags applies surge flag and COMPLETE latch to a node state.
// Must be called after buildState() to ensure emitted states are consistent.
// Consolidates the surge check and latch into a single lock acquisition.
func (w *NodeWatcher) applyLifecycleFlags(state *types.NodeState) {
	w.mu.Lock()
	defer w.mu.Unlock()

	lc := w.lifecycles[state.Name]
	if lc != nil {
		if lc.IsSurge {
			state.SurgeNode = true
			// Surge nodes never went through the reimage cycle — they were born at
			// target version. ComputeStage returns COMPLETE for them, but they should
			// show as READY (with SURGE tag) since COMPLETE means "reimaged successfully".
			if state.Stage == types.StageComplete {
				state.Stage = types.StageReady
			}
		}
		if lc.Completed && !lc.IsSurge && state.Stage != types.StageComplete {
			state.Stage = types.StageComplete
		}
	}
	if state.Stage == types.StageComplete && !state.SurgeNode {
		completeLc := w.getOrCreateLifecycle(state.Name)
		if !completeLc.Completed {
			completeLc.Completed = true
			completeLc.CompletedAt = time.Now()
		}
	}
}

// RefreshState rebuilds and emits the current state of a node, applying all
// lifecycle flags (surge, COMPLETE latch). Called when pod counts change.
func (w *NodeWatcher) RefreshState(node *corev1.Node) {
	w.updateDrainTracking(node)
	state := w.buildState(node)
	w.applyLifecycleFlags(&state)
	w.emitter.EmitNodeState(state)
}

// Start registers event handlers
func (w *NodeWatcher) Start(ctx context.Context) error {
	_, err := w.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    w.onAdd,
		UpdateFunc: w.onUpdate,
		DeleteFunc: w.onDelete,
	})
	if err != nil {
		return fmt.Errorf("node handler: %w", err)
	}
	return nil
}

// updateDrainTracking updates drain lifecycle fields for a node.
// Called from informer callbacks and RefreshState — isolates side effects
// so that buildState() can be a pure reader.
func (w *NodeWatcher) updateDrainTracking(node *corev1.Node) {
	stage := w.stages.ComputeStage(node)
	evictableCount := 0
	if w.evictablePodCounter != nil {
		evictableCount = w.evictablePodCounter(node.Name)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	lc := w.lifecycles[node.Name]

	if stage == types.StageDraining || stage == types.StageCordoned {
		if lc == nil {
			lc = &NodeLifecycle{}
			w.lifecycles[node.Name] = lc
		}
		if lc.DrainStartTime.IsZero() {
			// First time seeing this node cordoned — record initial state
			lc.DrainStartTime = time.Now()
			lc.InitialEvictableCount = evictableCount
			lc.LastEvictableCount = evictableCount
			lc.LastEvictionTime = time.Now()

			// Capture pre-upgrade version on first cordon (for ghost "from" version)
			if lc.PreUpgradeVersion == "" {
				lc.PreUpgradeVersion = node.Status.NodeInfo.KubeletVersion
			}
		} else {
			// Track eviction activity: if evictable count decreased, record time
			if evictableCount < lc.LastEvictableCount {
				lc.LastEvictionTime = time.Now()
			}
			lc.LastEvictableCount = evictableCount
		}
	} else if lc != nil {
		lc.clearDrain()
	}
}

func (w *NodeWatcher) onAdd(obj interface{}) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		return
	}

	nodeVersion := node.Status.NodeInfo.KubeletVersion

	// Update target if this node has higher version
	w.stages.SetTargetVersion(nodeVersion)

	// Check lifecycle state: reimaging return? surge node?
	w.mu.Lock()
	lc := w.lifecycles[node.Name]
	var wasReimaging bool
	var ghostState *types.NodeState
	var preUpgradeVersion string
	if lc != nil {
		wasReimaging = lc.Reimaging
		ghostState = lc.GhostState
		preUpgradeVersion = lc.PreUpgradeVersion
		if wasReimaging {
			lc.Reimaging = false
			lc.GhostState = nil
		}
		lc.LastKnownVersion = nodeVersion
	}
	w.mu.Unlock()

	if wasReimaging {
		targetVersion := w.stages.TargetVersion()

		if nodeVersion == targetVersion {
			// Reimaged to target version — mark COMPLETE
			w.updateDrainTracking(node)
			state := w.buildState(node)
			state.Stage = types.StageComplete
			w.applyLifecycleFlags(&state)

			w.emitter.EmitNodeState(state)

			// Use PreUpgradeVersion for the "from" version (fixes ghost version bug).
			// Falls back to ghost state version if PreUpgradeVersion wasn't captured.
			fromVersion := preUpgradeVersion
			if fromVersion == "" && ghostState != nil {
				fromVersion = ghostState.Version
			}
			w.emitter.Emit(types.Event{
				Type:      types.EventNodeReady,
				Severity:  types.SeverityInfo,
				Timestamp: time.Now(),
				Message:   fmt.Sprintf("Node %s reimaged (%s → %s)", node.Name, fromVersion, nodeVersion),
				NodeName:  node.Name,
			})

			if w.onStageChangeFunc != nil {
				w.onStageChangeFunc()
			}
			return
		}

		// Came back at old version — recompute stage normally
	}

	// Emit event for events panel
	w.emitter.Emit(types.Event{
		Type:      types.EventNodeReady,
		Severity:  types.SeverityInfo,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Node %s discovered (%s)", node.Name, nodeVersion),
		NodeName:  node.Name,
	})

	// Update drain tracking before building state (side effects isolated here)
	w.updateDrainTracking(node)

	// Track version + version-based surge detection
	w.mu.Lock()
	addLc := w.getOrCreateLifecycle(node.Name)
	addLc.LastKnownVersion = nodeVersion

	// Version-based surge detection: new node at target version during active
	// upgrade with no prior drain history = surge candidate. This fires <1s after
	// node registers vs ~49s delay for AKS Surge event.
	//
	// Uses version mismatch (lowest != target) as the trigger, NOT isUpgradeActive().
	// isUpgradeActive() can return true from transient NotReady nodes (ComputeStage
	// returns REIMAGING), causing false surge tags on normal scale-up operations.
	// Version mismatch is the definitive signal that a K8s upgrade is in progress.
	if !addLc.IsSurge && !wasReimaging && !addLc.WasReimaged && addLc.DrainStartTime.IsZero() {
		targetVersion := w.stages.TargetVersion()
		lowestVersion := w.stages.LowestVersion()
		versionMismatch := lowestVersion != "" && targetVersion != "" && lowestVersion != targetVersion
		if nodeVersion == targetVersion && versionMismatch {
			addLc.IsSurge = true
		}
	}
	w.mu.Unlock()

	// Emit computed state with lifecycle flags (surge + COMPLETE latch)
	state := w.buildState(node)
	w.applyLifecycleFlags(&state)
	w.emitter.EmitNodeState(state)

	// If the new node is cordoned or draining, re-evaluate PDB blockers
	if (state.Stage == types.StageDraining || state.Stage == types.StageCordoned) && w.onStageChangeFunc != nil {
		w.onStageChangeFunc()
	}
}

func (w *NodeWatcher) onUpdate(oldObj, newObj interface{}) {
	oldNode, ok := oldObj.(*corev1.Node)
	if !ok {
		return
	}
	newNode, ok := newObj.(*corev1.Node)
	if !ok {
		return
	}

	nodeVersion := newNode.Status.NodeInfo.KubeletVersion

	// Update target if this node has higher version
	w.stages.SetTargetVersion(nodeVersion)

	// Emit events for significant changes (for events panel)
	w.emitChangeEvents(oldNode, newNode)

	// Capture raw old stage before any lifecycle processing
	oldStage := w.stages.ComputeStage(oldNode)

	// Update drain tracking before building state (side effects isolated here)
	w.updateDrainTracking(newNode)

	// Track version
	w.mu.Lock()
	updateLc := w.getOrCreateLifecycle(newNode.Name)
	updateLc.LastKnownVersion = nodeVersion
	w.mu.Unlock()

	// Build and emit state with lifecycle flags (surge + COMPLETE latch)
	newState := w.buildState(newNode)
	w.applyLifecycleFlags(&newState)
	w.emitter.EmitNodeState(newState)

	// Re-evaluate PDB blockers when effective stage changes.
	// Uses newState.Stage (post-latch) to detect COMPLETE transitions
	// that raw ComputeStage would miss (e.g., latch overrides READY→COMPLETE).
	if oldStage != newState.Stage && w.onStageChangeFunc != nil {
		w.onStageChangeFunc()
	}
}

func (w *NodeWatcher) onDelete(obj interface{}) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			return
		}
		node, ok = tombstone.Obj.(*corev1.Node)
		if !ok {
			return
		}
	}

	// Read lifecycle status
	w.mu.RLock()
	lc := w.lifecycles[node.Name]
	wasSurge := lc != nil && lc.IsSurge
	wasCompleted := lc != nil && lc.Completed
	preUpgradeVersion := ""
	if lc != nil {
		preUpgradeVersion = lc.PreUpgradeVersion
	}
	w.mu.RUnlock()

	// Surge node deletion is expected — emit as info, not warning
	if wasSurge {
		w.emitter.Emit(types.Event{
			Type:      types.EventK8sNormal,
			Severity:  types.SeverityInfo,
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("Surge node %s deleted", node.Name),
			NodeName:  node.Name,
		})
	} else {
		w.emitter.Emit(types.Event{
			Type:      types.EventK8sWarning,
			Severity:  types.SeverityWarning,
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("Node %s deleted", node.Name),
			NodeName:  node.Name,
		})
	}

	// If an upgrade is active AND this is NOT a surge node, retain as REIMAGING (ghost node).
	// Surge nodes are temporary — they don't reimage, they just get removed.
	// Double-delete protection: if node already completed upgrade, don't create ghost.
	if w.isUpgradeActive() && !wasSurge && !wasCompleted {
		ghostState := w.buildState(node)
		ghostState.Stage = types.StageReimaging
		ghostState.Deleted = false

		// Use PreUpgradeVersion for ghost (fixes wrong version bug).
		// AKS Pattern A flips kubeletVersion to target before deletion,
		// so buildState captures the wrong (target) version.
		if preUpgradeVersion != "" {
			ghostState.Version = preUpgradeVersion
		}

		w.mu.Lock()
		ghostLc := w.getOrCreateLifecycle(node.Name)
		ghostLc.Reimaging = true
		ghostLc.WasReimaged = true // Survives TTL cleanup — prevents false surge on re-register
		ghostLc.ReimageStartTime = time.Now()
		ghostLc.GhostState = &ghostState
		ghostLc.Completed = false // Clear latch — node is reimaging
		ghostLc.clearDrain()
		w.mu.Unlock()

		w.emitter.EmitNodeState(ghostState)
	} else {
		// No upgrade active or surge node — normal deletion
		w.emitter.EmitNodeState(types.NodeState{
			Name:    node.Name,
			Deleted: true,
		})

		// Clean up lifecycle entirely
		w.mu.Lock()
		delete(w.lifecycles, node.Name)
		w.mu.Unlock()
	}

	// Re-evaluate PDB blockers since draining set may have changed
	if w.onStageChangeFunc != nil {
		w.onStageChangeFunc()
	}

	// Recompute versions from remaining nodes + ghost nodes
	var versions []string
	for _, obj := range w.informer.GetStore().List() {
		remainingNode, ok := obj.(*corev1.Node)
		if !ok {
			continue
		}
		versions = append(versions, remainingNode.Status.NodeInfo.KubeletVersion)
	}
	w.mu.RLock()
	for _, ghostLc := range w.lifecycles {
		if ghostLc.Reimaging && ghostLc.GhostState != nil {
			versions = append(versions, ghostLc.GhostState.Version)
		}
	}
	w.mu.RUnlock()
	w.stages.RecomputeVersions(versions)
}

// buildState creates NodeState from a k8s Node (single source of truth).
// Pure reader — all side effects are in updateDrainTracking().
func (w *NodeWatcher) buildState(node *corev1.Node) types.NodeState {
	// Total pods for display (includes DaemonSets)
	podCount := 0
	if w.podCounter != nil {
		podCount = w.podCounter(node.Name)
	}

	// Evictable pods for drain progress (excludes DaemonSets)
	evictableCount := 0
	if w.evictablePodCounter != nil {
		evictableCount = w.evictablePodCounter(node.Name)
	}

	stage := w.stages.ComputeStage(node)

	// Read drain state from lifecycle under lock — copy into locals to avoid
	// data race with updateDrainTracking() writing these fields concurrently.
	var drainStart time.Time
	var initialEvictable int
	var drainProgress int

	w.mu.RLock()
	lc := w.lifecycles[node.Name]
	if lc != nil && (stage == types.StageDraining || stage == types.StageCordoned) {
		drainStart = lc.DrainStartTime
		initialEvictable = lc.InitialEvictableCount
	}
	w.mu.RUnlock()

	if initialEvictable > 0 && (stage == types.StageDraining || stage == types.StageCordoned) {
		// Correct stage based on actual drain activity:
		// DRAINING only when evictable pods remain and some have been evicted.
		// When evictableCount hits 0, drain is complete — revert to CORDONED.
		if evictableCount > 0 && evictableCount < initialEvictable {
			stage = types.StageDraining
		}

		// Calculate drain progress based on evictable pods
		evicted := initialEvictable - evictableCount
		if evicted < 0 {
			evicted = 0
		}
		drainProgress = (evicted * 100) / initialEvictable
	}

	return types.NodeState{
		Name:              node.Name,
		Stage:             stage,
		Version:           node.Status.NodeInfo.KubeletVersion,
		Ready:             isNodeReady(node),
		Schedulable:       !node.Spec.Unschedulable,
		PodCount:          podCount,
		EvictablePodCount: evictableCount,
		Conditions:        extractConditions(node),
		Taints:            extractTaints(node),
		Age:               formatAge(node.CreationTimestamp.Time),
		DrainStartTime:    drainStart,
		InitialPodCount:   initialEvictable,
		DrainProgress:     drainProgress,
	}
}

// buildNodeStates returns current state of all nodes (for initial load)
func (w *NodeWatcher) buildNodeStates() []types.NodeState {
	var states []types.NodeState

	for _, obj := range w.informer.GetStore().List() {
		node, ok := obj.(*corev1.Node)
		if !ok {
			continue
		}
		state := w.buildState(node)
		w.applyLifecycleFlags(&state)
		states = append(states, state)
	}

	// Include reimaging ghost nodes
	w.mu.RLock()
	for _, lc := range w.lifecycles {
		if lc.Reimaging && lc.GhostState != nil {
			states = append(states, *lc.GhostState)
		}
	}
	w.mu.RUnlock()

	return states
}

// hasUpgradeSignals checks version mismatch and active upgrade stages.
// Does not check ghost nodes — caller must check separately if needed.
// Safe to call regardless of mu lock state (does not access lifecycles).
func (w *NodeWatcher) hasUpgradeSignals() bool {
	lowest := w.stages.LowestVersion()
	target := w.stages.TargetVersion()
	if lowest != "" && target != "" && lowest != target {
		return true
	}

	for _, obj := range w.informer.GetStore().List() {
		node, ok := obj.(*corev1.Node)
		if !ok {
			continue
		}
		stage := w.stages.ComputeStage(node)
		if stage == types.StageCordoned || stage == types.StageDraining || stage == types.StageReimaging {
			return true
		}
	}
	return false
}

// hasReimagingUnlocked checks if any nodes are in reimaging state.
// Caller MUST hold w.mu (read or write lock).
func (w *NodeWatcher) hasReimagingUnlocked() bool {
	for _, lc := range w.lifecycles {
		if lc.Reimaging {
			return true
		}
	}
	return false
}

// isUpgradeActive returns true if an upgrade appears to be in progress.
// Uses mixed versions, active upgrade stages, or reimaging ghosts as signals.
// NOTE: Caller must NOT hold mu — this method acquires mu.RLock.
func (w *NodeWatcher) isUpgradeActive() bool {
	if w.hasUpgradeSignals() {
		return true
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.hasReimagingUnlocked()
}

// CleanupExpiredGhosts removes ghost nodes that have exceeded the TTL.
// Called periodically to prevent orphaned ghosts from accumulating.
func (w *NodeWatcher) CleanupExpiredGhosts() {
	w.mu.Lock()
	var expired []string
	for name, lc := range w.lifecycles {
		if lc.Reimaging && time.Since(lc.ReimageStartTime) > GhostTTL {
			expired = append(expired, name)
		}
	}
	for _, name := range expired {
		// Preserve WasReimaged marker so that if the node re-registers at target
		// version, it won't be falsely detected as a surge node.
		w.lifecycles[name] = &NodeLifecycle{WasReimaged: true}
	}
	w.mu.Unlock()

	// Emit deletion for each expired ghost so TUI removes them
	for _, name := range expired {
		w.emitter.EmitNodeState(types.NodeState{
			Name:    name,
			Deleted: true,
		})
		w.emitter.Emit(types.Event{
			Type:      types.EventK8sWarning,
			Severity:  types.SeverityWarning,
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("Ghost node %s expired (no re-registration within %v)", name, GhostTTL),
			NodeName:  name,
		})
	}
}

// IsDrainStalled returns true if the node is draining and no pods have been
// evicted for longer than the threshold. This distinguishes real PDB blockers
// (drain stuck) from transient PDB pacing (drain progressing normally).
func (w *NodeWatcher) IsDrainStalled(nodeName string, threshold time.Duration) bool {
	w.mu.RLock()
	lc := w.lifecycles[nodeName]
	var lastEvictionTime time.Time
	if lc != nil {
		lastEvictionTime = lc.LastEvictionTime
	}
	w.mu.RUnlock()
	if lastEvictionTime.IsZero() {
		return false
	}
	return time.Since(lastEvictionTime) > threshold
}

// MarkSurgeNode marks a node as surge via event confirmation
func (w *NodeWatcher) MarkSurgeNode(nodeName, poolName string) {
	w.mu.Lock()
	lc := w.getOrCreateLifecycle(nodeName)
	lc.IsSurge = true
	lc.SurgeConfirmedByEvent = true
	w.mu.Unlock()

	// Retroactively mark the node as surge if it already exists in the informer
	for _, obj := range w.informer.GetStore().List() {
		node, ok := obj.(*corev1.Node)
		if !ok {
			continue
		}
		if node.Name == nodeName {
			state := w.buildState(node)
			w.applyLifecycleFlags(&state)
			w.emitter.EmitNodeState(state)
			return
		}
	}
}

// UnmarkSurgeNode removes surge status from a node
func (w *NodeWatcher) UnmarkSurgeNode(nodeName string) {
	w.mu.Lock()
	if lc := w.lifecycles[nodeName]; lc != nil {
		lc.IsSurge = false
		lc.SurgeConfirmedByEvent = false
	}
	w.mu.Unlock()
}

// IsSurgeNode returns true if the node is a tracked surge node
func (w *NodeWatcher) IsSurgeNode(nodeName string) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	lc := w.lifecycles[nodeName]
	return lc != nil && lc.IsSurge
}

// emitChangeEvents emits events for significant node changes (for events panel).
func (w *NodeWatcher) emitChangeEvents(oldNode, newNode *corev1.Node) {
	// Cordon/uncordon
	if !oldNode.Spec.Unschedulable && newNode.Spec.Unschedulable {
		w.emitter.Emit(types.Event{
			Type:      types.EventNodeCordon,
			Severity:  types.SeverityWarning,
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("Node %s cordoned", newNode.Name),
			NodeName:  newNode.Name,
		})
	} else if oldNode.Spec.Unschedulable && !newNode.Spec.Unschedulable {
		w.emitter.Emit(types.Event{
			Type:      types.EventNodeUncordon,
			Severity:  types.SeverityInfo,
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("Node %s uncordoned", newNode.Name),
			NodeName:  newNode.Name,
		})
	}

	// Ready condition
	oldReady := isNodeReady(oldNode)
	newReady := isNodeReady(newNode)
	if oldReady && !newReady {
		w.emitter.Emit(types.Event{
			Type:      types.EventNodeNotReady,
			Severity:  types.SeverityWarning,
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("Node %s is NotReady", newNode.Name),
			NodeName:  newNode.Name,
		})
	} else if !oldReady && newReady {
		w.emitter.Emit(types.Event{
			Type:      types.EventNodeReady,
			Severity:  types.SeverityInfo,
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("Node %s is Ready", newNode.Name),
			NodeName:  newNode.Name,
		})
	}

	// Version change
	oldVer := oldNode.Status.NodeInfo.KubeletVersion
	newVer := newNode.Status.NodeInfo.KubeletVersion
	if oldVer != newVer && oldVer != "" && newVer != "" {
		w.emitter.Emit(types.Event{
			Type:      types.EventNodeVersion,
			Severity:  types.SeverityInfo,
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("Node %s upgraded: %s → %s", newNode.Name, oldVer, newVer),
			NodeName:  newNode.Name,
		})
	}
}
