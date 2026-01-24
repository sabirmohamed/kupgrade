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

// PodWatcher watches pod resources for upgrade-relevant changes
type PodWatcher struct {
	informer   cache.SharedIndexInformer
	emitter    EventEmitter
	stages     StageComputer
	migrations MigrationTracker
	namespace  string
}

// NewPodWatcher creates a new pod watcher
func NewPodWatcher(factory informers.SharedInformerFactory, namespace string, emitter EventEmitter, stages StageComputer, migrations MigrationTracker) *PodWatcher {
	informer := factory.Core().V1().Pods().Informer()

	return &PodWatcher{
		informer:   informer,
		emitter:    emitter,
		stages:     stages,
		migrations: migrations,
		namespace:  namespace,
	}
}

// Start registers event handlers
func (w *PodWatcher) Start(ctx context.Context) error {
	_, err := w.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    w.onAdd,
		UpdateFunc: w.onUpdate,
		DeleteFunc: w.onDelete,
	})
	if err != nil {
		return fmt.Errorf("failed to add pod event handler: %w", err)
	}

	return nil
}

func (w *PodWatcher) onAdd(obj interface{}) {
	pod := obj.(*corev1.Pod)

	// Filter by namespace if specified
	if w.namespace != "" && pod.Namespace != w.namespace {
		return
	}

	// Update pod count for node
	if pod.Spec.NodeName != "" {
		w.stages.UpdatePodCount(pod.Spec.NodeName, 1)
	}

	// Check for migration completion
	if migration := w.migrations.OnPodAdd(pod); migration != nil {
		w.emitter.Emit(types.Event{
			Type:      types.EventMigration,
			Severity:  types.SeverityInfo,
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("Pod %s/%s migrated: %s → %s", pod.Namespace, migration.NewPod, migration.FromNode, migration.ToNode),
			NodeName:  migration.ToNode,
			PodName:   migration.NewPod,
			Namespace: pod.Namespace,
		})
	}

	// Emit scheduling event if pod just got scheduled
	if pod.Spec.NodeName != "" && pod.Status.Phase == corev1.PodPending {
		w.emitter.Emit(types.Event{
			Type:      types.EventPodScheduled,
			Severity:  types.SeverityInfo,
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("Pod %s/%s scheduled to %s", pod.Namespace, pod.Name, pod.Spec.NodeName),
			NodeName:  pod.Spec.NodeName,
			PodName:   pod.Name,
			Namespace: pod.Namespace,
		})
	}
}

func (w *PodWatcher) onUpdate(oldObj, newObj interface{}) {
	oldPod := oldObj.(*corev1.Pod)
	newPod := newObj.(*corev1.Pod)

	// Filter by namespace if specified
	if w.namespace != "" && newPod.Namespace != w.namespace {
		return
	}

	// Check for eviction
	if newPod.Status.Reason == "Evicted" && oldPod.Status.Reason != "Evicted" {
		w.emitter.Emit(types.Event{
			Type:      types.EventPodEvicted,
			Severity:  types.SeverityWarning,
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("Pod %s/%s evicted from %s", newPod.Namespace, newPod.Name, newPod.Spec.NodeName),
			NodeName:  newPod.Spec.NodeName,
			PodName:   newPod.Name,
			Namespace: newPod.Namespace,
		})
	}

	// Check for Ready transition
	oldReady := isPodReady(oldPod)
	newReady := isPodReady(newPod)
	if !oldReady && newReady {
		w.emitter.Emit(types.Event{
			Type:      types.EventPodReady,
			Severity:  types.SeverityInfo,
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("Pod %s/%s is Ready on %s", newPod.Namespace, newPod.Name, newPod.Spec.NodeName),
			NodeName:  newPod.Spec.NodeName,
			PodName:   newPod.Name,
			Namespace: newPod.Namespace,
		})
	}

	// Check for Failed phase
	if newPod.Status.Phase == corev1.PodFailed && oldPod.Status.Phase != corev1.PodFailed {
		w.emitter.Emit(types.Event{
			Type:      types.EventPodFailed,
			Severity:  types.SeverityError,
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("Pod %s/%s failed on %s", newPod.Namespace, newPod.Name, newPod.Spec.NodeName),
			NodeName:  newPod.Spec.NodeName,
			PodName:   newPod.Name,
			Namespace: newPod.Namespace,
		})
	}
}

func (w *PodWatcher) onDelete(obj interface{}) {
	pod := obj.(*corev1.Pod)

	// Filter by namespace if specified
	if w.namespace != "" && pod.Namespace != w.namespace {
		return
	}

	// Update pod count for node
	if pod.Spec.NodeName != "" {
		w.stages.UpdatePodCount(pod.Spec.NodeName, -1)
	}

	// Track for potential migration
	w.migrations.OnPodDelete(pod)

	w.emitter.Emit(types.Event{
		Type:      types.EventPodDeleted,
		Severity:  types.SeverityInfo,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Pod %s/%s deleted from %s", pod.Namespace, pod.Name, pod.Spec.NodeName),
		NodeName:  pod.Spec.NodeName,
		PodName:   pod.Name,
		Namespace: pod.Namespace,
	})
}

// isPodReady checks if pod has Ready condition True
func isPodReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}
