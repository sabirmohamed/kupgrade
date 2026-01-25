package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sabirmohamed/kupgrade/pkg/types"
)

var spinnerFrames = []string{"◐", "◓", "◑", "◒"}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case NodeUpdateMsg:
		m.handleNodeUpdate(msg.Node)
		return m, waitForNodeState(m.config.NodeStateCh)

	case PodUpdateMsg:
		m.handlePodUpdate(msg.Pod)
		return m, waitForPodState(m.config.PodStateCh)

	case EventMsg:
		m.handleEvent(msg.Event)
		return m, waitForEvent(m.config.EventCh)

	case ErrorMsg:
		if !msg.Recoverable {
			m.fatalError = msg.Err
			return m, tea.Quit
		}
		return m, nil

	case TickMsg:
		m.currentTime = time.Now()
		return m, tick()

	case SpinnerMsg:
		m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
		return m, spinnerTick()
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Handle overlays first (they capture input)
	if m.overlay != OverlayNone {
		return m.handleOverlayKey(msg)
	}

	// Quit: from Overview exits, from other screens returns to Overview
	if matchKey(msg, keys.Quit) {
		if m.screen != ScreenOverview {
			m.screen = ScreenOverview
			return *m, nil
		}
		return *m, tea.Quit
	}

	// Help overlay toggle
	if matchKey(msg, keys.Help) {
		m.overlay = OverlayHelp
		return *m, nil
	}

	// Escape: return to Overview
	if matchKey(msg, keys.Escape) {
		m.screen = ScreenOverview
		return *m, nil
	}

	// Screen navigation (0-6)
	if screen := screenFromKey(msg); screen >= 0 {
		m.screen = screen
		m.listIndex = 0 // Reset list position on screen change
		return *m, nil
	}

	// Delegate to screen-specific handler
	switch m.screen {
	case ScreenOverview:
		return m.handleOverviewKey(msg)
	case ScreenNodes:
		return m.handleNodesKey(msg)
	case ScreenDrains:
		return m.handleDrainsKey(msg)
	case ScreenPods:
		return m.handlePodsKey(msg)
	case ScreenBlockers:
		return m.handleBlockersKey(msg)
	case ScreenEvents:
		return m.handleEventsKey(msg)
	case ScreenStats:
		return m.handleStatsKey(msg)
	}

	return *m, nil
}

func (m *Model) handleOverlayKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Any key closes overlay
	if matchKey(msg, keys.Escape) || matchKey(msg, keys.Enter) || matchKey(msg, keys.Help) {
		m.overlay = OverlayNone
		return *m, nil
	}
	// For Help overlay, also close on any other key
	if m.overlay == OverlayHelp {
		m.overlay = OverlayNone
		return *m, nil
	}
	return *m, nil
}

func (m *Model) handleOverviewKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	stages := types.AllStages()

	if matchKey(msg, keys.Left) {
		if m.selectedStage > 0 {
			m.selectedStage--
			m.selectedNode = 0
		}
		return *m, nil
	}

	if matchKey(msg, keys.Right) {
		if m.selectedStage < len(stages)-1 {
			m.selectedStage++
			m.selectedNode = 0
		}
		return *m, nil
	}

	if matchKey(msg, keys.Up) {
		if m.selectedNode > 0 {
			m.selectedNode--
		}
		return *m, nil
	}

	if matchKey(msg, keys.Down) {
		nodes := m.nodesInSelectedStage()
		if m.selectedNode < len(nodes)-1 {
			m.selectedNode++
		}
		return *m, nil
	}

	if matchKey(msg, keys.Enter) {
		if _, ok := m.selectedNodeState(); ok {
			m.overlay = OverlayNodeDetail
		}
		return *m, nil
	}

	return *m, nil
}

// Placeholder handlers for new screens - will be implemented in E1-E7

func (m *Model) handleNodesKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	return m.handleListNavigation(msg, len(m.nodes))
}

func (m *Model) handleDrainsKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Count cordoned + draining nodes (both are in the drain pipeline)
	drainCount := len(m.nodesByStage[types.StageCordoned]) + len(m.nodesByStage[types.StageDraining])
	return m.handleListNavigation(msg, drainCount)
}

func (m *Model) handlePodsKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	return m.handleListNavigation(msg, m.filteredPodCount())
}

// filteredPodCount returns count of pods on upgrading nodes (or all if none upgrading)
func (m *Model) filteredPodCount() int {
	upgradeNodes := make(map[string]bool)
	for _, name := range m.nodesByStage[types.StageCordoned] {
		upgradeNodes[name] = true
	}
	for _, name := range m.nodesByStage[types.StageDraining] {
		upgradeNodes[name] = true
	}
	for _, name := range m.nodesByStage[types.StageUpgrading] {
		upgradeNodes[name] = true
	}

	if len(upgradeNodes) == 0 {
		return len(m.pods)
	}

	count := 0
	for _, pod := range m.pods {
		if upgradeNodes[pod.NodeName] {
			count++
		}
	}
	return count
}

