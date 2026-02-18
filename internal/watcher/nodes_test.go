package watcher

import (
	"testing"
	"time"

	"github.com/sabirmohamed/kupgrade/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	testCurrentVersion = "v1.32.9"
	testTargetVersion  = "v1.33.2"
	testNodeName       = "aks-agentpool-32099259-vmss000003"
	testSurgeNodeName  = "aks-agentpool-32099259-vmss000005"
)

// mockEmitter captures emitted events and node states
type mockEmitter struct {
	events     []types.Event
	nodeStates []types.NodeState
	blockers   []types.Blocker
	podStates  []types.PodState
}

func (e *mockEmitter) Emit(event types.Event) {
	e.events = append(e.events, event)
}
func (e *mockEmitter) EmitNodeState(state types.NodeState) {
	e.nodeStates = append(e.nodeStates, state)
}
func (e *mockEmitter) EmitPodState(state types.PodState) {
	e.podStates = append(e.podStates, state)
}
func (e *mockEmitter) EmitBlocker(blocker types.Blocker) {
	e.blockers = append(e.blockers, blocker)
}
func (e *mockEmitter) RefreshNodeState(nodeName string) {}

// mockStageComputer implements StageComputer for tests
type mockStageComputer struct {
	targetVersion string
	lowestVersion string
}

func (s *mockStageComputer) ComputeStage(node *corev1.Node) types.NodeStage {
	version := node.Status.NodeInfo.KubeletVersion
	schedulable := !node.Spec.Unschedulable
	ready := false
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
			ready = true
		}
	}

	upgradeActive := s.lowestVersion != "" && s.targetVersion != "" && s.lowestVersion != s.targetVersion
	switch {
	case upgradeActive && version == s.targetVersion && ready && schedulable:
		return types.StageComplete
	case !ready:
		return types.StageReimaging
	case !schedulable:
		return types.StageCordoned
	default:
		return types.StageReady
	}
}

func (s *mockStageComputer) UpdatePodCount(nodeName string, delta int) {}
func (s *mockStageComputer) SetTargetVersion(version string) {
	if s.targetVersion == "" || version > s.targetVersion {
		s.targetVersion = version
	}
	if s.lowestVersion == "" || version < s.lowestVersion {
		s.lowestVersion = version
	}
}
func (s *mockStageComputer) TargetVersion() string        { return s.targetVersion }
func (s *mockStageComputer) LowestVersion() string        { return s.lowestVersion }
func (s *mockStageComputer) UpgradeCompleted() bool       { return false }
func (s *mockStageComputer) PodCount(nodeName string) int { return 0 }
func (s *mockStageComputer) RecomputeVersions(versions []string) {
	s.targetVersion = ""
	s.lowestVersion = ""
	for _, v := range versions {
		if v == "" {
			continue
		}
		if s.targetVersion == "" || v > s.targetVersion {
			s.targetVersion = v
		}
		if s.lowestVersion == "" || v < s.lowestVersion {
			s.lowestVersion = v
		}
	}
}

func newTestK8sNode(name, version string, ready, schedulable bool) *corev1.Node {
	return newTestK8sNodeWithProvider(name, version, ready, schedulable, "")
}

func newTestK8sNodeWithProvider(name, version string, ready, schedulable bool, providerID string) *corev1.Node {
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: metav1.Time{Time: time.Now().Add(-24 * time.Hour)},
		},
		Spec: corev1.NodeSpec{
			Unschedulable: !schedulable,
			ProviderID:    providerID,
		},
		Status: corev1.NodeStatus{
			NodeInfo: corev1.NodeSystemInfo{
				KubeletVersion: version,
			},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: status},
			},
		},
	}
}

func newNodeWatcher(emitter EventEmitter, stages StageComputer, storeObjects []interface{}) *NodeWatcher {
	return &NodeWatcher{
		informer:            &fakeInformer{objects: storeObjects},
		emitter:             emitter,
		stages:              stages,
		podCounter:          func(string) int { return 0 },
		evictablePodCounter: func(string) int { return 0 },
		lifecycles:          make(map[string]*NodeLifecycle),
	}
}

// seedGhostNode sets up a reimaging ghost in the lifecycle map
func seedGhostNode(w *NodeWatcher, name, version string) {
	ghostState := types.NodeState{
		Name:    name,
		Stage:   types.StageReimaging,
		Version: version,
	}
	w.lifecycles[name] = &NodeLifecycle{
		Reimaging:        true,
		ReimageStartTime: time.Now(),
		GhostState:       &ghostState,
		LastKnownVersion: version,
	}
}

// seedSurgeNode marks a node as surge in the lifecycle map
func seedSurgeNode(w *NodeWatcher, name string) {
	lc := w.lifecycles[name]
	if lc == nil {
		lc = &NodeLifecycle{}
		w.lifecycles[name] = lc
	}
	lc.IsSurge = true
}

// seedCompletedNode marks a node as completed in the lifecycle map
func seedCompletedNode(w *NodeWatcher, name string) {
	lc := w.lifecycles[name]
	if lc == nil {
		lc = &NodeLifecycle{}
		w.lifecycles[name] = lc
	}
	lc.Completed = true
	lc.CompletedAt = time.Now()
}

