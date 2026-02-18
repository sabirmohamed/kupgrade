package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/sabirmohamed/kupgrade/pkg/types"
)

// newPopulatedModel creates a model with representative data for smoke tests
func newPopulatedModel() Model {
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
		ServerVersion: "v1.28.0",
		TargetVersion: "v1.29.0",
		EventCh:       eventCh,
		NodeStateCh:   nodeCh,
		PodStateCh:    podCh,
		BlockerCh:     blockerCh,
	})

	m.width = 120
	m.height = 40

	// Nodes across stages (with Pool for pool grouping)
	m.nodes = map[string]types.NodeState{
		"node-ready-1":     {Name: "node-ready-1", Stage: types.StageReady, Version: "v1.28.0", PodCount: 12, Ready: true, Age: "30d", Pool: "systempool"},
		"node-cordoned-1":  {Name: "node-cordoned-1", Stage: types.StageCordoned, Version: "v1.28.0", PodCount: 10, Ready: true, Age: "30d", Pool: "systempool"},
		"node-draining-1":  {Name: "node-draining-1", Stage: types.StageDraining, Version: "v1.28.0", PodCount: 5, Ready: true, DrainProgress: 50, InitialPodCount: 10, EvictablePodCount: 5, Age: "30d", Pool: "stdpool"},
		"node-reimaging-1": {Name: "node-reimaging-1", Stage: types.StageReimaging, Version: "v1.28.0", PodCount: 0, Ready: false, Age: "30d", Pool: "stdpool"},
		"node-complete-1":  {Name: "node-complete-1", Stage: types.StageComplete, Version: "v1.29.0", PodCount: 8, Ready: true, Age: "30d", Pool: "stdpool"},
	}
	m.rebuildNodesByStage()
	m.recomputeVersionRange()

	// Pods
	m.pods = map[string]types.PodState{
		"default/nginx-abc": {Name: "nginx-abc", Namespace: "default", NodeName: "node-ready-1", Phase: "Running",
			ReadyContainers: 1, TotalContainers: 1, HasReadiness: true, ReadinessOK: true, Age: "2d"},
		"kube-system/coredns-xyz": {Name: "coredns-xyz", Namespace: "kube-system", NodeName: "node-draining-1", Phase: "Running",
			ReadyContainers: 1, TotalContainers: 1, OwnerKind: "DaemonSet", Age: "30d"},
	}

	// Events
	m.events = []types.Event{
		{Type: types.EventNodeCordon, Severity: types.SeverityWarning, Message: "[Cordon] node-cordoned-1", NodeName: "node-cordoned-1", Timestamp: time.Now()},
		{Type: types.EventPodEvicted, Severity: types.SeverityWarning, Message: "[Evicted] pod nginx-abc from node-draining-1", NodeName: "node-draining-1", PodName: "nginx-abc", Timestamp: time.Now()},
		{Type: types.EventK8sWarning, Severity: types.SeverityWarning, Message: "[BackOff] Back-off restarting failed container", NodeName: "node-ready-1", Timestamp: time.Now()},
	}

	// Blockers
	m.blockers = []types.Blocker{
		{Type: "PDB", Name: "coredns", Namespace: "kube-system", Detail: "minAvailable=1, current=1", NodeName: "node-draining-1"},
	}

	// Migrations
	m.migrations = []types.Migration{
		{NewPod: "nginx-def", Namespace: "default", ToNode: "node-ready-1", Timestamp: time.Now()},
	}

	return m
}

func TestViewSmoke_AllScreens(t *testing.T) {
	m := newPopulatedModel()

	screens := []struct {
		name   string
		screen Screen
	}{
		{"Overview", ScreenOverview},
		{"Nodes", ScreenNodes},
		{"Drains", ScreenDrains},
		{"Pods", ScreenPods},
		{"Events", ScreenEvents},
	}

	for _, tt := range screens {
		t.Run(tt.name, func(t *testing.T) {
			m.screen = tt.screen
			output := m.View()
			if output == "" {
				t.Errorf("View() for screen %s returned empty string", tt.name)
			}
		})
	}
}

func TestViewSmoke_HelpOverlay(t *testing.T) {
	m := newPopulatedModel()
	m.overlay = OverlayHelp
	output := m.View()
	if output == "" {
		t.Error("View() with OverlayHelp returned empty string")
	}
}

func TestViewSmoke_EmptyModel(t *testing.T) {
	eventCh := make(chan types.Event)
	nodeCh := make(chan types.NodeState)
	podCh := make(chan types.PodState)
	blockerCh := make(chan types.Blocker)
	close(eventCh)
	close(nodeCh)
	close(podCh)
	close(blockerCh)

	m := New(Config{EventCh: eventCh, NodeStateCh: nodeCh, PodStateCh: podCh, BlockerCh: blockerCh})
	m.width = 80
	m.height = 24

	screens := []Screen{ScreenOverview, ScreenNodes, ScreenDrains, ScreenPods, ScreenEvents}
	for _, screen := range screens {
		m.screen = screen
		output := m.View()
		if output == "" {
			t.Errorf("View() for empty model screen %d returned empty string", screen)
		}
	}
}

