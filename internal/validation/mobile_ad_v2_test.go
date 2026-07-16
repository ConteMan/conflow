package validation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ConteMan/conflow/internal/entities"
)

func TestValidateV2MinimalConfiguration(t *testing.T) {
	diagnostics := Validate(v2ValidationInput(t, v2ValidationFixture(t)))
	for _, item := range diagnostics {
		if item.Severity == SeverityBlocking {
			t.Fatalf("unexpected blocking diagnostic: %#v", item)
		}
	}
}

func TestValidateV2IgnoresLegacyCachePolicy(t *testing.T) {
	configuration := v2ValidationFixture(t)
	v2RecordFields(t, configuration, "placements", "interstitial_main")["cache_policy"] = "disk"
	for _, item := range Validate(v2ValidationInput(t, configuration)) {
		if item.Path == "/placements/interstitial_main/cache_policy" {
			t.Fatalf("legacy cache_policy diagnostic = %#v", item)
		}
	}
}

func TestValidateV2RequiresSingletons(t *testing.T) {
	tests := []struct {
		name, collection, code string
	}{
		{name: "layout", collection: "remote_config_layouts", code: "remote_config_layout_not_singleton"},
		{name: "network settings", collection: "network_settings", code: "network_settings_not_singleton"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configuration := v2ValidationFixture(t)
			configuration[test.collection] = []any{}
			if !hasV2Diagnostic(Validate(v2ValidationInput(t, configuration)), test.code, SeverityBlocking) {
				t.Fatalf("missing %s diagnostic", test.code)
			}
		})
	}
}

func TestValidateV2PlacementReference(t *testing.T) {
	configuration := v2ValidationFixture(t)
	v2RecordFields(t, configuration, "placements", "interstitial_main")["enabled_switch_id"] = "missing_switch"
	if !hasV2Diagnostic(Validate(v2ValidationInput(t, configuration)), "reference_not_found", SeverityError) {
		t.Fatal("missing placement feature switch reference diagnostic")
	}
}

func TestValidateV2BindingCompositeKey(t *testing.T) {
	configuration := v2ValidationFixture(t)
	copy := v2Clone(t, configuration["unit_bindings"].([]any)[0]).(map[string]any)
	copy["id"] = "ub_dev_ios_interstitial_duplicate"
	configuration["unit_bindings"] = append(configuration["unit_bindings"].([]any), copy)
	if !hasV2Diagnostic(Validate(v2ValidationInput(t, configuration)), "unit_binding_composite_key_duplicate", SeverityBlocking) {
		t.Fatal("missing duplicate binding diagnostic")
	}
}

func TestValidateV2PresetRequiresPolicyReference(t *testing.T) {
	configuration := v2ValidationFixture(t)
	v2RecordFields(t, configuration, "placements", "interstitial_main")["frequency_policy_id"] = nil
	if !hasV2Diagnostic(Validate(v2ValidationInput(t, configuration)), "preset_custom_exclusive", SeverityError) {
		t.Fatal("missing preset/custom diagnostic")
	}
}

func TestValidateV2RejectsInvalidDuration(t *testing.T) {
	configuration := v2ValidationFixture(t)
	v2RecordFields(t, configuration, "frequency_policies", "global_cap")["cooldown"] = map[string]any{"unit": "weeks", "value": 1}
	if !hasV2Diagnostic(Validate(v2ValidationInput(t, configuration)), "duration_invalid", SeverityError) {
		t.Fatal("missing invalid duration diagnostic")
	}
}

func TestValidateV2RejectsCustomParameterKeyConflict(t *testing.T) {
	configuration := v2ValidationFixture(t)
	configuration["custom_parameters"] = []any{map[string]any{"id": "ads_enabled", "fields": map[string]any{"key": "ads_enabled", "value_type": "string", "value": "manual", "description": nil}}}
	if !hasV2Diagnostic(Validate(v2ValidationInput(t, configuration)), "parameter_key_conflict", SeverityBlocking) {
		t.Fatal("missing custom parameter key conflict diagnostic")
	}
}

func TestValidateV2RejectsCustomParameterValueTypeMismatch(t *testing.T) {
	configuration := v2ValidationFixture(t)
	configuration["custom_parameters"] = []any{map[string]any{"id": "min_version", "fields": map[string]any{"key": "min_version", "value_type": "number", "value": "42", "description": nil}}}
	if !hasV2Diagnostic(Validate(v2ValidationInput(t, configuration)), "custom_parameter_value_type_mismatch", SeverityBlocking) {
		t.Fatal("missing custom parameter value type mismatch diagnostic")
	}
}

func v2ValidationInput(t *testing.T, configuration map[string]any) Input {
	t.Helper()
	return Input{PackRef: "mobile-ad-monetization/v2", EnvironmentID: "development", EnvironmentKind: "development", Effective: configuration}
}

func v2ValidationFixture(t *testing.T) map[string]any {
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

func v2RecordFields(t *testing.T, configuration map[string]any, collection, id string) map[string]any {
	t.Helper()
	for _, raw := range configuration[collection].([]any) {
		record := raw.(map[string]any)
		if record["id"] == id {
			return record["fields"].(map[string]any)
		}
	}
	t.Fatalf("record %s/%s not found", collection, id)
	return nil
}

func v2Clone(t *testing.T, value any) any {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned any
	if err := json.Unmarshal(content, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}

func hasV2Diagnostic(diagnostics []Diagnostic, code, severity string) bool {
	for _, item := range diagnostics {
		if item.Code == code && item.Severity == severity {
			return true
		}
	}
	return false
}