func TestOnDelete_UpgradeActive_RetainsAsReimaging(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}

	// Store has another node so versions stay mixed
	otherNode := newTestK8sNode("other-node", testCurrentVersion, true, true)
	w := newNodeWatcher(emitter, stages, []interface{}{otherNode})
	w.platform = types.PlatformAKS // AKS reimages in-place

	// Delete a node during active upgrade
	deletedNode := newTestK8sNode(testNodeName, testCurrentVersion, true, false)
	w.onDelete(deletedNode)

	// Verify ghost node retained in lifecycle
	lc := w.lifecycles[testNodeName]
	if lc == nil || !lc.Reimaging {
		t.Fatal("deleted node should be retained as Reimaging in lifecycle during upgrade")
	}
	if lc.GhostState == nil {
		t.Fatal("ghost state should be set")
	}

	// Verify emitted state is REIMAGING, not Deleted
	found := false
	for _, state := range emitter.nodeStates {
		if state.Name == testNodeName {
			if state.Deleted {
				t.Error("ghost node should not have Deleted=true")
			}
			if state.Stage != types.StageReimaging {
				t.Errorf("ghost node stage = %v, want %v", state.Stage, types.StageReimaging)
			}
			found = true
		}
	}
	if !found {
		t.Error("expected emitted node state for ghost node")
	}
}

func TestOnDelete_NoUpgrade_DeletesNormally(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testTargetVersion, // Same = no upgrade active
	}

	w := newNodeWatcher(emitter, stages, nil)
	deletedNode := newTestK8sNode(testNodeName, testTargetVersion, true, true)
	w.onDelete(deletedNode)

	// Verify NOT retained as reimaging
	if lc := w.lifecycles[testNodeName]; lc != nil && lc.Reimaging {
		t.Error("node should not be retained as reimaging when no upgrade is active")
	}

	// Verify emitted as Deleted
	found := false
	for _, state := range emitter.nodeStates {
		if state.Name == testNodeName && state.Deleted {
			found = true
		}
	}
	if !found {
		t.Error("expected Deleted=true node state when no upgrade active")
	}
}

func TestOnAdd_ReturningFromReimage_AtTargetVersion(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}

	w := newNodeWatcher(emitter, stages, nil)

	// Simulate ghost node via lifecycle
	seedGhostNode(w, testNodeName, testCurrentVersion)

	// Node returns at target version
	returnedNode := newTestK8sNode(testNodeName, testTargetVersion, true, true)
	w.onAdd(returnedNode)

	// Verify ghost cleared
	lc := w.lifecycles[testNodeName]
	if lc != nil && lc.Reimaging {
		t.Error("ghost node should be cleared after re-registration")
	}

	// Verify emitted as COMPLETE
	found := false
	for _, state := range emitter.nodeStates {
		if state.Name == testNodeName && state.Stage == types.StageComplete {
			found = true
		}
	}
	if !found {
		t.Error("expected COMPLETE stage for node returning at target version")
	}

	// Verify completed latch set
	if lc == nil || !lc.Completed {
		t.Error("expected Completed latch to be set")
	}
}

func TestOnAdd_ReturningFromReimage_AtOldVersion(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}

	w := newNodeWatcher(emitter, stages, nil)

	// Simulate ghost node via lifecycle
	seedGhostNode(w, testNodeName, testCurrentVersion)

	// Node returns at OLD version (didn't upgrade)
	returnedNode := newTestK8sNode(testNodeName, testCurrentVersion, true, true)
	w.onAdd(returnedNode)

	// Verify ghost cleared
	lc := w.lifecycles[testNodeName]
	if lc != nil && lc.Reimaging {
		t.Error("ghost node should be cleared after re-registration")
	}

	// Verify emitted with recomputed stage (not stuck as REIMAGING)
	found := false
	for _, state := range emitter.nodeStates {
		if state.Name == testNodeName {
			if state.Stage == types.StageReimaging {
				t.Error("node at old version should not be REIMAGING after re-registration")
			}
			found = true
		}
	}
	if !found {
		t.Error("expected emitted node state for re-registered node")
	}
}

func TestSurgeNode_MarkAndUnmark(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}
	w := newNodeWatcher(emitter, stages, nil)

	// Mark as surge
	w.MarkSurgeNode(testSurgeNodeName, "agentpool")
	if !w.IsSurgeNode(testSurgeNodeName) {
		t.Error("expected node to be marked as surge")
	}

	// Verify event confirmation flag
	lc := w.lifecycles[testSurgeNodeName]
	if lc == nil || !lc.SurgeConfirmedByEvent {
		t.Error("expected SurgeConfirmedByEvent to be set")
	}

	// Unmark
	w.UnmarkSurgeNode(testSurgeNodeName)
	if w.IsSurgeNode(testSurgeNodeName) {
		t.Error("expected node to be unmarked as surge")
	}
}

func TestSurgeNode_MarkedInEmittedState(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}

	surgeNode := newTestK8sNode(testSurgeNodeName, testTargetVersion, true, true)
	w := newNodeWatcher(emitter, stages, []interface{}{surgeNode})

	// Mark as surge (retroactively — node already exists)
	w.MarkSurgeNode(testSurgeNodeName, "agentpool")

	// Verify emitted state has SurgeNode=true
	found := false
	for _, state := range emitter.nodeStates {
		if state.Name == testSurgeNodeName && state.SurgeNode {
			found = true
		}
	}
	if !found {
		t.Error("expected SurgeNode=true in emitted state for retroactively marked surge node")
	}
}

func TestSurgeNode_CleanedUpOnDelete(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testTargetVersion, // No upgrade active
	}
	w := newNodeWatcher(emitter, stages, nil)

	seedSurgeNode(w, testSurgeNodeName)

	// Delete the surge node
	deletedNode := newTestK8sNode(testSurgeNodeName, testTargetVersion, true, true)
	w.onDelete(deletedNode)

	// Lifecycle cleaned up on normal delete (no upgrade active)
	if w.IsSurgeNode(testSurgeNodeName) {
		t.Error("surge node should be cleaned up after deletion")
	}
}

