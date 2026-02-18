package tui

import (
	"context"
	"fmt"
	"os/exec"
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
			m.detailViewport.SetContent(colorizeDescribe(msg.Content))
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

	case NodeMetricsMsg:
		for name, metrics := range msg {
			if node, ok := m.nodes[name]; ok {
				node.CPUPercent = metrics.CPUPercent
				node.MemPercent = metrics.MemPercent
				m.nodes[name] = node
			}
		}
		return m, scheduleMetricsRefresh()

	case metricsRefreshMsg:
		return m, fetchNodeMetrics(m.config.Clientset)

	case cpVersionMsg:
		if msg.Version != "" {
			m.controlPlaneVersion = msg.Version
			if m.initialCPVersion == "" {
				m.initialCPVersion = msg.Version
			}
			if versionCore(m.controlPlaneVersion) != versionCore(m.initialCPVersion) {
				m.cpUpgraded = true
			}
		}
		return m, scheduleCPVersionCheck()

	case cpVersionCheckMsg:
		return m, fetchControlPlaneVersion(m.config.Clientset)

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

	// Screen navigation (0-4)
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
		if pod := m.podAtRow(podList, m.listIndex); pod != nil {
			podKey := pod.Namespace + "/" + pod.Name
			cmd := m.openDetail(DetailPod, podKey)
			return *m, cmd
		}
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
		pageSize := m.height - 12
		if pageSize < 5 {
			pageSize = 5
		}
		m.listIndex -= pageSize
		if m.listIndex < 0 {
			m.listIndex = 0
		}
		return *m, nil

	case tea.KeyPgDown, tea.KeyCtrlD:
		pageSize := m.height - 12
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

// displayPodCount returns count of table rows (including section separators)
// after both stage filter and fuzzy search
func (m *Model) displayPodCount() int {
	podList := m.getDisplayPodList()
	return m.podRowCount(podList)
}

func (m *Model) handleEventsKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	events := m.sortedEvents()
	aggregated := aggregateEvents(events)

	// Enter/d opens detail overlay for the selected row
	if key.Matches(msg, m.keys.Enter) || key.Matches(msg, m.keys.Describe) {
		groupIdx, isSubRow := m.eventGroupForVisualRow(m.listIndex, aggregated, events)
		if groupIdx < len(aggregated) {
			var event types.Event
			if isSubRow {
				// Find the specific sub-event for this visual row
				event = m.eventAtVisualRow(m.listIndex, aggregated, events)
			} else {
				event = aggregated[groupIdx].SampleEvent
			}
			// K8s events: use namespace/eventName for kubectl describe
			// Internal events: use timestamp:message for in-memory lookup
			detailKey := eventKey(event)
			if event.EventName != "" {
				detailKey = event.Namespace + "/" + event.EventName
			}
			cmd := m.openDetail(DetailEvent, detailKey)
			return *m, cmd
		}
		return *m, nil
	}

	// Expand/collapse aggregated group
	if key.Matches(msg, m.keys.EventExpand) {
		groupIdx, _ := m.eventGroupForVisualRow(m.listIndex, aggregated, events)
		if groupIdx < len(aggregated) {
			selectedReason := aggregated[groupIdx].Reason
			if m.expandedGroup == selectedReason {
				// Collapsing: snap cursor to the group header
				m.expandedGroup = ""
				m.listIndex = eventGroupHeaderRow(groupIdx, aggregated, events, "")
			} else {
				// Expanding: cursor stays on group header
				headerRow := eventGroupHeaderRow(groupIdx, aggregated, events, m.expandedGroup)
				m.expandedGroup = selectedReason
				// Recompute header position after expansion changes row layout
				m.listIndex = eventGroupHeaderRow(groupIdx, aggregated, events, m.expandedGroup)
				_ = headerRow // suppress unused
			}
		}
		return *m, nil
	}

	itemCount := m.eventVisualRowCount()
	return m.handleListNavigation(msg, itemCount)
}

