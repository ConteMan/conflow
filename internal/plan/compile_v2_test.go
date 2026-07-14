package plan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ConteMan/conflow/internal/entities"
	"github.com/ConteMan/conflow/internal/remote"
)

func TestCompileV2ParametersIsDeterministic(t *testing.T) {
	desired := v2CompileFixture(t)
	first := compileV2Parameters(desired, "development")
	second := compileV2Parameters(desired, "development")
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("v2 compiler output changed: %s != %s", firstJSON, secondJSON)
	}
}

func TestCompileV2ParametersFeatureSwitch(t *testing.T) {
	values := compileV2Parameters(v2CompileFixture(t), "development")
	if got, ok := values["ads_enabled"].(bool); !ok || !got {
		t.Fatalf("feature switch value = %#v", values["ads_enabled"])
	}
	if got := values["ad_active_network"]; got != "admob" {
		t.Fatalf("active network = %#v", got)
	}
}

func TestCompileV2ParametersFrequencyPolicies(t *testing.T) {
	desired := v2CompileFixture(t)
	desired["frequency_policies"] = append(desired["frequency_policies"].([]any), map[string]any{"id": "alpha_cap", "fields": map[string]any{"cooldown": nil, "interval": nil, "max_count": nil, "shift_count": nil, "positions": []any{"z", "a", "a"}}})
	values := compileV2Parameters(desired, "development")
	var payload struct {
		Version  int            `json:"version"`
		Policies map[string]any `json:"policies"`
	}
	if err := json.Unmarshal([]byte(values["ad_frequency_policies_config"].(string)), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Version != 2 || len(payload.Policies) != 2 {
		t.Fatalf("frequency payload = %#v", payload)
	}
	if got := payload.Policies["alpha_cap"].(map[string]any)["positions"].([]any); len(got) != 2 || got[0] != "a" || got[1] != "z" {
		t.Fatalf("normalized positions = %#v", got)
	}
}

func TestCompileV2ParametersPlacements(t *testing.T) {
	desired := v2CompileFixture(t)
	placementFields := desired["placements"].([]any)[0].(map[string]any)["fields"].(map[string]any)
	placementFields["network_mode"] = "max"
	placementFields["cache_ttl"] = map[string]any{"unit": "minutes", "value": float64(2)}
	desired["unit_bindings"] = append(desired["unit_bindings"].([]any),
		map[string]any{"id": "ub_dev_ios_max", "fields": map[string]any{"placement_id": "interstitial_main", "environment_id": "development", "platform": "ios", "network": "max", "unit_id_ref": "max-ios"}},
		map[string]any{"id": "ub_dev_android_max", "fields": map[string]any{"placement_id": "interstitial_main", "environment_id": "development", "platform": "android", "network": "max", "unit_id_ref": "max-android"}},
		map[string]any{"id": "ub_prod_android_max", "fields": map[string]any{"placement_id": "interstitial_main", "environment_id": "production", "platform": "android", "network": "max", "unit_id_ref": "max-production"}},
	)
	values := compileV2Parameters(desired, "development")
	var payload struct {
		Version    int `json:"version"`
		Placements []struct {
			ID                 string  `json:"id"`
			Placement          string  `json:"placement"`
			Type               string  `json:"type"`
			EnabledConfigKey   string  `json:"enabled_config_key"`
			NetworkMode        string  `json:"network_mode"`
			CacheTTLSeconds    float64 `json:"cache_ttl_seconds"`
			Fallback           string  `json:"fallback"`
			DeprecatedClientID any     `json:"client_id"`
			DeprecatedKey      any     `json:"key"`
			DeprecatedAdType   any     `json:"ad_type"`
			DeprecatedCache    any     `json:"cache_policy"`
			DeprecatedBindings any     `json:"unit_bindings"`
			DeprecatedCacheTTL any     `json:"cache_ttl"`
			DeprecatedFallback any     `json:"fallback_behavior"`
			Units              map[string]struct {
				UnitID string `json:"unit_id"`
			} `json:"units"`
		} `json:"placements"`
	}
	if err := json.Unmarshal([]byte(values["ad_placements_config"].(string)), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Version != 2 || len(payload.Placements) != 1 || payload.Placements[0].ID != "interstitial_main" {
		t.Fatalf("placement payload = %#v", payload)
	}
	if payload.Placements[0].EnabledConfigKey != "ads_enabled" {
		t.Fatalf("enabled config key = %q", payload.Placements[0].EnabledConfigKey)
	}
	if got := payload.Placements[0]; got.Placement != "main_interstitial" || got.Type != "interstitial" || got.NetworkMode != "max" || got.CacheTTLSeconds != 120 || got.Fallback != "continue" {
		t.Fatalf("compiled placement fields = %#v", got)
	}
	if len(payload.Placements[0].Units) != 2 || payload.Placements[0].Units["admob"].UnitID != "ca-app-pub-xxx/ios" || payload.Placements[0].Units["max"].UnitID != "max-android" {
		t.Fatalf("placement units = %#v", payload.Placements[0].Units)
	}
	if got := payload.Placements[0]; got.DeprecatedClientID != nil || got.DeprecatedKey != nil || got.DeprecatedAdType != nil || got.DeprecatedCache != nil || got.DeprecatedBindings != nil || got.DeprecatedCacheTTL != nil || got.DeprecatedFallback != nil {
		t.Fatalf("deprecated placement fields are still present: %#v", got)
	}
}

func TestV2DurationSeconds(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  any
	}{
		{name: "null", value: nil, want: nil},
		{name: "seconds", value: map[string]any{"unit": "seconds", "value": float64(2)}, want: float64(2)},
		{name: "minutes", value: map[string]any{"unit": "minutes", "value": float64(2)}, want: float64(120)},
		{name: "hours", value: map[string]any{"unit": "hours", "value": float64(2)}, want: float64(7200)},
		{name: "days", value: map[string]any{"unit": "days", "value": float64(2)}, want: float64(172800)},
		{name: "unknown", value: map[string]any{"unit": "weeks", "value": float64(2)}, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := v2DurationSeconds(tt.value); got != tt.want {
				t.Fatalf("v2DurationSeconds(%#v) = %#v, want %#v", tt.value, got, tt.want)
			}
		})
	}
}

