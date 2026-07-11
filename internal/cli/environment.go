package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/ConteMan/conflow/internal/project"
	"github.com/spf13/cobra"
)

func newEnvironmentCommand() *cobra.Command {
	command := &cobra.Command{Use: "environment", Short: "Manage project environments"}
	command.AddCommand(newEnvironmentListCommand(), newEnvironmentGetCommand(), newEnvironmentCreateCommand(), newEnvironmentUpdateCommand(), newEnvironmentDeleteCommand())
	return command
}

func newEnvironmentListCommand() *cobra.Command {
	var workspace string
	command := &cobra.Command{Use: "list", Short: "List environments", RunE: func(command *cobra.Command, _ []string) error {
		service, err := app.Open(workspace)
		if err != nil {
			return err
		}
		snapshot, err := service.Snapshot(context.Background())
		if err != nil {
			return err
		}
		return writeEnvironmentResult(command, snapshot.Manifest.Environments)
	}}
	command.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	return command
}

func newEnvironmentGetCommand() *cobra.Command {
	var workspace, id string
	command := &cobra.Command{Use: "get", Short: "Get one environment", RunE: func(command *cobra.Command, _ []string) error {
		if id == "" {
			return usageError("usage_error", "--id is required")
		}
		service, err := app.Open(workspace)
		if err != nil {
			return err
		}
		_, environment, err := service.GetEnvironment(context.Background(), id)
		if err != nil {
			return err
		}
		return writeEnvironmentResult(command, environment)
	}}
	command.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	command.Flags().StringVar(&id, "id", "", "environment ID")
	return command
}

func newEnvironmentCreateCommand() *cobra.Command {
	var workspace, id, name, kind, providerType, providerProjectID string
	var requiresConfirmation bool
	command := &cobra.Command{Use: "create", Short: "Create an environment", RunE: func(command *cobra.Command, _ []string) error {
		service, err := app.Open(workspace)
		if err != nil {
			return err
		}
		current, err := service.Snapshot(context.Background())
		if err != nil {
			return err
		}
		_, environment, err := service.CreateEnvironment(context.Background(), current.Revision, project.Environment{
			ID: id, Name: name, Kind: kind,
			Provider: project.Provider{Type: providerType, ProjectID: providerProjectID},
			Publish:  project.Publish{RequiresConfirmation: requiresConfirmation},
		})
		if err != nil {
			return err
		}
		return writeEnvironmentResult(command, environment)
	}}
	addEnvironmentMutationFlags(command, &workspace, &id, &name, &kind, &providerType, &providerProjectID, &requiresConfirmation)
	return command
}

func newEnvironmentUpdateCommand() *cobra.Command {
	var workspace, id, name, providerType, providerProjectID string
	var requiresConfirmation bool
	command := &cobra.Command{Use: "update", Short: "Update an environment", RunE: func(command *cobra.Command, _ []string) error {
		if id == "" {
			return usageError("usage_error", "--id is required")
		}
		service, err := app.Open(workspace)
		if err != nil {
			return err
		}
		snapshot, existing, err := service.GetEnvironment(context.Background(), id)
		if err != nil {
			return err
		}
		if name == "" {
			name = existing.Name
		}
		if providerType == "" {
			providerType = existing.Provider.Type
		}
		if providerProjectID == "" {
			providerProjectID = existing.Provider.ProjectID
		}
		if !command.Flags().Changed("requires-confirmation") {
			requiresConfirmation = existing.Publish.RequiresConfirmation
		}
		_, environment, err := service.UpdateEnvironment(context.Background(), snapshot.Revision, id, project.Environment{
			ID: id, Name: name, Kind: existing.Kind,
			Provider: project.Provider{Type: providerType, ProjectID: providerProjectID},
			Publish:  project.Publish{RequiresConfirmation: requiresConfirmation},
		})
		if err != nil {
			return err
		}
		return writeEnvironmentResult(command, environment)
	}}
	command.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	command.Flags().StringVar(&id, "id", "", "environment ID")
	command.Flags().StringVar(&name, "name", "", "environment display name")
	command.Flags().StringVar(&providerType, "provider-type", "", "provider type")
	command.Flags().StringVar(&providerProjectID, "provider-project-id", "", "provider project ID")
	command.Flags().BoolVar(&requiresConfirmation, "requires-confirmation", false, "require publish confirmation")
	return command
}

func newEnvironmentDeleteCommand() *cobra.Command {
	var workspace, id string
	command := &cobra.Command{Use: "delete", Short: "Delete an environment", RunE: func(command *cobra.Command, _ []string) error {
		if id == "" {
			return usageError("usage_error", "--id is required")
		}
		service, err := app.Open(workspace)
		if err != nil {
			return err
		}
		current, err := service.Snapshot(context.Background())
		if err != nil {
			return err
		}
		_, err = service.DeleteEnvironment(context.Background(), current.Revision, id)
		if err != nil {
			return err
		}
		return writeEnvironmentResult(command, map[string]any{"id": id, "deleted": true})
	}}
	command.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	command.Flags().StringVar(&id, "id", "", "environment ID")
	return command
}

func addEnvironmentMutationFlags(command *cobra.Command, workspace, id, name, kind, providerType, providerProjectID *string, requiresConfirmation *bool) {
	command.Flags().StringVar(workspace, "workspace", ".", "project workspace")
	command.Flags().StringVar(id, "id", "", "environment ID")
	command.Flags().StringVar(name, "name", "", "environment display name")
	command.Flags().StringVar(kind, "kind", "", "environment kind: development, staging, production, or custom")
	command.Flags().StringVar(providerType, "provider-type", "firebase-remote-config", "provider type")
	command.Flags().StringVar(providerProjectID, "provider-project-id", "", "provider project ID")
	command.Flags().BoolVar(requiresConfirmation, "requires-confirmation", false, "require publish confirmation")
}

func writeEnvironmentResult(command *cobra.Command, result any) error {
	if jsonMode(command) {
		return json.NewEncoder(command.OutOrStdout()).Encode(result)
	}
	_, err := fmt.Fprintln(command.OutOrStdout(), result)
	return err
}
