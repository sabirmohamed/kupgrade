package formatter

import (
	"github.com/sabirmohamed/kupgrade/internal/check"
	"github.com/sabirmohamed/kupgrade/internal/snapshot"
)

// Formatter renders check and diff results for output.
type Formatter interface {
	FormatCheckReport(report *check.Report) string
	FormatDiffReport(report *snapshot.DiffReport, showAll bool) string
}

// New creates a formatter for the given format name.
func New(format string) Formatter {
	switch format {
	case "json":
		return &JSONFormatter{}
	default:
		return &TerminalFormatter{}
	}
}
