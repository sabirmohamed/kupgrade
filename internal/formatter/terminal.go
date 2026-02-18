package formatter

import (
	"fmt"
	"strings"
	"time"

	"github.com/sabirmohamed/kupgrade/internal/check"
	"github.com/sabirmohamed/kupgrade/internal/snapshot"
)

// Compile-time interface check.
var _ Formatter = (*TerminalFormatter)(nil)

const (
	iconPass     = "\u2705"       // green check
	iconWarning  = "\u26a0\ufe0f" // warning sign
	iconBlocking = "\u274c"       // red X
	iconNew      = "\u2757"       // red exclamation
	iconResolved = "\u2728"       // sparkles
	iconRemoved  = "\U0001f5d1"   // wastebasket
	iconInfo     = "\u2139\ufe0f" // info

	firstElemPrefix = "├─"
	lastElemPrefix  = "└─"
)

// TerminalFormatter renders check and diff results as styled terminal output.
type TerminalFormatter struct{}

// FormatCheckReport renders a check report for terminal display.
func (f *TerminalFormatter) FormatCheckReport(report *check.Report) string {
	var builder strings.Builder

	builder.WriteString("\n")
	builder.WriteString("  kupgrade check results\n")
	builder.WriteString("  " + strings.Repeat("─", 40) + "\n")

	for _, result := range report.Results {
		icon := severityIcon(result.Severity)
		builder.WriteString(fmt.Sprintf("  %s  %-20s %s\n", icon, result.CheckName, result.Message))

		for _, detail := range result.Details {
			builder.WriteString(fmt.Sprintf("       └─ %s\n", detail))
		}
	}

	builder.WriteString("  " + strings.Repeat("─", 40) + "\n")
	builder.WriteString(fmt.Sprintf("  Exit code: %d (%s)\n\n", report.ExitCode, exitCodeLabel(report.ExitCode)))

	return builder.String()
}

