package tui

import (
	"fmt"
	"strings"

	"github.com/sabirmohamed/kupgrade/pkg/types"
)

// renderStatsScreen renders the statistics screen
func (m Model) renderStatsScreen() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")

	// Progress section
	b.WriteString(panelTitleStyle.Render("  PROGRESS"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  ├─ Nodes Complete:    %d / %d  (%d%%)\n",
		m.completedNodes(), m.totalNodes(), m.progressPercent()))
	b.WriteString(fmt.Sprintf("  ├─ Nodes In Progress: %d\n",
		len(m.nodesByStage[types.StageCordoned])+
			len(m.nodesByStage[types.StageDraining])+
			len(m.nodesByStage[types.StageUpgrading])))
	b.WriteString(fmt.Sprintf("  └─ Nodes Remaining:   %d\n",
		len(m.nodesByStage[types.StageReady])))

	b.WriteString("\n")

	// Stage breakdown
	b.WriteString(panelTitleStyle.Render("  BY STAGE"))
	b.WriteString("\n")
	for _, stage := range types.AllStages() {
		count := len(m.nodesByStage[stage])
		b.WriteString(fmt.Sprintf("  ├─ %-12s %d\n", stage, count))
	}

	b.WriteString("\n")
	b.WriteString(footerDescStyle.Render("  Timing & velocity metrics will be added in E7"))
	b.WriteString("\n\n")
	b.WriteString(m.renderFooter())

	return m.placeContent(b.String())
}
