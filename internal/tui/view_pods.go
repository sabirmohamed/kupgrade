package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/sabirmohamed/kupgrade/pkg/types"
	"github.com/sahilm/fuzzy"
)

// renderPodsScreen renders the pod list screen
func (m Model) renderPodsScreen() string {
	w := m.mainWidth()

	// Tab bar
	counts := m.stageCounts()
	tabBar := m.renderTabBar(counts)

	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	podList := m.getSortedPodList()
	totalPods := len(podList)

	var displayList []types.PodState
	if query := m.podSearchInput.Value(); query != "" {
		displayList = fuzzyFilterPods(podList, query)
	} else {
		displayList = podList
	}

	// Search bar
	if m.podSearchActive || m.podSearchInput.Value() != "" {
		b.WriteString("  " + m.renderPodSearchBar(totalPods, len(displayList)))
	}
	b.WriteString("\n")

	if len(displayList) == 0 {
		if m.podSearchInput.Value() != "" {
			b.WriteString(footerDescStyle.Render("  No matches"))
		} else {
			b.WriteString(footerDescStyle.Render("  No pods"))
		}
	} else if m.podSearchInput.Value() != "" {
		// Fuzzy search: flat table (no sections)
		b.WriteString(m.renderPodsTableFlat(displayList))
	} else {
		// Default: sectioned by priority (attention on top)
		b.WriteString(m.renderPodsTableSectioned(displayList))
	}

	panelBody := b.String()

	// Wrap in outer panel
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorSelected).
		Padding(0, 1).
		Width(w - 2).
		Render(panelBody)

	// Status bar + key hints
	statusBar := m.renderStatusBar(w)
	keyHints := m.renderKeyHints(w)

	content := lipgloss.JoinVertical(lipgloss.Left, tabBar, panel, statusBar, keyHints)
	return m.placeContent(content)
}

// classifyPod determines the priority for a single pod.
// Uses node state, blocker data, and migration history.
func (m Model) classifyPod(pod types.PodState) PodPriority {
	// Error phases — always Attention
	if isErrorPhase(pod.Phase) {
		return PodPriorityAttention
	}

	// Pending — can't be scheduled or stuck
	if pod.Phase == "Pending" {
		return PodPriorityAttention
	}

	// Pod on a draining/cordoned node — skip DaemonSets (tolerate drain)
	// and Completed/Succeeded pods (already finished)
	if pod.NodeName != "" && pod.OwnerKind != "DaemonSet" &&
		pod.Phase != "Completed" && pod.Phase != "Succeeded" {
		if node, ok := m.nodes[pod.NodeName]; ok {
			if node.Stage == types.StageDraining || node.Stage == types.StageCordoned {
				return PodPriorityAttention
			}
		}
	}

	// Running but not all containers ready (probe failures, sidecar crash)
	if pod.Phase == "Running" && pod.TotalContainers > 0 && pod.ReadyContainers < pod.TotalContainers {
		return PodPriorityAttention
	}

	// Rescheduled and running — disrupted but recovered
	if m.isPodRescheduled(pod) && pod.Phase == "Running" {
		return PodPriorityDisrupted
	}

	return PodPriorityHealthy
}

// classifyAllPods classifies every pod in m.pods for global counts.
func (m Model) classifyAllPods() []classifiedPod {
	allPods := make([]types.PodState, 0, len(m.pods))
	for _, pod := range m.pods {
		allPods = append(allPods, pod)
	}
	return m.classifyPods(allPods)
}

// classifyPods classifies a list of pods.
func (m Model) classifyPods(pods []types.PodState) []classifiedPod {
	classified := make([]classifiedPod, len(pods))
	for i, pod := range pods {
		classified[i] = classifiedPod{Pod: pod, Priority: m.classifyPod(pod)}
	}
	return classified
}

// isErrorPhase returns true if the pod phase indicates an error state
func isErrorPhase(phase string) bool {
	switch phase {
	case "CrashLoopBackOff", "Error", "Failed", "ImagePullBackOff", "ErrImagePull",
		"OOMKilled", "RunContainerError", "CreateContainerError", "Unknown":
		return true
	}
	// Init container errors: Init:Error, Init:CrashLoopBackOff, etc.
	// but NOT Init:0/2 (progress indicator)
	if strings.HasPrefix(phase, "Init:") {
		suffix := phase[5:]
		if len(suffix) > 0 && suffix[0] >= '0' && suffix[0] <= '9' {
			return false // Init:0/2 = progress, not error
		}
		return true // Init:Error, Init:CrashLoopBackOff, etc.
	}
	return false
}

