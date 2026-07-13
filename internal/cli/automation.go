package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/ConteMan/conflow/internal/draft"
	"github.com/ConteMan/conflow/internal/gitreview"
	"github.com/ConteMan/conflow/internal/plan"
	"github.com/ConteMan/conflow/internal/project"
	"github.com/ConteMan/conflow/internal/provider"
	"github.com/ConteMan/conflow/internal/release"
	"github.com/ConteMan/conflow/internal/source"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	ExitSuccess    = 0
	ExitValidation = 1
	ExitBlocking   = 2
	ExitConflict   = 3
	ExitProvider   = 4
	ExitUsage      = 64
)

// ExitError preserves a stable CLI exit status through Cobra.
type ExitError struct {
	Code      int
	ErrorCode string
	Message   string
	JSON      bool
}

func (e *ExitError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "command failed"
}

type jsonEnvelope struct {
	Data  any            `json:"data,omitempty"`
	Error *jsonErrorBody `json:"error,omitempty"`
}

type jsonErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type providerOperationError struct{ Code string }

func (e *providerOperationError) Error() string { return "provider operation failed: " + e.Code }

func jsonMode(command *cobra.Command) bool {
	value, err := command.Flags().GetBool("json")
	return err == nil && value
}

func writeJSON(command *cobra.Command, data any) error {
	return json.NewEncoder(command.OutOrStdout()).Encode(jsonEnvelope{Data: data})
}

func writeJSONError(command *cobra.Command, code, message string) {
	_ = json.NewEncoder(command.OutOrStdout()).Encode(jsonEnvelope{Error: &jsonErrorBody{Code: code, Message: message}})
}

func usageError(code, message string) *ExitError {
	return &ExitError{Code: ExitUsage, ErrorCode: code, Message: message}
}

func configureAutomation(root *cobra.Command, jsonOutput *bool) {
	root.SetFlagErrorFunc(func(command *cobra.Command, err error) error {
		if *jsonOutput {
			writeJSONError(command, "usage_error", err.Error())
			return &ExitError{Code: ExitUsage, ErrorCode: "usage_error", Message: err.Error(), JSON: true}
		}
		return usageError("usage_error", err.Error())
	})
	configureCommandAutomation(root, jsonOutput)
}

func configureCommandAutomation(command *cobra.Command, jsonOutput *bool) {
	if command.PreRunE != nil {
		originalPreRunE := command.PreRunE
		command.PreRunE = func(current *cobra.Command, args []string) error {
			if err := originalPreRunE(current, args); err != nil {
				return automationError(current, jsonOutput, err)
			}
			return nil
		}
	}
	if command.RunE != nil || command.Run != nil {
		originalRunE, originalRun := command.RunE, command.Run
		command.RunE = func(current *cobra.Command, args []string) error {
			if originalRunE != nil {
				return runWithJSONEnvelope(current, args, jsonOutput, originalRunE)
			}
			run := func(c *cobra.Command, a []string) error { originalRun(c, a); return nil }
			return runWithJSONEnvelope(current, args, jsonOutput, run)
		}
		command.Run = nil
	}
	wrapRequiredFlagValidation(command, jsonOutput)
	for _, child := range command.Commands() {
		configureCommandAutomation(child, jsonOutput)
	}
}

func runWithJSONEnvelope(command *cobra.Command, args []string, jsonOutput *bool, run func(*cobra.Command, []string) error) error {
	if !*jsonOutput {
		return run(command, args)
	}
	originalOutput := command.OutOrStdout()
	buffer := &bytes.Buffer{}
	command.SetOut(buffer)
	err := run(command, args)
	command.SetOut(originalOutput)
	if err != nil {
		var exit *ExitError
		if errors.As(err, &exit) && (exit.Code == ExitValidation || exit.Code == ExitBlocking) && buffer.Len() > 0 {
			writeCapturedJSON(command, buffer.Bytes())
			exit.JSON = true
			return err
		}
		code, message, exitCode := classifyError(err)
		writeJSONError(command, code, message)
		return &ExitError{Code: exitCode, ErrorCode: code, Message: message, JSON: true}
	}
	writeCapturedJSON(command, buffer.Bytes())
	return nil
}

func writeCapturedJSON(command *cobra.Command, output []byte) {
	trimmed := bytes.TrimSpace(output)
	if len(trimmed) == 0 {
		_ = writeJSON(command, map[string]any{})
		return
	}
	var data any
	if err := json.Unmarshal(trimmed, &data); err == nil {
		_ = writeJSON(command, data)
		return
	}
	_ = writeJSON(command, map[string]string{"output": string(trimmed)})
}

func wrapRequiredFlagValidation(command *cobra.Command, jsonOutput *bool) {
	required := make([]string, 0)
	command.Flags().VisitAll(func(flag *pflag.Flag) {
		if flag.Annotations != nil && len(flag.Annotations[cobra.BashCompOneRequiredFlag]) > 0 {
			required = append(required, flag.Name)
		}
	})
	originalArgs := command.Args
	if len(required) == 0 && originalArgs == nil {
		return
	}
	command.Args = func(current *cobra.Command, args []string) error {
		if originalArgs != nil {
			if err := originalArgs(current, args); err != nil {
				return automationError(current, jsonOutput, usageError("usage_error", err.Error()))
			}
		}
		for _, name := range required {
			if !current.Flags().Changed(name) {
				return automationError(current, jsonOutput, usageError("usage_error", fmt.Sprintf("--%s is required", name)))
			}
		}
		return nil
	}
}

func automationError(command *cobra.Command, jsonOutput *bool, err error) error {
	if !*jsonOutput {
		return err
	}
	code, message, exitCode := classifyError(err)
	writeJSONError(command, code, message)
	return &ExitError{Code: exitCode, ErrorCode: code, Message: message, JSON: true}
}

