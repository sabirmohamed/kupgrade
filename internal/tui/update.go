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

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case NodeUpdateMsg:
		m.handleNodeUpdate(msg.Node)
		return m, waitForNodeState(m.config.NodeStateCh)

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
	if matchKey(msg, keys.Quit) {
		if m.viewMode != ViewOverview {
			m.viewMode = ViewOverview
			return *m, nil
		}
		return *m, tea.Quit
	}

	if matchKey(msg, keys.Help) {
		if m.viewMode == ViewHelp {
			m.viewMode = ViewOverview
		} else {
			m.viewMode = ViewHelp
		}
		return *m, nil
	}

	if matchKey(msg, keys.Escape) {
		m.viewMode = ViewOverview
		return *m, nil
	}

	switch m.viewMode {
	case ViewOverview:
		return m.handleOverviewKey(msg)
	case ViewNodeDetail:
		return m.handleNodeDetailKey(msg)
	case ViewHelp:
		return m.handleHelpKey(msg)
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
			m.viewMode = ViewNodeDetail
		}
		return *m, nil
	}

	return *m, nil
}

func (m *Model) handleNodeDetailKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if matchKey(msg, keys.Escape) || matchKey(msg, keys.Enter) {
		m.viewMode = ViewOverview
		return *m, nil
	}
	return *m, nil
}

func (m *Model) handleHelpKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	m.viewMode = ViewOverview
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
