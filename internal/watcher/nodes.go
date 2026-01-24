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
	informer cache.SharedIndexInformer
	emitter  EventEmitter
	stages   StageComputer
}

// NewNodeWatcher creates a new node watcher
func NewNodeWatcher(factory informers.SharedInformerFactory, emitter EventEmitter, stages StageComputer) *NodeWatcher {
	informer := factory.Core().V1().Nodes().Informer()

	return &NodeWatcher{
		informer: informer,
		emitter:  emitter,
		stages:   stages,
	}
}

// Start registers event handlers and waits for cache sync
func (w *NodeWatcher) Start(ctx context.Context) error {
	_, err := w.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    w.onAdd,
		UpdateFunc: w.onUpdate,
		DeleteFunc: w.onDelete,
	})
	if err != nil {
		return fmt.Errorf("failed to add node event handler: %w", err)
	}

	return nil
}

func (w *NodeWatcher) onAdd(obj interface{}) {
	node := obj.(*corev1.Node)

	// Update target version detection on first nodes
	w.stages.SetTargetVersion("") // Trigger auto-detection if not set

	// Emit initial state as info
	w.emitter.Emit(types.Event{
		Type:      types.EventNodeReady,
		Severity:  types.SeverityInfo,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Node %s discovered (version %s)", node.Name, node.Status.NodeInfo.KubeletVersion),
		NodeName:  node.Name,
	})
}

func (w *NodeWatcher) onUpdate(oldObj, newObj interface{}) {
	oldNode := oldObj.(*corev1.Node)
	newNode := newObj.(*corev1.Node)

	// Check cordon/uncordon
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

	// Check Ready condition changes
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

	// Check version changes
	oldVersion := oldNode.Status.NodeInfo.KubeletVersion
	newVersion := newNode.Status.NodeInfo.KubeletVersion
	if oldVersion != newVersion && oldVersion != "" && newVersion != "" {
		w.emitter.Emit(types.Event{
			Type:      types.EventNodeVersion,
			Severity:  types.SeverityInfo,
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("Node %s upgraded: %s → %s", newNode.Name, oldVersion, newVersion),
			NodeName:  newNode.Name,
		})
	}
}

func (w *NodeWatcher) onDelete(obj interface{}) {
	node := obj.(*corev1.Node)
	w.emitter.Emit(types.Event{
		Type:      types.EventK8sWarning,
		Severity:  types.SeverityWarning,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Node %s deleted", node.Name),
		NodeName:  node.Name,
	})
}

// GetNodeStates returns current state of all watched nodes
func (w *NodeWatcher) GetNodeStates() []types.NodeState {
	var states []types.NodeState
	for _, obj := range w.informer.GetStore().List() {
		node := obj.(*corev1.Node)
		states = append(states, types.NodeState{
			Name:        node.Name,
			Stage:       w.stages.ComputeStage(node),
			Version:     node.Status.NodeInfo.KubeletVersion,
			Ready:       isNodeReady(node),
			Schedulable: !node.Spec.Unschedulable,
		})
	}
	return states
}

// isNodeReady checks if node has Ready condition True
func isNodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}
