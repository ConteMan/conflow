package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/ConteMan/conflow/internal/draft"
	"github.com/ConteMan/conflow/internal/entities"
	"github.com/ConteMan/conflow/internal/project"
	"github.com/ConteMan/conflow/internal/provider"
	"github.com/ConteMan/conflow/internal/server"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func nonInteractiveInitArgs(workspace string) []string {
	return []string{"init", "--non-interactive", "--dir", workspace, "--project-id", "photo-editor", "--project-name", "Photo Editor"}
}

func initWorkspace(t *testing.T, workspace string) {
	t.Helper()
	command := New("test")
	command.SetArgs(nonInteractiveInitArgs(workspace))
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
}

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
	initWorkspace(t, workspace)
	output := &bytes.Buffer{}
	command := New("test")
	command.SetOut(output)
	command.SetArgs([]string{"validate", "--workspace", workspace, "--environment", "production", "--json"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Data struct {
			EnvironmentID string `json:"environment_id"`
			Readiness     string `json:"readiness"`
			Diagnostics   []any  `json:"diagnostics"`
		} `json:"data"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.EnvironmentID != "production" || result.Data.Readiness != "ready" || result.Data.Diagnostics == nil {
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
	initWorkspace(t, workspace)
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
		Data struct {
			Readiness string `json:"readiness"`
		} `json:"data"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.Readiness != "blocked" {
		t.Fatalf("readiness = %q, want blocked", result.Data.Readiness)
	}
}

func TestInitAndValidateCommands(t *testing.T) {
	workspace := t.TempDir()

	initOutput := &bytes.Buffer{}
	initCommand := New("test")
	initCommand.SetOut(initOutput)
	initCommand.SetArgs(nonInteractiveInitArgs(workspace))
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

func TestInitJSONIncludesNextSteps(t *testing.T) {
	workspace := t.TempDir()
	output := &bytes.Buffer{}
	command := New("test")
	command.SetOut(output)
	args := append(nonInteractiveInitArgs(workspace), "--json")
	command.SetArgs(args)
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Data initResult `json:"data"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.ProjectPath == "" || len(result.Data.NextSteps) != 3 || !strings.Contains(result.Data.NextSteps[0], "conflow serve --workspace") {
		t.Fatalf("init JSON = %s", output.String())
	}
}

func TestInitWizardAndNonInteractiveValidation(t *testing.T) {
	workspace := t.TempDir()
	wizard := New("test")
	wizardOutput := &bytes.Buffer{}
	wizard.SetOut(wizardOutput)
	wizard.SetIn(strings.NewReader("Sample App\n\n\n\n\n"))
	wizard.SetArgs([]string{"init", "--dir", workspace})
	if err := wizard.Execute(); err != nil {
		t.Fatal(err)
	}
	manifest, err := project.Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Project.ID != "sample-app" || len(manifest.Environments) != 2 || manifest.Environments[0].Provider.ProjectID != "" {
		t.Fatalf("wizard manifest = %#v", manifest)
	}
	if !strings.Contains(wizardOutput.String(), "项目名称（显示用") || !strings.Contains(wizardOutput.String(), "项目 ID（小写字母/数字/连字符，字母开头） [sample-app]") || !strings.Contains(wizardOutput.String(), "下一步：") {
		t.Fatalf("wizard output = %q", wizardOutput.String())
	}

	invalidWorkspace := t.TempDir()
	invalidOutput := &bytes.Buffer{}
	invalid := New("test")
	invalid.SetOut(invalidOutput)
	invalid.SetIn(strings.NewReader("Sample App\nINVALID\nsample-app\n\ninvalid project\nsample-dev\n\n"))
	invalid.SetArgs([]string{"init", "--dir", invalidWorkspace})
	if err := invalid.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(invalidOutput.String(), "项目 ID 必须以小写字母开头") || !strings.Contains(invalidOutput.String(), "Firebase 项目 ID 必须以小写字母开头") {
		t.Fatalf("invalid wizard output = %q", invalidOutput.String())
	}
	if got := deriveProjectID("记账应用"); got != "" {
		t.Fatalf("Chinese display name derived unexpected project ID %q", got)
	}

	adjustedWorkspace := t.TempDir()
	adjustedOutput := &bytes.Buffer{}
	adjusted := New("test")
	adjusted.SetOut(adjustedOutput)
	adjusted.SetIn(strings.NewReader("Custom App\n\ny\nd\ndevelopment\nd\na\nstaging\nStaging\nstaging\n\n\n"))
	adjusted.SetArgs([]string{"init", "--dir", adjustedWorkspace})
	if err := adjusted.Execute(); err != nil {
		t.Fatal(err)
	}
	adjustedManifest, err := project.Load(adjustedWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(adjustedManifest.Environments) != 2 || !hasEnvironment(adjustedManifest.Environments, "production") || !hasEnvironment(adjustedManifest.Environments, "staging") || !strings.Contains(adjustedOutput.String(), "至少保留一个环境") {
		t.Fatalf("adjusted manifest = %#v, output = %q", adjustedManifest, adjustedOutput.String())
	}

	eofOutput := &bytes.Buffer{}
	eof := New("test")
	eof.SetOut(eofOutput)
	eof.SetIn(strings.NewReader("Sample App\n"))
	eof.SetArgs([]string{"init", "--dir", t.TempDir()})
	err = eof.Execute()
	var eofExit *ExitError
	if !errors.As(err, &eofExit) || eofExit.Code != ExitUsage {
		t.Fatalf("EOF error = %#v", err)
	}

	reader, writer, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatal(pipeErr)
	}
	defer reader.Close()
	defer writer.Close()
	nonTTYOutput := &bytes.Buffer{}
	nonTTY := New("test")
	nonTTY.SetIn(reader)
	nonTTY.SetOut(nonTTYOutput)
	nonTTY.SetArgs([]string{"init", "--dir", t.TempDir()})
	err = nonTTY.Execute()
	var nonTTYExit *ExitError
	if !errors.As(err, &nonTTYExit) || nonTTYExit.Code != ExitUsage || nonTTYOutput.Len() != 0 {
		t.Fatalf("non-TTY error = %#v, output = %q", err, nonTTYOutput.String())
	}

	command := New("test")
	command.SetArgs([]string{"init", "--non-interactive", "--project-id", "sample-app"})
	err = command.Execute()
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != ExitUsage {
		t.Fatalf("error = %#v, want usage exit", err)
	}
}

func TestProviderConnectCommandReportsLocalCredentialFailuresAndSuccess(t *testing.T) {
	workspace := t.TempDir()
	initWorkspace(t, workspace)
	service, err := app.Open(workspace)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, production, err := service.GetEnvironment(context.Background(), "production")
	if err != nil {
		t.Fatal(err)
	}
	production.Provider.ProjectID = "photo-editor-prod"
	if _, _, err := service.UpdateEnvironment(context.Background(), snapshot.Revision, "production", production); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	validPath := filepath.Join(directory, "sa.json")
	if err := os.WriteFile(validPath, []byte(`{"type":"service_account","client_email":"bot@example.invalid","private_key":"test-private-key","project_id":"photo-editor-prod"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	output := &bytes.Buffer{}
	command := New("test")
	command.SetOut(output)
	command.SetArgs([]string{"provider", "connect", "--workspace", workspace, "--environment", "production", "--path", validPath})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "已连接 production 环境的 Firebase（photo-editor-prod）") || !strings.Contains(output.String(), "凭据引用：…/sa.json") || !strings.Contains(output.String(), "conflow pull --environment production 拉取线上配置") || strings.Contains(output.String(), validPath) {
		t.Fatalf("success output = %q", output.String())
	}

	tests := []struct {
		name    string
		path    string
		content string
		want    string
	}{
		{name: "missing path", path: filepath.Join(directory, "nope.json"), want: "错误：凭据文件不存在："},
		{name: "bad JSON", path: filepath.Join(directory, "bad.json"), content: `{`, want: "错误：凭据文件不是有效的 JSON"},
		{name: "missing fields", path: filepath.Join(directory, "partial.json"), content: `{"type":"service_account"}`, want: "错误：凭据文件缺少字段：client_email、private_key、project_id"},
		{name: "wrong type", path: filepath.Join(directory, "wrong-type.json"), content: `{"type":"authorized_user"}`, want: "错误：凭据文件不是 Firebase 服务账号 JSON"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.content != "" {
				if err := os.WriteFile(test.path, []byte(test.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			failed := New("test")
			failed.SetArgs([]string{"provider", "connect", "--workspace", workspace, "--environment", "production", "--path", test.path})
			err := failed.Execute()
			var exit *ExitError
			if !errors.As(err, &exit) || exit.Code != ExitValidation || !strings.Contains(exit.Message, test.want) {
				t.Fatalf("error = %#v, want %q", err, test.want)
			}
		})
	}
}

func TestPlanCommandWritesArtifactsToOutputDirectory(t *testing.T) {
	workspace := t.TempDir()
	initWorkspace(t, workspace)
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
	initWorkspace(t, workspace)
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

func TestProjectAndEnvironmentCommandsUseManifestRevisions(t *testing.T) {
	workspace := t.TempDir()
	initWorkspace(t, workspace)

	projectOutput := executeCLI(t, "project", "get", "--workspace", workspace, "--json")
	var gotProject struct {
		Data project.Project `json:"data"`
	}
	if err := json.Unmarshal(projectOutput, &gotProject); err != nil {
		t.Fatal(err)
	}
	if gotProject.Data.ID != "photo-editor" {
		t.Fatalf("project get = %#v", gotProject)
	}

	updated := executeCLI(t, "project", "update", "--workspace", workspace, "--id", "photo-editor", "--name", "Updated Photo Editor", "--json")
	if !bytes.Contains(updated, []byte(`"name":"Updated Photo Editor"`)) {
		t.Fatalf("project update = %s", updated)
	}
	created := executeCLI(t, "environment", "create", "--workspace", workspace, "--id", "staging", "--name", "Staging", "--kind", "staging", "--provider-project-id", "photo-editor-staging", "--json")
	if !bytes.Contains(created, []byte(`"id":"staging"`)) {
		t.Fatalf("environment create = %s", created)
	}
	list := executeCLI(t, "environment", "list", "--workspace", workspace, "--json")
	if !bytes.Contains(list, []byte(`"id":"staging"`)) {
		t.Fatalf("environment list = %s", list)
	}
}

func TestJSONUsageErrorsForNonInteractivePublishAndRollback(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code string
	}{
		{"publish confirmation", []string{"publish", "--json"}, "confirmation_required"},
		{"publish idempotency", []string{"publish", "--json", "--confirm"}, "idempotency_key_required"},
		{"rollback confirmation", []string{"rollback", "--json"}, "confirmation_required"},
		{"rollback idempotency", []string{"rollback", "--json", "--confirm"}, "idempotency_key_required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := &bytes.Buffer{}
			command := New("test")
			command.SetOut(output)
			command.SetArgs(test.args)
			err := command.Execute()
			var exit *ExitError
			if !errors.As(err, &exit) || exit.Code != ExitUsage {
				t.Fatalf("error = %#v, want usage exit", err)
			}
			var envelope struct {
				Error jsonErrorBody `json:"error"`
			}
			if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
				t.Fatalf("output is not JSON: %s", output.String())
			}
			if envelope.Error.Code != test.code {
				t.Fatalf("error code = %q, want %q", envelope.Error.Code, test.code)
			}
		})
	}
}

func TestJSONErrorsCoverArgumentAndPreRunValidation(t *testing.T) {
	tests := [][]string{
		{"release", "show", "--json", "--environment", "production"},
		{"plan", "--json", "--environment", "production", "--format", "yaml"},
	}
	for _, args := range tests {
		output := &bytes.Buffer{}
		command := New("test")
		command.SetOut(output)
		command.SetArgs(args)
		err := command.Execute()
		var exit *ExitError
		if !errors.As(err, &exit) || exit.Code != ExitUsage || !exit.JSON {
			t.Fatalf("conflow %s error = %#v", strings.Join(args, " "), err)
		}
		var envelope struct {
			Error jsonErrorBody `json:"error"`
		}
		if err := json.Unmarshal(output.Bytes(), &envelope); err != nil || envelope.Error.Code != "usage_error" {
			t.Fatalf("conflow %s output = %s, error = %v", strings.Join(args, " "), output, err)
		}
	}
}

func TestExitCodeContract(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"validation", &ExitError{Code: ExitValidation, ErrorCode: "validation_failed"}, ExitValidation},
		{"blocking", &ExitError{Code: ExitBlocking, ErrorCode: "validation_failed"}, ExitBlocking},
		{"conflict", project.ErrAlreadyExists, ExitConflict},
		{"provider", provider.ErrNotConfigured, ExitProvider},
		{"usage", usageError("usage_error", "invalid input"), ExitUsage},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, got := classifyError(test.err)
			if got != test.want {
				t.Fatalf("exit code = %d, want %d", got, test.want)
			}
		})
	}
}

