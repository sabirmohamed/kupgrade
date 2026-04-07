package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/sabirmohamed/kupgrade/pkg/types"
)

// renderOverview renders the dashboard screen matching the final mockup.
// Layout: tab bar → outer panel (header, platform, dialog, table, cards) → status bar → key hints.
func (m Model) renderOverview() string {
	w := m.mainWidth()
	counts := m.stageCounts()

	// Tab bar (outside the panel)
	tabBar := m.renderTabBar(counts)

	// Detect upgrade state
	current := m.currentVersion()
	target := m.targetVersion()
	versionMismatch := current != "" && target != "" && versionCore(current) != versionCore(target)
	upgradeActive := counts["CORDONED"]+counts["DRAINING"]+counts["REIMAGING"] > 0 || versionMismatch || m.isCPAhead()
	upgradeComplete := m.progressPercent() == 100 && m.totalNodes() > 0 && counts["COMPLETE"] > 0

	panelWidth := w - 6 // panel .Width(w-2) content; inner = (w-2) - padding(4) = w-6

	// === Build sections above the table ===
	var aboveParts []string
	aboveParts = append(aboveParts, m.renderDashboardHeader(panelWidth))

	if m.platform != "" {
		poolCount := len(m.nodesByPool)
		platformLine := lipgloss.NewStyle().Foreground(colorTextMuted).
			Render(fmt.Sprintf("%s · %d pools", m.platform, poolCount))
		aboveParts = append(aboveParts, platformLine)
	}

	aboveParts = append(aboveParts, m.renderPillDialog(counts, panelWidth, upgradeActive, upgradeComplete))

	if !upgradeActive && !upgradeComplete {
		aboveParts = append(aboveParts, m.renderPreFlightSection(panelWidth))
	}
	if upgradeComplete {
		aboveParts = append(aboveParts, m.renderCompleteBanner())
	}

	if upgradeActive {
		if drains := m.renderActiveDrains(); drains != "" {
			aboveParts = append(aboveParts, drains)
		}
	}

	// === Build sections below the table ===
	var belowParts []string
	if upgradeActive || upgradeComplete {
		belowParts = append(belowParts, m.renderInfoCards(panelWidth))
	}

	// === Calculate available height for the table ===
	aboveHeight := lipgloss.Height(lipgloss.JoinVertical(lipgloss.Left, aboveParts...))
	belowHeight := 0
	if len(belowParts) > 0 {
		belowHeight = lipgloss.Height(lipgloss.JoinVertical(lipgloss.Left, belowParts...))
	}

	// Fixed chrome lines outside the panel body:
	//   tabBar:         1
	//   panel border:   2 (top + bottom)
	//   panel padding:  2 (Padding(1,2) → 1 top + 1 bottom)
	//   status bar:     1
	//   key hints:      1
	//   Total:          7
	const fixedChrome = 7
	availableForTable := m.height - fixedChrome - aboveHeight - belowHeight

	// === Assemble panel body ===
	var bodyParts []string
	bodyParts = append(bodyParts, aboveParts...)

	if m.totalNodes() > 0 {
		bodyParts = append(bodyParts, m.renderDashboardNodeTable(panelWidth, availableForTable))
	}

	bodyParts = append(bodyParts, belowParts...)

	panelBody := lipgloss.JoinVertical(lipgloss.Left, bodyParts...)

	// Wrap body in bordered outer panel
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorSelected).
		Padding(1, 2).
		Width(w - 2).
		Render(panelBody)

	// Status bar + key hints (outside panel)
	statusBar := m.renderStatusBar(w)
	keyHints := m.renderKeyHints(w)
	footer := lipgloss.JoinVertical(lipgloss.Left, statusBar, keyHints)

	// Compose: tab bar + panel + spacer + footer (footer pinned to bottom)
	mainContent := lipgloss.JoinVertical(lipgloss.Left, tabBar, panel)
	mainHeight := lipgloss.Height(mainContent)
	footerHeight := lipgloss.Height(footer)
	gap := m.height - mainHeight - footerHeight
	if gap < 0 {
		gap = 0
	}
	spacer := strings.Repeat("\n", gap)
	content := mainContent + spacer + footer

	if w > 0 && m.height > 0 {
		return fillLinesBg(content, m.width, colorBg)
	}
	return content
}