func TestOnAdd_SurgeNodeFlagged(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}
	w := newNodeWatcher(emitter, stages, nil)

	// Pre-mark as surge
	seedSurgeNode(w, testSurgeNodeName)

	// Add the node
	surgeNode := newTestK8sNode(testSurgeNodeName, testTargetVersion, true, true)
	w.onAdd(surgeNode)

	// Verify SurgeNode flag in emitted state
	found := false
	for _, state := range emitter.nodeStates {
		if state.Name == testSurgeNodeName && state.SurgeNode {
			found = true
		}
	}
	if !found {
		t.Error("expected SurgeNode=true when node is in surgeNodes set")
	}
}

func TestOnUpdate_SurgeNodeFlagged(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}
	w := newNodeWatcher(emitter, stages, nil)

	// Pre-mark as surge
	seedSurgeNode(w, testSurgeNodeName)

	// Update the node
	oldNode := newTestK8sNode(testSurgeNodeName, testTargetVersion, true, true)
	newNode := newTestK8sNode(testSurgeNodeName, testTargetVersion, true, true)
	w.onUpdate(oldNode, newNode)

	// Verify SurgeNode flag in emitted state
	found := false
	for _, state := range emitter.nodeStates {
		if state.Name == testSurgeNodeName && state.SurgeNode {
			found = true
		}
	}
	if !found {
		t.Error("expected SurgeNode=true on update when node is in surgeNodes set")
	}
}

func TestBuildNodeStates_IncludesReimaging(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}

	realNode := newTestK8sNode("real-node", testCurrentVersion, true, true)
	w := newNodeWatcher(emitter, stages, []interface{}{realNode})

	// Add a ghost node via lifecycle
	seedGhostNode(w, testNodeName, testCurrentVersion)

	states := w.buildNodeStates()
	foundReal := false
	foundGhost := false
	for _, s := range states {
		if s.Name == "real-node" {
			foundReal = true
		}
		if s.Name == testNodeName && s.Stage == types.StageReimaging {
			foundGhost = true
		}
	}
	if !foundReal {
		t.Error("expected real node in buildNodeStates")
	}
	if !foundGhost {
		t.Error("expected reimaging ghost node in buildNodeStates")
	}
}

func TestBuildNodeStates_SurgeNodesFlagged(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}

	surgeNode := newTestK8sNode(testSurgeNodeName, testTargetVersion, true, true)
	w := newNodeWatcher(emitter, stages, []interface{}{surgeNode})
	seedSurgeNode(w, testSurgeNodeName)

	states := w.buildNodeStates()
	for _, s := range states {
		if s.Name == testSurgeNodeName {
			if !s.SurgeNode {
				t.Error("expected SurgeNode=true in buildNodeStates for surge node")
			}
			return
		}
	}
	t.Error("surge node not found in buildNodeStates")
}

func TestSurgeNode_DeletedDuringUpgrade_NotRetainedAsReimaging(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion, // Upgrade active
	}

	otherNode := newTestK8sNode("other-node", testCurrentVersion, true, true)
	w := newNodeWatcher(emitter, stages, []interface{}{otherNode})

	// Mark as surge before deletion
	seedSurgeNode(w, testSurgeNodeName)

	// Delete the surge node during active upgrade
	deletedNode := newTestK8sNode(testSurgeNodeName, testTargetVersion, true, true)
	w.onDelete(deletedNode)

	// Verify NOT retained as reimaging (surge nodes don't reimage)
	lc := w.lifecycles[testSurgeNodeName]
	if lc != nil && lc.Reimaging {
		t.Error("surge node should NOT be retained as reimaging during upgrade")
	}

	// Verify emitted as Deleted
	found := false
	for _, state := range emitter.nodeStates {
		if state.Name == testSurgeNodeName && state.Deleted {
			found = true
		}
	}
	if !found {
		t.Error("expected Deleted=true for surge node even during active upgrade")
	}

	// Verify event is info severity (not warning)
	for _, event := range emitter.events {
		if event.NodeName == testSurgeNodeName {
			if event.Severity != types.SeverityInfo {
				t.Errorf("surge node deletion event severity = %v, want %v", event.Severity, types.SeverityInfo)
			}
		}
	}
}

func TestLatchCompleteStage_PreventsRegression(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}
	w := newNodeWatcher(emitter, stages, nil)

	// Simulate a node returning from reimage (not a fresh surge node).
	// Seed a reimaging ghost so onAdd takes the reimage-return path.
	seedGhostNode(w, testNodeName, testCurrentVersion)

	// Node re-registers at target version → COMPLETE via reimage path
	completeNode := newTestK8sNode(testNodeName, testTargetVersion, true, true)
	w.onAdd(completeNode)

	// Verify it was emitted as COMPLETE and tracked
	lastState := emitter.nodeStates[len(emitter.nodeStates)-1]
	if lastState.Stage != types.StageComplete {
		t.Fatalf("expected COMPLETE, got %s", lastState.Stage)
	}
	lc := w.lifecycles[testNodeName]
	if lc == nil || !lc.Completed {
		t.Fatal("expected node to be tracked as Completed in lifecycle")
	}

	// Stale update: ComputeStage returns READY (e.g., lowestVersion changes)
	stages.lowestVersion = testTargetVersion // makes upgrade "inactive" → ComputeStage returns READY
	staleNode := newTestK8sNode(testNodeName, testTargetVersion, true, true)
	w.onUpdate(completeNode, staleNode)

	// Verify latch held — still COMPLETE
	lastState = emitter.nodeStates[len(emitter.nodeStates)-1]
	if lastState.Stage != types.StageComplete {
		t.Errorf("expected latched COMPLETE, got %s", lastState.Stage)
	}
}

