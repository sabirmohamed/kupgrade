package formatter

import (
	"strings"
	"testing"

	"github.com/sabirmohamed/kupgrade/internal/check"
)

func TestFormatReportAllSeverities(t *testing.T) {
	report := &check.Report{
		Results: []check.Result{
			{CheckName: "Node Conditions", Severity: check.SeverityPass, Message: "All 3 nodes Ready"},
			{CheckName: "Deprecations", Severity: check.SeverityWarning, Message: "2 deprecated APIs found"},
			{CheckName: "PDB Health", Severity: check.SeverityBlocking, Message: "1 PDB blocking", Details: []string{"default/web-pdb: 0 disruptions allowed"}},
		},
		ExitCode: check.ExitCodeBlocking,
	}

	formatter := &TerminalFormatter{}
	output := formatter.FormatReport(report)

	// Verify all check names appear.
	for _, result := range report.Results {
		if !strings.Contains(output, result.CheckName) {
			t.Errorf("output missing check name %q", result.CheckName)
		}
		if !strings.Contains(output, result.Message) {
			t.Errorf("output missing message %q", result.Message)
		}
	}

	// Verify detail lines render.
	if !strings.Contains(output, "default/web-pdb: 0 disruptions allowed") {
		t.Error("output missing detail line")
	}

	// Verify severity icons.
	if !strings.Contains(output, iconPass) {
		t.Error("output missing pass icon")
	}
	if !strings.Contains(output, iconWarning) {
		t.Error("output missing warning icon")
	}
	if !strings.Contains(output, iconBlocking) {
		t.Error("output missing blocking icon")
	}

	// Verify exit code line.
	if !strings.Contains(output, "Exit code: 2") {
		t.Error("output missing exit code")
	}
	if !strings.Contains(output, "blocking issues found") {
		t.Error("output missing exit code label")
	}
}

func TestFormatReportEmpty(t *testing.T) {
	report := &check.Report{
		ExitCode: check.ExitCodePass,
	}

	formatter := &TerminalFormatter{}
	output := formatter.FormatReport(report)

	if !strings.Contains(output, "Exit code: 0") {
		t.Error("output missing exit code for empty report")
	}
	if !strings.Contains(output, "all checks passed") {
		t.Error("output missing pass label for empty report")
	}
}