// renderDashboardHeader renders the main single-line header.
// Format: ★ kupgrade  cluster  │  CP v1.33.6 ✓  Nodes v1.32.9 → v1.33.6  ██░░░░  40%    ▸ 5m 12s  3/5 nodes
func (m Model) renderDashboardHeader(panelWidth int) string {
	title := headerStyle.Render("★ kupgrade")
	context := contextStyle.Render(m.contextName())
	sep := lipgloss.NewStyle().Foreground(colorTextDim).Render(" │ ")

	current := m.currentVersion()
	target := m.targetVersion()
	nodeUpgradeDetected := current != "" && target != "" && versionCore(current) != versionCore(target)

	return m.renderSingleLineHeader(panelWidth, title, context, sep, nodeUpgradeDetected)
}

// shouldShowCPLine returns true when the control plane version line should be displayed.
// Always shown when a CP version is available so users see the CP version from the start
// and the transition when CP upgrades ahead of nodes is natural rather than jarring.
func (m Model) shouldShowCPLine() bool {
	return m.controlPlaneVersion != ""
}

// isCPAhead returns true when the control plane version is ahead of the lowest node version.
func (m Model) isCPAhead() bool {
	return m.controlPlaneVersion != "" && m.lowestVersion != "" &&
		versionCore(m.controlPlaneVersion) != versionCore(m.lowestVersion)
}

// renderCPVersionDisplay returns the combined "CP v1.33 ✓  Nodes v1.32 → v1.33" string,
// or just the node version display when no CP version is available.
func (m Model) renderCPVersionDisplay(upgradeDetected bool) string {
	if !m.shouldShowCPLine() {
		return m.renderVersionDisplay()
	}
	cpVersion := m.controlPlaneVersion
	var cpDisplay string
	if m.cpUpgraded || (upgradeDetected && m.highestVersion != "" && versionCore(cpVersion) == versionCore(m.highestVersion)) {
		cpDisplay = successStyle.Render("CP " + cpVersion + " ✓")
	} else {
		cpDisplay = versionStyle.Render("CP " + cpVersion)
	}
	nodeLabel := lipgloss.NewStyle().Foreground(colorTextMuted).Render("Nodes")
	return cpDisplay + "  " + nodeLabel + " " + m.renderVersionDisplay()
}

// renderSingleLineHeader renders the single-line header with inline CP + node versions.
func (m Model) renderSingleLineHeader(panelWidth int, title, context, sep string, upgradeDetected bool) string {
	versionDisplay := m.renderCPVersionDisplay(upgradeDetected)

	var left string
	if upgradeDetected {
		percent := m.progressPercent()
		filled := (percent * headerProgressBarWidth) / 100
		bar := progressBar(headerProgressBarWidth, filled)
		percentStr := lipgloss.NewStyle().Foreground(colorSuccess).Bold(true).Render(fmt.Sprintf("%3d%%", percent))
		left = title + "  " + context + sep + versionDisplay + "  " + bar + " " + percentStr
	} else {
		left = title + "  " + context + sep + versionDisplay
	}

	// Right side: elapsed + node count
	var rightParts []string
	if upgradeDetected {
		if elapsed := m.elapsedDisplay(); elapsed != "" {
			rightParts = append(rightParts, "▸ "+elapsed)
		}
		rightParts = append(rightParts, fmt.Sprintf("%d/%d nodes", m.completedNodes(), m.totalNodes()))
	} else {
		rightParts = append(rightParts, fmt.Sprintf("%d nodes", m.totalNodes()))
	}
	right := lipgloss.NewStyle().Foreground(colorTextMuted).Render(strings.Join(rightParts, "  "))

	spacing := panelWidth - lipgloss.Width(left) - lipgloss.Width(right)
	if spacing < 2 {
		spacing = 2
	}

	return left + strings.Repeat(" ", spacing) + right
}