func TestCLIRejectsPlaintextCredentialFlags(t *testing.T) {
	output := &bytes.Buffer{}
	command := New("test")
	command.SetOut(output)
	command.SetArgs([]string{"pull", "--json", "--environment", "production", "--token", "top-secret"})
	err := command.Execute()
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != ExitUsage {
		t.Fatalf("error = %#v, want usage error", err)
	}
	if strings.Contains(output.String(), "top-secret") {
		t.Fatalf("plaintext token leaked in output: %s", output.String())
	}
	if !json.Valid(output.Bytes()) {
		t.Fatalf("output is not JSON: %s", output.String())
	}
	forEachCommand(New("test"), func(command *cobra.Command) {
		command.Flags().VisitAll(func(flag *pflag.Flag) {
			if regexp.MustCompile(`(?i)(token|secret|credential)`).MatchString(flag.Name) {
				t.Errorf("command %q accepts sensitive flag --%s", command.CommandPath(), flag.Name)
			}
		})
	})
}

func TestAllCommandHelpIncludesAnExample(t *testing.T) {
	forEachCommand(New("test"), func(expected *cobra.Command) {
		output := &bytes.Buffer{}
		command := New("test")
		command.SetOut(output)
		path := strings.Fields(strings.TrimPrefix(expected.CommandPath(), "conflow"))
		command.SetArgs(append(path, "--help"))
		if err := command.Execute(); err != nil {
			t.Fatalf("%s help: %v", expected.CommandPath(), err)
		}
		if !strings.Contains(output.String(), "Examples:") {
			t.Fatalf("%s help is missing an example: %s", expected.CommandPath(), output.String())
		}
	})
}

