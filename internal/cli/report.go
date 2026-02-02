package cli

import (
	"context"
	"fmt"

	"github.com/sabirmohamed/kupgrade/internal/formatter"
	"github.com/sabirmohamed/kupgrade/internal/kube"
	"github.com/sabirmohamed/kupgrade/internal/snapshot"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// NewReportCmd creates the report command.
func NewReportCmd(configFlags *genericclioptions.ConfigFlags) *cobra.Command {
	var (
		snapshotFile string
		outputFormat string
		showAll      bool
	)

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Diff cluster state against pre-upgrade snapshot",
		Long: `Compare the current cluster state against a pre-upgrade snapshot to identify
issues caused by the upgrade.

Automatically detects the most recent snapshot for the current context.
Use --snapshot-file to specify a particular snapshot.

Exit codes: 0 = no new issues, 1 = new issues found.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			exitCode, err := runReport(cmd.Context(), configFlags, snapshotFile, outputFormat, showAll)
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

	cmd.Flags().StringVar(&snapshotFile, "snapshot-file", "", "Path to a specific snapshot file (default: auto-detect latest)")
	cmd.Flags().StringVar(&outputFormat, "format", "terminal", "Output format (terminal, json)")
	cmd.Flags().BoolVar(&showAll, "all", false, "Show unchanged workloads in output")

	return cmd
}

func runReport(ctx context.Context, configFlags *genericclioptions.ConfigFlags, snapshotFile string, outputFormat string, showAll bool) (int, error) {
	client, err := kube.NewClient(configFlags)
	if err != nil {
		return 0, fmt.Errorf("report: %w", err)
	}

	// Load "before" snapshot.
	beforePath := snapshotFile
	if beforePath == "" {
		snapshotDir, err := snapshot.DefaultDir()
		if err != nil {
			return 0, fmt.Errorf("report: %w", err)
		}

		beforePath, err = snapshot.FindLatest(snapshotDir, client.Context)
		if err != nil {
			fmt.Println("No snapshot found. Run kupgrade snapshot before upgrading.")
			return 0, nil
		}
	}

	before, err := snapshot.Load(beforePath)
	if err != nil {
		return 0, fmt.Errorf("report: load snapshot: %w", err)
	}

	// Collect "after" state live from cluster.
	after, _, err := snapshot.Collect(ctx, client.Clientset, client.Context)
	if err != nil {
		return 0, fmt.Errorf("report: collect live state: %w", err)
	}

	// Run differ.
	report := snapshot.Diff(before, after)

	// Format and print.
	output := formatter.New(outputFormat)
	fmt.Print(output.FormatDiffReport(report, showAll))

	// Exit code: 0 = no new issues, 1 = new issues found.
	if report.HasNewIssues {
		return 1, nil
	}
	return 0, nil
}
