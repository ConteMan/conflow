package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/ConteMan/conflow/internal/validation"
	"github.com/spf13/cobra"
)

// ExitError preserves the Spec 007 validation exit status through Cobra.
type ExitError struct{ Code int }

func (e *ExitError) Error() string { return "validation failed" }

func newValidateCommand() *cobra.Command {
	var workspace string
	var environment string
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "validate",
		Short: "Validate a Conflow project manifest",
		RunE: func(command *cobra.Command, args []string) error {
			service, err := app.Open(workspace)
			if err != nil {
				return err
			}
			snapshot, err := service.Snapshot(context.Background())
			if err != nil {
				return err
			}
			// Preserve the manifest-only validation behavior for callers that have
			// not selected an environment. Complete validation is environment scoped.
			if environment == "" {
				fmt.Fprintf(command.OutOrStdout(), "validated %s with %d environments\n", snapshot.Manifest.Project.ID, len(snapshot.Manifest.Environments))
				return nil
			}
			result, _, err := service.ValidateDraft(context.Background(), environment)
			if err != nil {
				return err
			}
			if jsonOutput {
				encoder := json.NewEncoder(command.OutOrStdout())
				if err := encoder.Encode(result); err != nil {
					return err
				}
			} else {
				fmt.Fprintf(command.OutOrStdout(), "validated %s/%s: %s (%d diagnostics)\n", snapshot.Manifest.Project.ID, environment, result.Readiness, len(result.Diagnostics))
				for _, diagnostic := range result.Diagnostics {
					fmt.Fprintf(command.OutOrStdout(), "%s %s %s\n", diagnostic.Severity, diagnostic.Code, diagnostic.Path)
				}
			}
			if code := validation.ExitCodeFor(result.Diagnostics); code != 0 {
				return &ExitError{Code: code}
			}
			return nil
		},
	}
	command.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	command.Flags().StringVar(&environment, "environment", "", "environment ID for complete validation")
	command.Flags().BoolVar(&jsonOutput, "json", false, "write complete validation result as JSON")
	return command
}
