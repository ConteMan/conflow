package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/ConteMan/conflow/internal/importer"
	"github.com/spf13/cobra"
)

func newImportCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "import",
		Short: "Import configuration from another workspace or bundle file",
	}
	command.AddCommand(newImportExportCommand())
	command.AddCommand(newImportPreviewCommand())
	command.AddCommand(newImportApplyCommand())
	return command
}

// newImportExportCommand implements `conflow import export`.
func newImportExportCommand() *cobra.Command {
	var workspace, environment, output string
	command := &cobra.Command{
		Use:   "export",
		Short: "Export the current workspace draft as an ImportBundle",
		RunE: func(command *cobra.Command, args []string) error {
			service, err := app.Open(workspace)
			if err != nil {
				return err
			}
			bundle, err := service.ExportImportBundle(context.Background(), environment)
			if err != nil {
				return err
			}
			if jsonMode(command) {
				return json.NewEncoder(command.OutOrStdout()).Encode(bundle)
			}
			data, err := json.MarshalIndent(bundle, "", "  ")
			if err != nil {
				return err
			}
			if output != "" {
				if writeErr := os.WriteFile(output, append(data, '\n'), 0o600); writeErr != nil {
					return writeErr
				}
				_, err = fmt.Fprintf(command.OutOrStdout(), "bundle written to %s\n", output)
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "%s\n", data)
			return err
		},
	}
	command.Flags().StringVar(&workspace, "workspace", ".", "source workspace directory")
	command.Flags().StringVar(&environment, "environment", "", "environment ID to export")
	command.Flags().StringVar(&output, "output", "", "write bundle JSON to FILE instead of stdout")
	return command
}

// newImportPreviewCommand implements `conflow import preview`.
func newImportPreviewCommand() *cobra.Command {
	var workspace, environment, bundleFile, fromDir, conflictMode string
	command := &cobra.Command{
		Use:   "preview",
		Short: "Preview the effect of an import without modifying the draft",
		RunE: func(command *cobra.Command, args []string) error {
			if bundleFile == "" && fromDir == "" {
				return usageError("usage_error", "one of --bundle or --from is required")
			}
			if bundleFile != "" && fromDir != "" {
				return usageError("usage_error", "--bundle and --from are mutually exclusive")
			}
			cm, err := parseConflictMode(conflictMode)
			if err != nil {
				return err
			}
			bundle, err := loadBundle(bundleFile, fromDir, environment)
			if err != nil {
				return err
			}
			service, err := app.Open(workspace)
			if err != nil {
				return err
			}
			result, err := service.PreviewImport(context.Background(), environment, bundle, cm)
			if err != nil {
				return err
			}
			if jsonMode(command) {
				return json.NewEncoder(command.OutOrStdout()).Encode(result)
			}
			plan := result.EntityPlan
			fmt.Fprintf(command.OutOrStdout(), "preview_token: %s\n", result.PreviewToken)
			fmt.Fprintf(command.OutOrStdout(), "to_add: %d  to_replace: %d  to_skip: %d  to_keep: %d\n",
				len(plan.ToAdd), len(plan.ToReplace), len(plan.ToSkip), len(plan.ToKeep))
			if len(result.DecisionsRequired) > 0 {
				fmt.Fprintf(command.OutOrStdout(), "decisions_required: %d\n", len(result.DecisionsRequired))
				for _, d := range result.DecisionsRequired {
					fmt.Fprintf(command.OutOrStdout(), "  - %s: %s\n", d.Key, d.Reason)
				}
			}
			for _, risk := range result.Risks {
				fmt.Fprintf(command.OutOrStdout(), "risk: %s\n", risk)
			}
			return nil
		},
	}
	command.Flags().StringVar(&workspace, "workspace", ".", "target workspace directory")
	command.Flags().StringVar(&environment, "environment", "", "target environment ID")
	command.Flags().StringVar(&bundleFile, "bundle", "", "path to ImportBundle JSON file")
	command.Flags().StringVar(&fromDir, "from", "", "path to source workspace (exports bundle automatically)")
	command.Flags().StringVar(&conflictMode, "conflict-mode", "replace", "conflict resolution: replace | merge | skip")
	return command
}

