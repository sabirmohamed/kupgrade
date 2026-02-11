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
	// lastEmittedEvent deduplicates pod events: podKey → last emitted event type.
	// Safe without a mutex because client-go's sharedIndexInformer processes
	// all event callbacks sequentially on a single goroutine.
	lastEmittedEvent map[string]types.EventType
}

// NewPodWatcher creates a new pod watcher
func NewPodWatcher(factory informers.SharedInformerFactory, namespace string, emitter EventEmitter, stages StageComputer, migrations MigrationTracker) *PodWatcher {
	informer := factory.Core().V1().Pods().Informer()

	return &PodWatcher{
		informer:         informer,
		emitter:          emitter,
		stages:           stages,
		migrations:       migrations,
		namespace:        namespace,
		lastEmittedEvent: make(map[string]types.EventType),
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
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}

	// Filter by namespace if specified
	if w.namespace != "" && pod.Namespace != w.namespace {
		return
	}

	// Emit pod state for TUI
	w.emitter.EmitPodState(w.buildState(pod))

	// Update pod count for node and refresh node state
	if pod.Spec.NodeName != "" {
		w.stages.UpdatePodCount(pod.Spec.NodeName, 1)
		w.emitter.RefreshNodeState(pod.Spec.NodeName)
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
	oldPod, ok := oldObj.(*corev1.Pod)
	if !ok {
		return
	}
	newPod, ok := newObj.(*corev1.Pod)
	if !ok {
		return
	}

	// Filter by namespace if specified
	if w.namespace != "" && newPod.Namespace != w.namespace {
		return
	}

	// Emit pod state for TUI
	w.emitter.EmitPodState(w.buildState(newPod))

	podKey := newPod.Namespace + "/" + newPod.Name

	// Clear dedup tracking when pod transitions away from the last emitted state.
	// This allows legitimate cycles (e.g., Ready→NotReady→Ready) to emit again.
	if lastEvent, ok := w.lastEmittedEvent[podKey]; ok {
		switch lastEvent {
		case types.EventPodReady:
			if !isPodReady(newPod) {
				delete(w.lastEmittedEvent, podKey)
			}
		case types.EventPodEvicted:
			if newPod.Status.Reason != "Evicted" {
				delete(w.lastEmittedEvent, podKey)
			}
		case types.EventPodFailed:
			if newPod.Status.Phase != corev1.PodFailed {
				delete(w.lastEmittedEvent, podKey)
			}
		}
	}

	// Check for eviction
	if newPod.Status.Reason == "Evicted" && oldPod.Status.Reason != "Evicted" {
		w.emitTransition(podKey, types.Event{
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
		w.emitTransition(podKey, types.Event{
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
		w.emitTransition(podKey, types.Event{
			Type:      types.EventPodFailed,
			Severity:  types.SeverityError,
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("Pod %s/%s failed on %s", newPod.Namespace, newPod.Name, newPod.Spec.NodeName),
			NodeName:  newPod.Spec.NodeName,
			PodName:   newPod.Name,
			Namespace: newPod.Namespace,
		})
	}

	// Check for migration completion (pod scheduled to a node for the first time)
	if oldPod.Spec.NodeName == "" && newPod.Spec.NodeName != "" {
		if migration := w.migrations.OnPodAdd(newPod); migration != nil {
			w.emitter.Emit(types.Event{
				Type:      types.EventMigration,
				Severity:  types.SeverityInfo,
				Timestamp: time.Now(),
				Message:   fmt.Sprintf("Pod %s/%s migrated: %s → %s", newPod.Namespace, migration.NewPod, migration.FromNode, migration.ToNode),
				NodeName:  migration.ToNode,
				PodName:   migration.NewPod,
				Namespace: newPod.Namespace,
			})
		}
	}
}

func (w *PodWatcher) onDelete(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		// Handle DeletedFinalStateUnknown (object deleted while disconnected)
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			return
		}
		pod, ok = tombstone.Obj.(*corev1.Pod)
		if !ok {
			return
		}
	}

	// Filter by namespace if specified
	if w.namespace != "" && pod.Namespace != w.namespace {
		return
	}

	// Update pod count for node and refresh node state
	if pod.Spec.NodeName != "" {
		w.stages.UpdatePodCount(pod.Spec.NodeName, -1)
		w.emitter.RefreshNodeState(pod.Spec.NodeName)
	}

	// Track for potential migration
	w.migrations.OnPodDelete(pod)

	// Clean up dedup tracking for deleted pod
	w.clearLastEmitted(pod.Namespace + "/" + pod.Name)

	// Emit deleted pod state for TUI
	w.emitter.EmitPodState(types.PodState{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		NodeName:  pod.Spec.NodeName,
		Deleted:   true,
	})

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

// emitTransition emits an event for a pod state transition, deduplicating
// repeated emissions of the same event type for the same pod.
func (w *PodWatcher) emitTransition(podKey string, event types.Event) {
	if w.lastEmittedEvent[podKey] == event.Type {
		return
	}
	w.lastEmittedEvent[podKey] = event.Type
	w.emitter.Emit(event)
}

// clearLastEmitted removes the dedup tracking for a deleted pod.
func (w *PodWatcher) clearLastEmitted(podKey string) {
	delete(w.lastEmittedEvent, podKey)
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

// buildState creates PodState from a k8s Pod
func (w *PodWatcher) buildState(pod *corev1.Pod) types.PodState {
	readyContainers, totalContainers := countReadyContainers(pod)
	hasLiveness, hasReadiness := hasProbes(pod)
	livenessOK, readinessOK := checkProbeStatus(pod)
	restarts, lastRestartAge := getRestartInfo(pod)

	return types.PodState{
		Name:            pod.Name,
		Namespace:       pod.Namespace,
		NodeName:        pod.Spec.NodeName,
		Ready:           isPodReady(pod),
		ReadyContainers: readyContainers,
		TotalContainers: totalContainers,
		Phase:           computePodStatus(pod),
		Restarts:        restarts,
		LastRestartAge:  lastRestartAge,
		Age:             formatAge(pod.CreationTimestamp.Time),
		HasLiveness:     hasLiveness,
		HasReadiness:    hasReadiness,
		LivenessOK:      livenessOK,
		ReadinessOK:     readinessOK,
		OwnerKind:       getOwnerKind(pod),
		OwnerRef:        getOwnerRef(pod),
	}
}

// computePodStatus returns detailed pod status like k9s (checking container states)
func computePodStatus(pod *corev1.Pod) string {
	// Check if pod is being deleted
	if pod.DeletionTimestamp != nil {
		return "Terminating"
	}

	// Check init container status first
	for _, cs := range pod.Status.InitContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			return "Init:" + cs.State.Waiting.Reason
		}
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
			return "Init:Error"
		}
	}

	// Check regular container statuses
	for _, cs := range pod.Status.ContainerStatuses {
		// Waiting states (CrashLoopBackOff, ImagePullBackOff, etc.)
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			return cs.State.Waiting.Reason
		}
		// Terminated states
		if cs.State.Terminated != nil {
			if cs.State.Terminated.Reason != "" {
				return cs.State.Terminated.Reason
			}
			if cs.State.Terminated.ExitCode != 0 {
				return "Error"
			}
		}
	}

	// Fall back to pod phase
	return string(pod.Status.Phase)
}

// buildPodStates returns current state of all pods (for initial load)
func (w *PodWatcher) buildPodStates() []types.PodState {
	var states []types.PodState
	for _, obj := range w.informer.GetStore().List() {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			continue
		}
		if w.namespace != "" && pod.Namespace != w.namespace {
			continue
		}
		states = append(states, w.buildState(pod))
	}
	return states
}