// renderPillDialog renders the centered stage pill dialog (hero element).
// Purple-bordered box with stage pills, progress bar, and info line.
func (m Model) renderPillDialog(counts map[string]int, panelWidth int, upgradeActive, upgradeComplete bool) string {
	// Build stage pills row
	stages := types.AllStages()
	var pills []string
	for _, stage := range stages {
		count := counts[string(stage)]
		// Hide QUARANTINED pill when count is zero — only relevant on AKS with undrainableNodeBehavior=Cordon
		if stage == types.StageQuarantined && count == 0 {
			continue
		}
		pills = append(pills, renderStagePill(string(stage), count))
	}
	pillsRow := strings.Join(pills, "  ")

	// Dialog inner width (minus border + padding)
	innerWidth := dialogWidth - 6 // 2 border + 4 padding (2 each side)

	// Center pills
	centeredPills := lipgloss.PlaceHorizontal(innerWidth, lipgloss.Center, pillsRow)

	var dialogContent string
	if upgradeActive || upgradeComplete {
		// Progress bar
		percent := m.progressPercent()
		bar := progressBarFromPercent(percent, dialogProgressBarWidth)
		percentStr := lipgloss.NewStyle().Foreground(colorSuccess).Bold(true).Render(fmt.Sprintf(" %d%%", percent))
		centeredBar := lipgloss.PlaceHorizontal(innerWidth, lipgloss.Center, bar+percentStr)

		// Info line: elapsed + estimate (active) or completed summary (complete)
		var infoStr string
		if upgradeComplete {
			elapsed := m.elapsedDisplay()
			if elapsed == "" {
				elapsed = "—"
			}
			infoStr = lipgloss.NewStyle().Foreground(colorTextMuted).Render("completed in ") +
				lipgloss.NewStyle().Foreground(colorSuccess).Render(elapsed)
		} else if elapsed := m.elapsedDisplay(); elapsed != "" {
			infoStr = lipgloss.NewStyle().Foreground(colorTextMuted).Render("elapsed ") +
				lipgloss.NewStyle().Foreground(colorSuccess).Render(elapsed)
			if remaining := m.estimateRemaining(); remaining != "" {
				infoStr += lipgloss.NewStyle().Foreground(colorTextMuted).Render(" · est. remaining ") +
					lipgloss.NewStyle().Foreground(colorSuccess).Render("~"+remaining)
			}
		}
		centeredInfo := lipgloss.PlaceHorizontal(innerWidth, lipgloss.Center, infoStr)

		dialogContent = lipgloss.JoinVertical(lipgloss.Center,
			centeredPills,
			"",
			centeredBar,
			centeredInfo,
		)
	} else {
		// Pre-flight: just pills, no progress
		dialogContent = centeredPills
	}

	// Render dialog box
	dialogBox := dialogBoxStyle.Render(dialogContent)

	// Center dialog in whitespace area with ⎈ pattern
	dialogHeight := lipgloss.Height(dialogBox)
	return lipgloss.Place(
		panelWidth,
		dialogHeight,
		lipgloss.Center,
		lipgloss.Center,
		dialogBox,
		lipgloss.WithWhitespaceChars("⎈ "),
		lipgloss.WithWhitespaceForeground(colorBorderDim),
		lipgloss.WithWhitespaceBackground(colorBg),
	)
}

// renderCompleteBanner renders the upgrade completion banner.
func (m Model) renderCompleteBanner() string {
	elapsed := m.elapsedDisplay()
	msg := successStyle.Render("✓ All nodes upgraded")
	if elapsed != "" {
		msg += footerDescStyle.Render(fmt.Sprintf("  ·  Duration: %s", elapsed))
	}
	return msg
}

// Dashboard node table column indices (pool-grouped layout)
const (
	dashColName    = 0
	dashColVersion = 1
	dashColStage   = 2
	dashColAge     = 3
	dashColPods    = 4
	dashColCPU     = 5
	dashColMem     = 6
)

// poolSeparatorPrefix is used to detect pool separator rows in the table data.
const poolSeparatorPrefix = "▸ "

