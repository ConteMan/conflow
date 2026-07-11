package source

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestGitJSONPDFLauncherRoundTripGoldenAndPreview(t *testing.T) {
	workspace := gitJSONFixtureWorkspace(t)
	adapter, err := OpenGitJSON(workspace, "conflow-ad-profile.yaml")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := adapter.Load()
	if err != nil {
		t.Fatal(err)
	}
	assertGitJSONSnapshotGolden(t, snapshot)

	preview, err := adapter.Preview(SaveInput{ExpectedRevision: snapshot.Revision, EnvironmentID: "development", Baseline: snapshot.Baseline, EnvironmentOverride: snapshot.EnvironmentOverrides["development"]})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Digest != snapshot.Revision || len(preview.Files) != 1 || preview.Files[0].Changed || preview.Files[0].Diff != "" {
		t.Fatalf("no-op preview = %#v", preview)
	}
	if _, err := adapter.Save(SaveInput{ExpectedRevision: snapshot.Revision, EnvironmentID: "development", Baseline: snapshot.Baseline, EnvironmentOverride: snapshot.EnvironmentOverrides["development"]}); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(workspace, "config", "ads.json"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(gitJSONFixtureRoot(t), "config", "ads.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != string(want) {
		t.Fatalf("round-trip bytes differ\nwant:\n%s\ngot:\n%s", want, content)
	}

	changedBaseline := cloneMap(snapshot.Baseline)
	fields := gitJSONRecordFields(t, changedBaseline, "frequency_policies", "inter_global_cap")
	fields["cooldown_ms"] = float64(180000)
	fields = gitJSONRecordFields(t, changedBaseline, "feature_switches", "ads_enabled")
	fields["default_value"] = false
	preview, err = adapter.Preview(SaveInput{ExpectedRevision: snapshot.Revision, EnvironmentID: "development", Baseline: changedBaseline, EnvironmentOverride: snapshot.EnvironmentOverrides["development"]})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Files) != 1 || !preview.Files[0].Changed || !strings.Contains(preview.Files[0].Diff, "180") || !strings.Contains(preview.Files[0].Diff, "legacy_note") {
		t.Fatalf("changed preview = %#v", preview)
	}
	if _, err := adapter.Save(SaveInput{ExpectedRevision: snapshot.Revision, EnvironmentID: "development", Baseline: changedBaseline, EnvironmentOverride: snapshot.EnvironmentOverrides["development"]}); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(filepath.Join(workspace, "config", "ads.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(content, &document); err != nil {
		t.Fatal(err)
	}
	if document["metadata"].(map[string]any)["unmanaged"] != true || document["flags"].([]any)[0].(map[string]any)["legacy_note"] != "preserve this field" {
		t.Fatalf("unmanaged values were not preserved: %#v", document)
	}
	if document["frequency"].([]any)[0].(map[string]any)["cooldown_seconds"] != float64(180) || document["flags"].([]any)[0].(map[string]any)["enabled"] != "false" {
		t.Fatalf("mapped transforms were not written: %#v", document)
	}
}

func TestGitJSONBlocksConditionalValuesAndDirtyWorkspace(t *testing.T) {
	t.Run("conditional values", func(t *testing.T) {
		workspace := gitJSONFixtureWorkspace(t)
		path := filepath.Join(workspace, "config", "ads.json")
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		content = []byte(strings.Replace(string(content), `"legacy_note": "preserve this field",`, "\"conditionalValues\": {\"country\": \"true\"},\n      \"legacy_note\": \"preserve this field\",", 1))
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
		adapter, err := OpenGitJSON(workspace, "conflow-ad-profile.yaml")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := adapter.Load(); !errors.Is(err, ErrRoundTripBlocked) {
			t.Fatalf("Load() error = %v, want round-trip block", err)
		}
		inspect, err := adapter.Inspect()
		if err != nil {
			t.Fatal(err)
		}
		if inspect.Matched || len(inspect.Diagnostics) != 1 || inspect.Diagnostics[0].Code != "conditional_value" {
			t.Fatalf("inspect = %#v", inspect)
		}
	})
	t.Run("dirty workspace", func(t *testing.T) {
		workspace := gitJSONFixtureWorkspace(t)
		adapter, err := OpenGitJSON(workspace, "conflow-ad-profile.yaml")
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := adapter.Load()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(workspace, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err = adapter.Save(SaveInput{ExpectedRevision: snapshot.Revision, EnvironmentID: "development", Baseline: snapshot.Baseline, EnvironmentOverride: snapshot.EnvironmentOverrides["development"]})
		if !errors.Is(err, ErrGitWorkspaceDirty) {
			t.Fatalf("Save() error = %v, want dirty workspace", err)
		}
	})
}

func TestGitJSONPathValidationAcrossPlatforms(t *testing.T) {
	root := t.TempDir()
	for _, candidate := range []string{"../escape.json", `..\\escape.json`, "/tmp/escape.json", `C:\\escape.json`, "C:/escape.json"} {
		if _, err := safePath(root, candidate); err == nil {
			t.Fatalf("safePath(%q) unexpectedly succeeded", candidate)
		}
	}
	path, err := safePath(root, `config\\remote-config\\ads.json`)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(root, "config", "remote-config", "ads.json") {
		t.Fatalf("normalized path = %q", path)
	}
}

func gitJSONFixtureWorkspace(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	root := gitJSONFixtureRoot(t)
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
	runGitForTest(t, workspace, "init", "--quiet")
	runGitForTest(t, workspace, "add", ".")
	runGitForTest(t, workspace, "-c", "user.name=Conflow Test", "-c", "user.email=conflow@example.invalid", "commit", "--quiet", "-m", "fixture")
	return workspace
}

func gitJSONFixtureRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", "git-json-pdf-launcher")
}

func runGitForTest(t *testing.T, workspace string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", workspace}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}

func assertGitJSONSnapshotGolden(t *testing.T, snapshot Snapshot) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(gitJSONFixtureRoot(t), "expected-snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want struct {
		Baseline             map[string]any            `json:"baseline"`
		EnvironmentOverrides map[string]map[string]any `json:"environment_overrides"`
	}
	if err := json.Unmarshal(content, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot.Baseline, want.Baseline) || !reflect.DeepEqual(snapshot.EnvironmentOverrides, want.EnvironmentOverrides) {
		t.Fatalf("snapshot does not match golden\nwant: %#v\ngot: %#v", want, snapshot)
	}
}

func gitJSONRecordFields(t *testing.T, configuration map[string]any, collection, id string) map[string]any {
	t.Helper()
	for _, value := range recordsOf(configuration[collection]) {
		record, _ := value.(map[string]any)
		if record["id"] == id {
			fields, _ := record["fields"].(map[string]any)
			return fields
		}
	}
	t.Fatalf("record %s/%s not found", collection, id)
	return nil
}