func TestLatchCompleteStage_ClearedOnDelete(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testTargetVersion, // No upgrade — so delete is normal
	}
	w := newNodeWatcher(emitter, stages, nil)

	// Manually seed completed
	seedCompletedNode(w, testNodeName)

	// Delete the node
	deletedNode := newTestK8sNode(testNodeName, testTargetVersion, true, true)
	w.onDelete(deletedNode)

	// Verify lifecycle cleaned up (normal delete removes entry)
	if lc := w.lifecycles[testNodeName]; lc != nil {
		t.Error("lifecycle should be deleted on normal node deletion")
	}
}

func TestLatchCompleteStage_AllowsFieldUpdates(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}

	podCount := 10
	w := newNodeWatcher(emitter, stages, nil)
	w.podCounter = func(string) int { return podCount }

	// Simulate a node returning from reimage (not a fresh surge node)
	seedGhostNode(w, testNodeName, testCurrentVersion)

	// Node re-registers at target version → COMPLETE via reimage path
	node := newTestK8sNode(testNodeName, testTargetVersion, true, true)
	w.onAdd(node)

	firstState := emitter.nodeStates[len(emitter.nodeStates)-1]
	if firstState.PodCount != 10 {
		t.Fatalf("expected PodCount 10, got %d", firstState.PodCount)
	}

	// Pod count changes, node updates
	podCount = 15
	w.onUpdate(node, node)

	lastState := emitter.nodeStates[len(emitter.nodeStates)-1]
	if lastState.Stage != types.StageComplete {
		t.Errorf("expected COMPLETE, got %s", lastState.Stage)
	}
	if lastState.PodCount != 15 {
		t.Errorf("expected PodCount 15, got %d", lastState.PodCount)
	}
}

func TestBuildNodeStates_RespectsCompleteLatch(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testTargetVersion, // No upgrade — ComputeStage returns READY
	}

	node := newTestK8sNode(testNodeName, testTargetVersion, true, true)
	w := newNodeWatcher(emitter, stages, []interface{}{node})

	// Seed completed latch
	seedCompletedNode(w, testNodeName)

	states := w.buildNodeStates()
	for _, s := range states {
		if s.Name == testNodeName {
			if s.Stage != types.StageComplete {
				t.Errorf("buildNodeStates should respect COMPLETE latch, got %s", s.Stage)
			}
			return
		}
	}
	t.Error("node not found in buildNodeStates")
}

// --- Phase 3 bug fix regression tests ---

func TestVersionBasedSurgeDetection_NewNodeAtTargetDuringUpgrade(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion, // Upgrade active
	}

	// Existing node at old version keeps upgrade active
	oldNode := newTestK8sNode("old-node", testCurrentVersion, true, true)
	w := newNodeWatcher(emitter, stages, []interface{}{oldNode})

	// New node appears at target version — should be detected as surge
	surgeNode := newTestK8sNode(testSurgeNodeName, testTargetVersion, true, true)
	w.onAdd(surgeNode)

	// Verify version-based surge detection
	lc := w.lifecycles[testSurgeNodeName]
	if lc == nil || !lc.IsSurge {
		t.Fatal("expected version-based surge detection for new node at target version during upgrade")
	}

	// Verify emitted with SurgeNode=true
	found := false
	for _, state := range emitter.nodeStates {
		if state.Name == testSurgeNodeName && state.SurgeNode {
			found = true
		}
	}
	if !found {
		t.Error("expected SurgeNode=true in emitted state")
	}
}

func TestVersionBasedSurgeDetection_ScaleUpNotMarkedSurge(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testCurrentVersion,
		lowestVersion: testCurrentVersion, // Same version = no upgrade
	}

	// Existing node at current version
	existingNode := newTestK8sNode("existing-node", testCurrentVersion, true, true)
	w := newNodeWatcher(emitter, stages, []interface{}{existingNode})

	// New node added (scale-up) at same version — should NOT be surge
	newNode := newTestK8sNode("new-node", testCurrentVersion, true, true)
	w.onAdd(newNode)

	lc := w.lifecycles["new-node"]
	if lc != nil && lc.IsSurge {
		t.Error("new node at same version as cluster (scale-up) should NOT be detected as surge")
	}
}

func TestVersionBasedSurgeDetection_ReimageReturnNotMarkedSurge(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}
	w := newNodeWatcher(emitter, stages, nil)

	// Node was reimaging (has drain history) — returning at target version
	w.lifecycles[testNodeName] = &NodeLifecycle{
		Reimaging:         true,
		ReimageStartTime:  time.Now(),
		DrainStartTime:    time.Now().Add(-1 * time.Minute), // Has drain history
		PreUpgradeVersion: testCurrentVersion,
		GhostState: &types.NodeState{
			Name:    testNodeName,
			Stage:   types.StageReimaging,
			Version: testCurrentVersion,
		},
	}

	returnedNode := newTestK8sNode(testNodeName, testTargetVersion, true, true)
	w.onAdd(returnedNode)

	// Should NOT be marked as surge — it's a real node returning from reimage
	lc := w.lifecycles[testNodeName]
	if lc != nil && lc.IsSurge {
		t.Error("reimaged node returning at target version should NOT be detected as surge")
	}
}

