package tui

import "strings"

// renderHelpOverlay renders the keyboard shortcuts overlay
func (m Model) renderHelpOverlay() string {
	title := overlayTitleStyle.Render("Keyboard Shortcuts")

	sections := []string{
		panelTitleStyle.Render("SCREENS"),
		"  " + footerKeyStyle.Render("0") + "  Dashboard",
		"  " + footerKeyStyle.Render("1") + "  Nodes",
		"  " + footerKeyStyle.Render("2") + "  Drains + Blockers",
		"  " + footerKeyStyle.Render("3") + "  Pods",
		"  " + footerKeyStyle.Render("4") + "  Events",
		"",
		panelTitleStyle.Render("NAVIGATION"),
		"  " + footerKeyStyle.Render("↑/k  ↓/j") + "  Move up/down",
		"  " + footerKeyStyle.Render("g  G") + "      Top / bottom",
		"  " + footerKeyStyle.Render("^u  ^d") + "    Page up / down",
		"  " + footerKeyStyle.Render("d  Enter") + "  Describe resource",
		"",
		panelTitleStyle.Render("FILTERS"),
		"  " + footerKeyStyle.Render("Tab") + "  Cycle filter (Pods: stage, Events: type)",
		"  " + footerKeyStyle.Render("g") + "    Toggle grouped view (Events)",
		"  " + footerKeyStyle.Render("e") + "    Expand group (Events)",
		"  " + footerKeyStyle.Render("/") + "    Fuzzy search (Pods)",
		"",
		panelTitleStyle.Render("GENERAL"),
		"  " + footerKeyStyle.Render("Esc") + "  Back to Dashboard",
		"  " + footerKeyStyle.Render("?") + "    Toggle this help",
		"  " + footerKeyStyle.Render("q") + "    Quit (from Dashboard) / Back (from sub-screen)",
	}

	content := title + "\n\n" + strings.Join(sections, "\n")
	return overlayStyle.Render(content)
}
