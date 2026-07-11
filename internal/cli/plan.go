package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/spf13/cobra"
)

func newPlanCommand() *cobra.Command {
	var workspace, environment, format, output string
	command := &cobra.Command{Use: "plan", Short: "Build an immutable configuration plan", RunE: func(command *cobra.Command, args []string) error {
		if environment == "" {
			return fmt.Errorf("--environment is required")
		}
		service, err := app.Open(workspace)
		if err != nil {
			return err
		}
		op, err := service.StartPlan(context.Background(), environment)
		if err != nil {
			return err
		}
		for {
			op, err = service.Operation(context.Background(), op.OperationID)
			if err != nil {
				return err
			}
			if op.Status == "succeeded" || op.Status == "failed" || op.Status == "cancelled" {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if op.Status != "succeeded" {
			if op.Failure != nil {
				return fmt.Errorf("plan failed: %s", op.Failure.Message)
			}
			return fmt.Errorf("plan did not complete")
		}
		p, err := service.GetPlan(context.Background(), op.Result.ResourceID)
		if err != nil {
			return err
		}
		if output != "" {
			for _, artifact := range p.ArtifactMetadata {
				content, _, _, artifactErr := service.PlanArtifact(context.Background(), p.PlanID, artifact.ArtifactName)
				if artifactErr != nil {
					return artifactErr
				}
				if err := os.MkdirAll(output, 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(output, artifact.ArtifactName), content, 0o600); err != nil {
					return err
				}
			}
		}
		if format == "json" || jsonMode(command) {
			return json.NewEncoder(command.OutOrStdout()).Encode(p)
		}
		_, err = fmt.Fprintf(command.OutOrStdout(), "plan %s: %s (%d semantic changes, %s risk)\n", p.PlanID, p.Status, len(p.SemanticChanges), p.Severity)
		return err
	}}
	command.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	command.Flags().StringVar(&environment, "environment", "", "environment ID")
	command.Flags().StringVar(&format, "format", "text", "output format: text or json")
	command.Flags().StringVar(&output, "output", "", "directory for plan artifacts")
	command.PreRunE = func(_ *cobra.Command, _ []string) error {
		if format != "text" && format != "json" {
			return usageError("usage_error", "--format must be text or json")
		}
		return nil
	}
	return command
}