func TestVersionBasedSurgeDetection_EventConfirmsSurge(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}

	oldNode := newTestK8sNode("old-node", testCurrentVersion, true, true)
	w := newNodeWatcher(emitter, stages, []interface{}{oldNode})

	// Node detected as surge by version heuristic
	surgeNode := newTestK8sNode(testSurgeNodeName, testTargetVersion, true, true)
	w.onAdd(surgeNode)

	if !w.IsSurgeNode(testSurgeNodeName) {
		t.Fatal("expected version-based surge detection")
	}

	// AKS event arrives ~49s later — confirms surge
	w.MarkSurgeNode(testSurgeNodeName, "agentpool")

	lc := w.lifecycles[testSurgeNodeName]
	if lc == nil || !lc.SurgeConfirmedByEvent {
		t.Error("expected SurgeConfirmedByEvent after MarkSurgeNode")
	}
}

func TestPreUpgradeVersion_CapturedOnFirstCordon(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}
	w := newNodeWatcher(emitter, stages, nil)

	// Node starts as schedulable
	node := newTestK8sNode(testNodeName, testCurrentVersion, true, true)
	w.onAdd(node)

	// Node gets cordoned
	cordonedNode := newTestK8sNode(testNodeName, testCurrentVersion, true, false)
	w.onUpdate(node, cordonedNode)

	// Verify PreUpgradeVersion captured
	lc := w.lifecycles[testNodeName]
	if lc == nil {
		t.Fatal("expected lifecycle to exist")
	}
	if lc.PreUpgradeVersion != testCurrentVersion {
		t.Errorf("PreUpgradeVersion = %q, want %q", lc.PreUpgradeVersion, testCurrentVersion)
	}
}

func TestGhostVersion_UsesPreUpgradeVersion(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}

	otherNode := newTestK8sNode("other-node", testCurrentVersion, true, true)
	w := newNodeWatcher(emitter, stages, []interface{}{otherNode})
	w.platform = types.PlatformAKS // AKS reimages in-place

	// Simulate Pattern A: node was cordoned at v1.32.9, then AKS flips version
	// to v1.33.2 before deletion
	w.lifecycles[testNodeName] = &NodeLifecycle{
		PreUpgradeVersion: testCurrentVersion,
		LastKnownVersion:  testTargetVersion, // AKS flipped it
		DrainStartTime:    time.Now().Add(-30 * time.Second),
	}

	// Delete the node (version already flipped to target by AKS)
	deletedNode := newTestK8sNode(testNodeName, testTargetVersion, true, false)
	w.onDelete(deletedNode)

	// Ghost should use PreUpgradeVersion, not the flipped version
	lc := w.lifecycles[testNodeName]
	if lc == nil || !lc.Reimaging || lc.GhostState == nil {
		t.Fatal("expected ghost node to be created")
	}
	if lc.GhostState.Version != testCurrentVersion {
		t.Errorf("ghost version = %q, want %q (PreUpgradeVersion)", lc.GhostState.Version, testCurrentVersion)
	}
}

func TestDoubleDelete_CompletedNodeNotGhosted(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion, // Upgrade active
	}

	otherNode := newTestK8sNode("other-node", testCurrentVersion, true, true)
	w := newNodeWatcher(emitter, stages, []interface{}{otherNode})

	// Node already completed its upgrade
	seedCompletedNode(w, testNodeName)

	// Node gets deleted (double-delete pattern)
	deletedNode := newTestK8sNode(testNodeName, testTargetVersion, true, true)
	w.onDelete(deletedNode)

	// Should NOT create ghost — node already completed
	lc := w.lifecycles[testNodeName]
	if lc != nil && lc.Reimaging {
		t.Error("completed node should NOT be ghosted on double-delete")
	}

	// Should emit as deleted
	found := false
	for _, state := range emitter.nodeStates {
		if state.Name == testNodeName && state.Deleted {
			found = true
		}
	}
	if !found {
		t.Error("expected Deleted=true for completed node on double-delete")
	}
}

func TestGhostTTL_ExpiredGhostsCleanedUp(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}
	w := newNodeWatcher(emitter, stages, nil)

	// Create a ghost that's already expired
	w.lifecycles[testNodeName] = &NodeLifecycle{
		Reimaging:        true,
		ReimageStartTime: time.Now().Add(-6 * time.Minute), // Older than GhostTTL
		GhostState: &types.NodeState{
			Name:    testNodeName,
			Stage:   types.StageReimaging,
			Version: testCurrentVersion,
		},
	}

	// Create a fresh ghost that should survive
	freshGhostName := "fresh-ghost"
	w.lifecycles[freshGhostName] = &NodeLifecycle{
		Reimaging:        true,
		ReimageStartTime: time.Now(),
		GhostState: &types.NodeState{
			Name:    freshGhostName,
			Stage:   types.StageReimaging,
			Version: testCurrentVersion,
		},
	}

	w.CleanupExpiredGhosts()

	// Expired ghost: Reimaging cleared, WasReimaged preserved to prevent false surge
	if lc := w.lifecycles[testNodeName]; lc == nil {
		t.Fatal("expected WasReimaged marker to survive TTL cleanup")
	} else {
		if lc.Reimaging {
			t.Error("expired ghost should not be Reimaging")
		}
		if !lc.WasReimaged {
			t.Error("expected WasReimaged=true after TTL cleanup")
		}
		if lc.GhostState != nil {
			t.Error("GhostState should be nil after TTL cleanup")
		}
	}

	// Fresh ghost survives
	if lc := w.lifecycles[freshGhostName]; lc == nil || !lc.Reimaging {
		t.Error("fresh ghost should survive cleanup")
	}

	// Deleted emission for expired ghost
	found := false
	for _, state := range emitter.nodeStates {
		if state.Name == testNodeName && state.Deleted {
			found = true
		}
	}
	if !found {
		t.Error("expected Deleted=true emission for expired ghost")
	}
}