func TestValidationJSONMatchesAPIGoldenFixture(t *testing.T) {
	workspace := t.TempDir()
	initWorkspace(t, workspace)
	service, err := app.Open(workspace)
	if err != nil {
		t.Fatal(err)
	}
	fixture := loadCLIGoldenFixture(t)
	configureCLIGoldenDraft(t, service, fixture)

	handler := server.New(service)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/drafts/production:validate", nil)
	request.Host = "127.0.0.1:9010"
	request.Header.Set("Origin", "http://127.0.0.1:9010")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("API validation = %d: %s", response.Code, response.Body.String())
	}
	var apiResult struct {
		Data validationResult `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &apiResult); err != nil {
		t.Fatal(err)
	}

	output := &bytes.Buffer{}
	command := New("test")
	command.SetOut(output)
	command.SetArgs([]string{"validate", "--workspace", workspace, "--environment", "production", "--json"})
	err = command.Execute()
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != ExitBlocking {
		t.Fatalf("CLI validation error = %#v, want blocking exit", err)
	}
	var cliResult struct {
		Data validationResult `json:"data"`
	}
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatal(err)
	}
	if cliResult.Data.Readiness != apiResult.Data.Readiness {
		t.Fatalf("readiness CLI=%q API=%q", cliResult.Data.Readiness, apiResult.Data.Readiness)
	}
	if len(cliResult.Data.Diagnostics) != len(apiResult.Data.Diagnostics) {
		t.Fatalf("diagnostics CLI=%d API=%d", len(cliResult.Data.Diagnostics), len(apiResult.Data.Diagnostics))
	}
	for index := range cliResult.Data.Diagnostics {
		if cliResult.Data.Diagnostics[index] != apiResult.Data.Diagnostics[index] {
			t.Fatalf("diagnostic %d CLI=%#v API=%#v", index, cliResult.Data.Diagnostics[index], apiResult.Data.Diagnostics[index])
		}
	}
}

func executeCLI(t *testing.T, args ...string) []byte {
	t.Helper()
	output := &bytes.Buffer{}
	command := New("test")
	command.SetOut(output)
	command.SetArgs(args)
	if err := command.Execute(); err != nil {
		t.Fatalf("conflow %s: %v", strings.Join(args, " "), err)
	}
	if !json.Valid(output.Bytes()) {
		t.Fatalf("conflow %s did not produce JSON: %s", strings.Join(args, " "), output.String())
	}
	return output.Bytes()
}

func forEachCommand(command *cobra.Command, visit func(*cobra.Command)) {
	visit(command)
	for _, child := range command.Commands() {
		forEachCommand(child, visit)
	}
}

type validationDiagnostic struct {
	Code     string `json:"code"`
	Path     string `json:"path"`
	Severity string `json:"severity"`
}

type validationResult struct {
	Readiness   string                 `json:"readiness"`
	Diagnostics []validationDiagnostic `json:"diagnostics"`
}

type cliGoldenFixture struct {
	Placements        []any
	FrequencyPolicies []any
	FeatureSwitches   []any
	Bindings          []any
	Replacements      []cliGoldenReplacement
}

type cliGoldenReplacement struct {
	EntityType string         `json:"entity_type"`
	EntityID   string         `json:"entity_id"`
	Fields     map[string]any `json:"fields"`
}

func loadCLIGoldenFixture(t *testing.T) cliGoldenFixture {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "contracts", "mobile-ad-monetization", "v1")
	entitiesContent, err := os.ReadFile(filepath.Join(root, "entities.json"))
	if err != nil {
		t.Fatal(err)
	}
	overlaysContent, err := os.ReadFile(filepath.Join(root, "validation-overlays.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entitiesFixture struct {
		Entities struct {
			Placements        []any `json:"placements"`
			FrequencyPolicies []any `json:"frequency_policies"`
			FeatureSwitches   []any `json:"feature_switches"`
		} `json:"entities"`
		UnitBindingMatrix struct {
			Rows []struct {
				PlacementID string         `json:"placement_id"`
				Production  map[string]any `json:"production"`
			} `json:"rows"`
		} `json:"unit_binding_matrix"`
	}
	if err := json.Unmarshal(entitiesContent, &entitiesFixture); err != nil {
		t.Fatal(err)
	}
	var overlays struct {
		Scenarios []struct {
			ID      string `json:"id"`
			Overlay struct {
				EntityReplacements []cliGoldenReplacement `json:"entity_replacements"`
			} `json:"overlay"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(overlaysContent, &overlays); err != nil {
		t.Fatal(err)
	}
	fixture := cliGoldenFixture{Placements: entitiesFixture.Entities.Placements, FrequencyPolicies: entitiesFixture.Entities.FrequencyPolicies, FeatureSwitches: entitiesFixture.Entities.FeatureSwitches}
	for _, scenario := range overlays.Scenarios {
		if scenario.ID == "nine-human-fixture-diagnostics" {
			fixture.Replacements = scenario.Overlay.EntityReplacements
			break
		}
	}
	if fixture.Replacements == nil {
		t.Fatal("nine-human-fixture-diagnostics scenario is missing")
	}
	for _, row := range entitiesFixture.UnitBindingMatrix.Rows {
		for _, platform := range []string{"ios", "android"} {
			fixture.Bindings = append(fixture.Bindings, map[string]any{
				"id": "ub_production_" + platform + "_" + row.PlacementID, "placement_id": row.PlacementID,
				"environment_id": "production", "platform": platform, "unit_id_ref": row.Production[platform], "status": "configured",
			})
		}
	}
	return fixture
}