// renderDashboardNodeTable renders the node table grouped by pool.
// panelWidth is the content width inside the outer panel.
// availableHeight is the total terminal lines available for the table (including its own border/header).
func (m Model) renderDashboardNodeTable(panelWidth, availableHeight int) string {
	// Build rows grouped by pool
	poolOrder, nodesByPool := m.sortedPoolGroups()

	var rows [][]string
	var rowNodeNames []string // parallel: node name for each row (empty for separator)

	for _, pool := range poolOrder {
		nodes := nodesByPool[pool]

		// Pool separator row — show completion count during upgrade
		completeCount := 0
		for _, name := range nodes {
			if n, ok := m.nodes[name]; ok && n.Stage == types.StageComplete {
				completeCount++
			}
		}
		var header string
		if completeCount > 0 {
			header = fmt.Sprintf("%s%s (%d nodes · %d complete)", poolSeparatorPrefix, pool, len(nodes), completeCount)
		} else {
			header = fmt.Sprintf("%s%s (%d nodes)", poolSeparatorPrefix, pool, len(nodes))
		}
		rows = append(rows, []string{header, "", "", "", "", "", ""})
		rowNodeNames = append(rowNodeNames, "")

		// Node rows
		for _, name := range nodes {
			node := m.nodes[name]
			displayName := shortenNodeName(name)

			// Active drain pipeline accent: prefix CORDONED + DRAINING with orange ▎
			if node.Stage == types.StageDraining || node.Stage == types.StageCordoned {
				accentColor := colorDraining
				if node.Stage == types.StageCordoned {
					accentColor = colorCordoned
				}
				displayName = lipgloss.NewStyle().Foreground(accentColor).Render("▎") + " " + displayName
			}

			version := node.Version
			stage := string(node.Stage)
			if node.SurgeNode {
				stage = "SURGE"
			}

			age := node.Age
			if age == "" {
				age = "-"
			}

			pods := fmt.Sprintf("%d", node.PodCount)

			cpu := "—"
			if node.CPUPercent > 0 {
				cpu = fmt.Sprintf("%d%%", node.CPUPercent)
			}

			mem := "—"
			if node.MemPercent > 0 {
				mem = fmt.Sprintf("%d%%", node.MemPercent)
			}

			rows = append(rows, []string{displayName, version, stage, age, pods, cpu, mem})
			rowNodeNames = append(rowNodeNames, name)
		}
	}

	tableWidth := panelWidth - 3
	if tableWidth < 80 {
		tableWidth = 80
	}

	// Table chrome: border top(1) + header(1) + header sep(1) + border bottom(1) = 4 lines.
	// availableHeight is the total lines for the table including chrome.
	const tableChrome = 4
	maxDataRows := availableHeight - tableChrome
	if maxDataRows < 2 {
		maxDataRows = 2
	}

	targetVer := m.targetVersion()

	t := table.New().
		Headers("NAME", "VERSION", "STAGE", "AGE", "PODS", "CPU", "MEM").
		Rows(rows...).
		Width(tableWidth).
		Height(min(len(rows), maxDataRows) + tableChrome).
		Border(lipgloss.RoundedBorder()).
		BorderColumn(false).
		BorderRow(false).
		BorderHeader(true).
		BorderStyle(lipgloss.NewStyle().Foreground(colorSelected)).
		StyleFunc(func(row, col int) lipgloss.Style {
			return m.dashboardCellStyle(row, col, rowNodeNames, targetVer)
		})

	return t.String()
}

// dashboardCellStyle returns the style for a cell in the pool-grouped dashboard table.
// Pool separator rows are detected by nodeNames[row] == "".
func (m Model) dashboardCellStyle(row, col int, nodeNames []string, targetVer string) lipgloss.Style {
	style := lipgloss.NewStyle().Padding(0, 1)

	if row == table.HeaderRow {
		return style.Foreground(colorTextMuted).Bold(true)
	}

	if row >= len(nodeNames) {
		return style
	}

	// Pool separator row
	if nodeNames[row] == "" {
		if col == dashColName {
			return style.Foreground(colorPurple).Bold(true)
		}
		return style
	}

	node, ok := m.nodes[nodeNames[row]]
	if !ok {
		return style
	}

	switch col {
	case dashColName:
		style = style.Foreground(colorText)
	case dashColVersion:
		if targetVer != "" && versionCore(node.Version) == versionCore(targetVer) {
			style = style.Foreground(colorTextBold).Bold(true)
		} else {
			style = style.Foreground(colorTextMuted)
		}
	case dashColStage:
		stage := string(node.Stage)
		if node.SurgeNode {
			stage = "SURGE"
		}
		if fg, exists := stageForegroundColors[stage]; exists {
			style = style.Foreground(fg).Bold(true)
		}
	case dashColAge:
		style = style.Foreground(colorTextDim)
	case dashColPods:
		style = style.Foreground(colorText)
	case dashColCPU:
		if node.CPUPercent > 0 {
			style = style.Foreground(resourceColor(node.CPUPercent))
		} else {
			style = style.Foreground(colorTextDim)
		}
	case dashColMem:
		if node.MemPercent > 0 {
			style = style.Foreground(resourceColor(node.MemPercent))
		} else {
			style = style.Foreground(colorTextDim)
		}
	}

	return style
}