func TestMergeFirebaseTemplateCompilesV2ManagedParameters(t *testing.T) {
	desired := v2CompileFixture(t)
	encoded, err := json.Marshal(desired)
	if err != nil {
		t.Fatal(err)
	}
	remoteTemplate := []byte(`{"parameters":{"unmanaged":{"defaultValue":{"value":"keep"}}}}`)
	changes := []RemoteParameterChange{
		{ParameterKey: "ads_enabled", Managed: true},
		{ParameterKey: "ad_frequency_policies_config", Managed: true},
		{ParameterKey: "ad_placements_config", Managed: true},
	}
	merged, err := MergeFirebaseTemplate(remoteTemplate, encoded, changes, "mobile-ad-monetization/v2", "development")
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(merged, &document); err != nil {
		t.Fatal(err)
	}
	parameters := document["parameters"].(map[string]any)
	if got := parameters["ads_enabled"].(map[string]any)["defaultValue"].(map[string]any)["value"]; got != "true" {
		t.Fatalf("feature switch Firebase value = %#v", got)
	}
	if got := parameters["ad_frequency_policies_config"].(map[string]any)["defaultValue"].(map[string]any)["value"]; got == "" {
		t.Fatal("frequency aggregate was not encoded")
	}
	if got := parameters["unmanaged"].(map[string]any)["defaultValue"].(map[string]any)["value"]; got != "keep" {
		t.Fatalf("unmanaged Firebase value = %#v", got)
	}
}

func TestBuildV2MapsFrequencyChangeToAggregateParameter(t *testing.T) {
	baseline := v2CompileFixture(t)
	desired := v2CompileClone(t, baseline)
	desiredPolicy := desired["frequency_policies"].([]any)[0].(map[string]any)["fields"].(map[string]any)
	desiredPolicy["cooldown"] = map[string]any{"unit": "seconds", "value": 60}
	built, err := Build(Input{
		EnvironmentID:   "development",
		PackRef:         "mobile-ad-monetization/v2",
		Baseline:        baseline,
		Desired:         desired,
		ValidationReady: true,
		RemoteSnapshot: remote.Snapshot{
			Status: "available", RemoteETag: "etag-v2", Summary: &remote.Summary{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if built.Plan.PackRef != "mobile-ad-monetization/v2" {
		t.Fatalf("plan pack ref = %q", built.Plan.PackRef)
	}
	var mapped bool
	for _, change := range built.Plan.RemoteParameterChanges {
		if change.ParameterKey == "ad_frequency_policies_config" {
			mapped = true
			if change.ChangeKind != "updated" {
				t.Fatalf("aggregate change kind = %q", change.ChangeKind)
			}
		}
	}
	if !mapped || !hasRisk(built.Plan, "frequency_policy_changed") {
		t.Fatalf("v2 plan = %#v", built.Plan)
	}
}

func v2CompileFixture(t *testing.T) map[string]any {
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

func v2CompileClone(t *testing.T, configuration map[string]any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	var cloned map[string]any
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}
