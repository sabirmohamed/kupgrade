package tui

import (
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

// Config holds TUI configuration
type Config struct {
	Context       string
	ServerVersion string
	TargetVersion string
	InitialNodes  []types.NodeState
	EventCh       <-chan types.Event
	NodeStateCh   <-chan types.NodeState
}

// Model is the TUI state
type Model struct {
	config Config

	// Dimensions
	width  int
	height int

	// View state
	viewMode      ViewMode
	drainMode     DrainMode
	selectedStage int
	selectedNode  int

	// Data (display only - no computation)
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

// New creates a new TUI model
func New(cfg Config) Model {
	m := Model{
		config:       cfg,
		viewMode:     ViewOverview,
		drainMode:    DrainModeDrain,
		nodes:        make(map[string]types.NodeState),
		nodesByStage: make(map[types.NodeStage][]string),
		events:       make([]types.Event, 0, maxEvents),
		migrations:   make([]types.Migration, 0, maxMigrations),
		blockers:     make([]types.Blocker, 0),
		currentTime:  time.Now(),
	}

	// Load initial nodes
	for _, node := range cfg.InitialNodes {
		m.nodes[node.Name] = node
	}
	m.rebuildNodesByStage()

	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		waitForEvent(m.config.EventCh),
		waitForNodeState(m.config.NodeStateCh),
		tick(),
		spinnerTick(),
	)
}

func waitForEvent(ch <-chan types.Event) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return nil
		}
		return EventMsg{Event: event}
	}
}

func waitForNodeState(ch <-chan types.NodeState) tea.Cmd {
	return func() tea.Msg {
		state, ok := <-ch
		if !ok {
			return nil
		}
		return NodeUpdateMsg{Node: state}
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

// Helper accessors

func (m Model) contextName() string   { return m.config.Context }
func (m Model) serverVersion() string { return m.config.ServerVersion }
func (m Model) targetVersion() string { return m.config.TargetVersion }

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

func (m *Model) rebuildNodesByStage() {
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
