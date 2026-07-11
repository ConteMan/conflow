package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/ConteMan/conflow/internal/operation"
	"github.com/spf13/cobra"
)

func newProviderCommand() *cobra.Command {
	command := &cobra.Command{Use: "provider", Short: "Inspect provider connectivity"}
	var workspace, environment string
	status := &cobra.Command{Use: "status", Short: "Show provider status", RunE: func(command *cobra.Command, _ []string) error {
		service, err := app.Open(workspace)
		if err != nil {
			return err
		}
		info, err := service.ProviderStatus(context.Background(), environment)
		if err != nil {
			return err
		}
		return json.NewEncoder(command.OutOrStdout()).Encode(info)
	}}
	status.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	status.Flags().StringVar(&environment, "environment", "", "environment ID")
	_ = status.MarkFlagRequired("environment")
	command.AddCommand(status)
	return command
}

func newPullCommand() *cobra.Command {
	var workspace, environment string
	command := &cobra.Command{Use: "pull", Short: "Pull and protect the current remote template", RunE: func(command *cobra.Command, _ []string) error {
		service, err := app.Open(workspace)
		if err != nil {
			return err
		}
		op, err := service.StartPull(context.Background(), environment)
		if err != nil {
			return err
		}
		final, err := waitOperation(service, op.OperationID)
		if err != nil {
			return err
		}
		if final.Status != "succeeded" {
			return fmt.Errorf("pull failed: %s", final.Failure.Code)
		}
		return json.NewEncoder(command.OutOrStdout()).Encode(final)
	}}
	command.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	command.Flags().StringVar(&environment, "environment", "", "environment ID")
	_ = command.MarkFlagRequired("environment")
	return command
}

func newRemoteCommand() *cobra.Command {
	remote := &cobra.Command{Use: "remote", Short: "Remote provider operations"}
	var workspace, environment, planID string
	validate := &cobra.Command{Use: "validate", Short: "Validate a ready plan without publishing", RunE: func(command *cobra.Command, _ []string) error {
		service, err := app.Open(workspace)
		if err != nil {
			return err
		}
		op, err := service.StartRemoteValidate(context.Background(), environment, planID)
		if err != nil {
			return err
		}
		final, err := waitOperation(service, op.OperationID)
		if err != nil {
			return err
		}
		if final.Status != "succeeded" {
			return fmt.Errorf("remote validation failed: %s", final.Failure.Code)
		}
		return json.NewEncoder(command.OutOrStdout()).Encode(final)
	}}
	validate.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	validate.Flags().StringVar(&environment, "environment", "", "environment ID")
	validate.Flags().StringVar(&planID, "plan", "", "ready plan ID")
	_ = validate.MarkFlagRequired("environment")
	_ = validate.MarkFlagRequired("plan")
	remote.AddCommand(validate)
	return remote
}

func waitOperation(service *app.Service, id string) (operation.Operation, error) {
	deadline := time.Now().Add(30 * time.Second)
	for {
		op, err := service.Operation(context.Background(), id)
		if err != nil {
			return operation.Operation{}, err
		}
		if op.Status == "succeeded" || op.Status == "failed" || op.Status == "cancelled" {
			return op, nil
		}
		if time.Now().After(deadline) {
			return operation.Operation{}, fmt.Errorf("operation timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