// newImportApplyCommand implements `conflow import apply`.
func newImportApplyCommand() *cobra.Command {
	var workspace, environment, bundleFile, fromDir, conflictMode, decisionsFile string
	command := &cobra.Command{
		Use:   "apply",
		Short: "Apply an import bundle into the workspace draft (atomic)",
		RunE: func(command *cobra.Command, args []string) error {
			if bundleFile == "" && fromDir == "" {
				return usageError("usage_error", "one of --bundle or --from is required")
			}
			if bundleFile != "" && fromDir != "" {
				return usageError("usage_error", "--bundle and --from are mutually exclusive")
			}
			cm, err := parseConflictMode(conflictMode)
			if err != nil {
				return err
			}
			bundle, err := loadBundle(bundleFile, fromDir, environment)
			if err != nil {
				return err
			}
			service, err := app.Open(workspace)
			if err != nil {
				return err
			}

			// Collect operator decisions if provided.
			var decisions []importer.ImportDecision
			if decisionsFile != "" {
				data, readErr := os.ReadFile(decisionsFile)
				if readErr != nil {
					return fmt.Errorf("reading --decisions file: %w", readErr)
				}
				if unmarshalErr := json.Unmarshal(data, &decisions); unmarshalErr != nil {
					return fmt.Errorf("parsing --decisions file: %w", unmarshalErr)
				}
			}

			// Run preview first to obtain the token and check for required decisions.
			previewResult, err := service.PreviewImport(context.Background(), environment, bundle, cm)
			if err != nil {
				return err
			}

			// If there are unresolved required decisions and no decisions file was provided,
			// list them and instruct the operator to supply a --decisions file.
			if len(previewResult.DecisionsRequired) > 0 && decisionsFile == "" {
				fmt.Fprintf(command.ErrOrStderr(), "error: %d decision(s) required before apply:\n", len(previewResult.DecisionsRequired))
				for _, d := range previewResult.DecisionsRequired {
					fmt.Fprintf(command.ErrOrStderr(), "  key: %s\n  reason: %s\n", d.Key, d.Reason)
					if d.Hint != "" {
						fmt.Fprintf(command.ErrOrStderr(), "  hint: %s\n", d.Hint)
					}
				}
				fmt.Fprintf(command.ErrOrStderr(), "\nProvide a --decisions FILE containing [{\"key\":\"...\",\"value\":\"...\"}].\n")
				return usageError("decisions_required", "apply requires operator decisions; re-run with --decisions")
			}

			_, revision, result, err := service.ApplyImport(context.Background(), environment, bundle, previewResult.PreviewToken, decisions, cm)
			if err != nil {
				return err
			}

			appliedCount := result.AppliedCount
			if jsonMode(command) {
				return json.NewEncoder(command.OutOrStdout()).Encode(map[string]any{
					"applied_count": appliedCount,
					"revision":      revision,
				})
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "applied %d entities (revision %d)\n", appliedCount, revision)
			return err
		},
	}
	command.Flags().StringVar(&workspace, "workspace", ".", "target workspace directory")
	command.Flags().StringVar(&environment, "environment", "", "target environment ID")
	command.Flags().StringVar(&bundleFile, "bundle", "", "path to ImportBundle JSON file")
	command.Flags().StringVar(&fromDir, "from", "", "path to source workspace (exports bundle automatically)")
	command.Flags().StringVar(&conflictMode, "conflict-mode", "replace", "conflict resolution: replace | merge | skip")
	command.Flags().StringVar(&decisionsFile, "decisions", "", "path to JSON file containing []ImportDecision")
	return command
}

// parseConflictMode validates and converts the conflict-mode flag value.
func parseConflictMode(mode string) (importer.ConflictMode, error) {
	switch importer.ConflictMode(mode) {
	case importer.ConflictReplace, importer.ConflictMerge, importer.ConflictSkip:
		return importer.ConflictMode(mode), nil
	default:
		return "", usageError("usage_error", "--conflict-mode must be replace, merge, or skip")
	}
}

// loadBundle returns an ImportBundle either by reading a JSON file or by
// opening the source workspace and calling ExportImportBundle.
func loadBundle(bundleFile, fromDir, environmentID string) (importer.ImportBundle, error) {
	if bundleFile != "" {
		data, err := os.ReadFile(bundleFile)
		if err != nil {
			return importer.ImportBundle{}, fmt.Errorf("reading --bundle file: %w", err)
		}
		var bundle importer.ImportBundle
		if err := json.Unmarshal(data, &bundle); err != nil {
			return importer.ImportBundle{}, fmt.Errorf("parsing --bundle file: %w", err)
		}
		return bundle, nil
	}
	src, err := app.Open(fromDir)
	if err != nil {
		return importer.ImportBundle{}, fmt.Errorf("opening source workspace %q: %w", fromDir, err)
	}
	return src.ExportImportBundle(context.Background(), environmentID)
}
