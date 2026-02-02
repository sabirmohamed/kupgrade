package formatter

import "github.com/sabirmohamed/kupgrade/internal/check"

// Formatter renders check results for output.
type Formatter interface {
	FormatReport(report *check.Report) string
}

// New creates a formatter for the given format name.
// Currently only "terminal" is supported; JSON will be added in story 2-3.
func New(format string) Formatter {
	switch format {
	default:
		return &TerminalFormatter{}
	}
}
