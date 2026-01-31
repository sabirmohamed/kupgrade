package tui

// renderHelpOverlay renders the keyboard shortcuts overlay using bubbles/help
func (m Model) renderHelpOverlay() string {
	title := overlayTitleStyle.Render("Keyboard Shortcuts")
	helpContent := m.help.FullHelpView(m.keys.FullHelp())

	content := title + "\n\n" + helpContent
	return overlayStyle.Render(content)
}