// handleListNavigation provides common up/down/g/G/pgup/pgdown navigation for non-table list screens
func (m *Model) handleListNavigation(msg tea.KeyMsg, itemCount int) (Model, tea.Cmd) {
	pageSize := m.height - 12
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
		if node.Stage == types.StageComplete {
			// EKS/GKE: keep deleted-complete nodes for progress tracking.
			// totalNodes() counts them (non-surge, in map), completedNodes()
			// counts them (COMPLETE stage), so progress goes 33% → 67% → 100%.
			m.nodes[node.Name] = node
		} else {
			delete(m.nodes, node.Name)
			// Clamp listIndex to avoid out-of-bounds after deletion
			if count := len(m.nodes); m.listIndex >= count && count > 0 {
				m.listIndex = count - 1
			}
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
	if blocker.Cleared {
		m.handleBlockerClear(blocker)
		return
	}
	m.updateOrAddBlocker(blocker)
}

// handleBlockerClear removes blockers based on the clearing signal type.
func (m *Model) handleBlockerClear(blocker types.Blocker) {
	// Clear all blockers of this type (full replacement signal)
	if blocker.Name == "" && blocker.NodeName == "" {
		m.clearBlockersOfType(blocker.Type)
		return
	}

	// Clear all blockers for this PDB (PDB was deleted)
	if blocker.NodeName == "" {
		m.clearBlockersForPDB(blocker)
		return
	}

	// Clear specific node's blocker
	m.clearSpecificBlocker(blockerKey(blocker))
}

// clearBlockersOfType removes all blockers matching the given type.
func (m *Model) clearBlockersOfType(blockerType types.BlockerType) {
	filtered := m.blockers[:0]
	for _, b := range m.blockers {
		if b.Type != blockerType {
			filtered = append(filtered, b)
		}
	}
	m.blockers = filtered
}

// clearBlockersForPDB removes all blockers for a specific PDB (all nodes).
func (m *Model) clearBlockersForPDB(blocker types.Blocker) {
	baseKey := string(blocker.Type) + "/" + blocker.Namespace + "/" + blocker.Name + "/"
	filtered := m.blockers[:0]
	for _, b := range m.blockers {
		key := blockerKey(b)
		if len(key) < len(baseKey) || key[:len(baseKey)] != baseKey {
			filtered = append(filtered, b)
		}
	}
	m.blockers = filtered
}

// clearSpecificBlocker removes the blocker with the exact key, if found.
func (m *Model) clearSpecificBlocker(targetKey string) {
	for i, b := range m.blockers {
		if blockerKey(b) == targetKey {
			m.blockers = append(m.blockers[:i], m.blockers[i+1:]...)
			return
		}
	}
}

// updateOrAddBlocker updates an existing blocker or appends a new one.
func (m *Model) updateOrAddBlocker(blocker types.Blocker) {
	targetKey := blockerKey(blocker)
	for i, b := range m.blockers {
		if blockerKey(b) == targetKey {
			m.blockers[i] = m.mergeBlockerStartTime(blocker, b)
			return
		}
	}
	m.blockers = append(m.blockers, blocker)
}

// mergeBlockerStartTime preserves StartTime from existing blocker if new one has zero time.
func (m *Model) mergeBlockerStartTime(newBlocker, existingBlocker types.Blocker) types.Blocker {
	if newBlocker.StartTime.IsZero() && !existingBlocker.StartTime.IsZero() {
		newBlocker.StartTime = existingBlocker.StartTime
	}
	return newBlocker
}

// blockerKey builds a unique key for a blocker including NodeName for per-node tracking.
func blockerKey(b types.Blocker) string {
	return string(b.Type) + "/" + b.Namespace + "/" + b.Name + "/" + b.NodeName
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
	case ScreenNodes, ScreenDrains, ScreenPods, ScreenEvents:
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
		case DetailEvent:
			output, err = describeEventExec(key)
		default:
			return DescribeMsg{Err: fmt.Errorf("unsupported detail type")}
		}

		if err != nil {
			return DescribeMsg{Err: err}
		}
		return DescribeMsg{Content: output}
	}
}

// describeEventExec calls kubectl describe event for the given namespace/name key.
// Uses os/exec because the kubectl library has no EventDescriber.
func describeEventExec(key string) (string, error) {
	parts := strings.SplitN(key, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid event key: %s", key)
	}
	namespace, name := parts[0], parts[1]

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "kubectl", "describe", "event", name, "-n", namespace).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("kubectl describe event: %w\n%s", err, string(out))
	}
	return string(out), nil
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
	case ScreenEvents:
		return m.eventVisualRowCount()
	default:
		return 0
	}
}
