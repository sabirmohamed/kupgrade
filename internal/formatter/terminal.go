package formatter

import (
	"fmt"
	"strings"

	"github.com/sabirmohamed/kupgrade/internal/check"
)

// Compile-time interface check.
var _ Formatter = (*TerminalFormatter)(nil)

const (
	iconPass     = "\u2705"       // green check
	iconWarning  = "\u26a0\ufe0f" // warning sign
	iconBlocking = "\u274c"       // red X
)

// TerminalFormatter renders check results as styled terminal output.
type TerminalFormatter struct{}

// FormatReport renders a check report for terminal display.
func (f *TerminalFormatter) FormatReport(report *check.Report) string {
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