// --- RefreshState regression tests (H1 fix) ---

func TestRefreshState_PreservesCompleteLatch(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testTargetVersion, // No upgrade — ComputeStage returns READY
	}

	node := newTestK8sNode(testNodeName, testTargetVersion, true, true)
	w := newNodeWatcher(emitter, stages, []interface{}{node})

	// Seed completed latch
	seedCompletedNode(w, testNodeName)

	// RefreshState should preserve COMPLETE despite ComputeStage returning READY
	w.RefreshState(node)

	lastState := emitter.nodeStates[len(emitter.nodeStates)-1]
	if lastState.Stage != types.StageComplete {
		t.Errorf("RefreshState should preserve COMPLETE latch, got %s", lastState.Stage)
	}
}

func TestRefreshState_PreservesSurgeFlag(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}

	surgeNode := newTestK8sNode(testSurgeNodeName, testTargetVersion, true, true)
	w := newNodeWatcher(emitter, stages, []interface{}{surgeNode})

	// Mark as surge
	seedSurgeNode(w, testSurgeNodeName)

	// RefreshState should set SurgeNode=true
	w.RefreshState(surgeNode)

	lastState := emitter.nodeStates[len(emitter.nodeStates)-1]
	if !lastState.SurgeNode {
		t.Error("RefreshState should preserve SurgeNode flag")
	}
}

// --- L1: RefreshState base case (no lifecycle entry) ---

func TestRefreshState_NoLifecycle(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}
	node := newTestK8sNode(testNodeName, testCurrentVersion, true, true)
	w := newNodeWatcher(emitter, stages, nil)

	// RefreshState with no lifecycle entry — should not panic, should emit correctly
	w.RefreshState(node)

	if len(emitter.nodeStates) != 1 {
		t.Fatalf("expected 1 emitted state, got %d", len(emitter.nodeStates))
	}
	state := emitter.nodeStates[0]
	if state.Name != testNodeName {
		t.Errorf("emitted state name = %q, want %q", state.Name, testNodeName)
	}
	if state.Stage != types.StageReady {
		t.Errorf("expected StageReady for node with no lifecycle, got %s", state.Stage)
	}
}

// --- M1: Ghost TTL expiry does not cause false surge ---

func TestGhostTTLExpiry_NoFalseSurgeOnReRegister(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion, // Upgrade active
	}

	oldNode := newTestK8sNode("old-node", testCurrentVersion, true, true)
	w := newNodeWatcher(emitter, stages, []interface{}{oldNode})

	// Create an expired ghost
	w.lifecycles[testNodeName] = &NodeLifecycle{
		Reimaging:        true,
		WasReimaged:      true,
		ReimageStartTime: time.Now().Add(-6 * time.Minute),
		GhostState: &types.NodeState{
			Name:    testNodeName,
			Stage:   types.StageReimaging,
			Version: testCurrentVersion,
		},
	}

	// TTL cleanup expires the ghost but preserves WasReimaged
	w.CleanupExpiredGhosts()

	lc := w.lifecycles[testNodeName]
	if lc == nil || !lc.WasReimaged {
		t.Fatal("expected WasReimaged marker to survive TTL cleanup")
	}
	if lc.Reimaging {
		t.Error("Reimaging should be cleared after TTL cleanup")
	}

	// Node re-registers at target version — should NOT be detected as surge
	returnedNode := newTestK8sNode(testNodeName, testTargetVersion, true, true)
	w.onAdd(returnedNode)

	lc = w.lifecycles[testNodeName]
	if lc != nil && lc.IsSurge {
		t.Error("node re-registering after ghost TTL expiry should NOT be detected as surge")
	}
}

// --- H3: onUpdate detects COMPLETE latch transition ---

func TestOnUpdate_DetectsCompleteLatchTransition(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}
	stageChangeCalled := false
	w := newNodeWatcher(emitter, stages, nil)
	w.onStageChangeFunc = func() { stageChangeCalled = true }

	// Simulate a node returning from reimage (not a fresh surge node).
	// Seed a reimaging ghost so onAdd takes the reimage-return path.
	seedGhostNode(w, testNodeName, testCurrentVersion)

	// Node re-registers at target version → COMPLETE via reimage path
	node := newTestK8sNode(testNodeName, testTargetVersion, true, true)
	w.onAdd(node)
	stageChangeCalled = false // Reset after onAdd

	// Simulate: upgrade becomes "inactive" (all nodes at target) but latch should hold.
	// ComputeStage returns READY (not COMPLETE) since lowest==target now,
	// but applyLifecycleFlags overrides to COMPLETE via latch.
	stages.lowestVersion = testTargetVersion
	updatedNode := newTestK8sNode(testNodeName, testTargetVersion, true, true)
	w.onUpdate(node, updatedNode)

	// The effective stage is COMPLETE (via latch), but ComputeStage returns READY.
	// onUpdate should detect this as a stage change (READY→COMPLETE) and call the callback.
	lastState := emitter.nodeStates[len(emitter.nodeStates)-1]
	if lastState.Stage != types.StageComplete {
		t.Errorf("expected COMPLETE (latched), got %s", lastState.Stage)
	}
	if !stageChangeCalled {
		t.Error("onStageChangeFunc should fire when latch causes effective stage to differ from raw ComputeStage")
	}
}