func configureCLIGoldenDraft(t *testing.T, service *app.Service, fixture cliGoldenFixture) {
	t.Helper()
	collections := map[string][]any{"placement": fixture.Placements, "frequency_policy": fixture.FrequencyPolicies, "feature_switch": fixture.FeatureSwitches}
	for _, replacement := range fixture.Replacements {
		for _, value := range collections[replacement.EntityType] {
			record, ok := value.(map[string]any)
			if !ok {
				continue
			}
			if record["id"] == replacement.EntityID {
				for field, value := range replacement.Fields {
					record[field] = value
				}
			}
		}
	}
	view, revision, err := service.GetDraft(context.Background(), "production")
	if err != nil {
		t.Fatal(err)
	}
	baseline := entities.AdaptFlatFixture(map[string]any{"placements": fixture.Placements, "frequency_policies": fixture.FrequencyPolicies, "feature_switches": fixture.FeatureSwitches})
	_, revision, err = service.MutateDraft(context.Background(), "production", draft.Mutation{ExpectedRevision: revision, ExpectedSourceRevision: view.SourceRevision, Scope: draft.ScopeBaseline, Action: "put", Configuration: marshalFixtureConfiguration(t, baseline)})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = service.MutateDraft(context.Background(), "production", draft.Mutation{ExpectedRevision: revision, ExpectedSourceRevision: view.SourceRevision, Scope: draft.ScopeEnvironmentOverride, Action: "put", Configuration: marshalFixtureConfiguration(t, entities.AdaptFlatFixture(map[string]any{"unit_bindings": fixture.Bindings}))})
	if err != nil {
		t.Fatal(err)
	}
}

func marshalFixtureConfiguration(t *testing.T, configuration any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
