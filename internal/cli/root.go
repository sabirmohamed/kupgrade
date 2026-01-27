package cli

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// Version is set at build time
var Version = "dev"

// NewRootCmd creates the root command
func NewRootCmd() *cobra.Command {
	configFlags := genericclioptions.NewConfigFlags(true)

	cmd := &cobra.Command{
		Use:   "kupgrade",
		Short: "Real-time Kubernetes upgrade observer",
		Long:  "kupgrade watches your Kubernetes cluster during upgrades, showing node stages, pod migrations, and events in real-time.",
	}

	// Add kubectl-compatible flags
	configFlags.AddFlags(cmd.PersistentFlags())

	// Add subcommands
	cmd.AddCommand(NewWatchCmd(configFlags))
	cmd.AddCommand(NewVersionCmd())

	return cmd
}

// Execute runs the root command
func Execute() error {
	return NewRootCmd().Execute()
}
