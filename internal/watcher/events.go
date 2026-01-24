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

// upgradeRelevantReasons from ADR-008
var upgradeRelevantReasons = map[string]bool{
	// Node lifecycle
	"NodeReady":               true,
	"NodeNotReady":            true,
	"NodeSchedulable":         true,
	"NodeNotSchedulable":      true,
	"Rebooted":                true,
	"NodeAllocatableEnforced": true,

	// Drain operations
	"Drain":       true,
	"FailedDrain": true,
	"Cordon":      true,
	"Uncordon":    true,

	// Pod disruption
	"Evicted":       true,
	"Preempted":     true,
	"Killing":       true,
	"FailedKillPod": true,

	// Scheduling
	"FailedScheduling": true,
	"Scheduled":        true,
	"FailedBinding":    true,

	// PDB (blockers)
	"DisruptionBlocked":              true,
	"CalculateExpectedPodCountFailed": true,

	// Volume (blockers)
	"FailedMount":        true,
	"FailedAttachVolume": true,
	"FailedDetachVolume": true,
	"VolumeFailedDelete": true,

	// Health
	"Unhealthy":          true,
	"ProbeWarning":       true,
	"BackOff":            true,
	"SystemOOM":          true,
	"FreeDiskSpaceFailed": true,
	"ContainerGCFailed":  true,
	"ImageGCFailed":      true,
}

// EventWatcher watches Kubernetes events for upgrade-relevant occurrences
type EventWatcher struct {
	informer  cache.SharedIndexInformer
	emitter   EventEmitter
	namespace string
}

// NewEventWatcher creates a new event watcher
func NewEventWatcher(factory informers.SharedInformerFactory, namespace string, emitter EventEmitter) *EventWatcher {
	informer := factory.Core().V1().Events().Informer()

	return &EventWatcher{
		informer:  informer,
		emitter:   emitter,
		namespace: namespace,
	}
}

// Start registers event handlers
func (w *EventWatcher) Start(ctx context.Context) error {
	_, err := w.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: w.onAdd,
	})
	if err != nil {
		return fmt.Errorf("failed to add event handler: %w", err)
	}

	return nil
}

func (w *EventWatcher) onAdd(obj interface{}) {
	event := obj.(*corev1.Event)

	// Filter by namespace if specified
	if w.namespace != "" && event.Namespace != w.namespace {
		return
	}

	// Only process upgrade-relevant events
	if !upgradeRelevantReasons[event.Reason] {
		return
	}

	// Determine event type and severity
	var eventType types.EventType
	var severity types.Severity

	switch event.Type {
	case corev1.EventTypeWarning:
		eventType = types.EventK8sWarning
		severity = types.SeverityWarning
	case corev1.EventTypeNormal:
		eventType = types.EventK8sNormal
		severity = types.SeverityInfo
	default:
		eventType = types.EventK8sError
		severity = types.SeverityError
	}

	// Extract node name from involved object if it's a node
	nodeName := ""
	if event.InvolvedObject.Kind == "Node" {
		nodeName = event.InvolvedObject.Name
	}

	w.emitter.Emit(types.Event{
		Type:      eventType,
		Severity:  severity,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("[%s] %s: %s", event.Reason, event.InvolvedObject.Name, event.Message),
		NodeName:  nodeName,
		PodName:   event.InvolvedObject.Name,
		Namespace: event.Namespace,
		Reason:    event.Reason,
	})
}
