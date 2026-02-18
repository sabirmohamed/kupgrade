package watcher

import (
	"context"
	"fmt"
	"time"

	"github.com/sabirmohamed/kupgrade/pkg/types"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// PDBWatcher watches PodDisruptionBudgets and evaluates blocker state using
// only Kubernetes-standard objects (PDB status + pod placement). No provider-specific
// event parsing — works identically on AKS, EKS, GKE, and vanilla K8s.
type PDBWatcher struct {
	informer          cache.SharedIndexInformer
	emitter           EventEmitter
	onChangeFunc      func()               // Called when PDB state changes (add/update)
	blockerStartTimes map[string]time.Time // Preserves StartTime across BuildBlockers calls
}

// NewPDBWatcher creates a new PDB watcher
func NewPDBWatcher(factory informers.SharedInformerFactory, _ string, emitter EventEmitter) *PDBWatcher {
	return &PDBWatcher{
		informer:          factory.Policy().V1().PodDisruptionBudgets().Informer(),
		emitter:           emitter,
		blockerStartTimes: make(map[string]time.Time),
	}
}

// SetOnChange sets the callback invoked when PDB state changes.
// Used by Manager to trigger CheckPDBBlockers on add/update.
func (w *PDBWatcher) SetOnChange(fn func()) {
	w.onChangeFunc = fn
}

// Start registers event handlers (required for informer to populate cache)
func (w *PDBWatcher) Start(ctx context.Context) error {
	_, err := w.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    w.onAddOrUpdate,
		UpdateFunc: func(oldObj, newObj interface{}) { w.onAddOrUpdate(newObj) },
		DeleteFunc: w.onDelete,
	})
	if err != nil {
		return fmt.Errorf("pdb handler: %w", err)
	}
	return nil
}

func (w *PDBWatcher) onAddOrUpdate(obj interface{}) {
	if w.onChangeFunc != nil {
		w.onChangeFunc()
	}
}

func (w *PDBWatcher) onDelete(obj interface{}) {
	pdb, ok := obj.(*policyv1.PodDisruptionBudget)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			return
		}
		pdb, ok = tombstone.Obj.(*policyv1.PodDisruptionBudget)
		if !ok {
			return
		}
	}

	// When a PDB is deleted, clear any blockers associated with it.
	// Empty NodeName clears all blockers for this PDB across all nodes.
	w.emitter.EmitBlocker(types.Blocker{
		Type:      types.BlockerPDB,
		Name:      pdb.Name,
		Namespace: pdb.Namespace,
		Cleared:   true,
	})
}

// GetPDBDetail returns a formatted detail string for a PDB.
// Returns empty string if PDB not found.
func (w *PDBWatcher) GetPDBDetail(namespace, name string) string {
	key := namespace + "/" + name
	obj, exists, err := w.informer.GetStore().GetByKey(key)
	if err != nil || !exists {
		return ""
	}

	pdb, ok := obj.(*policyv1.PodDisruptionBudget)
	if !ok {
		return ""
	}
	if pdb.Spec.MinAvailable != nil {
		return fmt.Sprintf("minAvailable=%s, %d/%d healthy",
			pdb.Spec.MinAvailable.String(),
			pdb.Status.CurrentHealthy,
			pdb.Status.ExpectedPods)
	} else if pdb.Spec.MaxUnavailable != nil {
		return fmt.Sprintf("maxUnavailable=%s, %d/%d healthy",
			pdb.Spec.MaxUnavailable.String(),
			pdb.Status.CurrentHealthy,
			pdb.Status.ExpectedPods)
	}
	return fmt.Sprintf("%d/%d healthy", pdb.Status.CurrentHealthy, pdb.Status.ExpectedPods)
}

// isAtRisk returns true if a PDB currently has zero disruption budget and is
// protecting pods. Used as a precondition for active blocker detection —
// if the PDB has budget available, it can't be blocking a drain.
func isAtRisk(pdb *policyv1.PodDisruptionBudget) bool {
	return pdb.Status.DisruptionsAllowed == 0 && pdb.Status.ExpectedPods > 0
}

// willBlockDrain returns true if a PDB is structurally misconfigured such that
// it cannot tolerate any pod disruption. This means DesiredHealthy >= ExpectedPods:
// the PDB requires ALL pods to be healthy, leaving zero room for eviction.
//
// Examples that return true:
//   - minAvailable: 3 with 3 replicas (needs all 3 healthy)
//   - minAvailable: 1 with 1 replica (single-pod PDB)
//   - maxUnavailable: 0 (explicitly zero disruptions)
//
// Examples that return false:
//   - minAvailable: 2 with 3 replicas (can tolerate 1 eviction)
//   - maxUnavailable: 1 with 3 replicas (normal PDB)
//
// Used for pre-flight checks — surfaces PDBs that WILL block drains, not
// transient DisruptionsAllowed==0 states that are normal during drain pacing.
func willBlockDrain(pdb *policyv1.PodDisruptionBudget) bool {
	return pdb.Status.DesiredHealthy >= pdb.Status.ExpectedPods && pdb.Status.ExpectedPods > 0
}

