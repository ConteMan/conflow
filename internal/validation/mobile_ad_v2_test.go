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

func TestValidateV2KeepsAdStrategiesOptional(t *testing.T) {
	configuration := v2ValidationFixture(t)
	configuration["ad_strategy_settings"] = []any{}
	configuration["ad_strategies"] = []any{}
	if hasV2Diagnostic(Validate(v2ValidationInput(t, configuration)), "ad_strategy_settings_not_singleton", SeverityBlocking) {
		t.Fatal("empty strategy feature must remain compatible")
	}
}

func TestValidateV2AdStrategyRules(t *testing.T) {
	configuration := v2ValidationFixture(t)
	addV2ValidationStrategy(configuration)
	if diagnostics := Validate(v2ValidationInput(t, configuration)); hasV2Diagnostic(diagnostics, "ad_strategy_frequency_override_invalid", SeverityBlocking) {
		t.Fatalf("valid strategy diagnostics = %#v", diagnostics)
	}

	t.Run("override outside allowlist", func(t *testing.T) {
		invalid := v2Clone(t, configuration).(map[string]any)
		fields := v2RecordFields(t, invalid, "ad_strategies", "balanced")
		fields["allowlist_placement_ids"] = []any{}
		if !hasV2Diagnostic(Validate(v2ValidationInput(t, invalid)), "ad_strategy_override_outside_allowlist", SeverityBlocking) {
			t.Fatal("missing override outside allowlist diagnostic")
		}
	})

	t.Run("unknown override field", func(t *testing.T) {
		invalid := v2Clone(t, configuration).(map[string]any)
		fields := v2RecordFields(t, invalid, "ad_strategies", "balanced")
		overrides := fields["frequency_policy_overrides"].(map[string]any)
		overrides["interstitial_main"].(map[string]any)["unknown"] = true
		if !hasV2Diagnostic(Validate(v2ValidationInput(t, invalid)), "ad_strategy_frequency_override_invalid", SeverityBlocking) {
			t.Fatal("missing unknown override field diagnostic")
		}
	})

	t.Run("parameter key conflict", func(t *testing.T) {
		invalid := v2Clone(t, configuration).(map[string]any)
		v2RecordFields(t, invalid, "ad_strategy_settings", "default")["parameter_key"] = "ads_enabled"
		if !hasV2Diagnostic(Validate(v2ValidationInput(t, invalid)), "parameter_key_conflict", SeverityBlocking) {
			t.Fatal("missing strategy parameter key conflict diagnostic")
		}
	})

	t.Run("empty allowlist", func(t *testing.T) {
		valid := v2Clone(t, configuration).(map[string]any)
		fields := v2RecordFields(t, valid, "ad_strategies", "balanced")
		fields["allowlist_placement_ids"] = []any{}
		fields["frequency_policy_overrides"] = map[string]any{}
		if diagnostics := Validate(v2ValidationInput(t, valid)); hasV2Diagnostic(diagnostics, "ad_strategy_override_outside_allowlist", SeverityBlocking) || hasV2Diagnostic(diagnostics, "ad_strategy_frequency_override_invalid", SeverityBlocking) {
			t.Fatalf("empty allowlist diagnostics = %#v", diagnostics)
		}
	})

	t.Run("payload version two", func(t *testing.T) {
		valid := v2Clone(t, configuration).(map[string]any)
		v2RecordFields(t, valid, "ad_strategy_settings", "default")["payload_version"] = float64(2)
		if hasV2Diagnostic(Validate(v2ValidationInput(t, valid)), "ad_strategy_payload_version_invalid", SeverityBlocking) {
			t.Fatal("payload version 2 must remain valid")
		}
	})

	for _, test := range []struct {
		name  string
		value any
	}{
		{name: "missing", value: nil},
		{name: "zero", value: float64(0)},
		{name: "fractional", value: 1.5},
		{name: "wrong type", value: "2"},
	} {
		t.Run("invalid payload version "+test.name, func(t *testing.T) {
			invalid := v2Clone(t, configuration).(map[string]any)
			v2RecordFields(t, invalid, "ad_strategy_settings", "default")["payload_version"] = test.value
			if !hasV2Diagnostic(Validate(v2ValidationInput(t, invalid)), "ad_strategy_payload_version_invalid", SeverityBlocking) {
				t.Fatal("missing invalid strategy payload version diagnostic")
			}
		})
	}
}

func TestValidateV2SharedStrategyFixture(t *testing.T) {
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
	configuration := entities.AdaptFlatFixture(fixture.Entities)
	if diagnostics := Validate(v2ValidationInput(t, configuration)); len(diagnostics) != 0 {
		t.Fatalf("strategy fixture diagnostics = %#v", diagnostics)
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

func addV2ValidationStrategy(configuration map[string]any) {
	configuration["ad_strategy_settings"] = []any{map[string]any{"id": "default", "fields": map[string]any{"parameter_key": "ad_strategies_config", "payload_version": float64(1), "default_strategy_id": "balanced"}}}
	configuration["ad_strategies"] = []any{map[string]any{"id": "balanced", "fields": map[string]any{
		"placement_rule_mode":     "allowlist",
		"allowlist_placement_ids": []any{"interstitial_main"},
		"frequency_policy_overrides": map[string]any{"interstitial_main": map[string]any{
			"cooldown":  nil,
			"max_count": map[string]any{"unit": "day", "value": float64(8)},
		}},
	}}}
}