// sortedPoolGroups returns pools in sorted order and nodes within each pool sorted by stage priority.
func (m Model) sortedPoolGroups() ([]string, map[string][]string) {
	stageOrder := map[types.NodeStage]int{
		types.StageDraining:  0,
		types.StageReimaging: 1,
		types.StageCordoned:  2,
		types.StageComplete:  3,
		types.StageReady:     4,
	}

	// Collect pools, sorting nodes within each pool
	poolNodes := make(map[string][]string)
	for name, node := range m.nodes {
		pool := node.Pool
		if pool == "" {
			pool = "default"
		}
		poolNodes[pool] = append(poolNodes[pool], name)
	}

	// Sort nodes within each pool by stage priority, then name
	for pool := range poolNodes {
		nodes := poolNodes[pool]
		sort.SliceStable(nodes, func(i, j int) bool {
			ni, nj := m.nodes[nodes[i]], m.nodes[nodes[j]]
			// Surge nodes last
			if ni.SurgeNode != nj.SurgeNode {
				return !ni.SurgeNode
			}
			oi, oj := stageOrder[ni.Stage], stageOrder[nj.Stage]
			if oi != oj {
				return oi < oj
			}
			return nodes[i] < nodes[j]
		})
	}

	// Sort pool names
	pools := make([]string, 0, len(poolNodes))
	for pool := range poolNodes {
		pools = append(pools, pool)
	}
	sort.Strings(pools)

	return pools, poolNodes
}

// renderInfoCards renders 2 side-by-side info cards: Recent Activity + Blockers.
func (m Model) renderInfoCards(panelWidth int) string {
	cardWidth := (panelWidth - 1) / 2 // 1 char gap between cards
	if cardWidth < 30 {
		cardWidth = 30
	}

	activityCard := m.renderActivityCard(cardWidth)
	blockersCard := m.renderBlockersCard(cardWidth)

	return lipgloss.JoinHorizontal(lipgloss.Top, activityCard, " ", blockersCard)
}

// cardTitleSeparator renders the underline below card titles.
func cardTitleSeparator(innerWidth int) string {
	return lipgloss.NewStyle().Foreground(colorBorderDim).
		Render(strings.Repeat("─", innerWidth))
}

// renderActivityCard renders the "Recent Activity" info card.
func (m Model) renderActivityCard(width int) string {
	innerWidth := width - 6 // border(2) + padding(4)
	title := cardTitleStyle.Render("Recent Activity")
	sep := cardTitleSeparator(innerWidth)

	var lines []string
	eventsToShow := 5
	if len(m.events) < eventsToShow {
		eventsToShow = len(m.events)
	}

	if eventsToShow == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(colorTextMuted).Render("Waiting for events..."))
	} else {
		// timestamp(5) + spacing(2) + icon(1) + space(1)
		maxMsgLen := innerWidth - 9
		if maxMsgLen < 20 {
			maxMsgLen = 20
		}
		for i := 0; i < eventsToShow; i++ {
			e := m.events[i]
			ts := lipgloss.NewStyle().Foreground(colorTextDim).Render(e.Timestamp.Format("15:04"))

			var icon string
			switch e.Severity {
			case types.SeverityWarning:
				icon = lipgloss.NewStyle().Foreground(colorWarning).Render("▲")
			case types.SeverityError:
				icon = lipgloss.NewStyle().Foreground(colorError).Render("✖")
			default:
				icon = lipgloss.NewStyle().Foreground(colorInfo).Render("●")
			}

			msg := formatEventConcise(e)
			if len(msg) > maxMsgLen {
				msg = msg[:maxMsgLen-3] + "..."
			}

			lines = append(lines, fmt.Sprintf("%s %s %s", ts, icon, msg))
		}
	}

	body := title + "\n" + sep + "\n" + strings.Join(lines, "\n")

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorInfo).
		Padding(1, 2).
		Width(width - 2).
		Render(body)
}

