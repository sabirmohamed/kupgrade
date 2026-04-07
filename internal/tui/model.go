package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/sabirmohamed/kupgrade/internal/kube"
	"github.com/sabirmohamed/kupgrade/pkg/types"
	"golang.org/x/mod/semver"
	"k8s.io/client-go/kubernetes"
)

const (
	maxEvents     = 100
	maxMigrations = 50
)

// Screen represents the main navigation screens (0-4)
type Screen int

const (
	ScreenOverview Screen = iota // 0 - Dashboard with pills, pools, active nodes
	ScreenNodes                  // 1 - Full node details, conditions
	ScreenDrains                 // 2 - Eviction progress + blockers
	ScreenPods                   // 3 - Pod health, phase by node
	ScreenEvents                 // 4 - Full event log with filtering
)

// PodPriority determines which section a pod belongs to
type PodPriority int

const (
	PodPriorityAttention PodPriority = iota // Red — needs action
	PodPriorityDisrupted                    // Orange — moved, verify healthy
	PodPriorityHealthy                      // Green — unaffected
	PodPrioritySeparator PodPriority = -1   // Sentinel for section separator rows
)

// classifiedPod holds a pod with its computed priority
type classifiedPod struct {
	Pod      types.PodState
	Priority PodPriority
}

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

// Config holds TUI configuration
type Config struct {
	Context             string
	ServerVersion       string
	TargetVersion       string
	ControlPlaneVersion string // Actual API server version at startup
	InitialNodes        []types.NodeState
	InitialPods         []types.PodState
	InitialBlockers     []types.Blocker
	PreFlightBlockers   []types.Blocker // PDBs that will block drains (structural misconfiguration)
	EventCh             <-chan types.Event
	NodeStateCh         <-chan types.NodeState
	PodStateCh          <-chan types.PodState
	BlockerCh           <-chan types.Blocker
	Clientset           kubernetes.Interface
}

// Model is the TUI state
type Model struct {
	config Config
	keys   keyMap

	// Dimensions
	width  int
	height int

	// Navigation state
	screen          Screen  // Current screen (0-4)
	overlay         Overlay // Modal overlay (none, help, detail)
	selectedStage   int     // For Overview screen
	selectedNode    int     // For Overview screen
	listIndex       int     // For list-based screens (Overview node list, Events, Blockers)
	expandedGroup   string  // Currently expanded event group (reason)
	podSearchActive bool    // Whether fuzzy search input is active
	podSearchInput  textinput.Model

	// Data (display only - no computation)
	nodes             map[string]types.NodeState
	nodesByStage      map[types.NodeStage][]string
	nodesByPool       map[string][]string       // pool name → node names
	pods              map[string]types.PodState // key: namespace/name
	events            []types.Event
	migrations        []types.Migration
	blockers          []types.Blocker
	preFlightBlockers []types.Blocker // PDBs that will block drains (structural)

	// Platform + timing
	platform         string    // "AKS", "EKS", "GKE", or ""
	upgradeStartTime time.Time // Set when first non-READY node appears

	// Cached version range (recomputed on NodeUpdateMsg, not render-time)
	lowestVersion  string // Current lowest version across all nodes
	highestVersion string // Current highest version across all nodes

	// Control plane version (polled from API server)
	controlPlaneVersion string // Current API server version (polled every 30s)
	initialCPVersion    string // Captured at startup (to detect CP upgrade)
	cpUpgraded          bool   // True when CP version changed from initial

	// Detail overlay state
	detailViewport viewport.Model
	detailType     DetailType
	detailKey      string // node name, "ns/pod", or event composite key

	// Bubbles components
	spinner  spinner.Model
	progress progress.Model
	help     help.Model

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

	// Help
	h := help.New()
	h.Styles.ShortKey = footerKeyStyle
	h.Styles.ShortDesc = footerDescStyle
	h.Styles.ShortSeparator = footerDescStyle
	h.Styles.FullKey = footerKeyStyle
	h.Styles.FullDesc = footerDescStyle
	h.Styles.FullSeparator = footerDescStyle

	vp := viewport.New(0, 0) // Sized on first WindowSizeMsg

	ti := textinput.New()
	ti.Placeholder = "type to filter pods..."
	ti.CharLimit = 64
	ti.Width = 30

	m := Model{
		config:         cfg,
		keys:           defaultKeys,
		screen:         ScreenOverview,
		overlay:        OverlayNone,
		detailViewport: vp,
		nodes:          make(map[string]types.NodeState),
		nodesByStage:   make(map[types.NodeStage][]string),
		nodesByPool:    make(map[string][]string),
		pods:           make(map[string]types.PodState),
		events:         make([]types.Event, 0, maxEvents),
		migrations:     make([]types.Migration, 0, maxMigrations),
		blockers:       make([]types.Blocker, 0),
		currentTime:    time.Now(),
		podSearchInput: ti,
		spinner:        sp,
		progress:       prog,
		help:           h,
	}

	// Set initial control plane version
	if cfg.ControlPlaneVersion != "" {
		m.controlPlaneVersion = cfg.ControlPlaneVersion
		m.initialCPVersion = cfg.ControlPlaneVersion
	}

	// Load initial nodes
	for _, node := range cfg.InitialNodes {
		m.nodes[node.Name] = node
	}
	m.rebuildNodesByStage()
	m.recomputeVersionRange()

	// Load initial pods
	for _, pod := range cfg.InitialPods {
		m.pods[pod.Namespace+"/"+pod.Name] = pod
	}

	// Load initial blockers
	m.blockers = append(m.blockers, cfg.InitialBlockers...)
	m.preFlightBlockers = cfg.PreFlightBlockers

	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		waitForEvent(m.config.EventCh),
		waitForNodeState(m.config.NodeStateCh),
		waitForPodState(m.config.PodStateCh),
		waitForBlocker(m.config.BlockerCh),
		tick(),
		m.spinner.Tick,
	}
	if cmd := fetchNodeMetrics(m.config.Clientset); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if cmd := fetchControlPlaneVersion(m.config.Clientset); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
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

