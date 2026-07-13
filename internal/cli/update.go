package cli

import (
	"context"

	"github.com/ConteMan/conflow/internal/update"
	"github.com/spf13/cobra"
)

func newUpdateCommand(info BuildInfo) *cobra.Command {
	var checkOnly bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update conflow to the latest release",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			resolved := resolveBuildInfo(info)
			return update.Run(context.Background(), update.Options{
				CurrentVersion: resolved.Version,
				CheckOnly:      checkOnly,
				Out:            command.OutOrStdout(),
			})
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, "check for the latest version without installing it")
	return cmd
}
