package tui

import (
	"testing"

	"github.com/sabirmohamed/kupgrade/pkg/types"
)

func newTestModel() *Model {
	eventCh := make(chan types.Event)
	nodeCh := make(chan types.NodeState)
	podCh := make(chan types.PodState)
	blockerCh := make(chan types.Blocker)
	close(eventCh)
	close(nodeCh)
	close(podCh)
	close(blockerCh)

	m := New(Config{
		Context:       "test-cluster",
		ServerVersion: "v1.32.9",
		TargetVersion: "v1.33.2",
		EventCh:       eventCh,
		NodeStateCh:   nodeCh,
		PodStateCh:    podCh,
		BlockerCh:     blockerCh,
	})
	m.width = 120
	m.height = 40
	return &m
}

func TestProgressPercent_ExcludesSurgeNodes(t *testing.T) {
	m := newTestModel()

	// 3 real nodes: 1 COMPLETE + 2 READY
	m.nodes = map[string]types.NodeState{
		"node-1": {Name: "node-1", Stage: types.StageComplete, Version: "v1.33.2"},
		"node-2": {Name: "node-2", Stage: types.StageReady, Version: "v1.32.9"},
		"node-3": {Name: "node-3", Stage: types.StageReady, Version: "v1.32.9"},
		// Surge node at target version — should be excluded
		"surge-1": {Name: "surge-1", Stage: types.StageComplete, Version: "v1.33.2", SurgeNode: true},
	}
	m.rebuildNodesByStage()

	// Progress should be 1/3 (33%), not 2/4 (50%)
	total := m.totalNodes()
	if total != 3 {
		t.Errorf("totalNodes() = %d, want 3 (excluding surge)", total)
	}

	completed := m.completedNodes()
	if completed != 1 {
		t.Errorf("completedNodes() = %d, want 1 (excluding surge)", completed)
	}

	percent := m.progressPercent()
	if percent != 33 {
		t.Errorf("progressPercent() = %d, want 33", percent)
	}
}

func TestStageCountExcludingSurge(t *testing.T) {
	m := newTestModel()

	m.nodes = map[string]types.NodeState{
		"node-1":  {Name: "node-1", Stage: types.StageComplete, Version: "v1.33.2"},
		"node-2":  {Name: "node-2", Stage: types.StageComplete, Version: "v1.33.2"},
		"surge-1": {Name: "surge-1", Stage: types.StageComplete, Version: "v1.33.2", SurgeNode: true},
	}
	m.rebuildNodesByStage()

	count := m.stageCountExcludingSurge(types.StageComplete)
	if count != 2 {
		t.Errorf("stageCountExcludingSurge(COMPLETE) = %d, want 2", count)
	}
}

func TestGetSortedNodeList_SurgeNodesLast(t *testing.T) {
	m := newTestModel()

	m.nodes = map[string]types.NodeState{
		"node-a":  {Name: "node-a", Stage: types.StageReady, Version: "v1.32.9"},
		"node-b":  {Name: "node-b", Stage: types.StageComplete, Version: "v1.33.2"},
		"surge-1": {Name: "surge-1", Stage: types.StageComplete, Version: "v1.33.2", SurgeNode: true},
	}
	m.rebuildNodesByStage()

	sorted := m.getSortedNodeList()
	if len(sorted) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(sorted))
	}
	// Surge node should be last
	if sorted[len(sorted)-1] != "surge-1" {
		t.Errorf("expected surge node last, got %q", sorted[len(sorted)-1])
	}
}

func TestProgressPercent_NoSurge_UnchangedBehavior(t *testing.T) {
	m := newTestModel()

	// No surge nodes — original behavior
	m.nodes = map[string]types.NodeState{
		"node-1": {Name: "node-1", Stage: types.StageComplete, Version: "v1.33.2"},
		"node-2": {Name: "node-2", Stage: types.StageComplete, Version: "v1.33.2"},
		"node-3": {Name: "node-3", Stage: types.StageReady, Version: "v1.32.9"},
	}
	m.rebuildNodesByStage()

	percent := m.progressPercent()
	if percent != 66 {
		t.Errorf("progressPercent() = %d, want 66", percent)
	}
}

func TestProgressPercent_EmptyCluster(t *testing.T) {
	m := newTestModel()
	m.nodes = map[string]types.NodeState{}
	m.rebuildNodesByStage()

	if m.progressPercent() != 0 {
		t.Errorf("progressPercent() should be 0 for empty cluster")
	}
}

