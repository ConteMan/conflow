package cli

import (
	"fmt"

	"github.com/ConteMan/conflow/internal/project"
	"github.com/spf13/cobra"
)

func newInitCommand() *cobra.Command {
	var directory string
	command := &cobra.Command{
		Use:   "init",
		Short: "Create a Conflow project manifest",
		RunE: func(command *cobra.Command, args []string) error {
			path, err := project.CreateExample(directory)
			if err != nil {
				return err
			}
			fmt.Fprintln(command.OutOrStdout(), path)
			return nil
		},
	}
	command.Flags().StringVar(&directory, "dir", ".", "project directory")
	return command
}
