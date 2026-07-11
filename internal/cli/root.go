package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func New(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "conflow",
		Short:         "Local-first ConfigOps workbench",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newVersionCommand(version))
	root.AddCommand(newInitCommand())
	root.AddCommand(newValidateCommand())
	root.AddCommand(newPlanCommand())
	root.AddCommand(newSourceCommand())
	root.AddCommand(newSaveCommand())
	root.AddCommand(newServeCommand())
	root.AddCommand(newProviderCommand())
	root.AddCommand(newPullCommand())
	root.AddCommand(newRemoteCommand())
	root.SetHelpFunc(func(command *cobra.Command, args []string) {
		fmt.Fprint(command.OutOrStdout(), command.UsageString())
	})
	return root
}
