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

// NodeWatcher watches node resources for upgrade-relevant changes
type NodeWatcher struct {
	informer              cache.SharedIndexInformer
	emitter               EventEmitter
	stages                StageComputer
	podCounter            func(nodeName string) int  // Total pods (for display)
	evictablePodCounter   func(nodeName string) int  // Non-DaemonSet pods (for drain progress)
	drainStartTimes       map[string]time.Time       // Track when each node started draining
	initialEvictableCount map[string]int             // Evictable pod count when drain started
	onStageChangeFunc     func()                     // Called when any node's stage changes
	surgeMu               sync.RWMutex               // Guards reimagingNodes and surgeNodes
	reimagingNodes        map[string]types.NodeState // Ghost nodes retained during reimage
	surgeNodes            map[string]bool            // Surge node names (from AKS Surge events)
}

// NewNodeWatcher creates a new node watcher
func NewNodeWatcher(factory informers.SharedInformerFactory, emitter EventEmitter, stages StageComputer, podCounter func(string) int, evictablePodCounter func(string) int) *NodeWatcher {
	return &NodeWatcher{
		informer:              factory.Core().V1().Nodes().Informer(),
		emitter:               emitter,
		stages:                stages,
		podCounter:            podCounter,
		evictablePodCounter:   evictablePodCounter,
		drainStartTimes:       make(map[string]time.Time),
		initialEvictableCount: make(map[string]int),
		reimagingNodes:        make(map[string]types.NodeState),
		surgeNodes:            make(map[string]bool),
	}
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

func (w *NodeWatcher) onAdd(obj interface{}) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		return
	}

	// Update target if this node has higher version
	w.stages.SetTargetVersion(node.Status.NodeInfo.KubeletVersion)

	// Check if this node is returning from reimaging
	w.surgeMu.Lock()
	ghostState, wasReimaging := w.reimagingNodes[node.Name]
	if wasReimaging {
		delete(w.reimagingNodes, node.Name)
	}
	isSurge := w.surgeNodes[node.Name]
	w.surgeMu.Unlock()

	if wasReimaging {
		targetVersion := w.stages.TargetVersion()
		nodeVersion := node.Status.NodeInfo.KubeletVersion

		if nodeVersion == targetVersion {
			// Reimaged to target version — mark COMPLETE
			state := w.buildState(node)
			state.Stage = types.StageComplete
			w.emitter.EmitNodeState(state)

			w.emitter.Emit(types.Event{
				Type:      types.EventNodeReady,
				Severity:  types.SeverityInfo,
				Timestamp: time.Now(),
				Message:   fmt.Sprintf("Node %s reimaged (%s → %s)", node.Name, ghostState.Version, nodeVersion),
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
		Message:   fmt.Sprintf("Node %s discovered (%s)", node.Name, node.Status.NodeInfo.KubeletVersion),
		NodeName:  node.Name,
	})

	// Emit computed state for TUI
	state := w.buildState(node)

	// Mark as surge node if tracked
	if isSurge {
		state.SurgeNode = true
	}

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

	// Update target if this node has higher version
	w.stages.SetTargetVersion(newNode.Status.NodeInfo.KubeletVersion)

	// Emit events for significant changes (for events panel)
	w.emitChangeEvents(oldNode, newNode)

	// Check if stage changed (for PDB blocker re-evaluation)
	oldStage := w.stages.ComputeStage(oldNode)
	newState := w.buildState(newNode)

	// Mark as surge node if tracked
	w.surgeMu.RLock()
	isSurge := w.surgeNodes[newNode.Name]
	w.surgeMu.RUnlock()
	if isSurge {
		newState.SurgeNode = true
	}

	// Always emit current state (TUI will update)
	w.emitter.EmitNodeState(newState)

	// Re-evaluate PDB blockers when draining set changes
	if oldStage != newState.Stage && w.onStageChangeFunc != nil {
		w.onStageChangeFunc()
	}
}

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

	// Check if this is a surge node before cleaning up tracking
	w.surgeMu.Lock()
	wasSurge := w.surgeNodes[node.Name]
	delete(w.surgeNodes, node.Name)
	w.surgeMu.Unlock()

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
	if w.isUpgradeActive() && !wasSurge {
		ghostState := w.buildState(node)
		ghostState.Stage = types.StageReimaging
		ghostState.Deleted = false
		w.surgeMu.Lock()
		w.reimagingNodes[node.Name] = ghostState
		w.surgeMu.Unlock()
		w.emitter.EmitNodeState(ghostState)
	} else {
		// No upgrade active or surge node — normal deletion
		w.emitter.EmitNodeState(types.NodeState{
			Name:    node.Name,
			Deleted: true,
		})
	}

	// Clean up drain tracking
	delete(w.drainStartTimes, node.Name)
	delete(w.initialEvictableCount, node.Name)

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
	w.surgeMu.RLock()
	for _, ghostState := range w.reimagingNodes {
		versions = append(versions, ghostState.Version)
	}
	w.surgeMu.RUnlock()
	w.stages.RecomputeVersions(versions)
}

