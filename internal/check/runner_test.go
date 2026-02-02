package check

import (
	"context"
	"errors"
	"testing"
)

const (
	testCheckerName   = "test-checker"
	testTargetVersion = "v1.32.0"
)

// stubChecker implements Checker for testing.
type stubChecker struct {
	name    string
	results []Result
	err     error
}

func (s *stubChecker) Name() string { return s.name }

func (s *stubChecker) Run(_ context.Context, _ Clients, _ string) ([]Result, error) {
	return s.results, s.err
}

func TestRunnerRunAll(t *testing.T) {
	tests := []struct {
		name         string
		checkers     []Checker
		wantExitCode int
		wantResults  int
		wantErr      bool
	}{
		{
			name:         "no checkers returns pass",
			checkers:     nil,
			wantExitCode: ExitCodePass,
			wantResults:  0,
		},
		{
			name: "single passing checker",
			checkers: []Checker{
				&stubChecker{
					name:    testCheckerName,
					results: []Result{{CheckName: testCheckerName, Severity: SeverityPass, Message: "ok"}},
				},
			},
			wantExitCode: ExitCodePass,
			wantResults:  1,
		},
		{
			name: "warning raises exit code to 1",
			checkers: []Checker{
				&stubChecker{
					name:    "warn-checker",
					results: []Result{{CheckName: "warn-checker", Severity: SeverityWarning, Message: "caution"}},
				},
			},
			wantExitCode: ExitCodeWarning,
			wantResults:  1,
		},
		{
			name: "blocking raises exit code to 2",
			checkers: []Checker{
				&stubChecker{
					name:    "block-checker",
					results: []Result{{CheckName: "block-checker", Severity: SeverityBlocking, Message: "stop"}},
				},
			},
			wantExitCode: ExitCodeBlocking,
			wantResults:  1,
		},
		{
			name: "blocking overrides warning",
			checkers: []Checker{
				&stubChecker{
					name:    "warn-checker",
					results: []Result{{CheckName: "warn-checker", Severity: SeverityWarning, Message: "caution"}},
				},
				&stubChecker{
					name:    "block-checker",
					results: []Result{{CheckName: "block-checker", Severity: SeverityBlocking, Message: "stop"}},
				},
			},
			wantExitCode: ExitCodeBlocking,
			wantResults:  2,
		},
		{
			name: "multiple results from one checker",
			checkers: []Checker{
				&stubChecker{
					name: "multi-checker",
					results: []Result{
						{CheckName: "multi-checker", Severity: SeverityPass, Message: "ok"},
						{CheckName: "multi-checker", Severity: SeverityWarning, Message: "caution"},
					},
				},
			},
			wantExitCode: ExitCodeWarning,
			wantResults:  2,
		},
		{
			name: "checker error propagates",
			checkers: []Checker{
				&stubChecker{
					name: "err-checker",
					err:  errors.New("api unavailable"),
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := NewRunner()
			for _, checker := range tt.checkers {
				runner.Register(checker)
			}

			report, err := runner.RunAll(context.Background(), Clients{}, testTargetVersion)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if report.ExitCode != tt.wantExitCode {
				t.Errorf("exit code = %d, want %d", report.ExitCode, tt.wantExitCode)
			}
			if len(report.Results) != tt.wantResults {
				t.Errorf("results count = %d, want %d", len(report.Results), tt.wantResults)
			}
		})
	}
}

func TestSeverityString(t *testing.T) {
	tests := []struct {
		severity Severity
		want     string
	}{
		{SeverityPass, "PASS"},
		{SeverityWarning, "WARN"},
		{SeverityBlocking, "FAIL"},
		{Severity(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.severity.String(); got != tt.want {
				t.Errorf("Severity.String() = %q, want %q", got, tt.want)
			}
		})
	}
}