func TestSurgeNode_ShowsReadyNotComplete(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{targetVersion: "v1.32.9", lowestVersion: "v1.32.7"}
	w := newNodeWatcher(emitter, stages, nil)

	// Simulate surge node arriving at target version (ready + schedulable)
	surgeNode := newTestK8sNode("surge-node", "v1.32.9", true, true)
	w.mu.Lock()
	lc := w.getOrCreateLifecycle("surge-node")
	lc.IsSurge = true
	w.mu.Unlock()

	// ComputeStage returns COMPLETE for target-version + ready + schedulable,
	// but surge nodes should show READY (they never went through reimage)
	state := w.buildState(surgeNode)
	w.applyLifecycleFlags(&state)

	if !state.SurgeNode {
		t.Error("expected SurgeNode=true")
	}
	if state.Stage != types.StageReady {
		t.Errorf("surge node should show READY, got %s", state.Stage)
	}

	// Verify the COMPLETE latch did NOT fire
	w.mu.RLock()
	if w.lifecycles["surge-node"].Completed {
		t.Error("COMPLETE latch must not fire for surge nodes")
	}
	w.mu.RUnlock()

	// Even after multiple calls, surge stays READY and unlatched
	state2 := w.buildState(surgeNode)
	w.applyLifecycleFlags(&state2)
	if state2.Stage != types.StageReady {
		t.Errorf("surge node should still show READY, got %s", state2.Stage)
	}
	w.mu.RLock()
	if w.lifecycles["surge-node"].Completed {
		t.Error("COMPLETE latch must never fire for surge nodes, even after repeated calls")
	}
	w.mu.RUnlock()
}

// --- EKS/GKE multi-provider tests ---

func TestOnDelete_EKS_TerminalDelete_MarksComplete(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}

	otherNode := newTestK8sNode("other-node", testCurrentVersion, true, true)
	w := newNodeWatcher(emitter, stages, []interface{}{otherNode})
	w.platform = types.PlatformEKS

	// Seed lifecycle with pre-upgrade version (set during cordon)
	w.lifecycles[testNodeName] = &NodeLifecycle{
		PreUpgradeVersion: testCurrentVersion,
		DrainStartTime:    time.Now().Add(-30 * time.Second),
	}

	deletedNode := newTestK8sNode(testNodeName, testCurrentVersion, true, false)
	w.onDelete(deletedNode)

	// Should NOT create ghost (EKS deletions are terminal)
	lc := w.lifecycles[testNodeName]
	if lc != nil && lc.Reimaging {
		t.Error("EKS node should NOT be retained as reimaging")
	}

	// Should be marked COMPLETE in lifecycle
	if lc == nil || !lc.Completed {
		t.Fatal("EKS deleted node should be marked Completed in lifecycle")
	}

	// Emitted state: Stage=COMPLETE, Deleted=true
	found := false
	for _, state := range emitter.nodeStates {
		if state.Name == testNodeName {
			if !state.Deleted {
				t.Error("EKS terminal delete should emit Deleted=true")
			}
			if state.Stage != types.StageComplete {
				t.Errorf("EKS terminal delete stage = %v, want COMPLETE", state.Stage)
			}
			if state.Version != testCurrentVersion {
				t.Errorf("EKS terminal delete version = %q, want %q", state.Version, testCurrentVersion)
			}
			found = true
		}
	}
	if !found {
		t.Error("expected emitted node state for EKS terminal delete")
	}
}

func TestOnDelete_GKE_TerminalDelete_MarksComplete(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}

	otherNode := newTestK8sNode("other-node", testCurrentVersion, true, true)
	w := newNodeWatcher(emitter, stages, []interface{}{otherNode})
	w.platform = types.PlatformGKE

	deletedNode := newTestK8sNode("gke-node-abc123", testCurrentVersion, true, false)
	w.onDelete(deletedNode)

	// Verify COMPLETE + Deleted emission
	found := false
	for _, state := range emitter.nodeStates {
		if state.Name == "gke-node-abc123" && state.Deleted && state.Stage == types.StageComplete {
			found = true
		}
	}
	if !found {
		t.Error("expected COMPLETE+Deleted state for GKE terminal delete")
	}
}

func TestOnDelete_PlatformUnknown_FallsBackToTerminal(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}

	otherNode := newTestK8sNode("other-node", testCurrentVersion, true, true)
	w := newNodeWatcher(emitter, stages, []interface{}{otherNode})
	// platform defaults to PlatformUnknown — should treat as terminal (safe default)

	deletedNode := newTestK8sNode(testNodeName, testCurrentVersion, true, false)
	w.onDelete(deletedNode)

	// Unknown platform: should NOT create ghost (terminal is safer default)
	lc := w.lifecycles[testNodeName]
	if lc != nil && lc.Reimaging {
		t.Error("unknown platform should not create reimaging ghost")
	}

	// Should emit COMPLETE+Deleted
	found := false
	for _, state := range emitter.nodeStates {
		if state.Name == testNodeName && state.Deleted && state.Stage == types.StageComplete {
			found = true
		}
	}
	if !found {
		t.Error("expected COMPLETE+Deleted for unknown platform terminal delete")
	}
}

