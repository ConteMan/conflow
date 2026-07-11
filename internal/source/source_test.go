package source

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedFileCanonicalSaveIsStable(t *testing.T) {
	workspace := t.TempDir()
	adapter := OpenManagedFile(workspace)
	initial, err := adapter.Load()
	if err != nil {
		t.Fatal(err)
	}
	first, err := adapter.Save(SaveInput{ExpectedRevision: initial.Revision, EnvironmentID: "development", Baseline: map[string]any{"z": "last", "a": map[string]any{"b": float64(2), "a": true}}, EnvironmentOverride: map[string]any{"enabled": true}})
	if err != nil {
		t.Fatal(err)
	}
	basePath := filepath.Join(workspace, ".conflow", "data", "base.yaml")
	firstBytes, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(string(firstBytes), "a:") > strings.Index(string(firstBytes), "z:") {
		t.Fatalf("base output is not key sorted: %s", firstBytes)
	}
	second, err := adapter.Save(SaveInput{ExpectedRevision: first.Revision, EnvironmentID: "development", Baseline: first.Baseline, EnvironmentOverride: first.EnvironmentOverrides["development"]})
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstBytes) != string(secondBytes) || first.Revision != second.Revision {
		t.Fatalf("canonical save changed bytes or digest: %q != %q; %s != %s", firstBytes, secondBytes, first.Revision, second.Revision)
	}
}

func TestManagedFileDetectsExternalModification(t *testing.T) {
	workspace := t.TempDir()
	adapter := OpenManagedFile(workspace)
	initial, err := adapter.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Save(SaveInput{ExpectedRevision: initial.Revision, EnvironmentID: "development", Baseline: map[string]any{"enabled": true}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".conflow", "data", "base.yaml"), []byte("enabled: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := adapter.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !status.ExternalModified {
		t.Fatalf("status = %#v, want external modification", status)
	}
	if _, err := adapter.Save(SaveInput{ExpectedRevision: initial.Revision, EnvironmentID: "development", Baseline: map[string]any{"enabled": true}}); !errors.Is(err, ErrRevisionMismatch) {
		t.Fatalf("save error = %v, want source revision mismatch", err)
	}
}

func TestManagedFileWriteFailurePreservesPreviousFile(t *testing.T) {
	workspace := t.TempDir()
	adapter := OpenManagedFile(workspace)
	initial, err := adapter.Load()
	if err != nil {
		t.Fatal(err)
	}
	first, err := adapter.Save(SaveInput{ExpectedRevision: initial.Revision, EnvironmentID: "development", Baseline: map[string]any{"enabled": true}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace, ".conflow", "data", "base.yaml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	adapter.writeFile = func(string, []byte) error { return errors.New("injected write failure") }
	if _, err := adapter.Save(SaveInput{ExpectedRevision: first.Revision, EnvironmentID: "development", Baseline: map[string]any{"enabled": false}}); err == nil {
		t.Fatal("save unexpectedly succeeded")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("old managed file changed after failed write: %q != %q", before, after)
	}
}
