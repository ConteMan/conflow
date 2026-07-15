package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ConteMan/conflow/internal/draft"
	"github.com/ConteMan/conflow/internal/entities"
	"github.com/ConteMan/conflow/internal/project"
	"github.com/ConteMan/conflow/internal/source"
)

func TestMutateV2PlacementDropsLegacyCachePolicy(t *testing.T) {
	workspace := t.TempDir()
	if _, err := project.CreateExample(workspace); err != nil {
		t.Fatal(err)
	}
	store, err := project.Open(workspace)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(snapshot.Revision, func(manifest *project.Manifest) error {
		manifest.Pack.ID = "mobile-ad-monetization/v2"
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	adapter := source.OpenManagedFile(workspace)
	initial, err := adapter.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Save(source.SaveInput{ExpectedRevision: initial.Revision, EnvironmentID: "development", Baseline: v2ConfigurationWithLegacyCachePolicy(t)}); err != nil {
		t.Fatal(err)
	}
	service, err := Open(workspace)
	if err != nil {
		t.Fatal(err)
	}

	before, revision, err := service.GetEntity(context.Background(), "development", "placement", "interstitial_main")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := before.Effective.Value.Fields["cache_policy"]; !ok {
		t.Fatal("legacy cache_policy must remain readable before replacement")
	}

	fields := make(map[string]any, len(before.Effective.Value.Fields)-1)
	for name, value := range before.Effective.Value.Fields {
		if name != "cache_policy" {
			fields[name] = value
		}
	}
	replaced, _, err := service.MutateEntity(context.Background(), "development", EntityMutation{
		ExpectedRevision: revision, ExpectedSourceRevision: before.SourceRevision, Scope: draft.ScopeBaseline,
		EntityType: "placement", EntityID: "interstitial_main", Entity: &EntityRecord{ID: "interstitial_main", Fields: fields}, Action: "replace",
	})
	if err != nil {
		t.Fatalf("replace placement with legacy cache_policy in source: %v", err)
	}
	if _, ok := replaced.Effective.Value.Fields["cache_policy"]; ok {
		t.Fatalf("replaced entity fields still contain cache_policy: %#v", replaced.Effective.Value.Fields)
	}

	after, _, err := service.GetEntity(context.Background(), "development", "placement", "interstitial_main")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := after.Effective.Value.Fields["cache_policy"]; ok {
		t.Fatalf("saved placement fields still contain cache_policy: %#v", after.Effective.Value.Fields)
	}
}

func v2ConfigurationWithLegacyCachePolicy(t *testing.T) map[string]any {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "mobile-ad-monetization", "v2", "minimal-valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Entities map[string]any `json:"entities"`
	}
	if err := json.Unmarshal(content, &fixture); err != nil {
		t.Fatal(err)
	}
	return entities.AdaptFlatFixture(fixture.Entities)
}
