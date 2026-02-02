package formatter

import (
	"encoding/json"

	"github.com/sabirmohamed/kupgrade/internal/check"
	"github.com/sabirmohamed/kupgrade/internal/snapshot"
)

// Compile-time interface check.
var _ Formatter = (*JSONFormatter)(nil)

// JSONFormatter renders check and diff results as JSON.
type JSONFormatter struct{}

// checkReportJSON is the JSON structure for check results.
type checkReportJSON struct {
	ExitCode int               `json:"exitCode"`
	Results  []checkResultJSON `json:"results"`
}

type checkResultJSON struct {
	CheckName string   `json:"checkName"`
	Severity  string   `json:"severity"`
	Message   string   `json:"message"`
	Details   []string `json:"details,omitempty"`
}

// FormatCheckReport renders check results as JSON.
func (f *JSONFormatter) FormatCheckReport(report *check.Report) string {
	output := checkReportJSON{
		ExitCode: report.ExitCode,
		Results:  make([]checkResultJSON, 0, len(report.Results)),
	}

	for _, result := range report.Results {
		output.Results = append(output.Results, checkResultJSON{
			CheckName: result.CheckName,
			Severity:  result.Severity.String(),
			Message:   result.Message,
			Details:   result.Details,
		})
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "{\"error\": \"failed to marshal check report\"}\n"
	}
	return string(data) + "\n"
}

// diffReportJSON is the JSON structure for diff reports.
type diffReportJSON struct {
	Summary       snapshot.DiffSummary    `json:"summary"`
	NewIssues     []snapshot.WorkloadDiff `json:"newIssues"`
	PreExisting   []snapshot.WorkloadDiff `json:"preExisting"`
	Resolved      []snapshot.WorkloadDiff `json:"resolved"`
	Removed       []snapshot.WorkloadDiff `json:"removed"`
	NewWorkloads  []snapshot.WorkloadDiff `json:"newWorkloads"`
	Unchanged     []snapshot.WorkloadDiff `json:"unchanged,omitempty"`
	NodeChanges   []snapshot.NodeDiff     `json:"nodeChanges"`
	BeforeVersion string                  `json:"beforeVersion"`
	AfterVersion  string                  `json:"afterVersion"`
	BeforeContext string                  `json:"beforeContext"`
	HasNewIssues  bool                    `json:"hasNewIssues"`
}

// FormatDiffReport renders a diff report as JSON.
func (f *JSONFormatter) FormatDiffReport(report *snapshot.DiffReport, showAll bool) string {
	grouped := make(map[snapshot.DiffCategory][]snapshot.WorkloadDiff)
	for _, diff := range report.WorkloadDiffs {
		grouped[diff.Category] = append(grouped[diff.Category], diff)
	}

	output := diffReportJSON{
		Summary:       report.Summary,
		NewIssues:     emptyIfNil(grouped[snapshot.CategoryNewIssue]),
		PreExisting:   emptyIfNil(grouped[snapshot.CategoryPreExisting]),
		Resolved:      emptyIfNil(grouped[snapshot.CategoryResolved]),
		Removed:       emptyIfNil(grouped[snapshot.CategoryRemoved]),
		NewWorkloads:  emptyIfNil(grouped[snapshot.CategoryNewWorkload]),
		NodeChanges:   report.NodeDiffs,
		BeforeVersion: report.BeforeVersion,
		AfterVersion:  report.AfterVersion,
		BeforeContext: report.BeforeContext,
		HasNewIssues:  report.HasNewIssues,
	}

	if showAll {
		output.Unchanged = emptyIfNil(grouped[snapshot.CategoryUnchanged])
	}

	if output.NodeChanges == nil {
		output.NodeChanges = []snapshot.NodeDiff{}
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "{\"error\": \"failed to marshal diff report\"}\n"
	}
	return string(data) + "\n"
}

func emptyIfNil(diffs []snapshot.WorkloadDiff) []snapshot.WorkloadDiff {
	if diffs == nil {
		return []snapshot.WorkloadDiff{}
	}
	return diffs
}