// isPodRescheduled checks if a pod appears in the migrations list
func (m Model) isPodRescheduled(pod types.PodState) bool {
	podKey := pod.Namespace + "/" + pod.Name
	for _, mig := range m.migrations {
		migKey := mig.Namespace + "/" + mig.NewPod
		if migKey == podKey {
			return true
		}
	}
	return false
}

// countByPriority counts pods in each priority bucket
func countByPriority(classified []classifiedPod) (attention, disrupted, healthy int) {
	for _, cp := range classified {
		switch cp.Priority {
		case PodPriorityAttention:
			attention++
		case PodPriorityDisrupted:
			disrupted++
		case PodPriorityHealthy:
			healthy++
		}
	}
	return
}

// Pod table column indices
const (
	podColNamespace = 0
	podColName      = 1
	podColReady     = 2
	podColStatus    = 3
	podColRestarts  = 4
	podColNode      = 5
	podColAge       = 6

	podColumnCount = 7 // total columns in pods table
)

// renderPodsTableSectioned renders the pods table with priority sections.
// Section separator rows divide Attention, Disrupted, and Healthy groups.
func (m Model) renderPodsTableSectioned(podList []types.PodState) string {
	classified := m.classifyPods(podList)
	attentionCount, disruptedCount, healthyCount := countByPriority(classified)

	// Build rows with section separators
	var rows [][]string
	var rowPriorities []PodPriority // parallel slice: priority for each row
	var rowPodIndices []int         // parallel slice: index into podList (-1 for non-pod rows)

	// Emit Attention section
	if attentionCount > 0 {
		rows = append(rows, sectionSeparatorRow(fmt.Sprintf("! NEEDS ATTENTION (%d)", attentionCount)))
		rowPriorities = append(rowPriorities, PodPrioritySeparator)
		rowPodIndices = append(rowPodIndices, -1)

		for i, cp := range classified {
			if cp.Priority == PodPriorityAttention {
				rows = append(rows, buildPodRow(cp.Pod))
				rowPriorities = append(rowPriorities, PodPriorityAttention)
				rowPodIndices = append(rowPodIndices, i)
			}
		}
	}

	// Emit Disrupted section
	if disruptedCount > 0 {
		rows = append(rows, sectionSeparatorRow(fmt.Sprintf("~ DISRUPTED (%d)", disruptedCount)))
		rowPriorities = append(rowPriorities, PodPrioritySeparator)
		rowPodIndices = append(rowPodIndices, -1)

		for i, cp := range classified {
			if cp.Priority == PodPriorityDisrupted {
				rows = append(rows, buildPodRow(cp.Pod))
				rowPriorities = append(rowPriorities, PodPriorityDisrupted)
				rowPodIndices = append(rowPodIndices, i)
			}
		}
	}

	// Emit Healthy section
	if healthyCount > 0 {
		rows = append(rows, sectionSeparatorRow(fmt.Sprintf("OK HEALTHY (%d)", healthyCount)))
		rowPriorities = append(rowPriorities, PodPrioritySeparator)
		rowPodIndices = append(rowPodIndices, -1)

		for i, cp := range classified {
			if cp.Priority == PodPriorityHealthy {
				rows = append(rows, buildPodRow(cp.Pod))
				rowPriorities = append(rowPriorities, PodPriorityHealthy)
				rowPodIndices = append(rowPodIndices, i)
			}
		}
	}

	return m.renderPodsTableWithRows(rows, rowPriorities, rowPodIndices, podList, len(podList))
}

// renderPodsTableFlat renders the pods table without priority sections.
// Used for specific filter tabs or during fuzzy search.
func (m Model) renderPodsTableFlat(podList []types.PodState) string {
	classified := m.classifyPods(podList)
	rows := make([][]string, len(podList))
	rowPriorities := make([]PodPriority, len(podList))
	rowPodIndices := make([]int, len(podList))

	for i, cp := range classified {
		rows[i] = buildPodRow(cp.Pod)
		rowPriorities[i] = cp.Priority
		rowPodIndices[i] = i
	}

	return m.renderPodsTableWithRows(rows, rowPriorities, rowPodIndices, podList, len(podList))
}

