package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ConteMan/conflow/internal/draft"
	"github.com/ConteMan/conflow/internal/entities"
	"github.com/ConteMan/conflow/internal/packs"
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

func TestV2ReferenceRulesCoverStrategyArraysAndObjectKeys(t *testing.T) {
	configuration := v2ConfigurationWithLegacyCachePolicy(t)
	configuration["ad_strategy_settings"] = []any{map[string]any{"id": "default", "fields": map[string]any{"parameter_key": "ad_strategies_config", "payload_version": float64(1), "default_strategy_id": "balanced"}}}
	configuration["ad_strategies"] = []any{map[string]any{"id": "balanced", "fields": map[string]any{
		"placement_rule_mode":        "allowlist",
		"allowlist_placement_ids":    []any{"interstitial_main"},
		"frequency_policy_overrides": map[string]any{"global_cap": map[string]any{"cooldown": nil}},
	}}}
	definition, _, err := packs.BuiltinRegistry().Resolve("mobile-ad-monetization/v2")
	if err != nil {
		t.Fatal(err)
	}
	for _, metadata := range definition.Metadata.EntityTypes {
		for _, record := range records(configuration, metadata.Collection) {
			if err := validateRecordReferences(definition, metadata, configuration, record.ID); err != nil {
				t.Fatalf("valid reference %s/%s: %v", metadata.Name, record.ID, err)
			}
		}
	}
	got := references(definition, "mobile-ad-monetization/v2", configuration, "placement", "interstitial_main")
	paths := map[string]bool{}
	for _, reference := range got {
		if reference.EntityType == "ad_strategy" {
			paths[reference.Path] = true
		}
	}
	if !paths["/allowlist_placement_ids/0"] {
		t.Fatalf("strategy reference paths = %#v", got)
	}
	policyReferences := references(definition, "mobile-ad-monetization/v2", configuration, "frequency_policy", "global_cap")
	policyPaths := map[string]bool{}
	for _, reference := range policyReferences {
		if reference.EntityType == "ad_strategy" {
			policyPaths[reference.Path] = true
		}
	}
	if !policyPaths["/frequency_policy_overrides/global_cap"] {
		t.Fatalf("strategy policy reference paths = %#v", policyReferences)
	}

	configuration["ad_strategies"].([]any)[0].(map[string]any)["fields"].(map[string]any)["allowlist_placement_ids"] = []any{"missing"}
	strategyMetadata, _ := findEntityMetadata(definition, "ad_strategy")
	if err := validateRecordReferences(definition, strategyMetadata, configuration, "balanced"); err == nil {
		t.Fatal("missing array item reference must fail")
	}
	configuration["ad_strategies"].([]any)[0].(map[string]any)["fields"].(map[string]any)["allowlist_placement_ids"] = []any{"interstitial_main"}
	configuration["placements"].([]any)[0].(map[string]any)["fields"].(map[string]any)["enabled_switch_id"] = ""
	placementMetadata, _ := findEntityMetadata(definition, "placement")
	if err := validateRecordReferences(definition, placementMetadata, configuration, "interstitial_main"); err == nil {
		t.Fatal("empty scalar reference must fail")
	}
}

func TestMutateV2PlacementRejectsMissingOrEmptyEnabledSwitch(t *testing.T) {
	for _, test := range []struct {
		name     string
		wantCode string
		mutate   func(map[string]any)
	}{
		{name: "missing", wantCode: "required_field_missing", mutate: func(fields map[string]any) { delete(fields, "enabled_switch_id") }},
		{name: "empty", wantCode: "value_not_allowed", mutate: func(fields map[string]any) { fields["enabled_switch_id"] = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := openV2EntityService(t, v2ConfigurationWithLegacyCachePolicy(t))
			current, revision, err := service.GetEntity(context.Background(), "development", "placement", "interstitial_main")
			if err != nil {
				t.Fatal(err)
			}
			created := cloneRecord(*current.Effective.Value)
			created.ID = "interstitial_secondary"
			created.Fields["client_id"] = "AD-SECONDARY-001"
			created.Fields["key"] = "secondary_interstitial"
			test.mutate(created.Fields)

			_, _, err = service.MutateEntity(context.Background(), "development", EntityMutation{
				ExpectedRevision: revision, ExpectedSourceRevision: current.SourceRevision, Scope: draft.ScopeBaseline,
				EntityType: "placement", EntityID: created.ID, Entity: &created, Action: "create",
			})
			var validation *draft.ValidationError
			if !errors.As(err, &validation) || len(validation.Details) != 1 {
				t.Fatalf("create placement error = %#v", err)
			}
			detail := validation.Details[0]
			if detail.Code != test.wantCode || detail.Path != "/placements/interstitial_secondary/fields/enabled_switch_id" {
				t.Fatalf("validation detail = %#v", detail)
			}
		})
	}
}

func TestMutateV2PlacementIgnoresHistoricalInvalidSibling(t *testing.T) {
	configuration := v2ConfigurationWithLegacyCachePolicy(t)
	placements := records(configuration, "placements")
	placements[0].Fields["enabled_switch_id"] = ""
	valid := cloneRecord(placements[0])
	valid.ID = "interstitial_secondary"
	valid.Fields["client_id"] = "AD-SECONDARY-001"
	valid.Fields["key"] = "secondary_interstitial"
	valid.Fields["enabled_switch_id"] = "ads_enabled"
	configuration["placements"] = recordsValue(append(placements, valid))
	service := openV2EntityService(t, configuration)

	current, revision, err := service.GetEntity(context.Background(), "development", "placement", valid.ID)
	if err != nil {
		t.Fatal(err)
	}
	replacement := cloneRecord(*current.Effective.Value)
	replacement.Fields["description"] = "updated while repairing legacy data"
	if _, _, err := service.MutateEntity(context.Background(), "development", EntityMutation{
		ExpectedRevision: revision, ExpectedSourceRevision: current.SourceRevision, Scope: draft.ScopeBaseline,
		EntityType: "placement", EntityID: valid.ID, Entity: &replacement, Action: "replace",
	}); err != nil {
		t.Fatalf("replace valid sibling with historical invalid reference: %v", err)
	}
}

func openV2EntityService(t *testing.T, configuration map[string]any) *Service {
	t.Helper()
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
	if _, err := adapter.Save(source.SaveInput{ExpectedRevision: initial.Revision, EnvironmentID: "development", Baseline: configuration}); err != nil {
		t.Fatal(err)
	}
	service, err := Open(workspace)
	if err != nil {
		t.Fatal(err)
	}
	return service
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