func TestHandleNodeUpdate_SurgeNode(t *testing.T) {
	m := newTestModel()

	// Simulate receiving a surge node update
	m.handleNodeUpdate(types.NodeState{
		Name:      "surge-1",
		Stage:     types.StageComplete,
		Version:   "v1.33.2",
		SurgeNode: true,
	})

	if node, ok := m.nodes["surge-1"]; !ok {
		t.Error("surge node should be stored")
	} else if !node.SurgeNode {
		t.Error("SurgeNode flag should be preserved")
	}

	// Progress should not count it
	if m.totalNodes() != 0 {
		t.Errorf("totalNodes() = %d, want 0 (only surge nodes)", m.totalNodes())
	}
}

func TestHandleNodeUpdate_DisplaysWatcherStageAsIs(t *testing.T) {
	m := newTestModel()

	// TUI displays whatever stage the watcher sends — no local stage logic.
	// Regression guard lives in the watcher (NodeWatcher.latchCompleteStage).
	m.handleNodeUpdate(types.NodeState{
		Name:    "node-1",
		Stage:   types.StageComplete,
		Version: "v1.33.2",
		Ready:   true,
	})
	if m.nodes["node-1"].Stage != types.StageComplete {
		t.Fatalf("expected COMPLETE, got %s", m.nodes["node-1"].Stage)
	}

	// If watcher sends a different stage, TUI displays it (watcher owns the latch)
	m.handleNodeUpdate(types.NodeState{
		Name:    "node-1",
		Stage:   types.StageReady,
		Version: "v1.33.2",
		Ready:   true,
	})
	if m.nodes["node-1"].Stage != types.StageReady {
		t.Errorf("TUI should display watcher stage, got %s", m.nodes["node-1"].Stage)
	}
}

func TestHandleNodeUpdate_CompleteFieldsUpdated(t *testing.T) {
	m := newTestModel()

	// COMPLETE node with initial pod count
	m.handleNodeUpdate(types.NodeState{
		Name:     "node-1",
		Stage:    types.StageComplete,
		Version:  "v1.33.2",
		PodCount: 10,
	})
	if m.nodes["node-1"].PodCount != 10 {
		t.Fatalf("expected PodCount 10, got %d", m.nodes["node-1"].PodCount)
	}

	// Subsequent COMPLETE update with new pod count — fields should update
	m.handleNodeUpdate(types.NodeState{
		Name:     "node-1",
		Stage:    types.StageComplete,
		Version:  "v1.33.2",
		PodCount: 15,
	})
	if m.nodes["node-1"].PodCount != 15 {
		t.Errorf("expected PodCount 15 after update, got %d", m.nodes["node-1"].PodCount)
	}
}

func TestHandleNodeUpdate_SurgeDeleteDoesNotReduceCompleteCount(t *testing.T) {
	m := newTestModel()

	// 3 real nodes COMPLETE + 1 surge node COMPLETE
	m.handleNodeUpdate(types.NodeState{Name: "node-1", Stage: types.StageComplete, Version: "v1.33.2"})
	m.handleNodeUpdate(types.NodeState{Name: "node-2", Stage: types.StageComplete, Version: "v1.33.2"})
	m.handleNodeUpdate(types.NodeState{Name: "node-3", Stage: types.StageComplete, Version: "v1.33.2"})
	m.handleNodeUpdate(types.NodeState{Name: "surge-1", Stage: types.StageComplete, Version: "v1.33.2", SurgeNode: true})

	if m.stageCountExcludingSurge(types.StageComplete) != 3 {
		t.Fatalf("expected 3 COMPLETE (non-surge), got %d", m.stageCountExcludingSurge(types.StageComplete))
	}

	// Surge node deleted
	m.handleNodeUpdate(types.NodeState{Name: "surge-1", Deleted: true})

	// Non-surge COMPLETE count should remain 3
	if m.stageCountExcludingSurge(types.StageComplete) != 3 {
		t.Errorf("COMPLETE count changed after surge deletion: got %d, want 3", m.stageCountExcludingSurge(types.StageComplete))
	}
}

func TestHandleNodeUpdate_ReimagingGhostNode(t *testing.T) {
	m := newTestModel()

	// Simulate receiving a REIMAGING ghost node
	m.handleNodeUpdate(types.NodeState{
		Name:    "node-a",
		Stage:   types.StageReimaging,
		Version: "v1.32.9",
	})

	if _, ok := m.nodes["node-a"]; !ok {
		t.Error("reimaging ghost node should be stored")
	}

	// Should appear in nodesByStage
	if len(m.nodesByStage[types.StageReimaging]) != 1 {
		t.Errorf("expected 1 node in REIMAGING stage, got %d", len(m.nodesByStage[types.StageReimaging]))
	}
}
