package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ConteMan/conflow/internal/entities"
	"github.com/ConteMan/conflow/internal/importer"
	"github.com/ConteMan/conflow/internal/project"
	"github.com/ConteMan/conflow/internal/source"
)

func TestV2StrategyImportRoundTrip(t *testing.T) {
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
	if _, err := adapter.Save(source.SaveInput{ExpectedRevision: initial.Revision, EnvironmentID: "development", Baseline: v2StrategyFixture(t)}); err != nil {
		t.Fatal(err)
	}
	service, err := Open(workspace)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	before, err := service.ExportImportBundle(ctx, "development")
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Entities["ad_strategy_settings"]) != 1 || len(before.Entities["ad_strategy"]) != 2 {
		t.Fatalf("strategy export = %#v", before.Entities)
	}
	if _, exists := before.Entities["unit_binding"]; exists {
		t.Fatal("import bundle must exclude unit bindings")
	}
	preview, err := service.PreviewImport(ctx, "development", before, importer.ConflictReplace)
	if err != nil {
		t.Fatal(err)
	}
	_, _, result, err := service.ApplyImport(ctx, "development", before, preview.PreviewToken, nil, importer.ConflictReplace)
	if err != nil {
		t.Fatal(err)
	}
	if result.AppliedCount == 0 {
		t.Fatal("round-trip import applied no entities")
	}
	after, err := service.ExportImportBundle(ctx, "development")
	if err != nil {
		t.Fatal(err)
	}
	if before.SourceDigest != after.SourceDigest || !reflect.DeepEqual(before.Entities, after.Entities) {
		t.Fatalf("strategy import changed the exported bundle:\nbefore=%#v\nafter=%#v", before.Entities, after.Entities)
	}
}

func v2StrategyFixture(t *testing.T) map[string]any {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "mobile-ad-monetization", "v2", "strategy-valid.json"))
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
