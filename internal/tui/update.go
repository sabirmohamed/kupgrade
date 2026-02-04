package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sabirmohamed/kupgrade/pkg/types"
	"k8s.io/kubectl/pkg/describe"
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
		if m.overlay == OverlayDetail {
			m.resizeDetailViewport()
		}
		return m, nil

	case DescribeMsg:
		if msg.Err != nil {
			m.detailViewport.SetContent("Error: " + msg.Err.Error())
		} else {
			m.detailViewport.SetContent(msg.Content)
		}
		m.detailViewport.GotoTop()
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

	// Escape: clear pod search filter if set, otherwise return to Overview
	if key.Matches(msg, m.keys.Escape) {
		if m.screen == ScreenPods && m.podSearchInput.Value() != "" {
			m.clearPodSearch()
			m.listIndex = 0
			return *m, nil
		}
		m.screen = ScreenOverview
		m.clearPodSearch()
		return *m, nil
	}

	// Screen navigation (0-6)
	if screen := screenFromKey(msg); screen >= 0 {
		m.screen = screen
		m.listIndex = 0 // Reset list position on screen change
		m.clearPodSearch()
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
	}

	return *m, nil
}

func (m *Model) handleOverlayKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Detail overlay: Esc/q closes, j/k/arrows scroll viewport
	if m.overlay == OverlayDetail {
		if key.Matches(msg, m.keys.Escape) || key.Matches(msg, m.keys.Quit) {
			m.overlay = OverlayNone
			return *m, nil
		}
		if key.Matches(msg, m.keys.Down) {
			m.detailViewport.ScrollDown(1)
			return *m, nil
		}
		if key.Matches(msg, m.keys.Up) {
			m.detailViewport.ScrollUp(1)
			return *m, nil
		}
		if key.Matches(msg, m.keys.PageDown) {
			m.detailViewport.HalfPageDown()
			return *m, nil
		}
		if key.Matches(msg, m.keys.PageUp) {
			m.detailViewport.HalfPageUp()
			return *m, nil
		}
		if key.Matches(msg, m.keys.Top) {
			m.detailViewport.GotoTop()
			return *m, nil
		}
		if key.Matches(msg, m.keys.Bottom) {
			m.detailViewport.GotoBottom()
			return *m, nil
		}
		return *m, nil
	}

	// Help overlay: any key closes
	if key.Matches(msg, m.keys.Escape) || key.Matches(msg, m.keys.Enter) || key.Matches(msg, m.keys.Help) {
		m.overlay = OverlayNone
		return *m, nil
	}
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

	if key.Matches(msg, m.keys.Enter) || key.Matches(msg, m.keys.Describe) {
		if m.listIndex < len(allNodes) {
			nodeName := allNodes[m.listIndex]
			if _, ok := m.nodes[nodeName]; ok {
				cmd := m.openDetail(DetailNode, nodeName)
				return *m, cmd
			}
		}
		return *m, nil
	}

	return *m, nil
}

func (m *Model) handleNodesKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Enter) || key.Matches(msg, m.keys.Describe) {
		nodes := m.sortedNodeNames()
		if m.listIndex < len(nodes) {
			cmd := m.openDetail(DetailNode, nodes[m.listIndex])
			return *m, cmd
		}
		return *m, nil
	}
	return m.handleListNavigation(msg, len(m.nodes))
}

func (m *Model) handleDrainsKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Enter) || key.Matches(msg, m.keys.Describe) {
		drainNodes := m.getDrainNodes()
		if m.listIndex < len(drainNodes) {
			cmd := m.openDetail(DetailNode, drainNodes[m.listIndex])
			return *m, cmd
		}
		return *m, nil
	}
	return m.handleListNavigation(msg, len(m.getDrainNodes()))
}

func (m *Model) handlePodsKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// When search input is active, forward keys to textinput
	if m.podSearchActive {
		return m.handlePodSearchKey(msg)
	}

	if key.Matches(msg, m.keys.Enter) || key.Matches(msg, m.keys.Describe) {
		podList := m.getDisplayPodList()
		if m.listIndex < len(podList) {
			pod := podList[m.listIndex]
			podKey := pod.Namespace + "/" + pod.Name
			cmd := m.openDetail(DetailPod, podKey)
			return *m, cmd
		}
		return *m, nil
	}
	if key.Matches(msg, m.keys.PodFilter) {
		m.cyclePodFilter()
		m.listIndex = 0
		return *m, nil
	}
	if key.Matches(msg, m.keys.PodSearch) {
		m.podSearchActive = true
		m.podSearchInput.Focus()
		m.listIndex = 0
		return *m, textinput.Blink
	}
	return m.handleListNavigation(msg, m.displayPodCount())
}

