package watcher

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sabirmohamed/kupgrade/pkg/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// upgradeRelevantReasons filters K8s Events to upgrade-relevant ones.
// Note: Pod lifecycle events (Killing, Scheduled, Evicted) are intentionally
// excluded since PodWatcher already tracks these more precisely.
var upgradeRelevantReasons = map[string]bool{
	// Node lifecycle
	"NodeReady":               true,
	"NodeNotReady":            true,
	"NodeSchedulable":         true,
	"NodeNotSchedulable":      true,
	"Rebooted":                true,
	"NodeAllocatableEnforced": true,

	// Drain operations (node-level)
	"Drain":       true,
	"FailedDrain": true,
	"Cordon":      true,
	"Uncordon":    true,

	// Failures only (not normal pod lifecycle)
	"FailedKillPod":    true,
	"FailedScheduling": true,
	"FailedBinding":    true,

	// PDB (blockers) - these trigger blocker detection
	"DisruptionBlocked":               true,
	"CalculateExpectedPodCountFailed": true,
	"FailedEviction":                  true, // Added for PDB blocking

	// Volume (blockers)
	"FailedMount":        true,
	"FailedAttachVolume": true,
	"FailedDetachVolume": true,
	"VolumeFailedDelete": true,

	// Health issues
	"Unhealthy":           true,
	"ProbeWarning":        true,
	"BackOff":             true,
	"SystemOOM":           true,
	"FreeDiskSpaceFailed": true,
	"ContainerGCFailed":   true,
	"ImageGCFailed":       true,
}

// pdbBlockingPhrases are message substrings that indicate PDB-blocked eviction
var pdbBlockingPhrases = []string{
	"disruption budget",
	"Cannot evict pod",
	"would violate",
	"eviction blocked",
}

// EventWatcher watches Kubernetes events for upgrade-relevant occurrences
type EventWatcher struct {
	informer        cache.SharedIndexInformer
	emitter         EventEmitter
	blockerDetector BlockerDetector
	namespace       string
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

// SetBlockerDetector sets the blocker detector for correlating events with PDBs
func (w *EventWatcher) SetBlockerDetector(detector BlockerDetector) {
	w.blockerDetector = detector
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
	event, ok := obj.(*corev1.Event)
	if !ok {
		return
	}

	// Filter by namespace if specified
	if w.namespace != "" && event.Namespace != w.namespace {
		return
	}

	// Check if this is a PDB-blocking event (before filtering by reason)
	if w.isPDBBlockingEvent(event) {
		w.handlePDBBlockingEvent(event)
	}

	// Only process upgrade-relevant events for the event stream
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

// isPDBBlockingEvent checks if an event indicates a PDB-blocked eviction
func (w *EventWatcher) isPDBBlockingEvent(event *corev1.Event) bool {
	// Must be a warning about a pod
	if event.Type != corev1.EventTypeWarning {
		return false
	}
	if event.InvolvedObject.Kind != "Pod" {
		return false
	}

	// Check if message contains PDB-related phrases
	msg := strings.ToLower(event.Message)
	for _, phrase := range pdbBlockingPhrases {
		if strings.Contains(msg, strings.ToLower(phrase)) {
			return true
		}
	}

	return false
}

// handlePDBBlockingEvent emits a blocker when eviction is blocked by PDB
func (w *EventWatcher) handlePDBBlockingEvent(event *corev1.Event) {
	if w.blockerDetector == nil {
		return
	}

	podNamespace := event.InvolvedObject.Namespace
	podName := event.InvolvedObject.Name

	// Get the pod to find its node and match against PDBs
	pod := w.blockerDetector.GetPod(podNamespace, podName)
	if pod == nil {
		// Pod not found in cache, try to get just the node name
		nodeName := w.blockerDetector.GetPodNode(podNamespace, podName)
		if nodeName == "" {
			return // Can't determine which node is blocked
		}

		// Emit blocker without PDB details
		w.emitter.EmitBlocker(types.Blocker{
			Type:      types.BlockerPDB,
			Name:      "unknown",
			Namespace: podNamespace,
			NodeName:  nodeName,
			PodName:   podName,
			Detail:    fmt.Sprintf("Pod %s eviction blocked", podName),
			StartTime: time.Now(),
		})
		return
	}

	nodeName := pod.Spec.NodeName
	if nodeName == "" {
		return // Pod not scheduled, can't be blocking a drain
	}

	// Find which PDB is blocking this pod
	pdbNamespace, pdbName, detail := w.blockerDetector.FindBlockingPDB(pod)
	if pdbName == "" {
		// Couldn't find matching PDB, emit generic blocker
		pdbName = "unknown"
		pdbNamespace = podNamespace
		detail = fmt.Sprintf("Pod %s eviction blocked by PDB", podName)
	}

	w.emitter.EmitBlocker(types.Blocker{
		Type:      types.BlockerPDB,
		Name:      pdbName,
		Namespace: pdbNamespace,
		NodeName:  nodeName,
		PodName:   podName,
		Detail:    detail,
		StartTime: time.Now(),
	})
}
