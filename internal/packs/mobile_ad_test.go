package packs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type mobileAdFixture struct {
	PackRef  string `json:"pack_ref"`
	Entities struct {
		Placements        []map[string]any `json:"placements"`
		FrequencyPolicies []map[string]any `json:"frequency_policies"`
		FeatureSwitches   []map[string]any `json:"feature_switches"`
	} `json:"entities"`
	UnitBindingMatrix struct {
		Rows []struct {
			PlacementID string         `json:"placement_id"`
			Development map[string]any `json:"development"`
			Staging     map[string]any `json:"staging"`
			Production  map[string]any `json:"production"`
		} `json:"rows"`
	} `json:"unit_binding_matrix"`
	ExpectedCounts struct {
		Placements             int `json:"placements"`
		FrequencyPolicies      int `json:"frequency_policies"`
		FeatureSwitches        int `json:"feature_switches"`
		UnitBindings           int `json:"unit_bindings"`
		ConfiguredUnitBindings int `json:"configured_unit_bindings"`
		MissingProduction      int `json:"missing_production_unit_bindings"`
	} `json:"expected_counts"`
}

func TestMobileAdFixtureGoldenCountsAndSchema(t *testing.T) {
	fixture := loadMobileAdFixture(t)
	if fixture.PackRef != "mobile-ad-monetization/v1" {
		t.Fatalf("pack_ref = %q", fixture.PackRef)
	}
	if len(fixture.Entities.Placements) != fixture.ExpectedCounts.Placements || len(fixture.Entities.FrequencyPolicies) != fixture.ExpectedCounts.FrequencyPolicies || len(fixture.Entities.FeatureSwitches) != fixture.ExpectedCounts.FeatureSwitches {
		t.Fatalf("fixture entity counts do not match expected_counts")
	}

	bindings, configured, missingProduction := 0, 0, 0
	for _, row := range fixture.UnitBindingMatrix.Rows {
		for environment, cells := range map[string]map[string]any{"development": row.Development, "staging": row.Staging, "production": row.Production} {
			for _, platform := range []string{"ios", "android"} {
				bindings++
				if cells[platform] != nil {
					configured++
				} else if environment == "production" {
					missingProduction++
				}
			}
		}
	}
	if bindings != fixture.ExpectedCounts.UnitBindings || configured != fixture.ExpectedCounts.ConfiguredUnitBindings || missingProduction != fixture.ExpectedCounts.MissingProduction {
		t.Fatalf("binding matrix = %d total, %d configured, %d production missing", bindings, configured, missingProduction)
	}

	references := 0
	for _, placement := range fixture.Entities.Placements {
		if placement["frequency_policy_id"] == "inter_global_cap" {
			references++
		}
	}
	if references != 10 {
		t.Fatalf("inter_global_cap references = %d, want 10", references)
	}

	definition, _, err := BuiltinRegistry().Resolve(fixture.PackRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(definition.Metadata.EntityTypes) != 4 || len(definition.Schema.Entities) != 4 {
		t.Fatalf("mobile Pack shape = %#v", definition.Metadata)
	}
	for _, name := range []string{"placement", "frequency_policy", "feature_switch", "unit_binding"} {
		metadata, ok := findMetadata(definition, name)
		if !ok || metadata.Collection == "" || metadata.IDRule.Pattern == "" {
			t.Fatalf("metadata for %s = %#v", name, metadata)
		}
	}
	unitBinding, _ := findMetadata(definition, "unit_binding")
	if len(unitBinding.EnvironmentOverrideFields) == 0 {
		t.Fatal("unit_binding must be environment-overridable")
	}
	for _, name := range []string{"placement", "frequency_policy", "feature_switch"} {
		metadata, _ := findMetadata(definition, name)
		if len(metadata.EnvironmentOverrideFields) != 0 {
			t.Fatalf("%s unexpectedly permits an environment override", name)
		}
	}
	placementSchema, _ := findSchema(definition, "placement")
	assertFieldGolden(t, placementSchema, "ad_type", `"interstitial"`, 3)
	assertFieldGolden(t, placementSchema, "enabled", "true", 0)
	assertFieldGolden(t, placementSchema, "load_timeout_ms", "4000", 0)
	unitSchema, _ := findSchema(definition, "unit_binding")
	assertFieldGolden(t, unitSchema, "unit_id_ref", "null", 0)
	serialized, err := json.Marshal(definition.Schema)
	if err != nil || !json.Valid(serialized) || !containsJSON(serialized, `"ad_type"`) || !containsJSON(serialized, `"广告类型"`) || !containsJSON(serialized, `"default":"interstitial"`) {
		t.Fatalf("schema serialization = %s, err = %v", serialized, err)
	}
}

func containsJSON(document []byte, fragment string) bool {
	return string(document) != "" && contains(string(document), fragment)
}

func contains(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}

func loadMobileAdFixture(t *testing.T) mobileAdFixture {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "contracts", "mobile-ad-monetization", "v1", "entities.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture mobileAdFixture
	if err := json.Unmarshal(content, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func findMetadata(definition Definition, name string) (EntityMetadata, bool) {
	for _, metadata := range definition.Metadata.EntityTypes {
		if metadata.Name == name {
			return metadata, true
		}
	}
	return EntityMetadata{}, false
}

func findSchema(definition Definition, name string) (EntitySchema, bool) {
	for _, schema := range definition.Schema.Entities {
		if schema.Name == name {
			return schema, true
		}
	}
	return EntitySchema{}, false
}

func assertFieldGolden(t *testing.T, schema EntitySchema, name, defaultValue string, enumSize int) {
	t.Helper()
	for _, field := range schema.Fields {
		if field.Name == name {
			if string(field.Default) != defaultValue || len(field.Validation.Enum) != enumSize || field.UI.Label == "" || field.UI.Description == "" {
				t.Fatalf("field %s = %#v", name, field)
			}
			return
		}
	}
	t.Fatalf("field %s missing", name)
}
