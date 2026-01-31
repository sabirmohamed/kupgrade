package tui

import (
	"fmt"
	"strings"
)

// renderBlockersScreen renders the blockers detail screen
func (m Model) renderBlockersScreen() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")

	if len(m.blockers) == 0 {
		b.WriteString(successStyle.Render("  No blockers detected"))
	} else {
		for i, blocker := range m.blockers {
			cursor := "  "
			if i == m.listIndex {
				cursor = "► "
			}

			name := blocker.Name
			if blocker.Namespace != "" {
				name = blocker.Namespace + "/" + blocker.Name
			}

			line1 := fmt.Sprintf("%s%s: %s", cursor, blocker.Type, name)
			line2 := fmt.Sprintf("    └─ %s", blocker.Detail)

			if i == m.listIndex {
				b.WriteString(errorStyle.Render(line1))
				b.WriteString("\n")
				b.WriteString(warningStyle.Render(line2))
			} else {
				b.WriteString(warningStyle.Render(line1))
				b.WriteString("\n")
				b.WriteString(footerDescStyle.Render(line2))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(m.renderFooter())

	return m.placeContent(b.String())
}