// renderBlockersCard renders the "Blockers" info card.
func (m Model) renderBlockersCard(width int) string {
	innerWidth := width - 6 // border(2) + padding(4)
	title := cardTitleStyle.Render("Blockers")
	sep := cardTitleSeparator(innerWidth)

	blockers := m.activeBlockers()
	var lines []string

	if len(blockers) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(colorTextMuted).Render("No blockers"))
	}

	for _, b := range blockers {
		name := b.Name
		if b.Namespace != "" {
			name = b.Namespace + "/" + name
		}
		lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(colorDraining).Render("PDB: "+name))

		if b.Detail != "" {
			lines = append(lines, lipgloss.NewStyle().Foreground(colorTextDim).Render("  "+b.Detail))
		}
		if b.NodeName != "" {
			lines = append(lines, lipgloss.NewStyle().Foreground(colorTextDim).Render("  blocking: "+shortenNodeName(b.NodeName)))
		}
		if !b.StartTime.IsZero() {
			dur := m.currentTime.Sub(b.StartTime)
			lines = append(lines, lipgloss.NewStyle().Foreground(colorWarning).Render("  stalled: "+formatDuration(dur)))
		}
		lines = append(lines, "")
	}

	// Trim trailing empty line
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	body := title + "\n" + sep + "\n" + strings.Join(lines, "\n")

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorError).
		Padding(1, 2).
		Width(width - 2).
		Render(body)
}

// renderPreFlightSection renders health checks as indented text when no upgrade is active.
func (m Model) renderPreFlightSection(_ int) string {
	indent := "  "
	titleLine := lipgloss.NewStyle().Bold(true).Foreground(colorTextBold).
		Render("— PRE-FLIGHT CHECKS")

	var checks []string

	// Check 1: All nodes ready (excluding surge nodes)
	readyCount := m.stageCountExcludingSurge(types.StageReady)
	total := m.totalNodes()
	if total > 0 && readyCount == total {
		checks = append(checks, indent+successStyle.Render("✓")+fmt.Sprintf(" All nodes Ready (%d/%d)", readyCount, total))
	} else if total > 0 {
		notReady := total - readyCount
		checks = append(checks, indent+errorStyle.Render("✗")+fmt.Sprintf(" %d node(s) not Ready (%d/%d)", notReady, readyCount, total))
	} else {
		checks = append(checks, indent+footerDescStyle.Render("… Discovering nodes..."))
	}

	// Check 2: No cordoned nodes (excluding surge)
	cordonedCount := m.stageCountExcludingSurge(types.StageCordoned)
	if cordonedCount == 0 {
		checks = append(checks, indent+successStyle.Render("✓")+" No cordoned nodes")
	} else {
		checks = append(checks, indent+warningStyle.Render("⚠")+fmt.Sprintf(" %d node(s) cordoned", cordonedCount))
	}

	// Check 3: PDBs that will block drains (structural misconfiguration)
	if len(m.preFlightBlockers) == 0 {
		checks = append(checks, indent+successStyle.Render("✓")+" No PDBs will block drain")
	} else {
		checks = append(checks, indent+warningStyle.Render("⚠")+fmt.Sprintf(" %d PDB(s) will block drain", len(m.preFlightBlockers)))
		for _, pdb := range m.preFlightBlockers {
			name := pdb.Name
			if pdb.Namespace != "" {
				name = pdb.Namespace + "/" + name
			}
			checks = append(checks, indent+footerDescStyle.Render(fmt.Sprintf("  → %s: %s", name, pdb.Detail)))
		}
	}

	// Check 4: Error pods
	var errorPods int
	for _, pod := range m.pods {
		switch pod.Phase {
		case "CrashLoopBackOff", "Error", "Failed", "ImagePullBackOff", "OOMKilled":
			errorPods++
		}
	}
	if errorPods == 0 {
		checks = append(checks, indent+successStyle.Render("✓")+" No pods in error state")
	} else {
		checks = append(checks, indent+warningStyle.Render("⚠")+fmt.Sprintf(" %d pod(s) in error state", errorPods))
	}

	watchMsg := lipgloss.NewStyle().Foreground(colorTextDim).Italic(true).
		Render(indent + "Watching for upgrade — kupgrade will detect it automatically")

	return titleLine + "\n" + strings.Join(checks, "\n") + "\n" + watchMsg
}