func (m *Model) handleBlockersKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	return m.handleListNavigation(msg, len(m.blockers))
}

func (m *Model) handleEventsKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	return m.handleListNavigation(msg, len(m.events))
}

func (m *Model) handleStatsKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Stats screen has no list navigation
	return *m, nil
}

// handleListNavigation provides common up/down/g/G/pgup/pgdown navigation for list screens
func (m *Model) handleListNavigation(msg tea.KeyMsg, itemCount int) (Model, tea.Cmd) {
	// Calculate page size (visible rows)
	pageSize := m.height - 10
	if pageSize < 5 {
		pageSize = 5
	}

	if matchKey(msg, keys.Up) {
		if m.listIndex > 0 {
			m.listIndex--
		}
		return *m, nil
	}

	if matchKey(msg, keys.Down) {
		if m.listIndex < itemCount-1 {
			m.listIndex++
		}
		return *m, nil
	}

	if matchKey(msg, keys.PageUp) {
		m.listIndex -= pageSize
		if m.listIndex < 0 {
			m.listIndex = 0
		}
		return *m, nil
	}

	if matchKey(msg, keys.PageDown) {
		m.listIndex += pageSize
		if m.listIndex >= itemCount {
			m.listIndex = itemCount - 1
		}
		if m.listIndex < 0 {
			m.listIndex = 0
		}
		return *m, nil
	}

	if matchKey(msg, keys.Top) {
		m.listIndex = 0
		return *m, nil
	}

	if matchKey(msg, keys.Bottom) {
		if itemCount > 0 {
			m.listIndex = itemCount - 1
		}
		return *m, nil
	}

	if matchKey(msg, keys.Enter) {
		// Placeholder for item detail - screens will override
		return *m, nil
	}

	return *m, nil
}

// handleNodeUpdate stores node state from watcher (no computation here)
func (m *Model) handleNodeUpdate(node types.NodeState) {
	if node.Deleted {
		delete(m.nodes, node.Name)
	} else {
		m.nodes[node.Name] = node
	}
	m.rebuildNodesByStage()
}

// handlePodUpdate stores pod state from watcher
func (m *Model) handlePodUpdate(pod types.PodState) {
	key := pod.Namespace + "/" + pod.Name
	if pod.Deleted {
		delete(m.pods, key)
	} else {
		m.pods[key] = pod
	}
}

// handleEvent adds event to display list (no state changes)
func (m *Model) handleEvent(e types.Event) {
	m.eventCount++

	m.events = append(m.events, e)
	if len(m.events) > maxEvents {
		m.events = m.events[len(m.events)-maxEvents:]
	}

	// Handle migration events
	if e.Type == types.EventMigration {
		m.migrations = append(m.migrations, types.Migration{
			NewPod:    e.PodName,
			Namespace: e.Namespace,
			ToNode:    e.NodeName,
			Timestamp: e.Timestamp,
		})
		if len(m.migrations) > maxMigrations {
			m.migrations = m.migrations[len(m.migrations)-maxMigrations:]
		}
	}
}

func (m *Model) spinner() string {
	return spinnerFrames[m.spinnerFrame]
}

// handleMouse handles mouse events for scrolling
func (m *Model) handleMouse(msg tea.MouseMsg) (Model, tea.Cmd) {
	// Only handle scroll on list screens
	switch m.screen {
	case ScreenNodes, ScreenDrains, ScreenPods, ScreenBlockers, ScreenEvents:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if m.listIndex > 0 {
				m.listIndex--
			}
			return *m, nil
		case tea.MouseButtonWheelDown:
			itemCount := m.currentListCount()
			if m.listIndex < itemCount-1 {
				m.listIndex++
			}
			return *m, nil
		}
	case ScreenOverview:
		// Scroll through nodes in selected stage
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if m.selectedNode > 0 {
				m.selectedNode--
			}
			return *m, nil
		case tea.MouseButtonWheelDown:
			nodes := m.nodesInSelectedStage()
			if m.selectedNode < len(nodes)-1 {
				m.selectedNode++
			}
			return *m, nil
		}
	}
	return *m, nil
}

// currentListCount returns the item count for the current list screen
func (m *Model) currentListCount() int {
	switch m.screen {
	case ScreenNodes:
		return len(m.nodes)
	case ScreenDrains:
		return len(m.nodesByStage[types.StageCordoned]) + len(m.nodesByStage[types.StageDraining])
	case ScreenPods:
		return m.filteredPodCount()
	case ScreenBlockers:
		return len(m.blockers)
	case ScreenEvents:
		return len(m.events)
	default:
		return 0
	}
}
