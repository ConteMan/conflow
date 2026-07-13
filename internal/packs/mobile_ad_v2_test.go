package packs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMobileAdV2DefinitionContract(t *testing.T) {
	definition, _, err := BuiltinRegistry().Resolve("mobile-ad-monetization/v2")
	if err != nil {
		t.Fatal(err)
	}

	wantEntities := []string{"remote_config_layout", "feature_switch", "network_settings", "frequency_policy", "placement", "unit_binding"}
	if len(definition.Metadata.EntityTypes) != len(wantEntities) || len(definition.Schema.Entities) != len(wantEntities) {
		t.Fatalf("v2 entity count = metadata %d, schema %d", len(definition.Metadata.EntityTypes), len(definition.Schema.Entities))
	}
	for _, name := range wantEntities {
		if _, ok := findMetadata(definition, name); !ok {
			t.Fatalf("metadata for %s is missing", name)
		}
		if _, ok := findSchema(definition, name); !ok {
			t.Fatalf("schema for %s is missing", name)
		}
	}

	for _, name := range []string{"remote_config_layout", "network_settings"} {
		metadata, _ := findMetadata(definition, name)
		if metadata.IDRule.Pattern != "^default$" {
			t.Fatalf("%s ID pattern = %q", name, metadata.IDRule.Pattern)
		}
	}

	networkSettings, _ := findMetadata(definition, "network_settings")
	if !reflect.DeepEqual(networkSettings.EnvironmentOverrideFields, []string{"active_network", "mediation_strategy"}) {
		t.Fatalf("network_settings overrides = %#v", networkSettings.EnvironmentOverrideFields)
	}
	unitBinding, _ := findMetadata(definition, "unit_binding")
	if !containsString(unitBinding.EnvironmentOverrideFields, "network") {
		t.Fatalf("unit_binding overrides = %#v", unitBinding.EnvironmentOverrideFields)
	}

	placement, _ := findSchema(definition, "placement")
	enabledSwitch := findField(t, placement, "enabled_switch_id")
	if enabledSwitch.Type != FieldTypeReference {
		t.Fatalf("enabled_switch_id type = %q", enabledSwitch.Type)
	}
	frequencyPolicy, _ := findSchema(definition, "frequency_policy")
	cooldown := findField(t, frequencyPolicy, "cooldown")
	if !cooldown.Nullable || cooldown.Type != FieldTypeObject || string(cooldown.Default) != "null" {
		t.Fatalf("cooldown = %#v", cooldown)
	}

	if _, err := json.Marshal(definition); err != nil {
		t.Fatalf("marshal definition: %v", err)
	}

	golden := loadMobileAdV2SchemaGolden(t)
	if golden.PackRef != "mobile-ad-monetization/v2" || golden.SchemaVersion != 2 || !reflect.DeepEqual(golden.EntityTypes, wantEntities) {
		t.Fatalf("schema golden = %#v", golden)
	}
}

type mobileAdV2SchemaGolden struct {
	PackRef       string   `json:"pack_ref"`
	EntityTypes   []string `json:"entity_types"`
	SchemaVersion uint64   `json:"schema_version"`
}

func loadMobileAdV2SchemaGolden(t *testing.T) mobileAdV2SchemaGolden {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "mobile-ad-monetization", "v2", "schema-golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden mobileAdV2SchemaGolden
	if err := json.Unmarshal(content, &golden); err != nil {
		t.Fatal(err)
	}
	return golden
}

func findField(t *testing.T, schema EntitySchema, name string) FieldSchema {
	t.Helper()
	for _, field := range schema.Fields {
		if field.Name == name {
			return field
		}
	}
	t.Fatalf("field %s is missing", name)
	return FieldSchema{}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
