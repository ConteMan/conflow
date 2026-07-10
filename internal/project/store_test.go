package project

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/goccy/go-yaml"
)

func TestStoreSnapshotReturnsDeepCopy(t *testing.T) {
	store := openTestStore(t)
	first, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	first.Manifest.Environments[0].ID = "changed"
	second, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if second.Manifest.Environments[0].ID == "changed" {
		t.Fatal("Snapshot() exposed mutable store state")
	}
}

func TestStoreDetectsExternalChange(t *testing.T) {
	store := openTestStore(t)
	before, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	manifest := before.Manifest
	manifest.Project.Name = "Changed outside Conflow"
	writeManifest(t, store.path, manifest)

	after, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision+1 {
		t.Fatalf("revision = %d, want %d", after.Revision, before.Revision+1)
	}
	if after.Manifest.Project.Name != "Changed outside Conflow" {
		t.Fatalf("project name = %q", after.Manifest.Project.Name)
	}
	if after.Digest == before.Digest {
		t.Fatal("digest did not change")
	}
}

func TestStoreRejectsStaleRevision(t *testing.T) {
	store := openTestStore(t)
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Update(snapshot.Revision, func(manifest *Manifest) error {
		manifest.Project.Name = "First update"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Update(snapshot.Revision, func(manifest *Manifest) error {
		manifest.Project.Name = "Stale update"
		return nil
	})
	if !errors.Is(err, ErrRevisionMismatch) {
		t.Fatalf("error = %v, want ErrRevisionMismatch", err)
	}
	var mismatch *RevisionMismatchError
	if !errors.As(err, &mismatch) || mismatch.Current != snapshot.Revision+1 {
		t.Fatalf("mismatch = %#v", mismatch)
	}
}

func TestStoreWritesValidManifest(t *testing.T) {
	store := openTestStore(t)
	before, _ := store.Snapshot()
	after, err := store.Update(before.Revision, func(manifest *Manifest) error {
		manifest.Project.Name = "Saved by Conflow"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(filepath.Dir(filepath.Dir(store.path)))
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Project.Name != "Saved by Conflow" {
		t.Fatalf("project name = %q", loaded.Project.Name)
	}
	if after.Revision != before.Revision+1 {
		t.Fatalf("revision = %d", after.Revision)
	}
}

func TestStoreRejectsInvalidExternalManifest(t *testing.T) {
	store := openTestStore(t)
	if err := os.WriteFile(store.path, []byte("version: 1\nproject: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := store.Snapshot()
	if !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("error = %v, want ErrInvalidManifest", err)
	}
	_, err = store.Snapshot()
	if !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("second error = %v, want ErrInvalidManifest", err)
	}
}

func TestOpenRejectsInvalidManifest(t *testing.T) {
	workspace := t.TempDir()
	path := ManifestPath(workspace)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("version: 1\nproject: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Open(workspace)
	if !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("error = %v, want ErrInvalidManifest", err)
	}
}

func TestStoreConcurrentSnapshots(t *testing.T) {
	store := openTestStore(t)
	var wait sync.WaitGroup
	errorsFound := make(chan error, 32)
	for range 32 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := store.Snapshot()
			errorsFound <- err
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestStoreSerializesConcurrentUpdates(t *testing.T) {
	store := openTestStore(t)
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}

	results := make(chan error, 2)
	var wait sync.WaitGroup
	for _, name := range []string{"First", "Second"} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, updateErr := store.Update(snapshot.Revision, func(manifest *Manifest) error {
				manifest.Project.Name = name
				return nil
			})
			results <- updateErr
		}()
	}
	wait.Wait()
	close(results)

	successes := 0
	conflicts := 0
	for result := range results {
		switch {
		case result == nil:
			successes++
		case errors.Is(result, ErrRevisionMismatch):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", result)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes = %d, conflicts = %d", successes, conflicts)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	workspace := t.TempDir()
	path := ManifestPath(workspace)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, path, validTestManifest())
	store, err := Open(workspace)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func writeManifest(t *testing.T, path string, manifest Manifest) {
	t.Helper()
	content, err := yaml.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func validTestManifest() Manifest {
	return Manifest{
		Version: 1,
		Project: Project{ID: "photo-editor", Name: "Photo Editor"},
		Pack:    PackReference{ID: "mobile-ad-monetization/v1"},
		Source:  Source{Type: "managed-file"},
		Environments: []Environment{
			{ID: "development", Provider: Provider{Type: "firebase-remote-config", ProjectID: "photo-editor-dev"}},
			{ID: "production", Provider: Provider{Type: "firebase-remote-config", ProjectID: "photo-editor-prod"}, Publish: Publish{RequiresConfirmation: true}},
		},
	}
}
