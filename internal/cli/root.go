package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func New(version string) *cobra.Command {
	var jsonOutput bool
	root := &cobra.Command{
		Use:   "conflow",
		Short: "Local-first ConfigOps workbench",
		Long: `Local-first ConfigOps workbench.

Automation exit codes: 0 success; 1 validation failure; 2 blocking validation;
3 conflict; 4 provider failure; 64 command usage failure.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newVersionCommand(version))
	root.AddCommand(newInitCommand())
	root.AddCommand(newValidateCommand())
	root.AddCommand(newPlanCommand())
	root.AddCommand(newSourceCommand())
	root.AddCommand(newGitCommand())
	root.AddCommand(newSaveCommand())
	root.AddCommand(newServeCommand())
	root.AddCommand(newProviderCommand())
	root.AddCommand(newPullCommand())
	root.AddCommand(newRemoteCommand())
	root.AddCommand(newPublishCommand())
	root.AddCommand(newReleaseCommand())
	root.AddCommand(newRollbackCommand())
	root.AddCommand(newDefaultsCommand())
	root.AddCommand(newProjectCommand())
	root.AddCommand(newEnvironmentCommand())
	root.PersistentFlags().BoolVar(&jsonOutput, "json", false, "write a stable JSON automation envelope to stdout")
	configureAutomation(root, &jsonOutput)
	root.SetHelpFunc(func(command *cobra.Command, args []string) {
		fmt.Fprint(command.OutOrStdout(), command.UsageString())
	})
	ensureExamples(root)
	return root
}
