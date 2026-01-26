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
	ctx := signals.SetupSignalHandler()

	client, err := kube.NewClient(ConfigFlags)
	if err != nil {
		return fmt.Errorf("cli: %w", err)
	}

	serverVersion, err := client.ServerVersion(ctx)
	if err != nil {
		return fmt.Errorf("cli: %w", err)
	}

	manager := watcher.NewManager(client.Factory, client.Namespace, targetVersion)

	if err := manager.Start(ctx); err != nil {
		return fmt.Errorf("cli: %w", err)
	}

	stageComputer := manager.StageComputer()
	detectedTarget := stageComputer.TargetVersion()
	lowestVersion := stageComputer.LowestVersion()

	// Use lowest node version as "from", highest as "to"
	if lowestVersion == "" {
		lowestVersion = serverVersion
	}
	if detectedTarget == "" {
		detectedTarget = serverVersion
	}

	model := tui.New(tui.Config{
		Context:         client.Context,
		ServerVersion:   lowestVersion,
		TargetVersion:   detectedTarget,
		InitialNodes:    manager.InitialNodeStates(),
		InitialPods:     manager.InitialPodStates(),
		InitialBlockers: manager.InitialBlockers(),
		EventCh:         manager.Events(),
		NodeStateCh:     manager.NodeStateUpdates(),
		PodStateCh:      manager.PodStateUpdates(),
		BlockerCh:       manager.BlockerUpdates(),
	})

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("cli: TUI error: %w", err)
	}

	manager.Wait()

	return nil
}