// FormatDiffReport renders a diff report for terminal display.
func (f *TerminalFormatter) FormatDiffReport(report *snapshot.DiffReport, showAll bool) string {
	var builder strings.Builder

	// Header.
	builder.WriteString("\n")
	builder.WriteString("  kupgrade report\n")
	builder.WriteString("  " + strings.Repeat("─", 60) + "\n")
	builder.WriteString(fmt.Sprintf("  Context:  %s\n", report.BeforeContext))
	builder.WriteString(fmt.Sprintf("  Version:  %s → %s\n", report.BeforeVersion, report.AfterVersion))
	builder.WriteString(fmt.Sprintf("  Snapshot: %s (%s)\n",
		report.BeforeTimestamp.Format("2006-01-02T15:04:05"),
		formatElapsed(report.AfterTimestamp.Sub(report.BeforeTimestamp))))
	builder.WriteString("  " + strings.Repeat("─", 60) + "\n")

	// Summary.
	builder.WriteString("\n  Summary\n")
	summary := report.Summary
	if summary.NewIssues > 0 {
		builder.WriteString(fmt.Sprintf("    %s  NEW_ISSUE:     %d\n", iconBlocking, summary.NewIssues))
	} else {
		builder.WriteString(fmt.Sprintf("    %s  NEW_ISSUE:     %d\n", iconPass, summary.NewIssues))
	}
	if summary.PreExisting > 0 {
		builder.WriteString(fmt.Sprintf("    %s  PRE_EXISTING:  %d\n", iconWarning, summary.PreExisting))
	}
	if summary.Resolved > 0 {
		builder.WriteString(fmt.Sprintf("    %s  RESOLVED:      %d\n", iconResolved, summary.Resolved))
	}
	if summary.Removed > 0 {
		builder.WriteString(fmt.Sprintf("    %s  REMOVED:       %d\n", iconRemoved, summary.Removed))
	}
	if summary.NewWorkloads > 0 {
		builder.WriteString(fmt.Sprintf("    %s  NEW_WORKLOAD:  %d\n", iconNew, summary.NewWorkloads))
	}
	if showAll || summary.Unchanged > 0 {
		builder.WriteString(fmt.Sprintf("    %s  UNCHANGED:     %d\n", iconInfo, summary.Unchanged))
	}
	builder.WriteString("\n")

	// Group workload diffs by category.
	grouped := groupByCategory(report.WorkloadDiffs)

	// Define category sections in display order.
	categorySections := []categorySection{
		{snapshot.CategoryNewIssue, "NEW_ISSUE", "investigate, likely caused by upgrade", renderFull, true},
		{snapshot.CategoryPreExisting, "PRE_EXISTING", "already broken before upgrade", renderFull, true},
		{snapshot.CategoryResolved, "RESOLVED", "was broken, now healthy", renderFull, true},
		{snapshot.CategoryRemoved, "REMOVED", "existed before, gone now", renderSimple, true},
		{snapshot.CategoryNewWorkload, "NEW_WORKLOAD", "new and unhealthy, may be unrelated to upgrade", renderAfterOnly, true},
		{snapshot.CategoryUnchanged, "UNCHANGED", "healthy before and after", renderSimple, showAll},
	}

	// Render each category section.
	for _, section := range categorySections {
		if !section.show {
			continue
		}
		items, ok := grouped[section.category]
		if !ok {
			continue
		}
		writeCategorySection(&builder, section.title, section.description, items, section.renderer)
	}

	// PDB changes section.
	if len(report.PDBDiffs) > 0 {
		builder.WriteString("  PDB Changes\n")
		builder.WriteString("  " + strings.Repeat("─", 60) + "\n")
		for i, diff := range report.PDBDiffs {
			prefix := firstElemPrefix
			if i == len(report.PDBDiffs)-1 {
				prefix = lastElemPrefix
			}
			name := diff.Namespace + "/" + diff.Name
			switch diff.Category {
			case snapshot.PDBWillBlock:
				detail := ""
				if diff.After != nil {
					if diff.After.MinAvailable != "" {
						detail = fmt.Sprintf("minAvailable=%s, %d/%d healthy", diff.After.MinAvailable, diff.After.CurrentHealthy, diff.After.ExpectedPods)
					} else if diff.After.MaxUnavailable != "" {
						detail = fmt.Sprintf("maxUnavailable=%s, %d/%d healthy", diff.After.MaxUnavailable, diff.After.CurrentHealthy, diff.After.ExpectedPods)
					}
				}
				builder.WriteString(fmt.Sprintf("    %s %s [WILL_BLOCK] %s\n", prefix, iconWarning, name))
				if detail != "" {
					builder.WriteString(fmt.Sprintf("         %s\n", detail))
				}
			case snapshot.PDBResolved:
				builder.WriteString(fmt.Sprintf("    %s %s [RESOLVED] %s\n", prefix, iconResolved, name))
			}
		}
		builder.WriteString("\n")
	}

	// Node changes section.
	if len(report.NodeDiffs) > 0 {
		builder.WriteString("  Node Changes\n")
		builder.WriteString("  " + strings.Repeat("─", 60) + "\n")
		for i, diff := range report.NodeDiffs {
			prefix := firstElemPrefix
			if i == len(report.NodeDiffs)-1 {
				prefix = lastElemPrefix
			}
			switch diff.Category {
			case snapshot.NodeAdded:
				builder.WriteString(fmt.Sprintf("    %s [ADDED] %s (%s)\n", prefix, diff.Name, diff.AfterVersion))
			case snapshot.NodeRemoved:
				builder.WriteString(fmt.Sprintf("    %s [REMOVED] %s (%s)\n", prefix, diff.Name, diff.BeforeVersion))
			case snapshot.NodeChanged:
				builder.WriteString(fmt.Sprintf("    %s [CHANGED] %s\n", prefix, diff.Name))
				for _, change := range diff.ConditionChanges {
					builder.WriteString(fmt.Sprintf("         %s\n", change))
				}
			}
		}
		builder.WriteString("\n")
	}

	// Final verdict.
	if report.HasNewIssues {
		builder.WriteString(fmt.Sprintf("  %s  New issues found — investigate before proceeding\n\n", iconBlocking))
	} else {
		builder.WriteString(fmt.Sprintf("  %s  No new issues detected\n\n", iconPass))
	}

	return builder.String()
}

