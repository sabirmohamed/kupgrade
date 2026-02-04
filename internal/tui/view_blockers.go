package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/sabirmohamed/kupgrade/pkg/types"
)

// renderBlockersScreen renders the blockers detail screen with two sections:
// ACTIVE BLOCKERS (red) — PDBs blocking a draining node right now
// PDB RISKS (yellow) — PDBs with no disruption budget but not on draining nodes
func (m Model) renderBlockersScreen() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")

	activeBlockers, riskBlockers := m.splitBlockersByTier()

	if len(activeBlockers) == 0 && len(riskBlockers) == 0 {
		b.WriteString(successStyle.Render("  No active blockers"))
		b.WriteString("\n")
		b.WriteString(footerDescStyle.Render("  PDB blockers appear when disruption budget is exhausted"))
	} else {
		listIdx := 0

		// Active blockers section
		if len(activeBlockers) > 0 {
			b.WriteString(errorStyle.Render(fmt.Sprintf("  %s ACTIVE BLOCKERS (%d)", errorIcon, len(activeBlockers))))
			b.WriteString("\n")

			for _, blocker := range activeBlockers {
				b.WriteString(m.renderBlockerLine(blocker, listIdx))
				listIdx++
			}
			b.WriteString("\n")
		}

		// Risk section
		if len(riskBlockers) > 0 {
			b.WriteString(warningStyle.Render(fmt.Sprintf("  %s PDB RISKS (%d)", warningIcon, len(riskBlockers))))
			b.WriteString("\n")

			for _, blocker := range riskBlockers {
				b.WriteString(m.renderBlockerLine(blocker, listIdx))
				listIdx++
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(m.renderFooter())

	return m.placeContent(b.String())
}

// renderBlockerLine renders a single blocker entry with cursor and styling based on tier.
func (m Model) renderBlockerLine(blocker types.Blocker, index int) string {
	var b strings.Builder

	cursor := "  "
	if index == m.listIndex {
		cursor = "► "
	}

	name := blocker.Name
	if blocker.Namespace != "" {
		name = blocker.Namespace + "/" + blocker.Name
	}

	// Show duration if StartTime is set
	durationStr := ""
	if !blocker.StartTime.IsZero() {
		duration := m.currentTime.Sub(blocker.StartTime)
		durationStr = fmt.Sprintf(" (%s)", formatDuration(duration))
	}

	// Show node name if available
	nodeStr := ""
	if blocker.NodeName != "" {
		nodeStr = fmt.Sprintf(" on %s", blocker.NodeName)
	}

	line1 := fmt.Sprintf("%s%s: %s%s%s", cursor, blocker.Type, name, nodeStr, durationStr)
	line2 := fmt.Sprintf("    └─ %s", blocker.Detail)

	isActive := blocker.Tier == types.BlockerTierActive

	if index == m.listIndex {
		b.WriteString(errorStyle.Render(line1))
		b.WriteString("\n")
		b.WriteString(warningStyle.Render(line2))
	} else if isActive {
		b.WriteString(errorStyle.Render(line1))
		b.WriteString("\n")
		b.WriteString(footerDescStyle.Render(line2))
	} else {
		b.WriteString(warningStyle.Render(line1))
		b.WriteString("\n")
		b.WriteString(footerDescStyle.Render(line2))
	}
	b.WriteString("\n")

	return b.String()
}

// splitBlockersByTier separates PDB blockers into active (Tier 2) and risk (Tier 1).
// Non-PDB blockers (e.g., PV, DaemonSet) are excluded since they don't have tiers.
func (m Model) splitBlockersByTier() (active, risk []types.Blocker) {
	for _, b := range m.blockers {
		if b.Type != types.BlockerPDB {
			continue
		}
		if b.Tier == types.BlockerTierActive {
			active = append(active, b)
		} else {
			risk = append(risk, b)
		}
	}
	return active, risk
}

// formatDuration formats a duration as a human-readable string (e.g., "2m 14s")
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm %ds", m, s)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", h, m)
}
