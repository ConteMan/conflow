package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/ConteMan/conflow/internal/draft"
)

func TestValidateListenAddress(t *testing.T) {
	for _, address := range []string{"127.0.0.1:9010", "[::1]:9010", "localhost:9010"} {
		if err := validateListenAddress(address); err != nil {
			t.Fatalf("validateListenAddress(%q) error = %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:9010", ":9010", "192.168.1.10:9010"} {
		if err := validateListenAddress(address); err == nil {
			t.Fatalf("validateListenAddress(%q) error = nil", address)
		}
	}
}

func TestValidateEnvironmentJSONUsesCompleteValidation(t *testing.T) {
	workspace := t.TempDir()
	init := New("test")
	init.SetArgs([]string{"init", "--dir", workspace})
	if err := init.Execute(); err != nil {
		t.Fatal(err)
	}
	output := &bytes.Buffer{}
	command := New("test")
	command.SetOut(output)
	command.SetArgs([]string{"validate", "--workspace", workspace, "--environment", "production", "--json"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	var result struct {
		EnvironmentID string `json:"environment_id"`
		Readiness     string `json:"readiness"`
		Diagnostics   []any  `json:"diagnostics"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.EnvironmentID != "production" || result.Readiness != "ready" || result.Diagnostics == nil {
		t.Fatalf("result = %#v", result)
	}
}

func TestExitErrorPreservesValidationStatus(t *testing.T) {
	err := &ExitError{Code: 2}
	var typed *ExitError
	if !errors.As(err, &typed) || typed.Code != 2 {
		t.Fatalf("exit error = %#v", typed)
	}
}

func TestValidateCompleteValidationReturnsSeverityExitCode(t *testing.T) {
	workspace := t.TempDir()
	init := New("test")
	init.SetArgs([]string{"init", "--dir", workspace})
	if err := init.Execute(); err != nil {
		t.Fatal(err)
	}
	service, err := app.Open(workspace)
	if err != nil {
		t.Fatal(err)
	}
	view, revision, err := service.GetDraft(context.Background(), "production")
	if err != nil {
		t.Fatal(err)
	}
	configuration := []byte(`{"placements":[{"id":"invalid_placement","enabled":false,"load_timeout_ms":500}]}`)
	if _, _, err := service.MutateDraft(context.Background(), "production", draft.Mutation{
		ExpectedRevision: revision, ExpectedSourceRevision: view.SourceRevision, Scope: draft.ScopeBaseline, Action: "put", Configuration: configuration,
	}); err != nil {
		t.Fatal(err)
	}

	output := &bytes.Buffer{}
	command := New("test")
	command.SetOut(output)
	command.SetArgs([]string{"validate", "--workspace", workspace, "--environment", "production", "--json"})
	err = command.Execute()
	var exitError *ExitError
	if !errors.As(err, &exitError) || exitError.Code != 1 {
		t.Fatalf("validate error = %#v, want exit code 1", err)
	}
	var result struct {
		Readiness string `json:"readiness"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Readiness != "blocked" {
		t.Fatalf("readiness = %q, want blocked", result.Readiness)
	}
}

func TestInitAndValidateCommands(t *testing.T) {
	workspace := t.TempDir()

	initOutput := &bytes.Buffer{}
	initCommand := New("test")
	initCommand.SetOut(initOutput)
	initCommand.SetArgs([]string{"init", "--dir", workspace})
	if err := initCommand.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(initOutput.String(), ".conflow/project.yaml") {
		t.Fatalf("init output = %q", initOutput.String())
	}

	validateOutput := &bytes.Buffer{}
	validateCommand := New("test")
	validateCommand.SetOut(validateOutput)
	validateCommand.SetArgs([]string{"validate", "--workspace", workspace})
	if err := validateCommand.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := validateOutput.String(); got != "validated photo-editor with 2 environments\n" {
		t.Fatalf("validate output = %q", got)
	}
}

func TestSourceStatusAndSaveCommands(t *testing.T) {
	workspace := t.TempDir()
	init := New("test")
	init.SetArgs([]string{"init", "--dir", workspace})
	if err := init.Execute(); err != nil {
		t.Fatal(err)
	}
	statusOutput := &bytes.Buffer{}
	status := New("test")
	status.SetOut(statusOutput)
	status.SetArgs([]string{"source", "status", "--workspace", workspace})
	if err := status.Execute(); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Type   string `json:"type"`
		Digest string `json:"digest"`
	}
	if err := json.Unmarshal(statusOutput.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Type != "managed-file" || result.Digest == "" {
		t.Fatalf("source status = %#v", result)
	}
	saveOutput := &bytes.Buffer{}
	save := New("test")
	save.SetOut(saveOutput)
	save.SetArgs([]string{"save", "--workspace", workspace, "--environment", "development"})
	if err := save.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(saveOutput.String(), "saved development") {
		t.Fatalf("save output = %q", saveOutput.String())
	}
}
