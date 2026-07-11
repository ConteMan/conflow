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
	inspect := &cobra.Command{
		Use: "inspect", Short: "Detect the Git workspace and mapping profile",
		RunE: func(command *cobra.Command, _ []string) error {
			workspace, err := command.Flags().GetString("workspace")
			if err != nil {
				return err
			}
			service, err := app.Open(workspace)
			if err != nil {
				return err
			}
			result, err := service.InspectSource(context.Background())
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	inspect.Flags().String("workspace", ".", "project workspace")
	command.AddCommand(inspect)
	importSource := &cobra.Command{
		Use: "import", Short: "Import the Git JSON source into a draft",
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
			result, next, err := service.ImportSource(context.Background(), environment, revision, view.SourceRevision)
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(struct {
				View     any    `json:"view"`
				Revision uint64 `json:"revision"`
			}{View: result, Revision: next})
		},
	}
	importSource.Flags().String("workspace", ".", "project workspace")
	importSource.Flags().String("environment", "", "target environment")
	_ = importSource.MarkFlagRequired("environment")
	command.AddCommand(importSource)
	preview := &cobra.Command{
		Use: "preview-save", Short: "Preview Git JSON source changes before saving",
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
			result, err := service.PreviewSourceSave(context.Background(), environment)
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	preview.Flags().String("workspace", ".", "project workspace")
	preview.Flags().String("environment", "", "target environment")
	_ = preview.MarkFlagRequired("environment")
	command.AddCommand(preview)
	return command
}