// handlePodSearchKey handles key events when the pod search input is active.
// Only arrow keys and ctrl-sequences navigate the list — all printable keys
// (including vim j/k/g/G) go to the textinput so users can type freely.
func (m *Model) handlePodSearchKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	itemCount := m.displayPodCount()

	switch msg.Type {
	case tea.KeyEscape:
		m.podSearchActive = false
		m.podSearchInput.SetValue("")
		m.podSearchInput.Blur()
		m.listIndex = 0
		return *m, nil

	case tea.KeyEnter:
		// Commit filter, return to normal navigation
		m.podSearchActive = false
		m.podSearchInput.Blur()
		m.listIndex = 0
		return *m, nil

	case tea.KeyUp:
		if m.listIndex > 0 {
			m.listIndex--
		}
		return *m, nil

	case tea.KeyDown:
		if m.listIndex < itemCount-1 {
			m.listIndex++
		}
		return *m, nil

	case tea.KeyPgUp, tea.KeyCtrlU:
		pageSize := m.height - 10
		if pageSize < 5 {
			pageSize = 5
		}
		m.listIndex -= pageSize
		if m.listIndex < 0 {
			m.listIndex = 0
		}
		return *m, nil

	case tea.KeyPgDown, tea.KeyCtrlD:
		pageSize := m.height - 10
		if pageSize < 5 {
			pageSize = 5
		}
		m.listIndex += pageSize
		if m.listIndex >= itemCount {
			m.listIndex = itemCount - 1
		}
		if m.listIndex < 0 {
			m.listIndex = 0
		}
		return *m, nil
	}

	// All other keys (printable characters) → textinput
	prevValue := m.podSearchInput.Value()
	var cmd tea.Cmd
	m.podSearchInput, cmd = m.podSearchInput.Update(msg)
	if m.podSearchInput.Value() != prevValue {
		m.listIndex = 0
	}
	return *m, cmd
}

// displayPodCount returns count of pods after both stage filter and fuzzy search
func (m *Model) displayPodCount() int {
	return len(m.getDisplayPodList())
}

func (m *Model) handleBlockersKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Enter/d on a blocker opens the associated node detail
	if key.Matches(msg, m.keys.Enter) || key.Matches(msg, m.keys.Describe) {
		if m.listIndex < len(m.blockers) {
			blocker := m.blockers[m.listIndex]
			if blocker.NodeName != "" {
				cmd := m.openDetail(DetailNode, blocker.NodeName)
				return *m, cmd
			}
		}
		return *m, nil
	}
	return m.handleListNavigation(msg, len(m.blockers))
}

func (m *Model) handleEventsKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Enter/d opens event detail overlay
	if key.Matches(msg, m.keys.Enter) || key.Matches(msg, m.keys.Describe) {
		events := m.filteredEvents()
		if m.listIndex < len(events) {
			cmd := m.openDetail(DetailEvent, eventKey(events[m.listIndex]))
			return *m, cmd
		}
		return *m, nil
	}

	// Event filter toggles (close detail overlay since filtered list changes)
	if key.Matches(msg, m.keys.EventUpgrade) {
		m.eventFilter = EventFilterUpgrade
		m.listIndex = 0
		m.overlay = OverlayNone
		return *m, nil
	}
	if key.Matches(msg, m.keys.EventWarnings) {
		m.eventFilter = EventFilterWarnings
		m.listIndex = 0
		m.overlay = OverlayNone
		return *m, nil
	}
	if key.Matches(msg, m.keys.EventAll) {
		m.eventFilter = EventFilterAll
		m.listIndex = 0
		m.overlay = OverlayNone
		return *m, nil
	}
	// Aggregation toggle (g key — intentionally shadows Top binding on events screen)
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
		// Clamp listIndex to avoid out-of-bounds after deletion
		if count := len(m.nodes); m.listIndex >= count && count > 0 {
			m.listIndex = count - 1
		}
	} else {
		m.nodes[node.Name] = node
	}
	m.rebuildNodesByStage()
	m.recomputeVersionRange()
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

