package cli

import (
	"context"
	"encoding/json"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/spf13/cobra"
)

func newPublishCommand() *cobra.Command {
	var workspace, environmentID, planID, idempotencyKey string
	var confirm bool
	command := &cobra.Command{Use: "publish", Short: "Publish a ready plan to Firebase", RunE: func(command *cobra.Command, _ []string) error {
		if !confirm {
			return usageError("confirmation_required", "--confirm is required for publish")
		}
		if idempotencyKey == "" {
			return usageError("idempotency_key_required", "--idempotency-key is required for non-interactive publish")
		}
		service, err := app.Open(workspace)
		if err != nil {
			return err
		}
		p, err := service.GetPlan(context.Background(), planID)
		if err != nil {
			return err
		}
		if p.Status != "ready" || p.RemoteETag == nil {
			return &app.PlanInvalidatedError{PlanID: planID, Reason: "plan_not_ready"}
		}
		op, err := service.StartRelease(context.Background(), environmentID, idempotencyKey, app.ReleaseRequest{PlanID: planID, ExpectedDraftRevision: p.DraftRevision, ExpectedRemoteETag: *p.RemoteETag, Confirmation: app.ReleaseConfirmation{Acknowledged: true, EnvironmentID: environmentID, AcknowledgedRiskItemIDs: p.ConfirmationRequirements.RequiredRiskItemIDs}})
		if err != nil {
			return err
		}
		return json.NewEncoder(command.OutOrStdout()).Encode(op)
	}}
	command.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	command.Flags().StringVar(&environmentID, "environment", "", "environment ID")
	command.Flags().StringVar(&planID, "plan", "", "ready plan ID")
	command.Flags().BoolVar(&confirm, "confirm", false, "acknowledge the server-calculated release requirements")
	command.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "client-generated idempotency key")
	return command
}
