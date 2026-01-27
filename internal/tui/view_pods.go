package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sabirmohamed/kupgrade/pkg/types"
)

// renderPodsScreen renders the pod list screen
func (m Model) renderPodsScreen() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")

	// Get nodes in upgrade pipeline (CORDONED, DRAINING, UPGRADING)
	upgradeNodes := make(map[string]bool)
	for _, name := range m.nodesByStage[types.StageCordoned] {
		upgradeNodes[name] = true
	}
	for _, name := range m.nodesByStage[types.StageDraining] {
		upgradeNodes[name] = true
	}
	for _, name := range m.nodesByStage[types.StageUpgrading] {
		upgradeNodes[name] = true
	}

	// Collect pods on upgrade nodes (or all if no nodes upgrading)
	var podList []types.PodState
	showAll := len(upgradeNodes) == 0
	for _, pod := range m.pods {
		if showAll || upgradeNodes[pod.NodeName] {
			podList = append(podList, pod)
		}
	}

	// Sort by node, then namespace, then name
	sort.Slice(podList, func(i, j int) bool {
		if podList[i].NodeName != podList[j].NodeName {
			return podList[i].NodeName < podList[j].NodeName
		}
		if podList[i].Namespace != podList[j].Namespace {
			return podList[i].Namespace < podList[j].Namespace
		}
		return podList[i].Name < podList[j].Name
	})

	if len(podList) == 0 {
		if showAll {
			b.WriteString(footerDescStyle.Render("  No pods found"))
		} else {
			b.WriteString(footerDescStyle.Render("  No pods on upgrading nodes"))
			b.WriteString("\n")
			b.WriteString(footerDescStyle.Render("  (showing pods on CORDONED/DRAINING/UPGRADING nodes only)"))
		}
	} else {
		b.WriteString(m.renderPodTable(podList, showAll))
	}

	b.WriteString("\n\n")
	b.WriteString(m.renderFooter())

	return m.placeContent(b.String())
}

// renderPodTable renders the pod table
func (m Model) renderPodTable(podList []types.PodState, showAll bool) string {
	var b strings.Builder

	// Calculate responsive column widths
	availWidth := m.width - 4
	if availWidth < 120 {
		availWidth = 120
	}

	fixedWidth := 57
	varWidth := availWidth - fixedWidth
	nsWidth := varWidth * 15 / 100
	nameWidth := varWidth * 40 / 100
	nodeWidth := varWidth * 45 / 100

	// Min/max widths
	if nsWidth < 12 {
		nsWidth = 12
	}
	if nameWidth < 30 {
		nameWidth = 30
	}
	if nodeWidth < 25 {
		nodeWidth = 25
	}
	if nsWidth > 15 {
		nsWidth = 15
	}
	if nameWidth > 55 {
		nameWidth = 55
	}
	if nodeWidth > 40 {
		nodeWidth = 40
	}

	// Calculate visible rows
	visibleRows := m.height - 10
	if visibleRows < 5 {
		visibleRows = 5
	}

	scrollOffset := 0
	if m.listIndex >= visibleRows {
		scrollOffset = m.listIndex - visibleRows + 1
	}

	total := len(podList)
	filterNote := ""
	if !showAll {
		filterNote = " (upgrading nodes)"
	}
	scrollInfo := ""
	if total > visibleRows {
		scrollInfo = fmt.Sprintf(" [%d-%d of %d]", scrollOffset+1, min(scrollOffset+visibleRows, total), total)
	}
	b.WriteString(fmt.Sprintf("  pods(%d)%s%s\n", total, filterNote, scrollInfo))

	// Table header
	headerFmt := fmt.Sprintf("  %%-%ds %%-%ds %%5s %%-16s %%-10s %%-5s %%-12s %%-%ds %%5s",
		nsWidth, nameWidth, nodeWidth)
	header := fmt.Sprintf(headerFmt,
		"NAMESPACE", "NAME", "READY", "STATUS", "RESTARTS", "PROBE", "OWNER", "NODE", "AGE")
	b.WriteString(panelTitleStyle.Render(header))
	b.WriteString("\n")

	// Separator
	sepLen := nsWidth + nameWidth + nodeWidth + 50
	if sepLen > m.width-2 {
		sepLen = m.width - 2
	}
	b.WriteString(footerDescStyle.Render("  " + strings.Repeat("─", sepLen)))
	b.WriteString("\n")

	endIdx := scrollOffset + visibleRows
	if endIdx > len(podList) {
		endIdx = len(podList)
	}

	prevNode := ""
	for i := scrollOffset; i < endIdx; i++ {
		pod := podList[i]

		// Node group separator
		if pod.NodeName != prevNode && prevNode != "" && i > scrollOffset {
			b.WriteString(footerDescStyle.Render("  " + strings.Repeat("·", sepLen/2)))
			b.WriteString("\n")
		}
		prevNode = pod.NodeName

		b.WriteString(m.renderPodRow(pod, i, nsWidth, nameWidth, nodeWidth))
	}

	// Scroll indicator
	if total > visibleRows {
		b.WriteString("\n")
		if scrollOffset > 0 {
			b.WriteString(footerDescStyle.Render("  ↑ more above"))
		}
		if endIdx < total {
			if scrollOffset > 0 {
				b.WriteString(footerDescStyle.Render("  |  "))
			} else {
				b.WriteString(footerDescStyle.Render("  "))
			}
			b.WriteString(footerDescStyle.Render("↓ more below"))
		}
	}

	return b.String()
}

