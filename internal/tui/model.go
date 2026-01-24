package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sabirmohamed/kupgrade/pkg/types"
)

const (
	maxEvents     = 8
	maxMigrations = 5
)

type ViewMode int

const (
	ViewOverview ViewMode = iota
	ViewNodeDetail
	ViewHelp
)

type DrainMode int

const (
	DrainModeDrain DrainMode = iota
	DrainModeCordon
	DrainModeSchedule
)

type Model struct {
	ctx           context.Context
	eventCh       <-chan types.Event
	contextName   string
	serverVersion string
	targetVersion string

	// Dimensions
	width  int
	height int

	// View state
	viewMode      ViewMode
	drainMode     DrainMode
	selectedStage int
	selectedNode  int

	// Data
	nodes        map[string]types.NodeState
	nodesByStage map[types.NodeStage][]string
	events       []types.Event
	migrations   []types.Migration
	blockers     []types.Blocker
	eventCount   int

	// Animation
	spinnerFrame int
	currentTime  time.Time

	// Error
	fatalError error
}

func New(ctx context.Context, eventCh <-chan types.Event, contextName, serverVersion, targetVersion string, initialNodes []types.NodeState) Model {
	m := Model{
		ctx:           ctx,
		eventCh:       eventCh,
		contextName:   contextName,
		serverVersion: serverVersion,
		targetVersion: targetVersion,
		viewMode:      ViewOverview,
		drainMode:     DrainModeDrain,
		selectedStage: 0,
		selectedNode:  0,
		nodes:         make(map[string]types.NodeState),
		nodesByStage:  make(map[types.NodeStage][]string),
		events:        make([]types.Event, 0, maxEvents),
		migrations:    make([]types.Migration, 0, maxMigrations),
		blockers:      make([]types.Blocker, 0),
		currentTime:   time.Now(),
	}

	// Populate initial node states
	for _, node := range initialNodes {
		m.nodes[node.Name] = node
	}
	m.updateNodesByStage()

	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		waitForEvent(m.eventCh),
		tick(),
		spinnerTick(),
	)
}

func waitForEvent(eventCh <-chan types.Event) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-eventCh
		if !ok {
			return nil
		}
		return EventMsg{Event: event}
	}
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return TickMsg{}
	})
}

func spinnerTick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return SpinnerMsg{}
	})
}

// Helper methods

func (m *Model) stageAtIndex(idx int) types.NodeStage {
	stages := types.AllStages()
	if idx < 0 || idx >= len(stages) {
		return types.StageReady
	}
	return stages[idx]
}

func (m *Model) nodesInSelectedStage() []string {
	stage := m.stageAtIndex(m.selectedStage)
	return m.nodesByStage[stage]
}

func (m *Model) selectedNodeName() string {
	nodes := m.nodesInSelectedStage()
	if m.selectedNode < 0 || m.selectedNode >= len(nodes) {
		return ""
	}
	return nodes[m.selectedNode]
}

func (m *Model) selectedNodeState() (types.NodeState, bool) {
	name := m.selectedNodeName()
	if name == "" {
		return types.NodeState{}, false
	}
	state, ok := m.nodes[name]
	return state, ok
}

func (m *Model) updateNodesByStage() {
	m.nodesByStage = make(map[types.NodeStage][]string)
	for name, node := range m.nodes {
		m.nodesByStage[node.Stage] = append(m.nodesByStage[node.Stage], name)
	}
}

func (m *Model) totalNodes() int {
	return len(m.nodes)
}

func (m *Model) completedNodes() int {
	return len(m.nodesByStage[types.StageComplete])
}

func (m *Model) progressPercent() int {
	total := m.totalNodes()
	if total == 0 {
		return 0
	}
	return (m.completedNodes() * 100) / total
}
