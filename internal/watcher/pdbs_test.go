package watcher

import (
	"testing"
	"time"

	"github.com/sabirmohamed/kupgrade/pkg/types"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/cache"
)

const (
	testNamespace = "default"
	testNodeA     = "node-a"
	testNodeB     = "node-b"
)

func newTestPDB(name, namespace string, disruptionsAllowed, expectedPods int32, matchLabels map[string]string) *policyv1.PodDisruptionBudget {
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: matchLabels,
			},
			MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
		},
		Status: policyv1.PodDisruptionBudgetStatus{
			DisruptionsAllowed: disruptionsAllowed,
			ExpectedPods:       expectedPods,
			CurrentHealthy:     expectedPods,
		},
	}
}

func newTestPod(name, namespace, nodeName string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
		},
	}
}

func TestIsAtRisk(t *testing.T) {
	tests := []struct {
		name               string
		disruptionsAllowed int32
		expectedPods       int32
		want               bool
	}{
		{
			name:               "zero disruptions with expected pods is at risk",
			disruptionsAllowed: 0,
			expectedPods:       2,
			want:               true,
		},
		{
			name:               "has budget is not at risk",
			disruptionsAllowed: 1,
			expectedPods:       3,
			want:               false,
		},
		{
			name:               "zero expected pods is not at risk",
			disruptionsAllowed: 0,
			expectedPods:       0,
			want:               false,
		},
		{
			name:               "zero disruptions with one expected pod is at risk",
			disruptionsAllowed: 0,
			expectedPods:       1,
			want:               true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdb := newTestPDB("test-pdb", testNamespace, tt.disruptionsAllowed, tt.expectedPods, map[string]string{"app": "test"})
			got := isAtRisk(pdb)
			if got != tt.want {
				t.Errorf("isAtRisk() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsBlockingNode(t *testing.T) {
	appLabels := map[string]string{"app": "web"}
	otherLabels := map[string]string{"app": "other"}

	tests := []struct {
		name     string
		pdb      *policyv1.PodDisruptionBudget
		nodeName string
		pods     []*corev1.Pod
		want     bool
	}{
		{
			name:     "matching pod on draining node",
			pdb:      newTestPDB("web-pdb", testNamespace, 0, 2, appLabels),
			nodeName: testNodeA,
			pods: []*corev1.Pod{
				newTestPod("web-1", testNamespace, testNodeA, appLabels),
			},
			want: true,
		},
		{
			name:     "matching pod on different node",
			pdb:      newTestPDB("web-pdb", testNamespace, 0, 2, appLabels),
			nodeName: testNodeA,
			pods: []*corev1.Pod{
				newTestPod("web-1", testNamespace, testNodeB, appLabels),
			},
			want: false,
		},
		{
			name:     "non-matching pod on draining node",
			pdb:      newTestPDB("web-pdb", testNamespace, 0, 2, appLabels),
			nodeName: testNodeA,
			pods: []*corev1.Pod{
				newTestPod("other-1", testNamespace, testNodeA, otherLabels),
			},
			want: false,
		},
		{
			name:     "pdb has budget so not blocking",
			pdb:      newTestPDB("web-pdb", testNamespace, 1, 2, appLabels),
			nodeName: testNodeA,
			pods: []*corev1.Pod{
				newTestPod("web-1", testNamespace, testNodeA, appLabels),
			},
			want: false,
		},
		{
			name:     "pod in different namespace not matched",
			pdb:      newTestPDB("web-pdb", testNamespace, 0, 2, appLabels),
			nodeName: testNodeA,
			pods: []*corev1.Pod{
				newTestPod("web-1", "other-ns", testNodeA, appLabels),
			},
			want: false,
		},
		{
			name:     "no pods at all",
			pdb:      newTestPDB("web-pdb", testNamespace, 0, 2, appLabels),
			nodeName: testNodeA,
			pods:     nil,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBlockingNode(tt.pdb, tt.nodeName, tt.pods)
			if got != tt.want {
				t.Errorf("isBlockingNode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildBlockers(t *testing.T) {
	appLabels := map[string]string{"app": "web"}
	metricsLabels := map[string]string{"app": "metrics-server"}
	gatekeeperLabels := map[string]string{"app": "gatekeeper"}
	healthyLabels := map[string]string{"app": "healthy"}

	// Scenario: 1 active blocker (PDB with pod on draining node),
	// 2 risks (PDBs with pods elsewhere), 1 healthy PDB (has budget)
	pdbStore := []interface{}{
		// Active blocker: web PDB matches pod on draining node-a
		newTestPDB("web-pdb", testNamespace, 0, 2, appLabels),
		// Risk: metrics-server PDB, no pods on draining nodes
		newTestPDB("metrics-pdb", "kube-system", 0, 1, metricsLabels),
		// Risk: gatekeeper PDB, no pods on draining nodes
		newTestPDB("gatekeeper-pdb", "gatekeeper-system", 0, 3, gatekeeperLabels),
		// Healthy: has disruption budget available
		newTestPDB("healthy-pdb", testNamespace, 1, 3, healthyLabels),
	}

	pods := []*corev1.Pod{
		// web pod on draining node
		newTestPod("web-1", testNamespace, testNodeA, appLabels),
		newTestPod("web-2", testNamespace, testNodeB, appLabels),
		// metrics-server pod on non-draining node
		newTestPod("metrics-1", "kube-system", testNodeB, metricsLabels),
		// gatekeeper pod on non-draining node
		newTestPod("gatekeeper-1", "gatekeeper-system", testNodeB, gatekeeperLabels),
		// healthy pod on draining node (but PDB has budget)
		newTestPod("healthy-1", testNamespace, testNodeA, healthyLabels),
	}

	drainingNodes := []string{testNodeA}

	// Build a fake PDBWatcher with a populated store
	w := &PDBWatcher{
		informer:          &fakeInformer{objects: pdbStore},
		blockerStartTimes: make(map[string]time.Time),
	}

	blockers := w.BuildBlockers(drainingNodes, pods)

	// Count by tier
	var activeCount, riskCount int
	for _, b := range blockers {
		switch b.Tier {
		case types.BlockerTierActive:
			activeCount++
			if b.NodeName != testNodeA {
				t.Errorf("active blocker has wrong NodeName: got %q, want %q", b.NodeName, testNodeA)
			}
			if b.Name != "web-pdb" {
				t.Errorf("active blocker has wrong Name: got %q, want %q", b.Name, "web-pdb")
			}
		case types.BlockerTierRisk:
			riskCount++
			if b.NodeName != "" {
				t.Errorf("risk blocker should not have NodeName, got %q", b.NodeName)
			}
		}
	}

	if activeCount != 1 {
		t.Errorf("expected 1 active blocker, got %d", activeCount)
	}
	if riskCount != 2 {
		t.Errorf("expected 2 risk blockers, got %d (metrics-pdb + gatekeeper-pdb)", riskCount)
	}
}

func TestBuildBlockers_PDBBudgetRecovers(t *testing.T) {
	appLabels := map[string]string{"app": "web"}

	// PDB has budget now — should return no blockers
	pdbStore := []interface{}{
		newTestPDB("web-pdb", testNamespace, 1, 2, appLabels),
	}

	pods := []*corev1.Pod{
		newTestPod("web-1", testNamespace, testNodeA, appLabels),
	}

	w := &PDBWatcher{
		informer:          &fakeInformer{objects: pdbStore},
		blockerStartTimes: make(map[string]time.Time),
	}

	blockers := w.BuildBlockers([]string{testNodeA}, pods)
	if len(blockers) != 0 {
		t.Errorf("expected 0 blockers when PDB has budget, got %d", len(blockers))
	}
}

func TestBuildBlockers_NoDrainingNodes(t *testing.T) {
	appLabels := map[string]string{"app": "web"}

	// PDB at risk but no draining nodes — should return risk only
	pdbStore := []interface{}{
		newTestPDB("web-pdb", testNamespace, 0, 2, appLabels),
	}

	pods := []*corev1.Pod{
		newTestPod("web-1", testNamespace, testNodeA, appLabels),
	}

	w := &PDBWatcher{
		informer:          &fakeInformer{objects: pdbStore},
		blockerStartTimes: make(map[string]time.Time),
	}

	blockers := w.BuildBlockers(nil, pods)
	if len(blockers) != 1 {
		t.Fatalf("expected 1 blocker, got %d", len(blockers))
	}
	if blockers[0].Tier != types.BlockerTierRisk {
		t.Errorf("expected risk tier, got %v", blockers[0].Tier)
	}
}

func TestBuildBlockers_MultipleNodesBlocked(t *testing.T) {
	appLabels := map[string]string{"app": "web"}

	// PDB at risk, pods on both draining nodes
	pdbStore := []interface{}{
		newTestPDB("web-pdb", testNamespace, 0, 3, appLabels),
	}

	pods := []*corev1.Pod{
		newTestPod("web-1", testNamespace, testNodeA, appLabels),
		newTestPod("web-2", testNamespace, testNodeB, appLabels),
	}

	w := &PDBWatcher{
		informer:          &fakeInformer{objects: pdbStore},
		blockerStartTimes: make(map[string]time.Time),
	}

	blockers := w.BuildBlockers([]string{testNodeA, testNodeB}, pods)

	// Should produce 2 active blockers (one per draining node)
	if len(blockers) != 2 {
		t.Fatalf("expected 2 active blockers (one per node), got %d", len(blockers))
	}
	for _, b := range blockers {
		if b.Tier != types.BlockerTierActive {
			t.Errorf("expected active tier, got %v", b.Tier)
		}
	}

	// Verify different node names
	nodes := map[string]bool{blockers[0].NodeName: true, blockers[1].NodeName: true}
	if !nodes[testNodeA] || !nodes[testNodeB] {
		t.Errorf("expected blockers on both nodes, got %v and %v", blockers[0].NodeName, blockers[1].NodeName)
	}
}

func TestBuildBlockers_CordonedNodeTreatedAsBlockable(t *testing.T) {
	// Regression test: cordoned nodes must be included in the blockable set,
	// not just draining nodes. ComputeStage returns CORDONED for unschedulable
	// nodes; the CORDONED→DRAINING correction only happens in buildState.
	// BuildBlockers receives the node list from nodesBlockableByPDB which
	// includes both CORDONED and DRAINING nodes.
	appLabels := map[string]string{"app": "web"}

	pdbStore := []interface{}{
		newTestPDB("web-pdb", testNamespace, 0, 2, appLabels),
	}

	pods := []*corev1.Pod{
		newTestPod("web-1", testNamespace, testNodeA, appLabels),
	}

	w := &PDBWatcher{
		informer:          &fakeInformer{objects: pdbStore},
		blockerStartTimes: make(map[string]time.Time),
	}

	// Simulate passing a cordoned node (not yet draining) as blockable.
	// This is the scenario nodesBlockableByPDB covers — CORDONED nodes
	// are included alongside DRAINING nodes.
	blockers := w.BuildBlockers([]string{testNodeA}, pods)

	if len(blockers) != 1 {
		t.Fatalf("expected 1 blocker for cordoned node, got %d", len(blockers))
	}
	if blockers[0].Tier != types.BlockerTierActive {
		t.Errorf("expected active tier for cordoned node with matching pod, got %v", blockers[0].Tier)
	}
	if blockers[0].NodeName != testNodeA {
		t.Errorf("expected NodeName %q, got %q", testNodeA, blockers[0].NodeName)
	}
	if blockers[0].PodName != "web-1" {
		t.Errorf("expected PodName %q, got %q", "web-1", blockers[0].PodName)
	}
}

func TestBuildBlockers_StartTimePreservedAcrossCalls(t *testing.T) {
	appLabels := map[string]string{"app": "web"}

	pdbStore := []interface{}{
		newTestPDB("web-pdb", testNamespace, 0, 2, appLabels),
	}

	pods := []*corev1.Pod{
		newTestPod("web-1", testNamespace, testNodeA, appLabels),
	}

	w := &PDBWatcher{
		informer:          &fakeInformer{objects: pdbStore},
		blockerStartTimes: make(map[string]time.Time),
	}

	// First call sets StartTime
	blockers1 := w.BuildBlockers([]string{testNodeA}, pods)
	if len(blockers1) != 1 {
		t.Fatalf("expected 1 blocker, got %d", len(blockers1))
	}
	startTime := blockers1[0].StartTime
	if startTime.IsZero() {
		t.Fatal("expected non-zero StartTime on first call")
	}

	// Second call should preserve the same StartTime
	time.Sleep(10 * time.Millisecond)
	blockers2 := w.BuildBlockers([]string{testNodeA}, pods)
	if len(blockers2) != 1 {
		t.Fatalf("expected 1 blocker, got %d", len(blockers2))
	}
	if !blockers2[0].StartTime.Equal(startTime) {
		t.Errorf("StartTime changed across calls: %v → %v", startTime, blockers2[0].StartTime)
	}
}

// fakeInformer implements just enough of cache.SharedIndexInformer
// for BuildBlockers to work — specifically GetStore().List().
type fakeInformer struct {
	cache.SharedIndexInformer
	objects []interface{}
}

func (f *fakeInformer) GetStore() cache.Store {
	return &fakeStore{objects: f.objects}
}

// fakeStore implements cache.Store for testing.
type fakeStore struct {
	cache.Store
	objects []interface{}
}

func (s *fakeStore) List() []interface{} {
	return s.objects
}

func (s *fakeStore) GetByKey(key string) (interface{}, bool, error) {
	for _, obj := range s.objects {
		if pdb, ok := obj.(*policyv1.PodDisruptionBudget); ok {
			if pdb.Namespace+"/"+pdb.Name == key {
				return obj, true, nil
			}
		}
	}
	return nil, false, nil
}
