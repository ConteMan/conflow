package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ConteMan/conflow/internal/project"
)

func TestEnvironmentLifecycle(t *testing.T) {
	service := openTestService(t)
	ctx := context.Background()
	initial, err := service.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}

	createdSnapshot, created, err := service.CreateEnvironment(ctx, initial.Revision, project.Environment{
		ID:       "staging",
		Name:     "Staging",
		Kind:     "staging",
		Provider: project.Provider{Type: "firebase-remote-config", ProjectID: "photo-editor-staging"},
		Publish:  project.Publish{RequiresConfirmation: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "staging" {
		t.Fatalf("created ID = %q", created.ID)
	}

	updatedSnapshot, updated, err := service.UpdateEnvironment(ctx, createdSnapshot.Revision, "staging", project.Environment{
		Name:     "QA Staging",
		Kind:     "production",
		Provider: project.Provider{Type: "firebase-remote-config", ProjectID: "photo-editor-staging-2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != "staging" || updated.Name != "QA Staging" || updated.Kind != "staging" || updated.Provider.ProjectID != "photo-editor-staging-2" {
		t.Fatalf("updated environment = %#v", updated)
	}

	deletedSnapshot, err := service.DeleteEnvironment(ctx, updatedSnapshot.Revision, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if len(deletedSnapshot.Manifest.Environments) != 2 {
		t.Fatalf("environment count = %d", len(deletedSnapshot.Manifest.Environments))
	}
}

func TestCreateEnvironmentRejectsDuplicate(t *testing.T) {
	service := openTestService(t)
	snapshot, _ := service.Snapshot(context.Background())
	_, _, err := service.CreateEnvironment(context.Background(), snapshot.Revision, snapshot.Manifest.Environments[0])
	if !errors.Is(err, project.ErrAlreadyExists) {
		t.Fatalf("error = %v, want ErrAlreadyExists", err)
	}
}

func TestDeleteLastEnvironmentFails(t *testing.T) {
	service := openTestService(t)
	ctx := context.Background()
	snapshot, _ := service.Snapshot(ctx)
	for len(snapshot.Manifest.Environments) > 1 {
		var err error
		snapshot, err = service.DeleteEnvironment(ctx, snapshot.Revision, snapshot.Manifest.Environments[len(snapshot.Manifest.Environments)-1].ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := service.DeleteEnvironment(ctx, snapshot.Revision, snapshot.Manifest.Environments[0].ID)
	if !errors.Is(err, ErrLastEnvironment) {
		t.Fatalf("error = %v, want ErrLastEnvironment", err)
	}
}

func openTestService(t *testing.T) *Service {
	t.Helper()
	workspace := t.TempDir()
	path := project.ManifestPath(workspace)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte(`version: 1
project:
  id: photo-editor
  name: Photo Editor
pack:
  id: mobile-ad-monetization/v1
source:
  type: managed-file
environments:
  - id: development
    name: Development
    kind: development
    provider:
      type: firebase-remote-config
      project_id: photo-editor-dev
    publish:
      requires_confirmation: false
  - id: production
    name: Production
    kind: production
    provider:
      type: firebase-remote-config
      project_id: photo-editor-prod
    publish:
      requires_confirmation: true
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	service, err := Open(workspace)
	if err != nil {
		t.Fatal(err)
	}
	return service
}
