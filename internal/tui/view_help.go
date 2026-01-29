package tui

import (
	"fmt"
	"strings"
)

// renderHelpOverlay renders the keyboard shortcuts overlay using bubbles/help
func (m Model) renderHelpOverlay() string {
	title := overlayTitleStyle.Render("Keyboard Shortcuts")
	helpContent := m.help.FullHelpView(m.keys.FullHelp())

	content := title + "\n\n" + helpContent
	return overlayStyle.Render(content)
}

// renderNodeDetailOverlay renders the node detail overlay
func (m Model) renderNodeDetailOverlay() string {
	node, ok := m.selectedNodeState()
	if !ok {
		return overlayStyle.Render("No node selected")
	}

	title := overlayTitleStyle.Render("Node: " + node.Name)

	lines := []string{
		title,
		"",
		fmt.Sprintf("Stage:       %s", stageStyle(string(node.Stage)).Render(string(node.Stage))),
		fmt.Sprintf("Version:     %s", node.Version),
		fmt.Sprintf("Ready:       %v", node.Ready),
		fmt.Sprintf("Schedulable: %v", node.Schedulable),
		fmt.Sprintf("Pod Count:   %d", node.PodCount),
	}

	if node.Blocked {
		lines = append(lines, "")
		lines = append(lines, errorStyle.Render("⚠ BLOCKED: "+node.BlockerReason))
	}

	lines = append(lines, "")
	lines = append(lines, footerDescStyle.Render("Press ESC or Enter to close"))

	content := strings.Join(lines, "\n")
	return overlayStyle.Render(content)
}