// renderPodsTableWithRows renders the pod table from pre-built rows with metadata.
// podCount is the number of actual pods (excluding separator rows) for the footer display.
func (m Model) renderPodsTableWithRows(rows [][]string, rowPriorities []PodPriority, rowPodIndices []int, podList []types.PodState, podCount int) string {
	totalRows := len(rows)

	// Overhead: tabBar(1) + panel borders(2) + header(1) + blank(1) +
	// table header+border(2) + hint(1) + statusBar(1) + keyHints(1) + buffer(2) = 12
	visibleRows := m.height - 12
	if visibleRows < 5 {
		visibleRows = 5
	}

	colWidths := computeColumnWidths(m.tableWidth(), []columnLayout{
		{Weight: 3}, // NAMESPACE — flexible
		{Weight: 5}, // NAME — flexible, widest
		{Fixed: 7},  // READY
		{Fixed: 12}, // STATUS
		{Fixed: 14}, // RESTARTS
		{Weight: 4}, // NODE — flexible
		{Fixed: 8},  // AGE
	})

	t := table.New().
		Headers("NAMESPACE", "NAME", "READY", "STATUS", "RESTARTS", "NODE", "AGE").
		Rows(rows...).
		Width(m.tableWidth()).
		Border(lipgloss.RoundedBorder()).
		BorderColumn(false).
		BorderRow(false).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderHeader(true).
		BorderStyle(tableBorderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := m.podCellStyle(row, col, rowPriorities, rowPodIndices, podList)
			if col < len(colWidths) {
				s = s.Width(colWidths[col])
			}
			return s
		})

	// Always constrain height for consistent layout (prevents header/footer flicker)
	scrollOffset := calcScrollOffset(m.listIndex, visibleRows, totalRows)
	t = t.Height(visibleRows).Offset(scrollOffset)

	rendered := t.String()

	var hint string
	if m.podSearchActive {
		hint = " ↑↓ navigate  Enter commit  Esc cancel"
	} else if m.podSearchInput.Value() != "" {
		hint = fmt.Sprintf(" %d pods  •  d describe  •  / search  Esc clear", podCount)
	} else {
		hint = fmt.Sprintf(" %d pods  •  d describe  •  / search", podCount)
	}
	if totalRows > 0 {
		rendered += "\n" + footerDescStyle.Render(hint)
	}

	return rendered
}

// sectionSeparatorRow builds a table row for a section header.
// The label goes in column 0 (NAMESPACE); other columns are empty.
func sectionSeparatorRow(label string) []string {
	row := make([]string, podColumnCount)
	row[0] = label
	return row
}

// buildPodRow builds a table row for a pod (plain text, coloring via StyleFunc).
func buildPodRow(pod types.PodState) []string {
	readyStr := fmt.Sprintf("%d/%d", pod.ReadyContainers, pod.TotalContainers)

	var restartStr string
	if pod.Restarts == 0 {
		restartStr = "0"
	} else if pod.LastRestartAge != "" {
		restartStr = fmt.Sprintf("%d (%s)", pod.Restarts, pod.LastRestartAge)
	} else {
		restartStr = fmt.Sprintf("%d", pod.Restarts)
	}

	nodeName := pod.NodeName
	if nodeName == "" {
		nodeName = "<unscheduled>"
	}

	return []string{
		pod.Namespace,
		pod.Name,
		readyStr,
		pod.Phase,
		restartStr,
		nodeName,
		pod.Age,
	}
}

