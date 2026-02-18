package tui

import (
	"fmt"
	"testing"
	"time"

	"github.com/sabirmohamed/kupgrade/pkg/types"
)

func TestCalcScrollOffset(t *testing.T) {
	tests := []struct {
		name        string
		selected    int
		visibleRows int
		totalItems  int
		want        int
	}{
		{"zero items", 0, 10, 0, 0},
		{"fits in view", 2, 10, 5, 0},
		{"at top", 0, 5, 20, 0},
		{"middle", 7, 5, 20, 3},
		{"near bottom", 18, 5, 20, 14},
		{"at bottom", 19, 5, 20, 15},
		{"selected equals visible", 5, 5, 20, 1},
		{"single visible row", 3, 1, 10, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calcScrollOffset(tt.selected, tt.visibleRows, tt.totalItems)
			if got != tt.want {
				t.Errorf("calcScrollOffset(%d, %d, %d) = %d, want %d",
					tt.selected, tt.visibleRows, tt.totalItems, got, tt.want)
			}
		})
	}
}

func TestStatusColor(t *testing.T) {
	tests := []struct {
		phase string
		want  string // color hex
	}{
		{"Running", string(colorComplete)},
		{"Pending", string(colorCordoned)},
		{"Completed", string(colorTextMuted)},
		{"Succeeded", string(colorTextMuted)},
		{"CrashLoopBackOff", string(colorError)},
		{"Error", string(colorError)},
		{"Failed", string(colorError)},
		{"ImagePullBackOff", string(colorError)},
		{"ErrImagePull", string(colorError)},
		{"OOMKilled", string(colorError)},
		{"RunContainerError", string(colorError)},
		{"CreateContainerError", string(colorError)},
		{"Terminating", string(colorBrightYellow)},
		{"Unknown", string(colorBrightRed)},
		{"Init:0/2", string(colorCyan)},
		{"Init:Error", string(colorCyan)},
		{"PodInitializing", string(colorCyan)},
		{"SomeUnknownPhase", string(colorText)},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			got := string(statusColor(tt.phase))
			if got != tt.want {
				t.Errorf("statusColor(%q) = %q, want %q", tt.phase, got, tt.want)
			}
		})
	}
}

