package cli

import (
	"fmt"

	"github.com/ConteMan/conflow/internal/project"
	"github.com/spf13/cobra"
)

func newValidateCommand() *cobra.Command {
	var workspace string
	command := &cobra.Command{
		Use:   "validate",
		Short: "Validate a Conflow project manifest",
		RunE: func(command *cobra.Command, args []string) error {
			manifest, err := project.Load(workspace)
			if err != nil {
				return err
			}
			if err := project.Validate(manifest); err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "validated %s with %d environments\n", manifest.Project.ID, len(manifest.Environments))
			return nil
		},
	}
	command.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	return command
}
