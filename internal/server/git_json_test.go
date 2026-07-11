package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/ConteMan/conflow/internal/gitreview"
)

func TestGitJSONSourceHandlersAndErrorMapping(t *testing.T) {
	workspace := gitJSONServerWorkspace(t, false)
	service, err := app.Open(workspace)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(service)
	initial := getDraftForTest(t, handler, "development")

	inspect := executeRequest(t, handler, "POST", "/api/v1/source:inspect", "", []byte(`{}`))
	if inspect.Code != 200 || !strings.Contains(inspect.Body.String(), `"matched":true`) {
		t.Fatalf("inspect status = %d, body = %s", inspect.Code, inspect.Body.String())
	}
	importBody := []byte(`{"environment_id":"development","expected_source_revision":"` + initial.SourceRevision + `"}`)
	imported := executeRequest(t, handler, "POST", "/api/v1/source:import", `"1"`, importBody)
	if imported.Code != 200 || imported.Header().Get("ETag") != `"3"` {
		t.Fatalf("import status = %d, etag = %q, body = %s", imported.Code, imported.Header().Get("ETag"), imported.Body.String())
	}
	preview := executeRequest(t, handler, "POST", "/api/v1/source:preview-save", "", []byte(`{"environment_id":"development"}`))
	if preview.Code != 200 || !strings.Contains(preview.Body.String(), `"changed":false`) {
		t.Fatalf("preview status = %d, body = %s", preview.Code, preview.Body.String())
	}
	if err := os.WriteFile(filepath.Join(workspace, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	saveBody := []byte(`{"expected_source_revision":"` + initial.SourceRevision + `"}`)
	saved := executeRequest(t, handler, "POST", "/api/v1/drafts/development:save", `"3"`, saveBody)
	assertDraftError(t, saved, 409, "git_workspace_dirty")
}

func TestGitJSONImportHandlerBlocksConditionalValues(t *testing.T) {
	workspace := gitJSONServerWorkspace(t, true)
	service, err := app.Open(workspace)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(service)
	response := executeRequest(t, handler, "POST", "/api/v1/source:import", `"1"`, []byte(`{"environment_id":"development","expected_source_revision":"src-any"}`))
	assertDraftError(t, response, 422, "round_trip_blocked")
}

func TestGitReviewHandlersMapBranchAndCommitFailures(t *testing.T) {
	workspace := gitJSONServerWorkspace(t, false)
	service, err := app.Open(workspace)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(service)
	first := newAPIRequest(t, http.MethodPost, "/api/v1/git:create-branch", []byte(`{"branch":"conflow/review"}`))
	first.Header.Set("Idempotency-Key", "branch-key-0000001")
	firstRecorder := httptest.NewRecorder()
	handler.ServeHTTP(firstRecorder, first)
	if firstRecorder.Code != http.StatusOK {
		t.Fatalf("create branch = %d: %s", firstRecorder.Code, firstRecorder.Body.String())
	}
	exists := newAPIRequest(t, http.MethodPost, "/api/v1/git:create-branch", []byte(`{"branch":"conflow/review"}`))
	exists.Header.Set("Idempotency-Key", "branch-key-0000002")
	existsRecorder := httptest.NewRecorder()
	handler.ServeHTTP(existsRecorder, exists)
	assertGitError(t, existsRecorder, http.StatusConflict, "git_branch_exists")
	if err := os.WriteFile(filepath.Join(workspace, "config", "ads.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".git", "hooks", "pre-commit"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	commit := newAPIRequest(t, http.MethodPost, "/api/v1/git:commit", []byte(`{"files":["config/ads.json"],"message":"feat(development): update config"}`))
	commit.Header.Set("Idempotency-Key", "commit-key-0000001")
	commitRecorder := httptest.NewRecorder()
	handler.ServeHTTP(commitRecorder, commit)
	assertGitError(t, commitRecorder, http.StatusConflict, "git_operation_failed")
	if status := serverGitOutput(t, workspace, "status", "--porcelain"); !strings.Contains(status, "M  config/ads.json") {
		t.Fatalf("managed file must remain visibly staged after failed commit: %q", status)
	}
}

func TestGitReviewMapsNonRepositoryError(t *testing.T) {
	request := newAPIRequest(t, http.MethodGet, "/api/v1/git/status", nil)
	recorder := httptest.NewRecorder()
	newAPI(nil).writeGitError(recorder, request, gitreview.ErrNotGitRepository)
	assertGitError(t, recorder, http.StatusBadRequest, "git_not_repository")
}

func assertGitError(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, status, recorder.Body.String())
	}
	var response errorEnvelope
	decodeResponse(t, recorder, &response)
	if response.Error.Code != code {
		t.Fatalf("error code = %q, want %q", response.Error.Code, code)
	}
}

func gitJSONServerWorkspace(t *testing.T, conditional bool) string {
	t.Helper()
	workspace := t.TempDir()
	root := filepath.Join("..", "..", "testdata", "git-json-pdf-launcher")
	for _, relative := range []string{"conflow-ad-profile.yaml", filepath.Join("config", "ads.json")} {
		content, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			t.Fatal(err)
		}
		if conditional && relative == filepath.Join("config", "ads.json") {
			content = []byte(strings.Replace(string(content), `"legacy_note": "preserve this field",`, "\"conditionalValues\": {\"country\": \"true\"},\n      \"legacy_note\": \"preserve this field\",", 1))
		}
		path := filepath.Join(workspace, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifest := map[string]any{
		"version": 1,
		"project": map[string]any{"id": "pdf-launcher", "name": "PDF Launcher"},
		"pack":    map[string]any{"id": "mobile-ad-monetization/v1"},
		"source":  map[string]any{"type": "git-json", "profile": "conflow-ad-profile.yaml"},
		"environments": []any{
			map[string]any{"id": "development", "name": "Development", "kind": "development", "provider": map[string]any{"type": "firebase-remote-config", "project_id": "pdf-launcher-dev"}},
		},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, ".conflow"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".conflow", "project.yaml"), manifestJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	ignored := ".conflow/draft.json\n.conflow/validation-results.json\n.conflow/operations.json\n.conflow/plans/\n.conflow/releases.json\n.conflow/remote-snapshots/\n"
	if err := os.WriteFile(filepath.Join(workspace, ".gitignore"), []byte(ignored), 0o644); err != nil {
		t.Fatal(err)
	}
	serverRunGit(t, workspace, "init", "--quiet")
	serverRunGit(t, workspace, "add", ".")
	serverRunGit(t, workspace, "-c", "user.name=Conflow Test", "-c", "user.email=conflow@example.invalid", "commit", "--quiet", "-m", "fixture")
	return workspace
}

func serverRunGit(t *testing.T, workspace string, args ...string) {
	t.Helper()
	command := exec.CommandContext(context.Background(), "git", append([]string{"-C", workspace}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}

func serverGitOutput(t *testing.T, workspace string, args ...string) string {
	t.Helper()
	command := exec.CommandContext(context.Background(), "git", append([]string{"-C", workspace}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
	return string(output)
}
