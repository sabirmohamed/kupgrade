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
		informer:              &fakeInformer{objects: storeObjects},
		emitter:               emitter,
		stages:                stages,
		podCounter:            func(string) int { return 0 },
		evictablePodCounter:   func(string) int { return 0 },
		drainStartTimes:       make(map[string]time.Time),
		initialEvictableCount: make(map[string]int),
		reimagingNodes:        make(map[string]types.NodeState),
		surgeNodes:            make(map[string]bool),
	}
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

	// Delete a node during active upgrade
	deletedNode := newTestK8sNode(testNodeName, testCurrentVersion, true, false)
	w.onDelete(deletedNode)

	// Verify ghost node retained
	if _, ok := w.reimagingNodes[testNodeName]; !ok {
		t.Fatal("deleted node should be retained in reimagingNodes during upgrade")
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
	if _, ok := w.reimagingNodes[testNodeName]; ok {
		t.Error("node should not be retained when no upgrade is active")
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

	// Simulate ghost node
	w.reimagingNodes[testNodeName] = types.NodeState{
		Name:    testNodeName,
		Stage:   types.StageReimaging,
		Version: testCurrentVersion,
	}

	// Node returns at target version
	returnedNode := newTestK8sNode(testNodeName, testTargetVersion, true, true)
	w.onAdd(returnedNode)

	// Verify ghost node removed
	if _, ok := w.reimagingNodes[testNodeName]; ok {
		t.Error("ghost node should be removed after re-registration")
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
}

func TestOnAdd_ReturningFromReimage_AtOldVersion(t *testing.T) {
	emitter := &mockEmitter{}
	stages := &mockStageComputer{
		targetVersion: testTargetVersion,
		lowestVersion: testCurrentVersion,
	}

	w := newNodeWatcher(emitter, stages, nil)

	// Simulate ghost node
	w.reimagingNodes[testNodeName] = types.NodeState{
		Name:    testNodeName,
		Stage:   types.StageReimaging,
		Version: testCurrentVersion,
	}

	// Node returns at OLD version (didn't upgrade)
	returnedNode := newTestK8sNode(testNodeName, testCurrentVersion, true, true)
	w.onAdd(returnedNode)

	// Verify ghost node removed
	if _, ok := w.reimagingNodes[testNodeName]; ok {
		t.Error("ghost node should be removed after re-registration")
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

	w.surgeNodes[testSurgeNodeName] = true

	// Delete the surge node
	deletedNode := newTestK8sNode(testSurgeNodeName, testTargetVersion, true, true)
	w.onDelete(deletedNode)

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
	w.surgeNodes[testSurgeNodeName] = true

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
	w.surgeNodes[testSurgeNodeName] = true

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

	// Add a ghost node
	w.reimagingNodes[testNodeName] = types.NodeState{
		Name:    testNodeName,
		Stage:   types.StageReimaging,
		Version: testCurrentVersion,
	}

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
	w.surgeNodes[testSurgeNodeName] = true

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
	w.surgeNodes[testSurgeNodeName] = true

	// Delete the surge node during active upgrade
	deletedNode := newTestK8sNode(testSurgeNodeName, testTargetVersion, true, true)
	w.onDelete(deletedNode)

	// Verify NOT retained as reimaging (surge nodes don't reimage)
	if _, ok := w.reimagingNodes[testSurgeNodeName]; ok {
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

// Reuse fakeInformer/fakeStore from pdbs_test.go — same package.
// No redefinition needed since they're in the same test package.

// Verify that HasSynced is available on fakeInformer (compile check)
var _ cache.SharedIndexInformer = (*fakeInformer)(nil)
