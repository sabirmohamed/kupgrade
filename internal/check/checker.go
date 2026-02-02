package check

import (
	"context"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// Severity represents the severity level of a check result.
type Severity int

const (
	SeverityPass     Severity = iota
	SeverityWarning           // Non-blocking issue found
	SeverityBlocking          // Blocking issue — upgrade should not proceed
)

// String returns the human-readable name for a severity level.
func (s Severity) String() string {
	switch s {
	case SeverityPass:
		return "PASS"
	case SeverityWarning:
		return "WARN"
	case SeverityBlocking:
		return "FAIL"
	default:
		return "UNKNOWN"
	}
}

// Result represents the outcome of a single check.
type Result struct {
	CheckName string
	Severity  Severity
	Message   string
	Details   []string
}

// Clients bundles K8s API access for checkers. Includes dynamic client
// for the deprecation scanner (story 2-2) which needs arbitrary GVR listing.
type Clients struct {
	Kubernetes kubernetes.Interface
	Dynamic    dynamic.Interface
}

// Checker validates a cluster condition before upgrade.
type Checker interface {
	Name() string
	Run(ctx context.Context, clients Clients, targetVersion string) ([]Result, error)
}
