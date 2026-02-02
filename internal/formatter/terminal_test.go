package formatter

import (
	"strings"
	"testing"
	"time"

	"github.com/sabirmohamed/kupgrade/internal/check"
	"github.com/sabirmohamed/kupgrade/internal/snapshot"
	"github.com/sabirmohamed/kupgrade/pkg/types"
)

func TestFormatCheckReportAllSeverities(t *testing.T) {
	report := &check.Report{
		Results: []check.Result{
			{CheckName: "Node Conditions", Severity: check.SeverityPass, Message: "All 3 nodes Ready"},
			{CheckName: "Deprecations", Severity: check.SeverityWarning, Message: "2 deprecated APIs found"},
			{CheckName: "PDB Health", Severity: check.SeverityBlocking, Message: "1 PDB blocking", Details: []string{"default/web-pdb: 0 disruptions allowed"}},
		},
		ExitCode: check.ExitCodeBlocking,
	}

	formatter := &TerminalFormatter{}
	output := formatter.FormatCheckReport(report)

	for _, result := range report.Results {
		if !strings.Contains(output, result.CheckName) {
			t.Errorf("output missing check name %q", result.CheckName)
		}
		if !strings.Contains(output, result.Message) {
			t.Errorf("output missing message %q", result.Message)
		}
	}

	if !strings.Contains(output, "default/web-pdb: 0 disruptions allowed") {
		t.Error("output missing detail line")
	}
	if !strings.Contains(output, iconPass) {
		t.Error("output missing pass icon")
	}
	if !strings.Contains(output, iconWarning) {
		t.Error("output missing warning icon")
	}
	if !strings.Contains(output, iconBlocking) {
		t.Error("output missing blocking icon")
	}
	if !strings.Contains(output, "Exit code: 2") {
		t.Error("output missing exit code")
	}
	if !strings.Contains(output, "blocking issues found") {
		t.Error("output missing exit code label")
	}
}

func TestFormatCheckReportEmpty(t *testing.T) {
	report := &check.Report{ExitCode: check.ExitCodePass}

	formatter := &TerminalFormatter{}
	output := formatter.FormatCheckReport(report)

	if !strings.Contains(output, "Exit code: 0") {
		t.Error("output missing exit code for empty report")
	}
	if !strings.Contains(output, "all checks passed") {
		t.Error("output missing pass label for empty report")
	}
}

func TestFormatDiffReportNewIssue(t *testing.T) {
	report := &snapshot.DiffReport{
		Summary: snapshot.DiffSummary{NewIssues: 1, Unchanged: 1},
		WorkloadDiffs: []snapshot.WorkloadDiff{
			{
				Namespace: "default", Kind: "Deployment", Name: "web",
				Category: snapshot.CategoryNewIssue,
				Before: &types.WorkloadSnapshot{
					Namespace: "default", Kind: "Deployment", Name: "web",
					DesiredReplicas: 3, ReadyReplicas: 3,
					PodStatuses: map[string]int{"Running": 3},
				},
				After: &types.WorkloadSnapshot{
					Namespace: "default", Kind: "Deployment", Name: "web",
					DesiredReplicas: 3, ReadyReplicas: 0,
					PodStatuses: map[string]int{"CrashLoopBackOff": 3},
				},
			},
			{
				Namespace: "default", Kind: "Deployment", Name: "api",
				Category: snapshot.CategoryUnchanged,
			},
		},
		BeforeTimestamp: time.Now().Add(-3 * time.Hour),
		AfterTimestamp:  time.Now(),
		BeforeVersion:   "v1.28.0",
		AfterVersion:    "v1.29.0",
		BeforeContext:   "prod",
		HasNewIssues:    true,
	}

	f := &TerminalFormatter{}
	output := f.FormatDiffReport(report, false)

	// Verify header.
	if !strings.Contains(output, "kupgrade report") {
		t.Error("missing report header")
	}
	if !strings.Contains(output, "prod") {
		t.Error("missing context")
	}
	if !strings.Contains(output, "v1.28.0") || !strings.Contains(output, "v1.29.0") {
		t.Error("missing version transition")
	}

	// Verify NEW_ISSUE section.
	if !strings.Contains(output, "[NEW_ISSUE]") {
		t.Error("missing NEW_ISSUE section header")
	}
	if !strings.Contains(output, "Deployment/web") {
		t.Error("missing workload in NEW_ISSUE section")
	}
	if !strings.Contains(output, "Before:") || !strings.Contains(output, "After:") {
		t.Error("missing before/after details")
	}

	// Verify verdict.
	if !strings.Contains(output, "New issues found") {
		t.Error("missing new issues verdict")
	}

	// UNCHANGED should NOT appear without --all.
	if strings.Contains(output, "[UNCHANGED]") {
		t.Error("UNCHANGED section should be hidden without --all")
	}
}