// renderPodRow renders a single pod row
func (m Model) renderPodRow(pod types.PodState, idx, nsWidth, nameWidth, nodeWidth int) string {
	var b strings.Builder

	cursor := "  "
	if idx == m.listIndex {
		cursor = "► "
	}

	namespace := truncateString(pod.Namespace, nsWidth)
	name := truncateString(pod.Name, nameWidth)

	// Ready containers
	readyStr := fmt.Sprintf("%d/%d", pod.ReadyContainers, pod.TotalContainers)
	readyStyle := successStyle
	if pod.ReadyContainers < pod.TotalContainers {
		readyStyle = warningStyle
	}
	if pod.ReadyContainers == 0 && pod.TotalContainers > 0 {
		readyStyle = errorStyle
	}

	// Status
	status := truncateString(pod.Phase, 16)
	statusStyle := successStyle
	switch {
	case pod.Phase == "Running":
		statusStyle = successStyle
	case pod.Phase == "Pending":
		statusStyle = warningStyle
	case pod.Phase == "Succeeded" || pod.Phase == "Completed":
		statusStyle = footerDescStyle
	case pod.Phase == "CrashLoopBackOff" || pod.Phase == "ImagePullBackOff" ||
		pod.Phase == "ErrImagePull" || pod.Phase == "Error" ||
		pod.Phase == "Failed" || pod.Phase == "Unknown" ||
		pod.Phase == "Terminating" || pod.Phase == "OOMKilled" ||
		strings.HasPrefix(pod.Phase, "Init:"):
		statusStyle = errorStyle
	}

	// Restarts
	var restartStr string
	restartStyle := footerDescStyle
	if pod.Restarts == 0 {
		restartStr = "0"
	} else if pod.LastRestartAge != "" {
		restartStr = fmt.Sprintf("%d %s", pod.Restarts, pod.LastRestartAge)
	} else {
		restartStr = fmt.Sprintf("%d", pod.Restarts)
	}
	if pod.Restarts > 5 {
		restartStyle = errorStyle
	} else if pod.Restarts > 0 {
		restartStyle = warningStyle
	}

	// Probes
	var rProbe, lProbe string
	var rStyle, lStyle lipgloss.Style

	if pod.HasReadiness {
		if pod.ReadinessOK {
			rProbe = "R✓"
			rStyle = successStyle
		} else {
			rProbe = "R✗"
			rStyle = errorStyle
		}
	} else {
		rProbe = "··"
		rStyle = footerDescStyle
	}

	if pod.HasLiveness {
		if pod.LivenessOK {
			lProbe = "L✓"
			lStyle = successStyle
		} else {
			lProbe = "L✗"
			lStyle = errorStyle
		}
	} else {
		lProbe = "··"
		lStyle = footerDescStyle
	}

	// Owner
	owner := truncateString(pod.OwnerKind, 12)
	if owner == "" {
		owner = "<none>"
	}
	ownerStyle := footerDescStyle
	if pod.OwnerKind == "DaemonSet" {
		ownerStyle = warningStyle
	}

	// Node name
	nodeName := truncateString(pod.NodeName, nodeWidth)
	if nodeName == "" {
		nodeName = "<pending>"
	}

	// Build line
	lineFmt := fmt.Sprintf("%%s%%-%ds %%-%ds ", nsWidth, nameWidth)
	line := fmt.Sprintf(lineFmt, cursor, namespace, name)
	if idx == m.listIndex {
		b.WriteString(nodeNameStyle.Render(line))
	} else {
		b.WriteString(line)
	}

	b.WriteString(readyStyle.Render(fmt.Sprintf("%5s ", readyStr)))
	b.WriteString(statusStyle.Render(fmt.Sprintf("%-16s ", status)))
	b.WriteString(restartStyle.Render(fmt.Sprintf("%-10s ", restartStr)))
	b.WriteString(rStyle.Render(rProbe))
	b.WriteString(" ")
	b.WriteString(lStyle.Render(lProbe))
	b.WriteString(" ")
	b.WriteString(ownerStyle.Render(fmt.Sprintf("%-12s ", owner)))
	b.WriteString(footerDescStyle.Render(fmt.Sprintf("%-*s ", nodeWidth, nodeName)))
	b.WriteString(footerDescStyle.Render(fmt.Sprintf("%5s", pod.Age)))
	b.WriteString("\n")

	return b.String()
}
