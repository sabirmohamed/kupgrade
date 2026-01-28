package tui

import tea "github.com/charmbracelet/bubbletea"

type keyMap struct {
	Quit   []string
	Left   []string
	Right  []string
	Up     []string
	Down   []string
	Enter  []string
	Escape []string
	Help   []string
	// Screen navigation (0-6)
	Screen0 []string
	Screen1 []string
	Screen2 []string
	Screen3 []string
	Screen4 []string
	Screen5 []string
	Screen6 []string
	// List navigation
	Top      []string
	Bottom   []string
	PageUp   []string
	PageDown []string
	// Event filtering
	EventUpgrade   []string
	EventWarnings  []string
	EventAll       []string
	EventAggregate []string
	EventExpand    []string
}

var keys = keyMap{
	Quit:    []string{"q", "ctrl+c"},
	Left:    []string{"left", "h"},
	Right:   []string{"right", "l"},
	Up:      []string{"up", "k"},
	Down:    []string{"down", "j"},
	Enter:   []string{"enter"},
	Escape:  []string{"esc"},
	Help:    []string{"?"},
	Screen0: []string{"0"},
	Screen1: []string{"1"},
	Screen2: []string{"2"},
	Screen3: []string{"3"},
	Screen4: []string{"4"},
	Screen5: []string{"5"},
	Screen6: []string{"6"},
	Top:      []string{"g"},
	Bottom:   []string{"G"},
	PageUp:   []string{"ctrl+u"},
	PageDown: []string{"ctrl+d"},
	// Event filtering
	EventUpgrade:   []string{"u"},
	EventWarnings:  []string{"w"},
	EventAll:       []string{"a"},
	EventAggregate: []string{"g"},
	EventExpand:    []string{"e"},
}

func matchKey(msg tea.KeyMsg, bindings []string) bool {
	for _, b := range bindings {
		if msg.String() == b {
			return true
		}
	}
	return false
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