func TestFormatDiffReportShowAll(t *testing.T) {
	report := &snapshot.DiffReport{
		Summary: snapshot.DiffSummary{Unchanged: 2},
		WorkloadDiffs: []snapshot.WorkloadDiff{
			{Namespace: "default", Kind: "Deployment", Name: "api", Category: snapshot.CategoryUnchanged},
			{Namespace: "default", Kind: "Deployment", Name: "worker", Category: snapshot.CategoryUnchanged},
		},
		BeforeTimestamp: time.Now().Add(-1 * time.Hour),
		AfterTimestamp:  time.Now(),
		BeforeVersion:   "v1.28.0",
		AfterVersion:    "v1.28.0",
		BeforeContext:   "staging",
	}

	f := &TerminalFormatter{}
	output := f.FormatDiffReport(report, true)

	if !strings.Contains(output, "[UNCHANGED]") {
		t.Error("UNCHANGED section should appear with --all")
	}
	if !strings.Contains(output, "No new issues detected") {
		t.Error("missing no-issues verdict")
	}
}

func TestFormatDiffReportNodeChanges(t *testing.T) {
	report := &snapshot.DiffReport{
		NodeDiffs: []snapshot.NodeDiff{
			{Name: "node-1", Category: snapshot.NodeChanged, BeforeVersion: "v1.28.0", AfterVersion: "v1.29.0", ConditionChanges: []string{"version: v1.28.0 -> v1.29.0"}},
			{Name: "node-2", Category: snapshot.NodeRemoved, BeforeVersion: "v1.28.0"},
			{Name: "node-3", Category: snapshot.NodeAdded, AfterVersion: "v1.29.0"},
		},
		BeforeTimestamp: time.Now().Add(-2 * time.Hour),
		AfterTimestamp:  time.Now(),
		BeforeVersion:   "v1.28.0",
		AfterVersion:    "v1.29.0",
		BeforeContext:   "prod",
	}

	f := &TerminalFormatter{}
	output := f.FormatDiffReport(report, false)

	if !strings.Contains(output, "Node Changes") {
		t.Error("missing node changes section")
	}
	if !strings.Contains(output, "[ADDED]") {
		t.Error("missing ADDED node")
	}
	if !strings.Contains(output, "[REMOVED]") {
		t.Error("missing REMOVED node")
	}
	if !strings.Contains(output, "[CHANGED]") {
		t.Error("missing CHANGED node")
	}
}

func TestFormatElapsed(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{30 * time.Second, "just now"},
		{1 * time.Minute, "1 minute ago"},
		{45 * time.Minute, "45 minutes ago"},
		{1 * time.Hour, "1 hour ago"},
		{3 * time.Hour, "3 hours ago"},
		{24 * time.Hour, "1 day ago"},
		{72 * time.Hour, "3 days ago"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			result := formatElapsed(tc.duration)
			if result != tc.expected {
				t.Errorf("formatElapsed(%v) = %q, want %q", tc.duration, result, tc.expected)
			}
		})
	}
}
