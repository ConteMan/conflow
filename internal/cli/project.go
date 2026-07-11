package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/ConteMan/conflow/internal/project"
	"github.com/spf13/cobra"
)

func newProjectCommand() *cobra.Command {
	var workspace string
	var id, name, productionLowRiskMode string
	command := &cobra.Command{Use: "project", Short: "Read and update project metadata"}
	get := &cobra.Command{
		Use:   "get",
		Short: "Get project metadata",
		RunE: func(command *cobra.Command, _ []string) error {
			service, err := app.Open(workspace)
			if err != nil {
				return err
			}
			snapshot, err := service.Snapshot(context.Background())
			if err != nil {
				return err
			}
			return writeCommandResult(command, snapshot.Manifest.Project)
		},
	}
	update := &cobra.Command{
		Use:   "update",
		Short: "Update project metadata",
		RunE: func(command *cobra.Command, _ []string) error {
			if id == "" || name == "" {
				return usageError("usage_error", "--id and --name are required")
			}
			service, err := app.Open(workspace)
			if err != nil {
				return err
			}
			current, err := service.Snapshot(context.Background())
			if err != nil {
				return err
			}
			if productionLowRiskMode == "" {
				productionLowRiskMode = current.Manifest.Project.ReleaseConfirmationPolicy.ProductionLowRiskMode
			}
			snapshot, err := service.UpdateProject(context.Background(), current.Revision, project.Project{
				ID: id, Name: name,
				ReleaseConfirmationPolicy: project.ReleaseConfirmationPolicy{ProductionLowRiskMode: productionLowRiskMode},
			})
			if err != nil {
				return err
			}
			return writeCommandResult(command, snapshot.Manifest.Project)
		},
	}
	get.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	update.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	update.Flags().StringVar(&id, "id", "", "project ID")
	update.Flags().StringVar(&name, "name", "", "project display name")
	update.Flags().StringVar(&productionLowRiskMode, "production-low-risk-mode", "", "production low-risk confirmation mode")
	command.AddCommand(get, update)
	return command
}

func writeCommandResult(command *cobra.Command, result any) error {
	if jsonMode(command) {
		return json.NewEncoder(command.OutOrStdout()).Encode(result)
	}
	_, err := fmt.Fprintln(command.OutOrStdout(), result)
	return err
}
