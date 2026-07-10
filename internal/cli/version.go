package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCommand(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the Conflow version",
		Run: func(command *cobra.Command, args []string) {
			fmt.Fprintln(command.OutOrStdout(), version)
		},
	}
}
