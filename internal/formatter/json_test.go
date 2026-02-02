package formatter

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sabirmohamed/kupgrade/internal/check"
	"github.com/sabirmohamed/kupgrade/internal/snapshot"
	"github.com/sabirmohamed/kupgrade/pkg/types"
)

func TestJSONFormatCheckReport(t *testing.T) {
	report := &check.Report{
		Results: []check.Result{
			{CheckName: "Node Conditions", Severity: check.SeverityPass, Message: "All 3 nodes Ready"},
			{CheckName: "PDB Health", Severity: check.SeverityBlocking, Message: "1 PDB blocking", Details: []string{"detail1"}},
		},
		ExitCode: check.ExitCodeBlocking,
	}

	f := &JSONFormatter{}
	output := f.FormatCheckReport(report)

	// Verify it's valid JSON.
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, output)
	}

	// Verify structure.
	if parsed["exitCode"].(float64) != 2 {
		t.Errorf("expected exitCode=2, got %v", parsed["exitCode"])
	}

	results, ok := parsed["results"].([]interface{})
	if !ok || len(results) != 2 {
		t.Fatalf("expected 2 results, got %v", parsed["results"])
	}
}

func TestJSONFormatDiffReport(t *testing.T) {
	report := &snapshot.DiffReport{
		Summary: snapshot.DiffSummary{
			NewIssues:   1,
			PreExisting: 1,
			Unchanged:   2,
		},
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
				Namespace: "kube-system", Kind: "DaemonSet", Name: "kube-proxy",
				Category: snapshot.CategoryPreExisting,
			},
			{
				Namespace: "default", Kind: "Deployment", Name: "api",
				Category: snapshot.CategoryUnchanged,
			},
			{
				Namespace: "default", Kind: "Deployment", Name: "worker",
				Category: snapshot.CategoryUnchanged,
			},
		},
		NodeDiffs:       []snapshot.NodeDiff{},
		BeforeTimestamp: time.Now().Add(-3 * time.Hour),
		AfterTimestamp:  time.Now(),
		BeforeVersion:   "v1.28.0",
		AfterVersion:    "v1.29.0",
		BeforeContext:   "prod",
		HasNewIssues:    true,
	}

	f := &JSONFormatter{}

	// Without --all: unchanged should be omitted.
	output := f.FormatDiffReport(report, false)
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if _, ok := parsed["unchanged"]; ok {
		t.Error("unchanged should be omitted when showAll=false")
	}

	newIssues, ok := parsed["newIssues"].([]interface{})
	if !ok || len(newIssues) != 1 {
		t.Errorf("expected 1 newIssue, got %v", parsed["newIssues"])
	}

	if !parsed["hasNewIssues"].(bool) {
		t.Error("expected hasNewIssues=true")
	}

	// With --all: unchanged should be present.
	outputAll := f.FormatDiffReport(report, true)
	var parsedAll map[string]interface{}
	if err := json.Unmarshal([]byte(outputAll), &parsedAll); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	unchanged, ok := parsedAll["unchanged"].([]interface{})
	if !ok || len(unchanged) != 2 {
		t.Errorf("expected 2 unchanged, got %v", parsedAll["unchanged"])
	}
}

func TestJSONFormatDiffReportEmptyArrays(t *testing.T) {
	report := &snapshot.DiffReport{
		BeforeTimestamp: time.Now(),
		AfterTimestamp:  time.Now(),
	}

	f := &JSONFormatter{}
	output := f.FormatDiffReport(report, false)

	// All arrays should be [] not null.
	if strings.Contains(output, ": null") {
		t.Error("JSON output contains null arrays — should be empty []")
	}
}
