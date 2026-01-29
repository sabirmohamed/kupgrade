package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sabirmohamed/kupgrade/pkg/types"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		return m, nil

	case NodeUpdateMsg:
		m.handleNodeUpdate(msg.Node)
		return m, waitForNodeState(m.config.NodeStateCh)

	case PodUpdateMsg:
		m.handlePodUpdate(msg.Pod)
		return m, waitForPodState(m.config.PodStateCh)

	case BlockerMsg:
		m.handleBlockerUpdate(msg.Blocker)
		return m, waitForBlocker(m.config.BlockerCh)

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

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Handle overlays first (they capture input)
	if m.overlay != OverlayNone {
		return m.handleOverlayKey(msg)
	}

	// Quit: from Overview exits, from other screens returns to Overview
	if key.Matches(msg, m.keys.Quit) {
		if m.screen != ScreenOverview {
			m.screen = ScreenOverview
			return *m, nil
		}
		return *m, tea.Quit
	}

	// Help overlay toggle
	if key.Matches(msg, m.keys.Help) {
		m.overlay = OverlayHelp
		return *m, nil
	}

	// Escape: return to Overview
	if key.Matches(msg, m.keys.Escape) {
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
	if key.Matches(msg, m.keys.Escape) || key.Matches(msg, m.keys.Enter) || key.Matches(msg, m.keys.Help) {
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
	allNodes := m.getSortedNodeList()

	if key.Matches(msg, m.keys.Left) {
		if m.selectedStage > 0 {
			m.selectedStage--
		}
		return *m, nil
	}

	if key.Matches(msg, m.keys.Right) {
		if m.selectedStage < len(stages)-1 {
			m.selectedStage++
		}
		return *m, nil
	}

	if key.Matches(msg, m.keys.Up) {
		if m.listIndex > 0 {
			m.listIndex--
		}
		return *m, nil
	}

	if key.Matches(msg, m.keys.Down) {
		if m.listIndex < len(allNodes)-1 {
			m.listIndex++
		}
		return *m, nil
	}

	if key.Matches(msg, m.keys.Enter) {
		if m.listIndex < len(allNodes) {
			nodeName := allNodes[m.listIndex]
			if _, ok := m.nodes[nodeName]; ok {
				m.overlay = OverlayNodeDetail
			}
		}
		return *m, nil
	}

	return *m, nil
}

func (m *Model) handleNodesKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	return m.handleListNavigation(msg, len(m.nodes))
}

func (m *Model) handleDrainsKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	return m.handleListNavigation(msg, len(m.getDrainNodes()))
}

func (m *Model) handlePodsKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	return m.handleListNavigation(msg, m.filteredPodCount())
}

// filteredPodCount returns count of pods on upgrading nodes (or all if none upgrading)
func (m *Model) filteredPodCount() int {
	return len(m.getFilteredPodList())
}

func (m *Model) handleBlockersKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	return m.handleListNavigation(msg, len(m.blockers))
}

func (m *Model) handleEventsKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Event filter toggles
	if key.Matches(msg, m.keys.EventUpgrade) {
		m.eventFilter = EventFilterUpgrade
		m.listIndex = 0
		return *m, nil
	}
	if key.Matches(msg, m.keys.EventWarnings) {
		m.eventFilter = EventFilterWarnings
		m.listIndex = 0
		return *m, nil
	}
	if key.Matches(msg, m.keys.EventAll) {
		m.eventFilter = EventFilterAll
		m.listIndex = 0
		return *m, nil
	}
	// Aggregation toggle
	if key.Matches(msg, m.keys.EventAggregate) {
		m.eventAggregated = !m.eventAggregated
		m.expandedGroup = ""
		m.listIndex = 0
		return *m, nil
	}
	// Expand/collapse group (only in aggregated view)
	if key.Matches(msg, m.keys.EventExpand) && m.eventAggregated {
		aggregated := aggregateEvents(m.filteredEvents())
		if m.listIndex < len(aggregated) {
			selectedReason := aggregated[m.listIndex].Reason
			if m.expandedGroup == selectedReason {
				m.expandedGroup = ""
			} else {
				m.expandedGroup = selectedReason
			}
		}
		return *m, nil
	}

	itemCount := len(m.filteredEvents())
	if m.eventAggregated {
		itemCount = len(aggregateEvents(m.filteredEvents()))
	}

	return m.handleListNavigation(msg, itemCount)
}

func (m *Model) handleStatsKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	return *m, nil
}

// handleListNavigation provides common up/down/g/G/pgup/pgdown navigation for non-table list screens
func (m *Model) handleListNavigation(msg tea.KeyMsg, itemCount int) (Model, tea.Cmd) {
	pageSize := m.height - 10
	if pageSize < 5 {
		pageSize = 5
	}

	if key.Matches(msg, m.keys.Up) {
		if m.listIndex > 0 {
			m.listIndex--
		}
		return *m, nil
	}

	if key.Matches(msg, m.keys.Down) {
		if m.listIndex < itemCount-1 {
			m.listIndex++
		}
		return *m, nil
	}

	if key.Matches(msg, m.keys.PageUp) {
		m.listIndex -= pageSize
		if m.listIndex < 0 {
			m.listIndex = 0
		}
		return *m, nil
	}

	if key.Matches(msg, m.keys.PageDown) {
		m.listIndex += pageSize
		if m.listIndex >= itemCount {
			m.listIndex = itemCount - 1
		}
		if m.listIndex < 0 {
			m.listIndex = 0
		}
		return *m, nil
	}

	if key.Matches(msg, m.keys.Top) {
		m.listIndex = 0
		return *m, nil
	}

	if key.Matches(msg, m.keys.Bottom) {
		if itemCount > 0 {
			m.listIndex = itemCount - 1
		}
		return *m, nil
	}

	if key.Matches(msg, m.keys.Enter) {
		return *m, nil
	}

	return *m, nil
}

// handleNodeUpdate stores node state from watcher
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

// handleBlockerUpdate adds or removes blockers
func (m *Model) handleBlockerUpdate(blocker types.Blocker) {
	blockerKey := string(blocker.Type) + "/" + blocker.Namespace + "/" + blocker.Name

	if blocker.Cleared {
		for i, b := range m.blockers {
			key := string(b.Type) + "/" + b.Namespace + "/" + b.Name
			if key == blockerKey {
				m.blockers = append(m.blockers[:i], m.blockers[i+1:]...)
				return
			}
		}
	} else {
		for i, b := range m.blockers {
			key := string(b.Type) + "/" + b.Namespace + "/" + b.Name
			if key == blockerKey {
				m.blockers[i] = blocker
				return
			}
		}
		m.blockers = append(m.blockers, blocker)
	}
}

// handleEvent adds event to display list
func (m *Model) handleEvent(e types.Event) {
	m.eventCount++

	m.events = append(m.events, e)
	if len(m.events) > maxEvents {
		m.events = m.events[len(m.events)-maxEvents:]
	}

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

// handleMouse handles mouse events for scrolling
func (m *Model) handleMouse(msg tea.MouseMsg) (Model, tea.Cmd) {
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
		return len(m.getDrainNodes())
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