func TestViewSmoke_DashboardPreFlight(t *testing.T) {
	// Pre-flight: all nodes READY, same version
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
		ServerVersion: "v1.28.0",
		EventCh:       eventCh,
		NodeStateCh:   nodeCh,
		PodStateCh:    podCh,
		BlockerCh:     blockerCh,
	})
	m.width = 120
	m.height = 40

	m.nodes = map[string]types.NodeState{
		"node-1": {Name: "node-1", Stage: types.StageReady, Version: "v1.28.0", Ready: true, Pool: "pool1", Age: "10d"},
		"node-2": {Name: "node-2", Stage: types.StageReady, Version: "v1.28.0", Ready: true, Pool: "pool1", Age: "10d"},
	}
	m.rebuildNodesByStage()
	m.recomputeVersionRange()

	output := m.View()
	if output == "" {
		t.Error("Pre-flight dashboard returned empty")
	}
	if !strings.Contains(output, "PRE-FLIGHT") {
		t.Error("Pre-flight dashboard should contain PRE-FLIGHT section")
	}
	if !strings.Contains(output, "Watching") {
		t.Error("Pre-flight status bar should show Watching")
	}
}

func TestViewSmoke_DashboardComplete(t *testing.T) {
	// Complete: all nodes at target version
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
		ServerVersion: "v1.29.0",
		TargetVersion: "v1.29.0",
		EventCh:       eventCh,
		NodeStateCh:   nodeCh,
		PodStateCh:    podCh,
		BlockerCh:     blockerCh,
	})
	m.width = 120
	m.height = 40

	m.nodes = map[string]types.NodeState{
		"node-1": {Name: "node-1", Stage: types.StageComplete, Version: "v1.29.0", Ready: true, Pool: "pool1", Age: "10d"},
		"node-2": {Name: "node-2", Stage: types.StageComplete, Version: "v1.29.0", Ready: true, Pool: "pool1", Age: "10d"},
	}
	m.rebuildNodesByStage()
	m.recomputeVersionRange()
	m.upgradeStartTime = time.Now().Add(-10 * time.Minute)

	output := m.View()
	if output == "" {
		t.Error("Complete dashboard returned empty")
	}
	if !strings.Contains(output, "All nodes upgraded") {
		t.Error("Complete dashboard should contain completion banner")
	}
}

func TestViewSmoke_PoolGrouping(t *testing.T) {
	m := newPopulatedModel()
	output := m.View()
	if !strings.Contains(output, "systempool") {
		t.Error("Dashboard should contain pool group header for systempool")
	}
	if !strings.Contains(output, "stdpool") {
		t.Error("Dashboard should contain pool group header for stdpool")
	}
}

func TestViewSmoke_StatusBar(t *testing.T) {
	m := newPopulatedModel()
	output := m.View()
	if !strings.Contains(output, "STATUS") {
		t.Error("Dashboard should contain STATUS badge in status bar")
	}
	if !strings.Contains(output, "kupgrade") {
		t.Error("Dashboard should contain kupgrade brand in status bar")
	}
}

func TestViewSmoke_KeyHints(t *testing.T) {
	m := newPopulatedModel()
	output := m.View()
	if !strings.Contains(output, "Dashboard") {
		t.Error("Dashboard should contain key hint for Dashboard")
	}
	if !strings.Contains(output, "Help") {
		t.Error("Dashboard should contain key hint for Help")
	}
}

func TestProgressBar(t *testing.T) {
	tests := []struct {
		name         string
		percent      int
		width        int
		expectFilled int
		expectEmpty  int
	}{
		{"zero", 0, 10, 0, 10},
		{"half", 50, 10, 5, 5},
		{"full", 100, 10, 10, 0},
		{"overflow", 150, 10, 10, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := progressBarFromPercent(tt.percent, tt.width)
			if result == "" {
				t.Error("progressBarFromPercent returned empty")
			}
		})
	}
}

func TestResourceColor(t *testing.T) {
	if resourceColor(80) != colorError {
		t.Error("80% should return error (red)")
	}
	if resourceColor(60) != colorWarning {
		t.Error("60% should return warning (yellow)")
	}
	if resourceColor(30) != colorTextMuted {
		t.Error("30% should return muted")
	}
}

func TestViewSmoke_CPVersionDisplay(t *testing.T) {
	// CP ahead of nodes — should show "CP" label in header
	eventCh := make(chan types.Event)
	nodeCh := make(chan types.NodeState)
	podCh := make(chan types.PodState)
	blockerCh := make(chan types.Blocker)
	close(eventCh)
	close(nodeCh)
	close(podCh)
	close(blockerCh)

	m := New(Config{
		Context:             "test-cluster",
		ServerVersion:       "v1.28.0",
		TargetVersion:       "v1.29.0",
		ControlPlaneVersion: "v1.29.0",
		EventCh:             eventCh,
		NodeStateCh:         nodeCh,
		PodStateCh:          podCh,
		BlockerCh:           blockerCh,
	})
	m.width = 120
	m.height = 40

	m.nodes = map[string]types.NodeState{
		"node-1": {Name: "node-1", Stage: types.StageReady, Version: "v1.28.0", Ready: true, Pool: "pool1", Age: "10d"},
		"node-2": {Name: "node-2", Stage: types.StageReady, Version: "v1.28.0", Ready: true, Pool: "pool1", Age: "10d"},
	}
	m.rebuildNodesByStage()
	m.recomputeVersionRange()

	output := m.View()
	if output == "" {
		t.Error("View() with CP version returned empty string")
	}
	if !strings.Contains(output, "CP v1.29.0") {
		t.Error("Dashboard should show 'CP v1.29.0' when CP ahead of nodes")
	}
	if !strings.Contains(output, "Nodes") {
		t.Error("Dashboard should show 'Nodes' label when CP ahead of nodes")
	}
}

func TestViewSmoke_Events(t *testing.T) {
	m := newPopulatedModel()
	m.screen = ScreenEvents

	output := m.View()
	if output == "" {
		t.Error("View() for events screen returned empty string")
	}
}