const metricsRefreshInterval = 15 * time.Second

// fetchNodeMetrics returns a tea.Cmd that fetches CPU/Memory metrics from the metrics-server.
// Returns nil when clientset is unavailable (e.g., smoke tests).
func fetchNodeMetrics(clientset kubernetes.Interface) tea.Cmd {
	if clientset == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return NodeMetricsMsg(kube.FetchNodeMetrics(ctx, clientset))
	}
}

// scheduleMetricsRefresh returns a tea.Cmd that triggers the next metrics fetch after the interval.
func scheduleMetricsRefresh() tea.Cmd {
	return tea.Tick(metricsRefreshInterval, func(t time.Time) tea.Msg {
		return metricsRefreshMsg{}
	})
}

const cpVersionPollInterval = 30 * time.Second

// fetchControlPlaneVersion returns a tea.Cmd that fetches the API server version.
// Returns nil when clientset is unavailable (e.g., smoke tests).
func fetchControlPlaneVersion(clientset kubernetes.Interface) tea.Cmd {
	if clientset == nil {
		return nil
	}
	return func() tea.Msg {
		info, err := clientset.Discovery().ServerVersion()
		if err != nil {
			return cpVersionMsg{} // Empty version — keeps poll cycle alive
		}
		return cpVersionMsg{Version: info.GitVersion}
	}
}

// scheduleCPVersionCheck returns a tea.Cmd that triggers the next CP version poll after the interval.
func scheduleCPVersionCheck() tea.Cmd {
	return tea.Tick(cpVersionPollInterval, func(t time.Time) tea.Msg {
		return cpVersionCheckMsg{}
	})
}

// Helper accessors

func (m Model) contextName() string { return m.config.Context }

// currentVersion returns the current lowest version across nodes (dynamic).
// Falls back to config.ServerVersion if no nodes are loaded yet.
func (m Model) currentVersion() string {
	if m.lowestVersion != "" {
		return m.lowestVersion
	}
	return m.config.ServerVersion
}

// targetVersion returns the target (highest) version across nodes (dynamic).
// Falls back to config.TargetVersion if no nodes are loaded yet.
func (m Model) targetVersion() string {
	if m.highestVersion != "" {
		return m.highestVersion
	}
	return m.config.TargetVersion
}

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
	case ScreenEvents:
		return "EVENTS"
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

// getSortedNodeList returns nodes sorted by stage priority (action-needed first), then name.
// Surge nodes sort after all real nodes.
func (m *Model) getSortedNodeList() []string {
	var realNodes []string
	var surgeNodeNames []string

	// Priority order: DRAINING, CORDONED, REIMAGING, READY, COMPLETE
	stagePriority := []types.NodeStage{
		types.StageDraining,
		types.StageCordoned,
		types.StageReimaging,
		types.StageReady,
		types.StageComplete,
	}

	for _, stage := range stagePriority {
		nodes := m.nodesByStage[stage]
		sorted := make([]string, len(nodes))
		copy(sorted, nodes)
		sort.Strings(sorted)
		for _, name := range sorted {
			if node, ok := m.nodes[name]; ok && node.SurgeNode {
				surgeNodeNames = append(surgeNodeNames, name)
			} else {
				realNodes = append(realNodes, name)
			}
		}
	}

	return append(realNodes, surgeNodeNames...)
}

