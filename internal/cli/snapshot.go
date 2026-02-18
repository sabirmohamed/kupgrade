package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/sabirmohamed/kupgrade/internal/kube"
	"github.com/sabirmohamed/kupgrade/internal/snapshot"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// NewSnapshotCmd creates the snapshot command.
func NewSnapshotCmd(configFlags *genericclioptions.ConfigFlags) *cobra.Command {
	var snapshotDir string

	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Capture cluster workload state for upgrade comparison",
		Long: `Capture a point-in-time snapshot of all workloads, nodes, and PDBs in the cluster.

Run before upgrading to establish a baseline. After upgrading, use
'kupgrade report' to diff before/after and identify upgrade-caused issues.

Snapshots are saved to ~/.kupgrade/snapshots/ by default.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSnapshot(cmd.Context(), configFlags, snapshotDir)
		},
	}

	cmd.Flags().StringVar(&snapshotDir, "dir", "", "Override default snapshot directory (~/.kupgrade/snapshots)")

	return cmd
}

func runSnapshot(ctx context.Context, configFlags *genericclioptions.ConfigFlags, snapshotDir string) error {
	client, err := kube.NewClient(configFlags)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}

	if snapshotDir == "" {
		defaultDir, err := snapshot.DefaultDir()
		if err != nil {
			return fmt.Errorf("snapshot: %w", err)
		}
		snapshotDir = defaultDir
	}

	fmt.Printf("  Collecting cluster state from context %q...\n", client.Context)

	snap, warnings, err := snapshot.Collect(ctx, client.Clientset, client.Context)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}

	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "  warning: %s\n", warning)
	}

	path, err := snapshot.Save(snap, snapshotDir)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}

	// Count unique namespaces.
	namespaces := make(map[string]struct{})
	for _, workload := range snap.Workloads {
		namespaces[workload.Namespace] = struct{}{}
	}

	fmt.Printf("  Snapshot saved: %s\n", path)
	fmt.Printf("  %d workloads, %d nodes, %d PDBs across %d namespaces\n",
		len(snap.Workloads), len(snap.Nodes), len(snap.PDBs), len(namespaces))

	// Warn about PDBs that will block drains
	var blockingPDBs []string
	for _, pdb := range snap.PDBs {
		if pdb.WillBlockDrain {
			detail := pdb.Namespace + "/" + pdb.Name
			if pdb.MinAvailable != "" {
				detail += fmt.Sprintf(" (minAvailable=%s, %d/%d healthy)", pdb.MinAvailable, pdb.CurrentHealthy, pdb.ExpectedPods)
			}
			blockingPDBs = append(blockingPDBs, detail)
		}
	}
	if len(blockingPDBs) > 0 {
		fmt.Fprintf(os.Stderr, "  warning: %d PDB(s) will block drain:\n", len(blockingPDBs))
		for _, pdb := range blockingPDBs {
			fmt.Fprintf(os.Stderr, "    → %s\n", pdb)
		}
	}

	return nil
}
