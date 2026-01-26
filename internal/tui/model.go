package tui

import (
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sabirmohamed/kupgrade/pkg/types"
)

const (
	maxEvents     = 100
	maxMigrations = 50
)

// Screen represents the main navigation screens (0-6)
type Screen int

const (
	ScreenOverview  Screen = iota // 0 - Pipeline stages with node cards
	ScreenNodes                   // 1 - Full node details, conditions, taints
	ScreenDrains                  // 2 - Eviction progress per node
	ScreenPods                    // 3 - Pod health, probes, phase by node
	ScreenBlockers                // 4 - PDBs, local storage, stuck evictions
	ScreenEvents                  // 5 - Full event log with filtering
	ScreenStats                   // 6 - Timing, velocity, ETA, history
)

// Overlay represents modal overlays on top of screens
type Overlay int

const (
	OverlayNone Overlay = iota
	OverlayHelp
	OverlayNodeDetail
)

// Config holds TUI configuration
type Config struct {
	Context         string
	ServerVersion   string
	TargetVersion   string
	InitialNodes    []types.NodeState
	InitialPods     []types.PodState
	InitialBlockers []types.Blocker
	EventCh         <-chan types.Event
	NodeStateCh     <-chan types.NodeState
	PodStateCh      <-chan types.PodState
	BlockerCh       <-chan types.Blocker
}

// Model is the TUI state
type Model struct {
	config Config

	// Dimensions
	width  int
	height int

	// Navigation state
	screen        Screen  // Current screen (0-6)
	overlay       Overlay // Modal overlay (none, help, detail)
	selectedStage int     // For Overview screen
	selectedNode  int     // For Overview screen
	listIndex     int     // For list-based screens (Nodes, Pods, etc.)

	// Data (display only - no computation)
	nodes        map[string]types.NodeState
	nodesByStage map[types.NodeStage][]string
	pods         map[string]types.PodState // key: namespace/name
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
		screen:       ScreenOverview,
		overlay:      OverlayNone,
		nodes:        make(map[string]types.NodeState),
		nodesByStage: make(map[types.NodeStage][]string),
		pods:         make(map[string]types.PodState),
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

	// Load initial pods
	for _, pod := range cfg.InitialPods {
		m.pods[pod.Namespace+"/"+pod.Name] = pod
	}

	// Load initial blockers
	m.blockers = append(m.blockers, cfg.InitialBlockers...)

	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		waitForEvent(m.config.EventCh),
		waitForNodeState(m.config.NodeStateCh),
		waitForPodState(m.config.PodStateCh),
		waitForBlocker(m.config.BlockerCh),
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

func waitForPodState(ch <-chan types.PodState) tea.Cmd {
	return func() tea.Msg {
		state, ok := <-ch
		if !ok {
			return nil
		}
		return PodUpdateMsg{Pod: state}
	}
}

func waitForBlocker(ch <-chan types.Blocker) tea.Cmd {
	return func() tea.Msg {
		blocker, ok := <-ch
		if !ok {
			return nil
		}
		return BlockerMsg{Blocker: blocker}
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

// screenName returns the display name for the current screen
func (m Model) screenName() string {
	switch m.screen {
	case ScreenOverview:
		return ""
	case ScreenNodes:
		return "NODES"
	case ScreenDrains:
		return "DRAINS"
	case ScreenPods:
		return "PODS"
	case ScreenBlockers:
		return "BLOCKERS"
	case ScreenEvents:
		return "EVENTS"
	case ScreenStats:
		return "STATS"
	default:
		return ""
	}
}

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
	// New unified list approach using listIndex
	allNodes := m.getSortedNodeList()
	if m.listIndex < 0 || m.listIndex >= len(allNodes) {
		return ""
	}
	return allNodes[m.listIndex]
}

func (m *Model) selectedNodeState() (types.NodeState, bool) {
	name := m.selectedNodeName()
	if name == "" {
		return types.NodeState{}, false
	}
	state, ok := m.nodes[name]
	return state, ok
}

// getSortedNodeList returns nodes sorted by stage priority (action-needed first), then name
func (m *Model) getSortedNodeList() []string {
	var allNodes []string

	// Priority order: DRAINING, CORDONED, UPGRADING, READY, COMPLETE
	stagePriority := []types.NodeStage{
		types.StageDraining,
		types.StageCordoned,
		types.StageUpgrading,
		types.StageReady,
		types.StageComplete,
	}

	for _, stage := range stagePriority {
		nodes := m.nodesByStage[stage]
		sorted := make([]string, len(nodes))
		copy(sorted, nodes)
		sort.Strings(sorted)
		allNodes = append(allNodes, sorted...)
	}

	return allNodes
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
