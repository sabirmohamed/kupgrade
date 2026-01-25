package tui

import "github.com/charmbracelet/bubbletea"

type keyMap struct {
	Quit   []string
	Left   []string
	Right  []string
	Up     []string
	Down   []string
	Enter  []string
	Escape []string
	Help   []string
}

var keys = keyMap{
	Quit:   []string{"q", "ctrl+c"},
	Left:   []string{"left", "h"},
	Right:  []string{"right", "l"},
	Up:     []string{"up", "k"},
	Down:   []string{"down", "j"},
	Enter:  []string{"enter"},
	Escape: []string{"esc"},
	Help:   []string{"?"},
}

func matchKey(msg tea.KeyMsg, bindings []string) bool {
	for _, b := range bindings {
		if msg.String() == b {
			return true
		}
	}
	return false
}