func TestGhostTTL_ShorterOnEKS(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}
	w := newNodeWatcher(emitter, stages, nil)
	w.platform = types.PlatformEKS

	// Create a ghost that's 1 minute old (< 5min AKS TTL, > 30s EKS TTL)
	w.lifecycles[testNodeName] = &NodeLifecycle{
		Reimaging:        true,
		ReimageStartTime: time.Now().Add(-1 * time.Minute),
		GhostState: &types.NodeState{
			Name:    testNodeName,
			Stage:   types.StageReimaging,
			Version: testCurrentVersion,
		},
	}

	w.CleanupExpiredGhosts()

	// On EKS, 1-minute ghost should be cleaned up (TTL=30s)
	lc := w.lifecycles[testNodeName]
	if lc != nil && lc.Reimaging {
		t.Error("EKS ghost at 1 minute should be cleaned up (TTL=30s)")
	}
}

func TestPlatformDetection_FromOnAdd(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testCurrentVersion,
		lowestVersion: testCurrentVersion,
	}
	w := newNodeWatcher(emitter, stages, nil)

	if w.platform != types.PlatformUnknown {
		t.Fatal("platform should start as unknown")
	}

	// Add a node with EKS providerID
	eksNode := newTestK8sNodeWithProvider("eks-node", testCurrentVersion, true, true, "aws:///eu-north-1a/i-0abc123")
	w.onAdd(eksNode)

	if w.platform != types.PlatformEKS {
		t.Errorf("platform = %q, want EKS after adding EKS node", w.platform)
	}
}

func TestSurgePromotion_IsUpgradeComplete(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testTargetVersion, // All at target
	}

	// All live nodes at target version and schedulable
	node1 := newTestK8sNode("node-1", testTargetVersion, true, true)
	node2 := newTestK8sNode("node-2", testTargetVersion, true, true)
	surgeNode := newTestK8sNode("surge-node", testTargetVersion, true, true)
	w := newNodeWatcher(emitter, stages, []interface{}{node1, node2, surgeNode})

	// At least one completed/reimaged node
	w.lifecycles["node-1"] = &NodeLifecycle{Completed: true, CompletedAt: time.Now()}
	w.lifecycles["surge-node"] = &NodeLifecycle{IsSurge: true}

	if !w.isUpgradeComplete() {
		t.Error("expected isUpgradeComplete=true: all nodes at target, one completed")
	}
}

func TestSurgePromotion_NotCompleteWhenMixedVersions(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion, // Mixed versions
	}

	node1 := newTestK8sNode("node-1", testCurrentVersion, true, true)
	node2 := newTestK8sNode("node-2", testTargetVersion, true, true)
	w := newNodeWatcher(emitter, stages, []interface{}{node1, node2})

	if w.isUpgradeComplete() {
		t.Error("should NOT be complete while versions are mixed")
	}
}

func TestSurgePromotion_PromoteSurgeNodes(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testTargetVersion,
	}

	node1 := newTestK8sNode("node-1", testTargetVersion, true, true)
	surgeNode1 := newTestK8sNode("surge-1", testTargetVersion, true, true)
	surgeNode2 := newTestK8sNode("surge-2", testTargetVersion, true, true)
	w := newNodeWatcher(emitter, stages, []interface{}{node1, surgeNode1, surgeNode2})

	w.lifecycles["node-1"] = &NodeLifecycle{Completed: true, CompletedAt: time.Now()}
	w.lifecycles["surge-1"] = &NodeLifecycle{IsSurge: true}
	w.lifecycles["surge-2"] = &NodeLifecycle{IsSurge: true}

	w.promoteSurgeNodes()

	// Verify surge nodes promoted
	for _, name := range []string{"surge-1", "surge-2"} {
		lc := w.lifecycles[name]
		if lc == nil {
			t.Fatalf("lifecycle for %s should exist", name)
		}
		if lc.IsSurge {
			t.Errorf("%s should no longer be marked as surge", name)
		}
		if !lc.Completed {
			t.Errorf("%s should be marked as Completed", name)
		}
	}

	if !w.surgePromoted {
		t.Error("surgePromoted flag should be set")
	}

	// Verify promotion event emitted
	found := false
	for _, event := range emitter.events {
		if event.Type == types.EventNodeReady && event.Severity == types.SeverityInfo {
			found = true
		}
	}
	if !found {
		t.Error("expected promotion info event")
	}
}

func TestCheckUpgradeCompletion_StabilityTimer(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testTargetVersion,
	}

	node1 := newTestK8sNode("node-1", testTargetVersion, true, true)
	surgeNode := newTestK8sNode("surge-1", testTargetVersion, true, true)
	w := newNodeWatcher(emitter, stages, []interface{}{node1, surgeNode})

	w.lifecycles["node-1"] = &NodeLifecycle{Completed: true, CompletedAt: time.Now()}
	w.lifecycles["surge-1"] = &NodeLifecycle{IsSurge: true}

	// First call: records completionStableAt
	w.CheckUpgradeCompletion()
	if w.completionStableAt.IsZero() {
		t.Fatal("completionStableAt should be set on first complete check")
	}
	if w.surgePromoted {
		t.Error("should not promote immediately — needs stability period")
	}

	// Simulate 60s elapsed
	w.completionStableAt = time.Now().Add(-61 * time.Second)
	w.CheckUpgradeCompletion()

	if !w.surgePromoted {
		t.Error("should promote after 60s stability")
	}
}

// Reuse fakeInformer/fakeStore from pdbs_test.go — same package.
// No redefinition needed since they're in the same test package.

// Verify that HasSynced is available on fakeInformer (compile check)
var _ cache.SharedIndexInformer = (*fakeInformer)(nil)
