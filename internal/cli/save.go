package cli

import (
	"context"
	"fmt"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/spf13/cobra"
)

func newSaveCommand() *cobra.Command {
	command := &cobra.Command{
		Use: "save", Short: "Save the current draft to its source",
		RunE: func(command *cobra.Command, _ []string) error {
			workspace, err := command.Flags().GetString("workspace")
			if err != nil {
				return err
			}
			environment, err := command.Flags().GetString("environment")
			if err != nil {
				return err
			}
			service, err := app.Open(workspace)
			if err != nil {
				return err
			}
			view, revision, err := service.GetDraft(context.Background(), environment)
			if err != nil {
				return err
			}
			_, nextRevision, err := service.SaveDraft(context.Background(), environment, revision, view.SourceRevision)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "saved %s at draft revision %d\n", environment, nextRevision)
			return err
		},
	}
	command.Flags().String("workspace", ".", "project workspace")
	command.Flags().String("environment", "", "environment ID")
	_ = command.MarkFlagRequired("environment")
	return command
}
