package watcher

import (
	"context"
	"fmt"

	"github.com/sabirmohamed/kupgrade/pkg/types"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// PDBWatcher watches PodDisruptionBudgets for upgrade blockers
type PDBWatcher struct {
	informer cache.SharedIndexInformer
	emitter  EventEmitter
}

// NewPDBWatcher creates a new PDB watcher
func NewPDBWatcher(factory informers.SharedInformerFactory, _ string, emitter EventEmitter) *PDBWatcher {
	return &PDBWatcher{
		informer: factory.Policy().V1().PodDisruptionBudgets().Informer(),
		emitter:  emitter,
	}
}

// Start registers event handlers
func (w *PDBWatcher) Start(ctx context.Context) error {
	_, err := w.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    w.onAdd,
		UpdateFunc: w.onUpdate,
		DeleteFunc: w.onDelete,
	})
	if err != nil {
		return fmt.Errorf("pdb handler: %w", err)
	}
	return nil
}

func (w *PDBWatcher) onAdd(obj interface{}) {
	pdb := obj.(*policyv1.PodDisruptionBudget)
	if blocker := w.buildBlocker(pdb); blocker != nil {
		w.emitter.EmitBlocker(*blocker)
	}
}

func (w *PDBWatcher) onUpdate(oldObj, newObj interface{}) {
	oldPDB := oldObj.(*policyv1.PodDisruptionBudget)
	newPDB := newObj.(*policyv1.PodDisruptionBudget)

	oldBlocking := isBlocking(oldPDB)
	newBlocking := isBlocking(newPDB)

	if newBlocking {
		// PDB is blocking - emit blocker
		if blocker := w.buildBlocker(newPDB); blocker != nil {
			w.emitter.EmitBlocker(*blocker)
		}
	} else if oldBlocking && !newBlocking {
		// PDB was blocking but is now resolved - emit cleared blocker
		w.emitter.EmitBlocker(types.Blocker{
			Type:      types.BlockerPDB,
			Name:      newPDB.Name,
			Namespace: newPDB.Namespace,
			Cleared:   true,
		})
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

	// If PDB was blocking, emit cleared blocker
	if isBlocking(pdb) {
		w.emitter.EmitBlocker(types.Blocker{
			Type:      types.BlockerPDB,
			Name:      pdb.Name,
			Namespace: pdb.Namespace,
			Cleared:   true,
		})
	}
}

// buildBlocker creates a Blocker if the PDB is blocking evictions
func (w *PDBWatcher) buildBlocker(pdb *policyv1.PodDisruptionBudget) *types.Blocker {
	if !isBlocking(pdb) {
		return nil
	}

	// Build informative detail explaining why it's blocking
	var detail string
	if pdb.Spec.MinAvailable != nil {
		detail = fmt.Sprintf("minAvailable=%s, %d/%d healthy → 0 evictions allowed",
			pdb.Spec.MinAvailable.String(),
			pdb.Status.CurrentHealthy,
			pdb.Status.ExpectedPods)
	} else if pdb.Spec.MaxUnavailable != nil {
		detail = fmt.Sprintf("maxUnavailable=%s, %d/%d healthy → 0 evictions allowed",
			pdb.Spec.MaxUnavailable.String(),
			pdb.Status.CurrentHealthy,
			pdb.Status.ExpectedPods)
	} else {
		detail = fmt.Sprintf("%d/%d healthy → 0 evictions allowed",
			pdb.Status.CurrentHealthy,
			pdb.Status.ExpectedPods)
	}

	return &types.Blocker{
		Type:      types.BlockerPDB,
		Name:      pdb.Name,
		Namespace: pdb.Namespace,
		Detail:    detail,
	}
}

// isBlocking returns true if the PDB is blocking evictions
func isBlocking(pdb *policyv1.PodDisruptionBudget) bool {
	// PDB blocks when disruptionsAllowed is 0
	// This happens when currentHealthy <= desiredHealthy
	return pdb.Status.DisruptionsAllowed == 0 && pdb.Status.ExpectedPods > 0
}

// buildBlockers returns current blocking PDBs (for initial load)
func (w *PDBWatcher) buildBlockers() []types.Blocker {
	var blockers []types.Blocker
	for _, obj := range w.informer.GetStore().List() {
		pdb := obj.(*policyv1.PodDisruptionBudget)
		if blocker := w.buildBlocker(pdb); blocker != nil {
			blockers = append(blockers, *blocker)
		}
	}
	return blockers
}