func classifyError(err error) (code, message string, exitCode int) {
	var exit *ExitError
	if errors.As(err, &exit) {
		if exit.ErrorCode != "" {
			return exit.ErrorCode, exit.Error(), exit.Code
		}
		return "validation_failed", exit.Error(), exit.Code
	}
	message = err.Error()
	switch {
	case errors.As(err, new(*project.RevisionMismatchError)), errors.As(err, new(*app.RemoteETagMismatchError)), errors.As(err, new(*app.PlanInvalidatedError)),
		errors.Is(err, project.ErrAlreadyExists), errors.Is(err, app.ErrLastEnvironment), errors.Is(err, app.ErrConfirmationInvalid),
		errors.Is(err, release.ErrIdempotencyConflict), errors.Is(err, source.ErrGitWorkspaceDirty), errors.Is(err, gitreview.ErrBranchExists),
		errors.Is(err, gitreview.ErrIdempotencyConflict), errors.Is(err, app.ErrRollbackPreviewInvalid):
		return "conflict", message, ExitConflict
	case errors.As(err, new(*providerOperationError)), errors.Is(err, app.ErrProviderProjectIDMissing), errors.Is(err, provider.ErrUnauthorized), errors.Is(err, provider.ErrNotConfigured), errors.Is(err, provider.ErrValidation):
		return "provider_failed", message, ExitProvider
	case errors.Is(err, project.ErrInvalidManifest), errors.As(err, new(*draft.ValidationError)), errors.Is(err, source.ErrRoundTripBlocked):
		return "validation_failed", message, ExitValidation
	case errors.Is(err, project.ErrNotFound), errors.Is(err, draft.ErrEnvironmentNotFound), errors.Is(err, plan.ErrNotFound), errors.Is(err, app.ErrReleaseNotFound):
		return "not_found", message, ExitUsage
	default:
		return "operation_failed", message, ExitValidation
	}
}

func ensureExamples(command *cobra.Command) {
	if command.Example == "" {
		command.Example = commandExamples[command.CommandPath()]
		if command.Example == "" {
			command.Example = "conflow " + strings.TrimPrefix(command.CommandPath(), "conflow ")
		}
	}
	for _, child := range command.Commands() {
		ensureExamples(child)
	}
}

var commandExamples = map[string]string{
	"conflow":                     "conflow validate --workspace . --environment production --json",
	"conflow version":             "conflow version --json",
	"conflow init":                "conflow init --non-interactive --dir ./photo-editor --project-id photo-editor --project-name 'Photo Editor' --environment-id development --environment-name Development --environment-kind development --json",
	"conflow validate":            "conflow validate --workspace . --environment production --json",
	"conflow plan":                "conflow plan --workspace . --environment production --output plan-artifacts --json",
	"conflow source":              "conflow source status --workspace . --json",
	"conflow source status":       "conflow source status --workspace . --json",
	"conflow source inspect":      "conflow source inspect --workspace . --json",
	"conflow source import":       "conflow source import --workspace . --environment production --json",
	"conflow source preview-save": "conflow source preview-save --workspace . --environment production --json",
	"conflow git":                 "conflow git status --workspace . --json",
	"conflow git status":          "conflow git status --workspace . --json",
	"conflow git prepare":         "conflow git prepare --workspace . --environment production --slug update-config --json",
	"conflow git create-branch":   "conflow git create-branch --workspace . --branch config/update --idempotency-key ci-123 --json",
	"conflow git commit":          "conflow git commit --workspace . --file config/ads.json --message 'chore: update config' --idempotency-key ci-123 --json",
	"conflow save":                "conflow save --workspace . --environment production --json",
	"conflow serve":               "conflow serve --workspace . --address 127.0.0.1:9010",
	"conflow provider":            "conflow provider status --workspace . --environment production --json",
	"conflow provider connect":    "conflow provider connect --workspace . --environment production --path ~/.config/conflow/firebase.json --json",
	"conflow provider status":     "conflow provider status --workspace . --environment production --json",
	"conflow pull":                "conflow pull --workspace . --environment production --json",
	"conflow remote":              "conflow remote validate --workspace . --environment production --plan plan_123 --json",
	"conflow remote validate":     "conflow remote validate --workspace . --environment production --plan plan_123 --json",
	"conflow publish":             "conflow publish --workspace . --environment production --plan plan_123 --confirm --idempotency-key ci-123 --json",
	"conflow release":             "conflow release list --workspace . --environment production --json",
	"conflow release list":        "conflow release list --workspace . --environment production --json",
	"conflow release show":        "conflow release show release_123 --workspace . --environment production --json",
	"conflow rollback":            "conflow rollback --workspace . --environment production --release release_123 --confirm --idempotency-key ci-123 --json",
	"conflow defaults":            "conflow defaults download --workspace . --environment production --format json --output defaults.json --json",
	"conflow defaults download":   "conflow defaults download --workspace . --environment production --format json --output defaults.json --json",
	"conflow project":             "conflow project get --workspace . --json",
	"conflow project get":         "conflow project get --workspace . --json",
	"conflow project update":      "conflow project update --workspace . --id photo-editor --name 'Photo Editor' --json",
	"conflow environment":         "conflow environment list --workspace . --json",
	"conflow environment list":    "conflow environment list --workspace . --json",
	"conflow environment get":     "conflow environment get --workspace . --id production --json",
	"conflow environment create":  "conflow environment create --workspace . --id staging --name Staging --kind staging --provider-project-id photo-editor-staging --json",
	"conflow environment update":  "conflow environment update --workspace . --id production --name Production --json",
	"conflow environment delete":  "conflow environment delete --workspace . --id staging --json",
	"conflow update":              "conflow update --check",
}
