package cli

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sabirmohamed/kupgrade/internal/kube"
	"github.com/sabirmohamed/kupgrade/internal/signals"
	"github.com/sabirmohamed/kupgrade/internal/tui"
	"github.com/sabirmohamed/kupgrade/internal/watcher"
	"github.com/spf13/cobra"
)

var targetVersion string

// NewWatchCmd creates the watch command
func NewWatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Watch cluster upgrade progress",
		Long:  "Watch your Kubernetes cluster in real-time during upgrades, showing node state changes, pod migrations, and events.",
		RunE:  runWatch,
	}

	cmd.Flags().StringVar(&targetVersion, "target-version", "", "Override auto-detected target version")

	return cmd
}

func runWatch(cmd *cobra.Command, args []string) error {
	// Setup signal handling
	ctx := signals.SetupSignalHandler()

	// Create Kubernetes client
	client, err := kube.NewClient(ConfigFlags)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Get server version
	serverVersion, err := client.GetServerVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to get server version: %w", err)
	}

	// Create watcher manager
	manager := watcher.NewManager(client.Factory, client.Namespace, targetVersion)

	// Start watchers
	if err := manager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start watchers: %w", err)
	}

	// Get detected target version
	detectedTarget := manager.StageComputer().GetTargetVersion()
	if detectedTarget == "" {
		detectedTarget = serverVersion
	}

	// Create and run TUI
	model := tui.New(ctx, manager.Events(), client.Context, serverVersion, detectedTarget)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	// Wait for cleanup
	manager.Wait()

	return nil
}