// getRestartInfo returns total restart count and age since most recent restart
// Returns (restarts, lastRestartAge) where lastRestartAge is like "4m", "8h", or "" if no restarts
func getRestartInfo(pod *corev1.Pod) (int, string) {
	restarts := 0
	var mostRecentRestart time.Time

	for _, cs := range pod.Status.ContainerStatuses {
		restarts += int(cs.RestartCount)

		// Check lastState.terminated.finishedAt for when container last died
		if cs.LastTerminationState.Terminated != nil {
			finishedAt := cs.LastTerminationState.Terminated.FinishedAt.Time
			if finishedAt.After(mostRecentRestart) {
				mostRecentRestart = finishedAt
			}
		}
	}

	// Also check init containers
	for _, cs := range pod.Status.InitContainerStatuses {
		restarts += int(cs.RestartCount)
		if cs.LastTerminationState.Terminated != nil {
			finishedAt := cs.LastTerminationState.Terminated.FinishedAt.Time
			if finishedAt.After(mostRecentRestart) {
				mostRecentRestart = finishedAt
			}
		}
	}

	lastRestartAge := ""
	if restarts > 0 && !mostRecentRestart.IsZero() {
		lastRestartAge = formatAge(mostRecentRestart)
	}

	return restarts, lastRestartAge
}

// countReadyContainers returns ready/total container counts
func countReadyContainers(pod *corev1.Pod) (ready, total int) {
	total = len(pod.Spec.Containers)
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Ready {
			ready++
		}
	}
	return ready, total
}

// hasProbes checks if pod has liveness/readiness probes configured
func hasProbes(pod *corev1.Pod) (hasLiveness, hasReadiness bool) {
	for _, c := range pod.Spec.Containers {
		if c.LivenessProbe != nil {
			hasLiveness = true
		}
		if c.ReadinessProbe != nil {
			hasReadiness = true
		}
	}
	return hasLiveness, hasReadiness
}

// checkProbeStatus checks liveness and readiness probe status
func checkProbeStatus(pod *corev1.Pod) (livenessOK, readinessOK bool) {
	livenessOK = true
	readinessOK = true

	for _, cs := range pod.Status.ContainerStatuses {
		// If container is not ready, readiness probe is failing
		if !cs.Ready {
			readinessOK = false
		}
		// If container is in CrashLoopBackOff, liveness is failing
		if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
			livenessOK = false
		}
	}
	return livenessOK, readinessOK
}

// getOwnerKind returns the kind of the pod's controller (Deployment, DaemonSet, etc.)
func getOwnerKind(pod *corev1.Pod) string {
	for _, ref := range pod.OwnerReferences {
		if ref.Controller != nil && *ref.Controller {
			return ref.Kind
		}
	}
	return ""
}

// getOwnerRef returns the UID of the pod's controller
func getOwnerRef(pod *corev1.Pod) string {
	for _, ref := range pod.OwnerReferences {
		if ref.Controller != nil && *ref.Controller {
			return string(ref.UID)
		}
	}
	return ""
}