// buildState creates NodeState from a k8s Node (single source of truth)
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

	// Track drain timing and correct stage using evictable pods
	var drainStart time.Time
	var initialEvictable int
	var drainProgress int

	if stage == types.StageDraining || stage == types.StageCordoned {
		// Check if we're already tracking this drain
		if start, ok := w.drainStartTimes[node.Name]; ok {
			drainStart = start
			initialEvictable = w.initialEvictableCount[node.Name]

			// Correct stage based on actual drain activity:
			// If evictable pods have been evicted (current < initial), we're DRAINING
			if evictableCount < initialEvictable {
				stage = types.StageDraining
			}
		} else {
			// First time seeing this node cordoned - record initial state
			drainStart = time.Now()
			initialEvictable = evictableCount
			w.drainStartTimes[node.Name] = drainStart
			w.initialEvictableCount[node.Name] = initialEvictable
			// Keep stage as CORDONED until evictable pods start decreasing
		}

		// Calculate drain progress based on evictable pods
		if initialEvictable > 0 {
			evicted := initialEvictable - evictableCount
			if evicted < 0 {
				evicted = 0
			}
			drainProgress = (evicted * 100) / initialEvictable
		}
	} else {
		// Node not cordoned - clear tracking if exists
		delete(w.drainStartTimes, node.Name)
		delete(w.initialEvictableCount, node.Name)
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

// extractConditions returns non-Ready conditions that are True (problems)
func extractConditions(node *corev1.Node) []string {
	var conditions []string
	for _, cond := range node.Status.Conditions {
		// Skip Ready condition (handled separately) and False conditions
		if cond.Type == corev1.NodeReady {
			continue
		}
		// These conditions are problems when True
		if cond.Status == corev1.ConditionTrue {
			conditions = append(conditions, string(cond.Type))
		}
	}
	return conditions
}

// extractTaints returns taint effects (NoSchedule, NoExecute, etc.)
func extractTaints(node *corev1.Node) []string {
	var taints []string
	seen := make(map[string]bool)
	for _, taint := range node.Spec.Taints {
		effect := string(taint.Effect)
		if !seen[effect] {
			taints = append(taints, effect)
			seen[effect] = true
		}
	}
	return taints
}

// formatAge returns human-readable age matching kubectl format (e.g., "5d2h", "3h14m", "30m")
func formatAge(created time.Time) string {
	d := time.Since(created)

	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	switch {
	case days > 0:
		if hours > 0 {
			return fmt.Sprintf("%dd%dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	case hours > 0:
		if minutes > 0 {
			return fmt.Sprintf("%dh%dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	case minutes > 0:
		return fmt.Sprintf("%dm", minutes)
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

// buildNodeStates returns current state of all nodes (for initial load)
func (w *NodeWatcher) buildNodeStates() []types.NodeState {
	var states []types.NodeState

	w.surgeMu.RLock()
	defer w.surgeMu.RUnlock()

	for _, obj := range w.informer.GetStore().List() {
		node, ok := obj.(*corev1.Node)
		if !ok {
			continue
		}
		state := w.buildState(node)
		if w.surgeNodes[node.Name] {
			state.SurgeNode = true
		}
		states = append(states, state)
	}

	// Include reimaging ghost nodes
	for _, ghostState := range w.reimagingNodes {
		states = append(states, ghostState)
	}

	return states
}

// isUpgradeActive returns true if an upgrade appears to be in progress.
// Uses mixed versions or nodes in non-terminal stages as signals.
// NOTE: Caller must NOT hold surgeMu — this method reads reimagingNodes.
func (w *NodeWatcher) isUpgradeActive() bool {
	lowest := w.stages.LowestVersion()
	target := w.stages.TargetVersion()
	if lowest != "" && target != "" && lowest != target {
		return true
	}

	// Also check if any nodes are in active upgrade stages
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

	// Check if we already have reimaging ghost nodes
	w.surgeMu.RLock()
	hasGhosts := len(w.reimagingNodes) > 0
	w.surgeMu.RUnlock()
	return hasGhosts
}

// MarkSurgeNode adds a node to the surge tracking set
func (w *NodeWatcher) MarkSurgeNode(nodeName, poolName string) {
	w.surgeMu.Lock()
	w.surgeNodes[nodeName] = true
	w.surgeMu.Unlock()

	// Retroactively mark the node as surge if it already exists in the informer
	for _, obj := range w.informer.GetStore().List() {
		node, ok := obj.(*corev1.Node)
		if !ok {
			continue
		}
		if node.Name == nodeName {
			state := w.buildState(node)
			state.SurgeNode = true
			w.emitter.EmitNodeState(state)
			return
		}
	}
}

// UnmarkSurgeNode removes a node from the surge tracking set
func (w *NodeWatcher) UnmarkSurgeNode(nodeName string) {
	w.surgeMu.Lock()
	delete(w.surgeNodes, nodeName)
	w.surgeMu.Unlock()
}

// IsSurgeNode returns true if the node is a tracked surge node
func (w *NodeWatcher) IsSurgeNode(nodeName string) bool {
	w.surgeMu.RLock()
	defer w.surgeMu.RUnlock()
	return w.surgeNodes[nodeName]
}

func isNodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}