// podCellStyle computes the style for a cell in the sectioned pods table.
func (m Model) podCellStyle(row, col int, rowPriorities []PodPriority, rowPodIndices []int, podList []types.PodState) lipgloss.Style {
	style := lipgloss.NewStyle().Padding(0, 1)

	if row == table.HeaderRow {
		return m.podHeaderStyle(style, col)
	}

	if row >= len(rowPriorities) {
		return style
	}

	priority := rowPriorities[row]

	// Section separator row
	if priority == PodPrioritySeparator {
		if col == 0 {
			return style.Bold(true).Foreground(colorTextMuted)
		}
		return style.Foreground(colorBorderDim)
	}

	// Selected row highlight
	if row == m.listIndex {
		style = style.Background(colorSelected).Foreground(colorTextBold)
	}

	// Right-align numeric columns
	switch col {
	case podColReady, podColRestarts, podColAge:
		style = style.Align(lipgloss.Right)
	}

	// Pod-specific coloring
	podIdx := rowPodIndices[row]
	if podIdx < 0 || podIdx >= len(podList) {
		return style
	}
	pod := podList[podIdx]

	switch col {
	case podColName:
		// Attention accent
		if priority == PodPriorityAttention && row != m.listIndex {
			style = style.Foreground(colorError)
		}
	case podColReady:
		if row != m.listIndex {
			style = style.Foreground(readyColor(pod))
		}
	case podColStatus:
		if row != m.listIndex {
			c := statusColor(pod.Phase)
			if c == colorComplete && hasProbeFailure(pod) {
				c = colorError
			}
			style = style.Foreground(c)
		}
	case podColRestarts:
		if row != m.listIndex {
			style = style.Foreground(restartColor(pod.Restarts))
		}
	case podColNode:
		if row != m.listIndex {
			if pod.NodeName == "" {
				style = style.Foreground(colorError)
			} else if node, ok := m.nodes[pod.NodeName]; ok {
				if node.Stage == types.StageDraining || node.Stage == types.StageCordoned {
					style = style.Foreground(colorDraining)
				}
			}
		}
	}

	return style
}

// podHeaderStyle applies header-specific styling (muted, bold, right-align numerics)
func (m Model) podHeaderStyle(style lipgloss.Style, col int) lipgloss.Style {
	style = style.Foreground(colorTextMuted).Bold(true)
	switch col {
	case podColReady, podColRestarts, podColAge:
		style = style.Align(lipgloss.Right)
	}
	return style
}

// hasProbeFailure returns true if any probe is failing
func hasProbeFailure(pod types.PodState) bool {
	return (pod.HasReadiness && !pod.ReadinessOK) || (pod.HasLiveness && !pod.LivenessOK)
}

// statusColor returns foreground color for pod phase
func statusColor(phase string) lipgloss.Color {
	switch phase {
	case "Running":
		return colorComplete // green
	case "Pending":
		return colorCordoned // yellow
	case "Completed", "Succeeded":
		return colorTextMuted
	case "CrashLoopBackOff", "Error", "Failed", "ImagePullBackOff", "ErrImagePull",
		"OOMKilled", "RunContainerError", "CreateContainerError":
		return colorError // red
	case "Terminating":
		return colorBrightYellow // orange
	case "Unknown":
		return colorBrightRed // bright red
	default:
		// Handle Init:* and PodInitializing prefixes
		if strings.HasPrefix(phase, "Init:") || phase == "PodInitializing" {
			return colorCyan // init state
		}
		return colorText
	}
}

// readyColor returns foreground color based on container readiness
func readyColor(pod types.PodState) lipgloss.Color {
	if pod.TotalContainers == 0 {
		return colorTextMuted
	}
	if pod.ReadyContainers == pod.TotalContainers {
		return colorComplete // green
	}
	if pod.ReadyContainers == 0 {
		return colorError // red
	}
	return colorCordoned // yellow - partial
}

// restartColor returns foreground color based on restart count
func restartColor(restarts int) lipgloss.Color {
	if restarts > 5 {
		return colorError // red
	}
	if restarts > 0 {
		return colorCordoned // yellow
	}
	return colorTextMuted
}

// renderPodSearchBar renders the fuzzy search input bar
func (m Model) renderPodSearchBar(totalFiltered, matchCount int) string {
	if m.podSearchActive {
		return fmt.Sprintf("> %s  %d/%d",
			m.podSearchInput.View(), matchCount, totalFiltered)
	}
	// Filter is set but input is not focused
	return fmt.Sprintf("> %s  %d/%d",
		footerKeyStyle.Render(m.podSearchInput.Value()), matchCount, totalFiltered)
}

