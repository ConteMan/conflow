package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConteMan/conflow/internal/draft"
	"github.com/ConteMan/conflow/internal/plan"
)

func TestGitJSONServiceImportsPreviewsAndSavesPDFLauncherFixture(t *testing.T) {
	workspace := gitJSONAppWorkspace(t)
	service, err := Open(workspace)
	if err != nil {
		t.Fatal(err)
	}
	inspect, err := service.InspectSource(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !inspect.Matched || inspect.Workspace.Dirty || inspect.ProfilePath != "conflow-ad-profile.yaml" {
		t.Fatalf("inspect = %#v", inspect)
	}

	initial, revision, err := service.GetDraft(context.Background(), "development")
	if err != nil {
		t.Fatal(err)
	}
	imported, revision, err := service.ImportSource(context.Background(), "development", revision, initial.SourceRevision)
	if err != nil {
		t.Fatal(err)
	}
	if !imported.Dirty || !imported.Baseline.Draft.Present || !imported.EnvironmentOverride.Draft.Present {
		t.Fatalf("imported draft = %#v", imported)
	}
	if len(appGitJSONRecords(imported.Effective["placements"])) != 1 || len(appGitJSONRecords(imported.Effective["unit_bindings"])) != 1 {
		t.Fatalf("imported desired configuration = %#v", imported.Effective)
	}
	desired, err := json.Marshal(imported.Effective)
	if err != nil {
		t.Fatal(err)
	}
	template, err := plan.MergeFirebaseTemplate([]byte(`{"parameters":{}}`), desired, []plan.RemoteParameterChange{{ParameterKey: "ad_frequency_inter_global_cap", Managed: true}}, "mobile-ad-monetization/v1", "")
	if err != nil || !strings.Contains(string(template), `"value":"120000"`) {
		t.Fatalf("Firebase desired config = %s, err = %v", template, err)
	}
	preview, err := service.PreviewSourceSave(context.Background(), "development")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Files) != 1 || preview.Files[0].Changed || preview.Digest != imported.SourceRevision {
		t.Fatalf("preview = %#v", preview)
	}

	baseline := imported.Baseline.Resolved.Value
	appGitJSONRecordFields(t, baseline, "feature_switches", "ads_enabled")["default_value"] = false
	configuration, err := json.Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}
	changed, revision, err := service.MutateDraft(context.Background(), "development", draft.Mutation{ExpectedRevision: revision, ExpectedSourceRevision: imported.SourceRevision, Scope: draft.ScopeBaseline, Action: "put", Configuration: configuration})
	if err != nil {
		t.Fatal(err)
	}
	preview, err = service.PreviewSourceSave(context.Background(), "development")
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Files[0].Changed || !strings.Contains(preview.Files[0].Diff, "false") {
		t.Fatalf("changed preview = %#v", preview)
	}
	saved, _, err := service.SaveDraft(context.Background(), "development", revision, changed.SourceRevision)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Dirty || saved.Baseline.Draft.Present || saved.EnvironmentOverride.Draft.Present {
		t.Fatalf("saved draft = %#v", saved)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "config", "ads.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `"enabled": "false"`) {
		t.Fatalf("source was not saved: %s", content)
	}
}

func gitJSONAppWorkspace(t *testing.T) string {
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
  - id: production
    name: Production
    kind: production
    provider:
      type: firebase-remote-config
      project_id: pdf-launcher-prod
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
	appRunGit(t, workspace, "init", "--quiet")
	appRunGit(t, workspace, "add", ".")
	appRunGit(t, workspace, "-c", "user.name=Conflow Test", "-c", "user.email=conflow@example.invalid", "commit", "--quiet", "-m", "fixture")
	return workspace
}

func appRunGit(t *testing.T, workspace string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", workspace}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}

func appGitJSONRecordFields(t *testing.T, configuration map[string]any, collection, id string) map[string]any {
	t.Helper()
	for _, item := range appGitJSONRecords(configuration[collection]) {
		record, _ := item.(map[string]any)
		if record["id"] == id {
			fields, _ := record["fields"].(map[string]any)
			return fields
		}
	}
	t.Fatalf("record %s/%s not found", collection, id)
	return nil
}

func appGitJSONRecords(value any) []any {
	result, _ := value.([]any)
	return result
}
