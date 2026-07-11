package cli

import (
	"context"
	"encoding/json"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/spf13/cobra"
)

func newSourceCommand() *cobra.Command {
	command := &cobra.Command{Use: "source", Short: "Inspect the configuration source"}
	status := &cobra.Command{
		Use: "status", Short: "Show source digest and managed file paths",
		RunE: func(command *cobra.Command, _ []string) error {
			workspace, err := command.Flags().GetString("workspace")
			if err != nil {
				return err
			}
			service, err := app.Open(workspace)
			if err != nil {
				return err
			}
			info, err := service.SourceInfo(context.Background())
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(info.Status)
		},
	}
	status.Flags().String("workspace", ".", "project workspace")
	command.AddCommand(status)
	return command
}
