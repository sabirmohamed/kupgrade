package check

import (
	"context"
	"fmt"
)

// Exit codes for CI pipeline integration.
const (
	ExitCodePass     = 0
	ExitCodeWarning  = 1
	ExitCodeBlocking = 2
)

// Report contains the aggregated results of all checkers.
type Report struct {
	Results  []Result
	ExitCode int
}

// Runner executes registered checkers and collects results.
type Runner struct {
	checkers []Checker
}

// NewRunner creates a new check runner.
func NewRunner() *Runner {
	return &Runner{}
}

// Register adds a checker to the runner.
func (r *Runner) Register(checker Checker) {
	r.checkers = append(r.checkers, checker)
}

// RunAll executes all registered checkers and returns a report.
// The exit code reflects the worst severity found.
func (r *Runner) RunAll(ctx context.Context, clients Clients, targetVersion string) (*Report, error) {
	report := &Report{
		ExitCode: ExitCodePass,
	}

	for _, checker := range r.checkers {
		results, err := checker.Run(ctx, clients, targetVersion)
		if err != nil {
			return nil, fmt.Errorf("check: %s: %w", checker.Name(), err)
		}

		for _, result := range results {
			report.Results = append(report.Results, result)

			switch result.Severity {
			case SeverityBlocking:
				report.ExitCode = ExitCodeBlocking
			case SeverityWarning:
				if report.ExitCode < ExitCodeWarning {
					report.ExitCode = ExitCodeWarning
				}
			}
		}
	}

	return report, nil
}