// categoryRenderer defines how to render items within a category section.
type categoryRenderer int

const (
	renderFull      categoryRenderer = iota // Before/After with pod status details.
	renderAfterOnly                         // After-only with pod status details.
	renderSimple                            // Just workload identifier, no details.
)

// categorySection defines a category's display configuration.
type categorySection struct {
	category    snapshot.DiffCategory
	title       string
	description string
	renderer    categoryRenderer
	show        bool
}

// writeCategorySection writes a category header and its items using the specified renderer.
func writeCategorySection(builder *strings.Builder, title, description string, items []snapshot.WorkloadDiff, renderer categoryRenderer) {
	fmt.Fprintf(builder, "  [%s] — %s\n", title, description)
	builder.WriteString("  " + strings.Repeat("─", 60) + "\n")
	for _, diff := range items {
		switch renderer {
		case renderFull:
			writeWorkloadDiff(builder, &diff)
		case renderAfterOnly:
			writeWorkloadDiffAfterOnly(builder, &diff)
		case renderSimple:
			fmt.Fprintf(builder, "    %s/%s (%s)\n", diff.Kind, diff.Name, diff.Namespace)
		}
	}
	builder.WriteString("\n")
}

func writeWorkloadDiff(builder *strings.Builder, diff *snapshot.WorkloadDiff) {
	fmt.Fprintf(builder, "    %s/%s (%s)\n", diff.Kind, diff.Name, diff.Namespace)
	if diff.Before != nil {
		fmt.Fprintf(builder, "      %s Before: %d/%d ready  %s\n",
			firstElemPrefix,
			diff.Before.ReadyReplicas, diff.Before.DesiredReplicas,
			snapshot.PodStatusSummary(diff.Before.PodStatuses))
	}
	if diff.After != nil {
		fmt.Fprintf(builder, "      %s After:  %d/%d ready  %s\n",
			lastElemPrefix,
			diff.After.ReadyReplicas, diff.After.DesiredReplicas,
			snapshot.PodStatusSummary(diff.After.PodStatuses))
	}
}

func writeWorkloadDiffAfterOnly(builder *strings.Builder, diff *snapshot.WorkloadDiff) {
	fmt.Fprintf(builder, "    %s/%s (%s)\n", diff.Kind, diff.Name, diff.Namespace)
	if diff.After != nil {
		fmt.Fprintf(builder, "      %s %d/%d ready  %s\n",
			lastElemPrefix,
			diff.After.ReadyReplicas, diff.After.DesiredReplicas,
			snapshot.PodStatusSummary(diff.After.PodStatuses))
	}
}

func groupByCategory(diffs []snapshot.WorkloadDiff) map[snapshot.DiffCategory][]snapshot.WorkloadDiff {
	grouped := make(map[snapshot.DiffCategory][]snapshot.WorkloadDiff)
	for _, diff := range diffs {
		grouped[diff.Category] = append(grouped[diff.Category], diff)
	}
	return grouped
}

func formatElapsed(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		minutes := int(d.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

func severityIcon(severity check.Severity) string {
	switch severity {
	case check.SeverityPass:
		return iconPass
	case check.SeverityWarning:
		return iconWarning
	case check.SeverityBlocking:
		return iconBlocking
	default:
		return "?"
	}
}

func exitCodeLabel(code int) string {
	switch code {
	case check.ExitCodePass:
		return "all checks passed"
	case check.ExitCodeWarning:
		return "warnings found"
	case check.ExitCodeBlocking:
		return "blocking issues found"
	default:
		return "unknown"
	}
}
