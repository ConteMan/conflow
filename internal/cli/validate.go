package cli

import (
	"context"
	"fmt"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/spf13/cobra"
)

func newValidateCommand() *cobra.Command {
	var workspace string
	command := &cobra.Command{
		Use:   "validate",
		Short: "Validate a Conflow project manifest",
		RunE: func(command *cobra.Command, args []string) error {
			service, err := app.Open(workspace)
			if err != nil {
				return err
			}
			snapshot, err := service.Snapshot(context.Background())
			if err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "validated %s with %d environments\n", snapshot.Manifest.Project.ID, len(snapshot.Manifest.Environments))
			return nil
		},
	}
	command.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	return command
}