// handleBlockerUpdate adds or removes blockers.
// Blocker key includes NodeName for per-node tracking (same PDB can block multiple nodes).
// Clearing behavior:
//   - Cleared + empty Name + empty NodeName: clear all blockers of that Type (full refresh)
//   - Cleared + Name + empty NodeName: clear all blockers for that PDB (PDB deleted)
//   - Cleared + Name + NodeName: clear specific node's blocker
func (m *Model) handleBlockerUpdate(blocker types.Blocker) {
	// Build key function - includes NodeName for per-node tracking
	makeKey := func(b types.Blocker) string {
		return string(b.Type) + "/" + b.Namespace + "/" + b.Name + "/" + b.NodeName
	}

	blockerKey := makeKey(blocker)

	if blocker.Cleared {
		if blocker.Name == "" && blocker.NodeName == "" {
			// Clear all blockers of this type (full replacement signal)
			filtered := m.blockers[:0]
			for _, b := range m.blockers {
				if b.Type != blocker.Type {
					filtered = append(filtered, b)
				}
			}
			m.blockers = filtered
		} else if blocker.NodeName == "" {
			// Clear all blockers for this PDB (PDB was deleted)
			baseKey := string(blocker.Type) + "/" + blocker.Namespace + "/" + blocker.Name + "/"
			filtered := m.blockers[:0]
			for _, b := range m.blockers {
				key := makeKey(b)
				if len(key) < len(baseKey) || key[:len(baseKey)] != baseKey {
					filtered = append(filtered, b)
				}
			}
			m.blockers = filtered
		} else {
			// Clear specific node's blocker
			for i, b := range m.blockers {
				if makeKey(b) == blockerKey {
					m.blockers = append(m.blockers[:i], m.blockers[i+1:]...)
					return
				}
			}
		}
		return
	}

	// Update existing or add new
	for i, b := range m.blockers {
		if makeKey(b) == blockerKey {
			// Update existing - preserve StartTime if already set
			if !blocker.StartTime.IsZero() {
				m.blockers[i] = blocker
			} else if !b.StartTime.IsZero() {
				blocker.StartTime = b.StartTime
				m.blockers[i] = blocker
			} else {
				m.blockers[i] = blocker
			}
			return
		}
	}
	m.blockers = append(m.blockers, blocker)
}

// handleEvent adds event to display list
func (m *Model) handleEvent(e types.Event) {
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

// fetchDescribe returns a tea.Cmd that asynchronously calls kubectl describe
func (m Model) fetchDescribe() tea.Cmd {
	dt := m.detailType
	key := m.detailKey
	clientset := m.config.Clientset

	return func() tea.Msg {
		if clientset == nil {
			return DescribeMsg{Err: fmt.Errorf("no kubernetes client")}
		}

		settings := describe.DescriberSettings{ShowEvents: true}
		var output string
		var err error

		switch dt {
		case DetailNode:
			d := &describe.NodeDescriber{Interface: clientset}
			output, err = d.Describe("", key, settings)
		case DetailPod:
			parts := strings.SplitN(key, "/", 2)
			if len(parts) != 2 {
				return DescribeMsg{Err: fmt.Errorf("invalid pod key: %s", key)}
			}
			d := &describe.PodDescriber{Interface: clientset}
			output, err = d.Describe(parts[0], parts[1], settings)
		default:
			return DescribeMsg{Err: fmt.Errorf("unsupported detail type")}
		}

		if err != nil {
			return DescribeMsg{Err: err}
		}
		return DescribeMsg{Content: output}
	}
}

// currentListCount returns the item count for the current list screen
func (m *Model) currentListCount() int {
	switch m.screen {
	case ScreenNodes:
		return len(m.nodes)
	case ScreenDrains:
		return len(m.getDrainNodes())
	case ScreenPods:
		return m.displayPodCount()
	case ScreenBlockers:
		return len(m.blockers)
	case ScreenEvents:
		return len(m.events)
	default:
		return 0
	}
}