func (m *Model) rebuildNodesByStage() {
	m.nodesByStage = make(map[types.NodeStage][]string)
	m.nodesByPool = make(map[string][]string)
	for name, node := range m.nodes {
		m.nodesByStage[node.Stage] = append(m.nodesByStage[node.Stage], name)
		if node.Pool != "" {
			m.nodesByPool[node.Pool] = append(m.nodesByPool[node.Pool], name)
		}
	}

	// Track upgrade start: set when first non-READY, non-COMPLETE node appears
	if m.upgradeStartTime.IsZero() {
		for _, stage := range []types.NodeStage{types.StageCordoned, types.StageDraining, types.StageReimaging} {
			if len(m.nodesByStage[stage]) > 0 {
				m.upgradeStartTime = time.Now()
				break
			}
		}
	}

	// Detect platform once from providerID
	if m.platform == "" {
		for _, node := range m.nodes {
			if node.ProviderID != "" {
				m.platform = detectPlatform(node.ProviderID)
				break
			}
		}
	}
}

// recomputeVersionRange updates cached lowest/highest versions from current node data.
// Called on every NodeUpdateMsg alongside rebuildNodesByStage().
func (m *Model) recomputeVersionRange() {
	var lowest, highest string
	for _, node := range m.nodes {
		if node.Deleted || node.Version == "" {
			continue
		}
		version := node.Version
		if lowest == "" || semver.Compare(version, lowest) < 0 {
			lowest = version
		}
		if highest == "" || semver.Compare(version, highest) > 0 {
			highest = version
		}
	}
	m.lowestVersion = lowest
	m.highestVersion = highest
}

// versionCore extracts "vMAJOR.MINOR.PATCH" stripping pre-release/build metadata.
// e.g., "v1.33.5-gke.2019000" → "v1.33.5"
func versionCore(v string) string {
	c := semver.Canonical(v)
	if c == "" {
		return v
	}
	if idx := strings.Index(c, "-"); idx != -1 {
		return c[:idx]
	}
	return c
}

// elapsedDisplay returns the upgrade elapsed time as "Xm Ys" or "" if not started
func (m *Model) elapsedDisplay() string {
	if m.upgradeStartTime.IsZero() {
		return ""
	}
	d := m.currentTime.Sub(m.upgradeStartTime)
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	if mins > 0 {
		return fmt.Sprintf("%dm %ds", mins, secs)
	}
	return fmt.Sprintf("%ds", secs)
}

// stageCounts returns a map of stage name → count (excluding surge nodes)
func (m *Model) stageCounts() map[string]int {
	counts := make(map[string]int)
	for _, stage := range types.AllStages() {
		counts[string(stage)] = m.stageCountExcludingSurge(stage)
	}
	return counts
}

func (m *Model) totalNodes() int {
	count := 0
	for _, node := range m.nodes {
		if !node.SurgeNode {
			count++
		}
	}
	return count
}

func (m *Model) completedNodes() int {
	count := 0
	for _, name := range m.nodesByStage[types.StageComplete] {
		if node, ok := m.nodes[name]; ok && !node.SurgeNode {
			count++
		}
	}
	return count
}

func (m *Model) progressPercent() int {
	total := m.totalNodes()
	if total == 0 {
		return 0
	}
	return (m.completedNodes() * 100) / total
}

// estimateRemaining returns an estimated remaining time string like "~5m 30s".
// Only returns a value when progress >= 5% and elapsed > 0.
func (m *Model) estimateRemaining() string {
	if m.upgradeStartTime.IsZero() {
		return ""
	}
	percent := m.progressPercent()
	if percent < 5 || percent >= 100 {
		return ""
	}
	elapsed := m.currentTime.Sub(m.upgradeStartTime)
	remaining := time.Duration(float64(elapsed) * float64(100-percent) / float64(percent))
	return formatDuration(remaining)
}

// sortedEvents returns all events sorted by severity (errors first), then newest first.
func (m *Model) sortedEvents() []types.Event {
	sorted := make([]types.Event, len(m.events))
	copy(sorted, m.events)
	sort.SliceStable(sorted, func(i, j int) bool {
		ri := severityRank(sorted[i].Severity)
		rj := severityRank(sorted[j].Severity)
		if ri != rj {
			return ri > rj // higher severity first
		}
		return sorted[i].Timestamp.After(sorted[j].Timestamp)
	})
	return sorted
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