// isBlockingNode returns true if a PDB is actively blocking a specific node's
// drain — the PDB is at risk AND its selector matches at least one pod on
// the given node. This is Tier 2 (active blocker).
func isBlockingNode(pdb *policyv1.PodDisruptionBudget, nodeName string, pods []*corev1.Pod) bool {
	if !isAtRisk(pdb) {
		return false
	}

	selector, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
	if err != nil {
		return false
	}

	for _, pod := range pods {
		if pod.Spec.NodeName != nodeName {
			continue
		}
		if pod.Namespace != pdb.Namespace {
			continue
		}
		if selector.Matches(labels.Set(pod.Labels)) {
			return true
		}
	}
	return false
}

// firstMatchingPod returns the name of the first pod on the given node that
// matches the PDB selector. Returns empty string if no match.
func firstMatchingPod(pdb *policyv1.PodDisruptionBudget, nodeName string, pods []*corev1.Pod) string {
	selector, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
	if err != nil {
		return ""
	}

	for _, pod := range pods {
		if pod.Spec.NodeName != nodeName {
			continue
		}
		if pod.Namespace != pdb.Namespace {
			continue
		}
		if selector.Matches(labels.Set(pod.Labels)) {
			return pod.Name
		}
	}
	return ""
}

// BuildBlockers evaluates all PDBs against the current cluster state and returns
// only active blockers — PDBs that are blocking a specific node's drain right now
// AND the drain is stalled (no evictions for DrainStallThreshold).
//
// Transient DisruptionsAllowed==0 states (normal pacing during drain) are NOT
// reported. Only persistent blocks that require user intervention are shown.
//
// Uses only Kubernetes-standard objects. No provider-specific logic.
func blockerKey(namespace, name, nodeName string) string {
	return namespace + "/" + name + "/" + nodeName
}

func (w *PDBWatcher) BuildBlockers(drainingNodes []string, pods []*corev1.Pod, isDrainStalled func(nodeName string) bool) []types.Blocker {
	var blockers []types.Blocker
	now := time.Now()
	activeKeys := make(map[string]bool)

	for _, obj := range w.informer.GetStore().List() {
		pdb, ok := obj.(*policyv1.PodDisruptionBudget)
		if !ok {
			continue
		}

		if !isAtRisk(pdb) {
			continue
		}

		detail := w.GetPDBDetail(pdb.Namespace, pdb.Name)

		// Check if this PDB actively blocks any draining node.
		// A PDB is only an active blocker if:
		// 1. It's at risk (DisruptionsAllowed == 0)
		// 2. Its selector matches pods on the draining node
		// 3. The drain is stalled (no evictions for 30+ seconds)
		// This filters out transient PDB pacing during normal drain progression.
		for _, nodeName := range drainingNodes {
			if isBlockingNode(pdb, nodeName, pods) {
				key := blockerKey(pdb.Namespace, pdb.Name, nodeName)

				// Track start time even if not yet stalled (for accurate duration)
				startTime, exists := w.blockerStartTimes[key]
				if !exists {
					startTime = now
					w.blockerStartTimes[key] = startTime
				}
				activeKeys[key] = true

				// Only emit as active blocker if drain is stalled
				if isDrainStalled != nil && isDrainStalled(nodeName) {
					podName := firstMatchingPod(pdb, nodeName, pods)

					blockers = append(blockers, types.Blocker{
						Type:      types.BlockerPDB,
						Tier:      types.BlockerTierActive,
						Name:      pdb.Name,
						Namespace: pdb.Namespace,
						NodeName:  nodeName,
						PodName:   podName,
						Detail:    detail,
						StartTime: startTime,
					})
				}
			}
		}
	}

	// Clean up stale start times for blockers that no longer exist
	for key := range w.blockerStartTimes {
		if !activeKeys[key] {
			delete(w.blockerStartTimes, key)
		}
	}

	return blockers
}

// PreFlightBlockers returns PDBs that are structurally misconfigured and will
// block any drain that touches their pods. Unlike BuildBlockers (which detects
// active stalls during watch), this is for pre-flight/snapshot checks.
func (w *PDBWatcher) PreFlightBlockers() []types.Blocker {
	var blockers []types.Blocker
	for _, obj := range w.informer.GetStore().List() {
		pdb, ok := obj.(*policyv1.PodDisruptionBudget)
		if !ok {
			continue
		}
		if willBlockDrain(pdb) {
			blockers = append(blockers, types.Blocker{
				Type:      types.BlockerPDB,
				Tier:      types.BlockerTierRisk,
				Name:      pdb.Name,
				Namespace: pdb.Namespace,
				Detail:    w.GetPDBDetail(pdb.Namespace, pdb.Name),
			})
		}
	}
	return blockers
}