// podRowCount returns the total number of table rows (including separators)
// for the current pod view. Used by navigation to know the item count.
func (m Model) podRowCount(podList []types.PodState) int {
	// Flat rendering during search
	if m.podSearchInput.Value() != "" {
		return len(podList)
	}

	// Sectioned rendering
	classified := m.classifyPods(podList)
	attentionCount, disruptedCount, healthyCount := countByPriority(classified)

	total := 0
	if attentionCount > 0 {
		total += 1 + attentionCount // separator + pods
	}
	if disruptedCount > 0 {
		total += 1 + disruptedCount
	}
	if healthyCount > 0 {
		total += 1 + healthyCount // separator + pods
	}
	return total
}

// podAtRow returns the PodState at a given table row index, or nil if the row is a separator.
// Used by the key handler to determine which pod to describe.
func (m Model) podAtRow(podList []types.PodState, rowIndex int) *types.PodState {
	// Flat rendering during search
	if m.podSearchInput.Value() != "" {
		if rowIndex >= 0 && rowIndex < len(podList) {
			return &podList[rowIndex]
		}
		return nil
	}

	classified := m.classifyPods(podList)
	attentionCount, disruptedCount, healthyCount := countByPriority(classified)

	// Walk through the sectioned layout to find which pod is at rowIndex
	currentRow := 0

	// Attention section
	if attentionCount > 0 {
		if rowIndex == currentRow {
			return nil // separator
		}
		currentRow++ // skip separator

		for _, cp := range classified {
			if cp.Priority == PodPriorityAttention {
				if rowIndex == currentRow {
					pod := cp.Pod
					return &pod
				}
				currentRow++
			}
		}
	}

	// Disrupted section
	if disruptedCount > 0 {
		if rowIndex == currentRow {
			return nil // separator
		}
		currentRow++

		for _, cp := range classified {
			if cp.Priority == PodPriorityDisrupted {
				if rowIndex == currentRow {
					pod := cp.Pod
					return &pod
				}
				currentRow++
			}
		}
	}

	// Healthy section
	if healthyCount > 0 {
		if rowIndex == currentRow {
			return nil // separator
		}
		currentRow++

		for _, cp := range classified {
			if cp.Priority == PodPriorityHealthy {
				if rowIndex == currentRow {
					pod := cp.Pod
					return &pod
				}
				currentRow++
			}
		}
	}

	return nil
}

// getSortedPodList classifies all pods and sorts by priority (attention first), then node, namespace, name.
func (m *Model) getSortedPodList() []types.PodState {
	classified := m.classifyAllPods()

	sort.SliceStable(classified, func(i, j int) bool {
		if classified[i].Priority != classified[j].Priority {
			return classified[i].Priority < classified[j].Priority
		}
		if classified[i].Pod.NodeName != classified[j].Pod.NodeName {
			return classified[i].Pod.NodeName < classified[j].Pod.NodeName
		}
		if classified[i].Pod.Namespace != classified[j].Pod.Namespace {
			return classified[i].Pod.Namespace < classified[j].Pod.Namespace
		}
		return classified[i].Pod.Name < classified[j].Pod.Name
	})

	result := make([]types.PodState, len(classified))
	for i, cp := range classified {
		result[i] = cp.Pod
	}
	return result
}

// getDisplayPodList returns pods sorted by priority, then filtered by search query.
func (m *Model) getDisplayPodList() []types.PodState {
	podList := m.getSortedPodList()
	if query := m.podSearchInput.Value(); query != "" {
		return fuzzyFilterPods(podList, query)
	}
	return podList
}

// fuzzyFilterPods filters pods using fuzzy matching against name, namespace, node, and status.
func fuzzyFilterPods(pods []types.PodState, query string) []types.PodState {
	source := make(podSearchSource, len(pods))
	for i, pod := range pods {
		source[i] = pod.Namespace + "/" + pod.Name + " " + pod.NodeName + " " + pod.Phase
	}
	matches := fuzzy.FindFrom(query, source)
	result := make([]types.PodState, len(matches))
	for i, match := range matches {
		result[i] = pods[match.Index]
	}
	return result
}

type podSearchSource []string

func (s podSearchSource) String(i int) string { return s[i] }
func (s podSearchSource) Len() int            { return len(s) }
