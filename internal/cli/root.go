package cli

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var (
	// ConfigFlags provides kubectl-compatible flags
	ConfigFlags *genericclioptions.ConfigFlags

	// Version is set at build time
	Version = "dev"
)

// NewRootCmd creates the root command
func NewRootCmd() *cobra.Command {
	ConfigFlags = genericclioptions.NewConfigFlags(true)

	cmd := &cobra.Command{
		Use:   "kupgrade",
		Short: "Real-time Kubernetes upgrade observer",
		Long:  "kupgrade watches your Kubernetes cluster during upgrades, showing node stages, pod migrations, and events in real-time.",
	}

	// Add kubectl-compatible flags
	ConfigFlags.AddFlags(cmd.PersistentFlags())

	// Add subcommands
	cmd.AddCommand(NewWatchCmd())
	cmd.AddCommand(NewVersionCmd())

	return cmd
}

// Execute runs the root command
func Execute() error {
	return NewRootCmd().Execute()
}
