package tui

import (
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

	// Nodes across stages
	m.nodes = map[string]types.NodeState{
		"node-ready-1":     {Name: "node-ready-1", Stage: types.StageReady, Version: "v1.28.0", PodCount: 12, Ready: true, Age: "30d"},
		"node-cordoned-1":  {Name: "node-cordoned-1", Stage: types.StageCordoned, Version: "v1.28.0", PodCount: 10, Ready: true, Age: "30d"},
		"node-draining-1":  {Name: "node-draining-1", Stage: types.StageDraining, Version: "v1.28.0", PodCount: 5, Ready: true, DrainProgress: 50, InitialPodCount: 10, Age: "30d"},
		"node-upgrading-1": {Name: "node-upgrading-1", Stage: types.StageUpgrading, Version: "v1.28.0", PodCount: 0, Ready: false, Age: "30d"},
		"node-complete-1":  {Name: "node-complete-1", Stage: types.StageComplete, Version: "v1.29.0", PodCount: 8, Ready: true, Age: "30d"},
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
		{"Blockers", ScreenBlockers},
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

	screens := []Screen{ScreenOverview, ScreenNodes, ScreenDrains, ScreenPods, ScreenBlockers, ScreenEvents}
	for _, screen := range screens {
		m.screen = screen
		output := m.View()
		if output == "" {
			t.Errorf("View() for empty model screen %d returned empty string", screen)
		}
	}
}

func TestViewSmoke_PodFilters(t *testing.T) {
	m := newPopulatedModel()
	m.screen = ScreenPods

	filters := []PodFilterMode{PodFilterDisrupting, PodFilterRescheduled, PodFilterAll}
	for _, filter := range filters {
		m.podFilterMode = filter
		output := m.View()
		if output == "" {
			t.Errorf("View() for pod filter %d returned empty string", filter)
		}
	}
}

func TestViewSmoke_EventFilters(t *testing.T) {
	m := newPopulatedModel()
	m.screen = ScreenEvents

	filters := []EventFilter{EventFilterUpgrade, EventFilterWarnings, EventFilterAll}
	for _, filter := range filters {
		m.eventFilter = filter
		output := m.View()
		if output == "" {
			t.Errorf("View() for event filter %d returned empty string", filter)
		}
	}

	// Aggregated mode
	m.eventAggregated = true
	output := m.View()
	if output == "" {
		t.Error("View() with aggregated events returned empty string")
	}
}
