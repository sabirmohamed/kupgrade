package watcher

import (
	"context"
	"fmt"

	"github.com/sabirmohamed/kupgrade/pkg/types"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// PDBWatcher watches PodDisruptionBudgets and provides PDB details for blocker enrichment.
// NOTE: PDB blockers are now emitted based on FailedEviction events, not PDB status alone.
// This watcher maintains the PDB cache for detail lookup but does NOT emit blockers proactively.
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

// Start registers event handlers (required for informer to populate cache)
func (w *PDBWatcher) Start(ctx context.Context) error {
	// We need handlers registered for the informer cache to work,
	// but we don't emit blockers from here anymore.
	_, err := w.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) {},
		UpdateFunc: func(oldObj, newObj interface{}) {},
		DeleteFunc: w.onDelete,
	})
	if err != nil {
		return fmt.Errorf("pdb handler: %w", err)
	}
	return nil
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
	// We emit with empty NodeName to clear all blockers for this PDB.
	w.emitter.EmitBlocker(types.Blocker{
		Type:      types.BlockerPDB,
		Name:      pdb.Name,
		Namespace: pdb.Namespace,
		Cleared:   true,
	})
}

// GetPDBDetail returns a formatted detail string for a PDB, used to enrich blockers
// detected via events. Returns empty string if PDB not found.
func (w *PDBWatcher) GetPDBDetail(namespace, name string) string {
	key := namespace + "/" + name
	obj, exists, err := w.informer.GetStore().GetByKey(key)
	if err != nil || !exists {
		return ""
	}

	pdb := obj.(*policyv1.PodDisruptionBudget)
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

// buildBlockers returns empty slice - blockers are now event-driven.
// Kept for interface compatibility with InitialBlockers().
func (w *PDBWatcher) buildBlockers() []types.Blocker {
	// No longer return proactive blockers - they caused false positives.
	// Blockers are now emitted only when FailedEviction events occur.
	return []types.Blocker{}
}
