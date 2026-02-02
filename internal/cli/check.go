package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/sabirmohamed/kupgrade/internal/check"
	"github.com/sabirmohamed/kupgrade/internal/formatter"
	"github.com/sabirmohamed/kupgrade/internal/kube"
	"github.com/sabirmohamed/kupgrade/internal/snapshot"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// ExitCodeError wraps an exit code for propagation to main().
// Cobra only supports exit code 0 (success) or 1 (error). This allows
// the check command to signal semantic exit codes (1=warning, 2=blocking)
// without calling os.Exit inside RunE, which would bypass deferred cleanup.
type ExitCodeError struct {
	Code int
}

func (e *ExitCodeError) Error() string {
	return fmt.Sprintf("exit code %d", e.Code)
}

// NewCheckCmd creates the check command.
func NewCheckCmd(configFlags *genericclioptions.ConfigFlags) *cobra.Command {
	var (
		targetVersion string
		takeSnapshot  bool
		snapshotDir   string
		outputFormat  string
	)

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate cluster readiness for upgrade",
		Long:  "Run pre-upgrade checks against the cluster and optionally capture a workload snapshot for post-upgrade comparison.",
		RunE: func(cmd *cobra.Command, args []string) error {
			exitCode, err := runCheck(cmd.Context(), configFlags, targetVersion, takeSnapshot, snapshotDir, outputFormat)
			if err != nil {
				return err
			}
			if exitCode != 0 {
				return &ExitCodeError{Code: exitCode}
			}
			return nil
		},
	}

	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	cmd.Flags().StringVar(&targetVersion, "target-version", "", "Target Kubernetes version for version-aware checks")
	cmd.Flags().BoolVar(&takeSnapshot, "snapshot", false, "Capture workload snapshot for post-upgrade comparison")
	cmd.Flags().StringVar(&snapshotDir, "snapshot-dir", "", "Override default snapshot directory (~/.kupgrade/snapshots)")
	cmd.Flags().StringVar(&outputFormat, "format", "terminal", "Output format (terminal)")

	return cmd
}

func runCheck(ctx context.Context, configFlags *genericclioptions.ConfigFlags, targetVersion string, takeSnapshot bool, snapshotDir string, outputFormat string) (int, error) {
	client, err := kube.NewClient(configFlags)
	if err != nil {
		return 0, fmt.Errorf("check: %w", err)
	}

	dynamicClient, err := kube.NewDynamicClient(configFlags)
	if err != nil {
		return 0, fmt.Errorf("check: %w", err)
	}

	clients := check.Clients{
		Kubernetes: client.Clientset,
		Dynamic:    dynamicClient,
	}

	// Build and register checkers.
	runner := check.NewRunner()
	runner.Register(&check.NodeConditionsChecker{})

	// Run all checks.
	report, err := runner.RunAll(ctx, clients, targetVersion)
	if err != nil {
		return 0, fmt.Errorf("check: %w", err)
	}

	// Format and print results.
	output := formatter.New(outputFormat)
	fmt.Print(output.FormatReport(report))

	// Capture snapshot regardless of check results — SREs want baselines
	// even with blocking issues for before/after comparison.
	if takeSnapshot {
		if err := captureSnapshot(ctx, client, snapshotDir); err != nil {
			return 0, fmt.Errorf("check: %w", err)
		}
	}

	return report.ExitCode, nil
}

func captureSnapshot(ctx context.Context, client *kube.Client, snapshotDir string) error {
	if snapshotDir == "" {
		defaultDir, err := snapshot.DefaultDir()
		if err != nil {
			return fmt.Errorf("snapshot: %w", err)
		}
		snapshotDir = defaultDir
	}

	snap, warnings, err := snapshot.Collect(ctx, client.Clientset, client.Context)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}

	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "  warning: %s\n", warning)
	}

	path, err := snapshot.Save(snap, snapshotDir)
	if err != nil {
		return err
	}

	// Count unique namespaces.
	namespaces := make(map[string]struct{})
	for _, workload := range snap.Workloads {
		namespaces[workload.Namespace] = struct{}{}
	}

	fmt.Printf("  Snapshot saved: %s — %d workloads across %d namespaces\n", path, len(snap.Workloads), len(namespaces))
	return nil
}