// renderActiveDrains renders the ACTIVE DRAINS section shown during upgrades.
// Lists each CORDONED/DRAINING node with drain progress and PDB blocker info.
func (m Model) renderActiveDrains() string {
	drainNodes := m.getDrainNodes()
	if len(drainNodes) == 0 {
		return ""
	}

	orangeStyle := lipgloss.NewStyle().Foreground(colorDraining)
	titleLine := orangeStyle.Bold(true).Render("⚡ ACTIVE DRAINS")

	var lines []string
	for _, name := range drainNodes {
		node, ok := m.nodes[name]
		if !ok {
			continue
		}
		short := shortenNodeName(name)

		var detail string
		if node.EvictablePodCount > 0 {
			detail = fmt.Sprintf("draining %d pods", node.EvictablePodCount)
		} else if node.Stage == types.StageCordoned {
			detail = "cordoned"
		} else {
			detail = string(node.Stage)
		}

		if node.Blocked && node.BlockerReason != "" {
			detail += " · " + lipgloss.NewStyle().Foreground(colorWarning).Render("PDB "+node.BlockerReason+" blocking")
		}

		if !node.DrainStartTime.IsZero() {
			dur := m.currentTime.Sub(node.DrainStartTime)
			detail += " (" + formatDuration(dur) + ")"
		}

		lines = append(lines, orangeStyle.Render("  ▎ ")+lipgloss.NewStyle().Foreground(colorText).Render(short)+" — "+
			lipgloss.NewStyle().Foreground(colorTextMuted).Render(detail))
	}

	return titleLine + "\n" + strings.Join(lines, "\n")
}

// formatEventConcise returns a short-form description of an event for the activity card.
// Maps event types to concise action + target strings instead of raw messages.
func formatEventConcise(e types.Event) string {
	shortNode := shortenNodeName(e.NodeName)
	shortPod := lastSegment(e.PodName)

	switch e.Type {
	case types.EventNodeCordon:
		return "Cordoned " + shortNode
	case types.EventNodeUncordon:
		return "Uncordoned " + shortNode
	case types.EventNodeReady:
		return "Node ready " + shortNode
	case types.EventNodeNotReady:
		return "Node not ready " + shortNode
	case types.EventNodeVersion:
		return "Version change " + shortNode
	case types.EventPodEvicted:
		return "Evicted " + shortPod
	case types.EventPodScheduled:
		return "Scheduled " + shortPod
	case types.EventPodReady:
		return "Pod ready " + shortPod
	case types.EventPodFailed:
		return "Pod failed " + shortPod
	case types.EventPodDeleted:
		return "Pod deleted " + shortPod
	case types.EventK8sWarning:
		if e.Reason == "FailedEviction" || strings.Contains(e.Message, "PodDisruptionBudget") {
			name := e.PodName
			if name == "" {
				name = e.NodeName
			}
			return "PDB " + lastSegment(name) + " blocking"
		}
		return "Warning: " + truncateString(e.Message, 40)
	case types.EventMigration:
		return "Rescheduled " + shortPod
	default:
		return truncateString(e.Message, 45)
	}
}

// lastSegment returns the last slash-separated segment of a name.
// For "namespace/pod-name" returns "pod-name". For simple names, returns as-is.
func lastSegment(name string) string {
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		return name[idx+1:]
	}
	return name
}
