package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/ConteMan/conflow/internal/plan"
	"github.com/spf13/cobra"
)

func newRollbackCommand() *cobra.Command {
	var workspace, environmentID, releaseID, idempotencyKey string
	var confirm bool
	command := &cobra.Command{Use: "rollback", Short: "Create a rollback preview and restore a release", RunE: func(command *cobra.Command, _ []string) error {
		if !confirm {
			return fmt.Errorf("--confirm is required for rollback")
		}
		if idempotencyKey == "" {
			return fmt.Errorf("--idempotency-key is required for non-interactive rollback")
		}
		service, err := app.Open(workspace)
		if err != nil {
			return err
		}
		previewOp, err := service.CreateRollbackPreview(context.Background(), environmentID, releaseID)
		if err != nil {
			return err
		}
		for deadline := time.Now().Add(time.Minute); ; {
			op, err := service.Operation(context.Background(), previewOp.OperationID)
			if err != nil {
				return err
			}
			if op.Status == "succeeded" {
				break
			}
			if op.Status == "failed" || time.Now().After(deadline) {
				return fmt.Errorf("rollback preview did not complete")
			}
			time.Sleep(10 * time.Millisecond)
		}
		preview, err := service.RollbackPreview(context.Background(), environmentID, releaseID)
		if err != nil {
			return err
		}
		var requirements plan.ConfirmationRequirements
		if err := json.Unmarshal(preview.ConfirmationRequirements, &requirements); err != nil {
			return fmt.Errorf("read rollback confirmation requirements: %w", err)
		}
		op, err := service.StartRollback(context.Background(), environmentID, releaseID, idempotencyKey, app.RollbackRequest{RollbackPreviewID: preview.RollbackPreviewID, ExpectedRemoteETag: preview.ExpectedRemoteETag, Confirmation: app.ReleaseConfirmation{Acknowledged: true, EnvironmentID: environmentID, AcknowledgedRiskItemIDs: requirements.RequiredRiskItemIDs}})
		if err != nil {
			return err
		}
		return json.NewEncoder(command.OutOrStdout()).Encode(op)
	}}
	command.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	command.Flags().StringVar(&environmentID, "environment", "", "environment ID")
	command.Flags().StringVar(&releaseID, "release", "", "successful release ID to restore")
	command.Flags().BoolVar(&confirm, "confirm", false, "acknowledge the server-calculated rollback requirements")
	command.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "client-generated idempotency key")
	_ = command.MarkFlagRequired("environment")
	_ = command.MarkFlagRequired("release")
	return command
}
