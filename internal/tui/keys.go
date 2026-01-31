package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type keyMap struct {
	Quit   key.Binding
	Left   key.Binding
	Right  key.Binding
	Up     key.Binding
	Down   key.Binding
	Enter  key.Binding
	Escape key.Binding
	Help   key.Binding
	// List navigation
	Top      key.Binding
	Bottom   key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	// Detail
	Describe key.Binding
	Tab      key.Binding
	// Event filtering
	EventUpgrade   key.Binding
	EventWarnings  key.Binding
	EventAll       key.Binding
	EventAggregate key.Binding
	EventExpand    key.Binding
}

// ShortHelp returns key bindings for the short help view (footer).
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Help, k.Quit}
}

// FullHelp returns key bindings for the full help view (overlay).
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},
		{k.Enter, k.PageUp, k.PageDown},
		{k.Top, k.Bottom},
		{k.Help, k.Escape, k.Quit},
	}
}

var defaultKeys = keyMap{
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Left: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "prev stage"),
	),
	Right: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "next stage"),
	),
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("⏎", "describe"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	Top: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "top"),
	),
	Bottom: key.NewBinding(
		key.WithKeys("G"),
		key.WithHelp("G", "bottom"),
	),
	PageUp: key.NewBinding(
		key.WithKeys("ctrl+u"),
		key.WithHelp("ctrl+u", "page up"),
	),
	PageDown: key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("ctrl+d", "page down"),
	),
	Describe: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "describe"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "focus"),
	),
	EventUpgrade: key.NewBinding(
		key.WithKeys("u"),
		key.WithHelp("u", "upgrade"),
	),
	EventWarnings: key.NewBinding(
		key.WithKeys("w"),
		key.WithHelp("w", "warnings"),
	),
	EventAll: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "all"),
	),
	EventAggregate: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "group"),
	),
	EventExpand: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "expand"),
	),
}

// screenFromKey returns the screen number if a screen key was pressed, -1 otherwise
func screenFromKey(msg tea.KeyMsg) Screen {
	switch msg.String() {
	case "0":
		return ScreenOverview
	case "1":
		return ScreenNodes
	case "2":
		return ScreenDrains
	case "3":
		return ScreenPods
	case "4":
		return ScreenBlockers
	case "5":
		return ScreenEvents
	case "6":
		return ScreenStats
	default:
		return Screen(-1)
	}
}
