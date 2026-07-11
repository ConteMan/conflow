package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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
	configuration := []byte(`{"placements":[{"id":"invalid_placement","fields":{"enabled":false,"load_timeout_ms":500}}]}`)
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

func TestPlanCommandWritesArtifactsToOutputDirectory(t *testing.T) {
	workspace := t.TempDir()
	init := New("test")
	init.SetArgs([]string{"init", "--dir", workspace})
	if err := init.Execute(); err != nil {
		t.Fatal(err)
	}
	outputDirectory := filepath.Join(t.TempDir(), "plan-output")
	output := &bytes.Buffer{}
	command := New("test")
	command.SetOut(output)
	command.SetArgs([]string{"plan", "--workspace", workspace, "--environment", "development", "--format", "json", "--output", outputDirectory})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"review.json", "review.md"} {
		if _, err := os.Stat(filepath.Join(outputDirectory, name)); err != nil {
			t.Fatalf("output artifact %q: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outputDirectory, "provider-input.json")); !os.IsNotExist(err) {
		t.Fatalf("preview-only output provider artifact error = %v", err)
	}
	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "preview_only" {
		t.Fatalf("plan status = %q", result.Status)
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

func TestGitJSONSourceCommands(t *testing.T) {
	workspace := cliGitJSONWorkspace(t)
	for _, commandLine := range [][]string{
		{"source", "inspect", "--workspace", workspace},
		{"source", "import", "--workspace", workspace, "--environment", "development"},
		{"source", "preview-save", "--workspace", workspace, "--environment", "development"},
	} {
		output := &bytes.Buffer{}
		command := New("test")
		command.SetOut(output)
		command.SetArgs(commandLine)
		if err := command.Execute(); err != nil {
			t.Fatalf("%s: %v", strings.Join(commandLine, " "), err)
		}
		if !json.Valid(output.Bytes()) {
			t.Fatalf("%s output is not JSON: %s", strings.Join(commandLine, " "), output.String())
		}
	}
}

func cliGitJSONWorkspace(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	root := filepath.Join("..", "..", "testdata", "git-json-pdf-launcher")
	for _, relative := range []string{"conflow-ad-profile.yaml", filepath.Join("config", "ads.json")} {
		content, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(workspace, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifest := `version: 1
project:
  id: pdf-launcher
  name: PDF Launcher
pack:
  id: mobile-ad-monetization/v1
source:
  type: git-json
  profile: conflow-ad-profile.yaml
environments:
  - id: development
    name: Development
    kind: development
    provider:
      type: firebase-remote-config
      project_id: pdf-launcher-dev
`
	if err := os.MkdirAll(filepath.Join(workspace, ".conflow"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".conflow", "project.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	ignored := ".conflow/draft.json\n.conflow/validation-results.json\n.conflow/operations.json\n.conflow/plans/\n.conflow/releases.json\n.conflow/remote-snapshots/\n"
	if err := os.WriteFile(filepath.Join(workspace, ".gitignore"), []byte(ignored), 0o644); err != nil {
		t.Fatal(err)
	}
	cliRunGit(t, workspace, "init", "--quiet")
	cliRunGit(t, workspace, "add", ".")
	cliRunGit(t, workspace, "-c", "user.name=Conflow Test", "-c", "user.email=conflow@example.invalid", "commit", "--quiet", "-m", "fixture")
	return workspace
}

func cliRunGit(t *testing.T, workspace string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", workspace}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}