func TestReadyColor(t *testing.T) {
	tests := []struct {
		name string
		pod  types.PodState
		want string
	}{
		{"no containers", types.PodState{TotalContainers: 0}, string(colorTextMuted)},
		{"all ready", types.PodState{ReadyContainers: 3, TotalContainers: 3}, string(colorComplete)},
		{"none ready", types.PodState{ReadyContainers: 0, TotalContainers: 2}, string(colorError)},
		{"partial", types.PodState{ReadyContainers: 1, TotalContainers: 3}, string(colorCordoned)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(readyColor(tt.pod))
			if got != tt.want {
				t.Errorf("readyColor() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRestartColor(t *testing.T) {
	tests := []struct {
		name     string
		restarts int
		want     string
	}{
		{"zero", 0, string(colorTextMuted)},
		{"low", 2, string(colorCordoned)},
		{"five", 5, string(colorCordoned)},
		{"high", 10, string(colorError)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(restartColor(tt.restarts))
			if got != tt.want {
				t.Errorf("restartColor(%d) = %q, want %q", tt.restarts, got, tt.want)
			}
		})
	}
}

// newClassifyModel returns a Model with upgrade-active nodes for testing classifyPod.
func newClassifyModel() Model {
	eventCh := make(chan types.Event)
	nodeCh := make(chan types.NodeState)
	podCh := make(chan types.PodState)
	blockerCh := make(chan types.Blocker)
	close(eventCh)
	close(nodeCh)
	close(podCh)
	close(blockerCh)

	m := New(Config{
		Context:       "test",
		ServerVersion: "v1.28.0",
		TargetVersion: "v1.29.0",
		EventCh:       eventCh,
		NodeStateCh:   nodeCh,
		PodStateCh:    podCh,
		BlockerCh:     blockerCh,
	})

	m.nodes = map[string]types.NodeState{
		"node-draining": {Name: "node-draining", Stage: types.StageDraining, Version: "v1.28.0"},
		"node-cordoned": {Name: "node-cordoned", Stage: types.StageCordoned, Version: "v1.28.0"},
		"node-ready":    {Name: "node-ready", Stage: types.StageReady, Version: "v1.28.0"},
		"node-complete": {Name: "node-complete", Stage: types.StageComplete, Version: "v1.29.0"},
	}
	m.rebuildNodesByStage()
	m.recomputeVersionRange()

	return m
}

func TestClassifyPod(t *testing.T) {
	m := newClassifyModel()

	// Add a migration for rescheduled pod detection
	m.migrations = []types.Migration{
		{NewPod: "rescheduled-pod", Namespace: "default", ToNode: "node-complete", Timestamp: time.Now()},
		{NewPod: "crash-after-move", Namespace: "default", ToNode: "node-complete", Timestamp: time.Now()},
	}

	// Add an active PDB blocker on node-draining
	m.blockers = []types.Blocker{
		{
			Type:     types.BlockerPDB,
			Tier:     types.BlockerTierActive,
			Name:     "test-pdb",
			NodeName: "node-draining",
		},
	}

	tests := []struct {
		name         string
		pod          types.PodState
		wantPriority PodPriority
	}{
		{
			name:         "error pod (CrashLoopBackOff)",
			pod:          types.PodState{Name: "error-pod", Namespace: "default", Phase: "CrashLoopBackOff", NodeName: "node-ready"},
			wantPriority: PodPriorityAttention,
		},
		{
			name:         "ImagePullBackOff",
			pod:          types.PodState{Name: "bad-image", Namespace: "default", Phase: "ImagePullBackOff", NodeName: "node-ready"},
			wantPriority: PodPriorityAttention,
		},
		{
			name:         "OOMKilled",
			pod:          types.PodState{Name: "oom-pod", Namespace: "default", Phase: "OOMKilled", NodeName: "node-ready"},
			wantPriority: PodPriorityAttention,
		},
		{
			name:         "Failed (generic)",
			pod:          types.PodState{Name: "fail-pod", Namespace: "default", Phase: "Failed", NodeName: "node-ready"},
			wantPriority: PodPriorityAttention,
		},
		{
			name:         "pending no node",
			pod:          types.PodState{Name: "pending-pod", Namespace: "default", Phase: "Pending", NodeName: ""},
			wantPriority: PodPriorityAttention,
		},
		{
			name:         "pending with node (stuck)",
			pod:          types.PodState{Name: "stuck-pod", Namespace: "default", Phase: "Pending", NodeName: "node-ready"},
			wantPriority: PodPriorityAttention,
		},
		{
			name:         "pod on draining node with PDB blocker",
			pod:          types.PodState{Name: "blocked-pod", Namespace: "default", Phase: "Running", NodeName: "node-draining", ReadyContainers: 1, TotalContainers: 1},
			wantPriority: PodPriorityAttention,
		},
		{
			name:         "pod on cordoned node (no PDB)",
			pod:          types.PodState{Name: "cordon-pod", Namespace: "default", Phase: "Running", NodeName: "node-cordoned", ReadyContainers: 1, TotalContainers: 1},
			wantPriority: PodPriorityAttention,
		},
		{
			name:         "crash after rescheduling",
			pod:          types.PodState{Name: "crash-after-move", Namespace: "default", Phase: "CrashLoopBackOff", NodeName: "node-complete"},
			wantPriority: PodPriorityAttention,
		},
		{
			name:         "running but not ready (probe failure)",
			pod:          types.PodState{Name: "probe-fail", Namespace: "default", Phase: "Running", NodeName: "node-ready", ReadyContainers: 0, TotalContainers: 2},
			wantPriority: PodPriorityAttention,
		},
		{
			name:         "running partial ready (sidecar crash)",
			pod:          types.PodState{Name: "partial-pod", Namespace: "default", Phase: "Running", NodeName: "node-ready", ReadyContainers: 1, TotalContainers: 2},
			wantPriority: PodPriorityAttention,
		},
		{
			name:         "Unknown phase",
			pod:          types.PodState{Name: "unknown-pod", Namespace: "default", Phase: "Unknown", NodeName: "node-ready"},
			wantPriority: PodPriorityAttention,
		},
		{
			name:         "init container error",
			pod:          types.PodState{Name: "init-err", Namespace: "default", Phase: "Init:Error", NodeName: "node-ready"},
			wantPriority: PodPriorityAttention,
		},
		{
			name:         "init container progress (not error)",
			pod:          types.PodState{Name: "init-progress", Namespace: "default", Phase: "Init:0/2", NodeName: "node-ready", ReadyContainers: 0, TotalContainers: 0},
			wantPriority: PodPriorityHealthy,
		},
		{
			name:         "rescheduled and running",
			pod:          types.PodState{Name: "rescheduled-pod", Namespace: "default", Phase: "Running", NodeName: "node-complete", ReadyContainers: 1, TotalContainers: 1},
			wantPriority: PodPriorityDisrupted,
		},
		{
			name:         "normal healthy pod",
			pod:          types.PodState{Name: "healthy-pod", Namespace: "default", Phase: "Running", NodeName: "node-ready", ReadyContainers: 1, TotalContainers: 1},
			wantPriority: PodPriorityHealthy,
		},
		{
			name:         "succeeded pod (healthy)",
			pod:          types.PodState{Name: "done-pod", Namespace: "default", Phase: "Succeeded", NodeName: "node-ready"},
			wantPriority: PodPriorityHealthy,
		},
		{
			name:         "DaemonSet pod on draining node (healthy — tolerates drain)",
			pod:          types.PodState{Name: "kube-proxy-abc", Namespace: "kube-system", Phase: "Running", NodeName: "node-draining", OwnerKind: "DaemonSet", ReadyContainers: 1, TotalContainers: 1},
			wantPriority: PodPriorityHealthy,
		},
		{
			name:         "Completed job on draining node (healthy — already finished)",
			pod:          types.PodState{Name: "eraser-job", Namespace: "kube-system", Phase: "Completed", NodeName: "node-draining"},
			wantPriority: PodPriorityHealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			priority := m.classifyPod(tt.pod)
			if priority != tt.wantPriority {
				t.Errorf("classifyPod(%s) priority = %d, want %d", tt.pod.Name, priority, tt.wantPriority)
			}
		})
	}
}

func TestPodRowCount(t *testing.T) {
	m := newClassifyModel()
	m.width = 120
	m.height = 40

	// Mix: 1 attention pod + 10 healthy pods — all rows shown (no collapse)
	var pods []types.PodState
	pods = append(pods, types.PodState{
		Name: "error-pod", Namespace: "default", Phase: "CrashLoopBackOff", NodeName: "node-ready",
	})
	for i := 0; i < 10; i++ {
		pods = append(pods, types.PodState{
			Name:            fmt.Sprintf("healthy-pod-%d", i),
			Namespace:       "default",
			Phase:           "Running",
			NodeName:        "node-ready",
			ReadyContainers: 1,
			TotalContainers: 1,
		})
	}

	classified := m.classifyPods(pods)
	attentionCount, _, healthyCount := countByPriority(classified)

	if attentionCount != 1 {
		t.Fatalf("expected 1 attention pod, got %d", attentionCount)
	}
	if healthyCount != 10 {
		t.Fatalf("expected 10 healthy pods, got %d", healthyCount)
	}

	// Expected: 1 attention sep + 1 attention pod + 1 healthy sep + 10 healthy = 13
	rowCount := m.podRowCount(pods)
	if rowCount != 13 {
		t.Errorf("podRowCount = %d, want 13", rowCount)
	}

	// All healthy pods: 1 healthy sep + 10 healthy = 11
	healthyOnly := pods[1:]
	rowCount = m.podRowCount(healthyOnly)
	if rowCount != 11 {
		t.Errorf("podRowCount (all healthy) = %d, want 11", rowCount)
	}
}

func TestCountByPriority(t *testing.T) {
	classified := []classifiedPod{
		{Priority: PodPriorityAttention},
		{Priority: PodPriorityAttention},
		{Priority: PodPriorityDisrupted},
		{Priority: PodPriorityHealthy},
		{Priority: PodPriorityHealthy},
		{Priority: PodPriorityHealthy},
	}

	attention, disrupted, healthy := countByPriority(classified)
	if attention != 2 {
		t.Errorf("attention = %d, want 2", attention)
	}
	if disrupted != 1 {
		t.Errorf("disrupted = %d, want 1", disrupted)
	}
	if healthy != 3 {
		t.Errorf("healthy = %d, want 3", healthy)
	}
}

func TestPodAtRow_SectionedMode(t *testing.T) {
	m := newClassifyModel()
	// All healthy pods → one "HEALTHY" section separator + pod rows
	m.nodes = map[string]types.NodeState{
		"node-ready": {Name: "node-ready", Stage: types.StageReady, Version: "v1.28.0"},
	}
	m.rebuildNodesByStage()

	pods := []types.PodState{
		{Name: "pod-a", Namespace: "default", Phase: "Running", NodeName: "node-ready", ReadyContainers: 1, TotalContainers: 1},
		{Name: "pod-b", Namespace: "default", Phase: "Running", NodeName: "node-ready", ReadyContainers: 1, TotalContainers: 1},
	}

	// Row 0 = healthy section separator
	pod := m.podAtRow(pods, 0)
	if pod != nil {
		t.Errorf("podAtRow(0) = %v, want nil (separator)", pod)
	}

	// Row 1 = first pod
	pod = m.podAtRow(pods, 1)
	if pod == nil || pod.Name != "pod-a" {
		t.Errorf("podAtRow(1) = %v, want pod-a", pod)
	}

	// Row 2 = second pod
	pod = m.podAtRow(pods, 2)
	if pod == nil || pod.Name != "pod-b" {
		t.Errorf("podAtRow(2) = %v, want pod-b", pod)
	}

	// Row 3 = out of bounds
	pod = m.podAtRow(pods, 3)
	if pod != nil {
		t.Errorf("podAtRow(3) = %v, want nil (out of bounds)", pod)
	}
}

func TestPodAtRow_FuzzySearch(t *testing.T) {
	// During fuzzy search, flat indexing is used
	m := newClassifyModel()
	m.nodes = map[string]types.NodeState{
		"node-ready": {Name: "node-ready", Stage: types.StageReady, Version: "v1.28.0"},
	}
	m.rebuildNodesByStage()
	m.podSearchInput.SetValue("pod") // simulate active search

	pods := []types.PodState{
		{Name: "pod-a", Namespace: "default", Phase: "Running", NodeName: "node-ready"},
		{Name: "pod-b", Namespace: "default", Phase: "Running", NodeName: "node-ready"},
	}

	pod := m.podAtRow(pods, 0)
	if pod == nil || pod.Name != "pod-a" {
		t.Errorf("podAtRow(0) during search = %v, want pod-a", pod)
	}
}

func TestShouldShowCPLine(t *testing.T) {
	closedChans := func() (chan types.Event, chan types.NodeState, chan types.PodState, chan types.Blocker) {
		eCh := make(chan types.Event)
		nCh := make(chan types.NodeState)
		pCh := make(chan types.PodState)
		bCh := make(chan types.Blocker)
		close(eCh)
		close(nCh)
		close(pCh)
		close(bCh)
		return eCh, nCh, pCh, bCh
	}

	t.Run("false when no CP version", func(t *testing.T) {
		eCh, nCh, pCh, bCh := closedChans()
		m := New(Config{
			ServerVersion: "v1.28.0",
			EventCh:       eCh, NodeStateCh: nCh, PodStateCh: pCh, BlockerCh: bCh,
		})
		if m.shouldShowCPLine() {
			t.Error("shouldShowCPLine() = true, want false (no CP version)")
		}
	})

	t.Run("true when CP version set", func(t *testing.T) {
		eCh, nCh, pCh, bCh := closedChans()
		m := New(Config{
			ServerVersion:       "v1.28.0",
			ControlPlaneVersion: "v1.28.0",
			EventCh:             eCh, NodeStateCh: nCh, PodStateCh: pCh, BlockerCh: bCh,
		})
		if !m.shouldShowCPLine() {
			t.Error("shouldShowCPLine() = false, want true (CP version is set)")
		}
	})
}

func TestIsCPAhead(t *testing.T) {
	closedChans := func() (chan types.Event, chan types.NodeState, chan types.PodState, chan types.Blocker) {
		eCh := make(chan types.Event)
		nCh := make(chan types.NodeState)
		pCh := make(chan types.PodState)
		bCh := make(chan types.Blocker)
		close(eCh)
		close(nCh)
		close(pCh)
		close(bCh)
		return eCh, nCh, pCh, bCh
	}

	t.Run("false when versions match", func(t *testing.T) {
		eCh, nCh, pCh, bCh := closedChans()
		m := New(Config{
			ServerVersion:       "v1.28.0",
			ControlPlaneVersion: "v1.28.0",
			EventCh:             eCh, NodeStateCh: nCh, PodStateCh: pCh, BlockerCh: bCh,
		})
		m.nodes = map[string]types.NodeState{
			"n1": {Name: "n1", Version: "v1.28.0", Stage: types.StageReady},
		}
		m.recomputeVersionRange()
		if m.isCPAhead() {
			t.Error("isCPAhead() = true, want false (versions match)")
		}
	})

	t.Run("true when CP ahead of nodes", func(t *testing.T) {
		eCh, nCh, pCh, bCh := closedChans()
		m := New(Config{
			ServerVersion:       "v1.28.0",
			ControlPlaneVersion: "v1.29.0",
			EventCh:             eCh, NodeStateCh: nCh, PodStateCh: pCh, BlockerCh: bCh,
		})
		m.nodes = map[string]types.NodeState{
			"n1": {Name: "n1", Version: "v1.28.0", Stage: types.StageReady},
		}
		m.recomputeVersionRange()
		if !m.isCPAhead() {
			t.Error("isCPAhead() = false, want true (CP ahead)")
		}
	})

	t.Run("handles EKS version suffix", func(t *testing.T) {
		eCh, nCh, pCh, bCh := closedChans()
		m := New(Config{
			ServerVersion:       "v1.32.11-eks-aeac579",
			ControlPlaneVersion: "v1.33.7-eks-4938f21",
			EventCh:             eCh, NodeStateCh: nCh, PodStateCh: pCh, BlockerCh: bCh,
		})
		m.nodes = map[string]types.NodeState{
			"n1": {Name: "n1", Version: "v1.32.11-eks-aeac579", Stage: types.StageReady},
		}
		m.recomputeVersionRange()
		if !m.isCPAhead() {
			t.Error("isCPAhead() = false, want true (EKS CP ahead)")
		}
	})
}

func TestCPUpgradedDetection(t *testing.T) {
	eCh := make(chan types.Event)
	nCh := make(chan types.NodeState)
	pCh := make(chan types.PodState)
	bCh := make(chan types.Blocker)
	close(eCh)
	close(nCh)
	close(pCh)
	close(bCh)

	m := New(Config{
		ServerVersion:       "v1.28.0",
		ControlPlaneVersion: "v1.28.0",
		EventCh:             eCh, NodeStateCh: nCh, PodStateCh: pCh, BlockerCh: bCh,
	})

	if m.cpUpgraded {
		t.Error("cpUpgraded should be false initially")
	}

	// Simulate receiving a CP version poll with a different version
	m.controlPlaneVersion = "v1.29.0"
	if versionCore(m.controlPlaneVersion) != versionCore(m.initialCPVersion) {
		m.cpUpgraded = true
	}

	if !m.cpUpgraded {
		t.Error("cpUpgraded should be true after CP version change")
	}
}

func TestIsErrorPhase(t *testing.T) {
	errorPhases := []string{
		"CrashLoopBackOff", "Error", "Failed", "ImagePullBackOff",
		"ErrImagePull", "OOMKilled", "RunContainerError", "CreateContainerError",
		"Unknown", "Init:Error", "Init:CrashLoopBackOff", "Init:ImagePullBackOff",
	}
	for _, phase := range errorPhases {
		if !isErrorPhase(phase) {
			t.Errorf("isErrorPhase(%q) = false, want true", phase)
		}
	}

	nonErrorPhases := []string{"Running", "Pending", "Succeeded", "Terminating", "Init:0/2", "Init:1/3", "PodInitializing"}
	for _, phase := range nonErrorPhases {
		if isErrorPhase(phase) {
			t.Errorf("isErrorPhase(%q) = true, want false", phase)
		}
	}
}
