package tui

import (
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/sabirmohamed/kupgrade/pkg/types"
	"k8s.io/client-go/kubernetes"
)

const (
	maxEvents     = 100
	maxMigrations = 50
)

// Screen represents the main navigation screens (0-6)
type Screen int

const (
	ScreenOverview Screen = iota // 0 - Pipeline stages with node cards
	ScreenNodes                  // 1 - Full node details, conditions, taints
	ScreenDrains                 // 2 - Eviction progress per node
	ScreenPods                   // 3 - Pod health, probes, phase by node
	ScreenBlockers               // 4 - PDBs, local storage, stuck evictions
	ScreenEvents                 // 5 - Full event log with filtering
	ScreenStats                  // 6 - Timing, velocity, ETA, history
)

// Overlay represents modal overlays on top of screens
type Overlay int

const (
	OverlayNone Overlay = iota
	OverlayHelp
	OverlayDetail // Full-screen detail for node/pod describe
)

// DetailType represents the resource shown in the detail overlay
type DetailType int

const (
	DetailNone DetailType = iota
	DetailNode
	DetailPod
	DetailEvent
)

// EventFilter represents the event filtering mode
type EventFilter int

const (
	EventFilterUpgrade  EventFilter = iota // Upgrade-related events only (default)
	EventFilterWarnings                    // Warning and Error events only
	EventFilterAll                         // All events
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
	Clientset       kubernetes.Interface
}

// Model is the TUI state
type Model struct {
	config Config
	keys   keyMap

	// Dimensions
	width  int
	height int

	// Navigation state
	screen          Screen      // Current screen (0-6)
	overlay         Overlay     // Modal overlay (none, help, detail)
	selectedStage   int         // For Overview screen
	selectedNode    int         // For Overview screen
	listIndex       int         // For list-based screens (Overview node list, Events, Blockers)
	eventFilter     EventFilter // Event filtering mode (upgrade/warnings/all)
	eventAggregated bool        // Whether to show aggregated events
	expandedGroup   string      // Currently expanded event group (reason)

	// Data (display only - no computation)
	nodes        map[string]types.NodeState
	nodesByStage map[types.NodeStage][]string
	pods         map[string]types.PodState // key: namespace/name
	events       []types.Event
	migrations   []types.Migration
	blockers     []types.Blocker

	// Detail overlay state
	detailViewport viewport.Model
	detailType     DetailType
	detailKey      string // node name, "ns/pod", or event composite key

	// Bubbles components
	spinner   spinner.Model
	progress  progress.Model
	smallProg progress.Model // Compact progress bar for cards/drains
	help      help.Model

	// Animation
	currentTime time.Time

	// Error
	fatalError error
}

// New creates a new TUI model
func New(cfg Config) Model {
	// Ensure lipgloss renders colors. The default renderer auto-detects from
	// termenv.DefaultOutput(), which may resolve to Ascii when bubbletea owns
	// stdout. Force TrueColor so StyleFunc-based table coloring works.
	lipgloss.SetColorProfile(termenv.TrueColor)

	// Spinner
	sp := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(colorWarning)),
	)

	// Progress bar (header)
	prog := progress.New(
		progress.WithSolidFill(string(colorComplete)),
		progress.WithoutPercentage(),
		progress.WithFillCharacters('█', '░'),
		progress.WithWidth(headerProgressBarWidth),
	)

	// Small progress bar (cards/drains)
	smallProg := progress.New(
		progress.WithSolidFill(string(colorComplete)),
		progress.WithoutPercentage(),
		progress.WithFillCharacters('█', '░'),
		progress.WithWidth(12),
	)

	// Help
	h := help.New()
	h.Styles.ShortKey = footerKeyStyle
	h.Styles.ShortDesc = footerDescStyle
	h.Styles.ShortSeparator = footerDescStyle
	h.Styles.FullKey = footerKeyStyle
	h.Styles.FullDesc = footerDescStyle
	h.Styles.FullSeparator = footerDescStyle

	vp := viewport.New(0, 0) // Sized on first WindowSizeMsg

	m := Model{
		config:         cfg,
		keys:           defaultKeys,
		screen:         ScreenOverview,
		overlay:        OverlayNone,
		detailViewport: vp,
		nodes:          make(map[string]types.NodeState),
		nodesByStage:   make(map[types.NodeStage][]string),
		pods:           make(map[string]types.PodState),
		events:         make([]types.Event, 0, maxEvents),
		migrations:     make([]types.Migration, 0, maxMigrations),
		blockers:       make([]types.Blocker, 0),
		currentTime:    time.Now(),
		spinner:        sp,
		progress:       prog,
		smallProg:      smallProg,
		help:           h,
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
		m.spinner.Tick,
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

// filteredEvents returns events based on the current filter mode
func (m *Model) filteredEvents() []types.Event {
	if m.eventFilter == EventFilterAll {
		return m.events
	}

	var filtered []types.Event
	for _, e := range m.events {
		switch m.eventFilter {
		case EventFilterUpgrade:
			if isUpgradeEvent(e.Type) {
				filtered = append(filtered, e)
			}
		case EventFilterWarnings:
			if e.Severity == types.SeverityWarning || e.Severity == types.SeverityError {
				filtered = append(filtered, e)
			}
		}
	}
	return filtered
}

// isUpgradeEvent returns true if the event type is upgrade-related
func isUpgradeEvent(t types.EventType) bool {
	switch t {
	case types.EventNodeCordon,
		types.EventNodeUncordon,
		types.EventNodeReady,
		types.EventNodeNotReady,
		types.EventNodeVersion,
		types.EventPodEvicted,
		types.EventPodFailed,
		types.EventPodDeleted,
		types.EventMigration,
		types.EventK8sWarning,
		types.EventK8sError:
		return true
	default:
		return false
	}
}

// eventFilterName returns the display name for the current filter
func (m *Model) eventFilterName() string {
	switch m.eventFilter {
	case EventFilterUpgrade:
		return "UPGRADE"
	case EventFilterWarnings:
		return "WARNINGS"
	case EventFilterAll:
		return "ALL"
	default:
		return "UPGRADE"
	}
}

// calcScrollOffset returns scroll offset to keep selected item visible
func calcScrollOffset(selected, visibleRows, totalItems int) int {
	if totalItems <= visibleRows {
		return 0
	}
	offset := 0
	if selected >= visibleRows {
		offset = selected - visibleRows + 1
	}
	if offset > totalItems-visibleRows {
		offset = totalItems - visibleRows
	}
	return offset
}
