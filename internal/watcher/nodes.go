package watcher

import (
	"context"
	"fmt"
	"time"

	"github.com/sabirmohamed/kupgrade/pkg/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// NodeWatcher watches node resources for upgrade-relevant changes
type NodeWatcher struct {
	informer   cache.SharedIndexInformer
	emitter    EventEmitter
	stages     StageComputer
	podCounter func(nodeName string) int
}

// NewNodeWatcher creates a new node watcher
func NewNodeWatcher(factory informers.SharedInformerFactory, emitter EventEmitter, stages StageComputer, podCounter func(string) int) *NodeWatcher {
	return &NodeWatcher{
		informer:   factory.Core().V1().Nodes().Informer(),
		emitter:    emitter,
		stages:     stages,
		podCounter: podCounter,
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
	node := obj.(*corev1.Node)

	// Update target if this node has higher version
	w.stages.SetTargetVersion(node.Status.NodeInfo.KubeletVersion)

	// Emit event for events panel
	w.emitter.Emit(types.Event{
		Type:      types.EventNodeReady,
		Severity:  types.SeverityInfo,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Node %s discovered (%s)", node.Name, node.Status.NodeInfo.KubeletVersion),
		NodeName:  node.Name,
	})

	// Emit computed state for TUI
	w.emitter.EmitNodeState(w.buildState(node))
}

func (w *NodeWatcher) onUpdate(oldObj, newObj interface{}) {
	oldNode := oldObj.(*corev1.Node)
	newNode := newObj.(*corev1.Node)

	// Update target if this node has higher version
	w.stages.SetTargetVersion(newNode.Status.NodeInfo.KubeletVersion)

	// Emit events for significant changes (for events panel)
	w.emitChangeEvents(oldNode, newNode)

	// Always emit current state (TUI will update)
	w.emitter.EmitNodeState(w.buildState(newNode))
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

	w.emitter.Emit(types.Event{
		Type:      types.EventK8sWarning,
		Severity:  types.SeverityWarning,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Node %s deleted", node.Name),
		NodeName:  node.Name,
	})

	// Emit empty state to signal deletion
	w.emitter.EmitNodeState(types.NodeState{
		Name:    node.Name,
		Deleted: true,
	})
}

// buildState creates NodeState from a k8s Node (single source of truth)
func (w *NodeWatcher) buildState(node *corev1.Node) types.NodeState {
	podCount := 0
	if w.podCounter != nil {
		podCount = w.podCounter(node.Name)
	}

	return types.NodeState{
		Name:        node.Name,
		Stage:       w.stages.ComputeStage(node),
		Version:     node.Status.NodeInfo.KubeletVersion,
		Ready:       isNodeReady(node),
		Schedulable: !node.Spec.Unschedulable,
		PodCount:    podCount,
	}
}

// buildNodeStates returns current state of all nodes (for initial load)
func (w *NodeWatcher) buildNodeStates() []types.NodeState {
	var states []types.NodeState
	for _, obj := range w.informer.GetStore().List() {
		node := obj.(*corev1.Node)
		states = append(states, w.buildState(node))
	}
	return states
}

func isNodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}
