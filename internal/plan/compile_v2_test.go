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

func TestCompileV2ParametersKeepsStrategyFeatureOptional(t *testing.T) {
	values := compileV2Parameters(v2CompileFixture(t), "development")
	if _, exists := values["ad_strategies_config"]; exists {
		t.Fatalf("strategy parameter must stay absent for schema 2 compatible input: %#v", values)
	}
}

func TestCompileV2AdStrategiesFallsBackFromInvalidPayloadVersion(t *testing.T) {
	desired := v2CompileFixture(t)
	addV2Strategy(desired, map[string]any{})
	desired["ad_strategy_settings"].([]any)[0].(map[string]any)["fields"].(map[string]any)["payload_version"] = "invalid"
	raw := compileV2Parameters(desired, "development")["ad_strategies_config"].(string)
	var payload struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Version != 1 {
		t.Fatalf("strategy payload version = %d", payload.Version)
	}
}

func TestCompileV2AdStrategiesAppliesSparseOverrides(t *testing.T) {
	desired := v2CompileFixture(t)
	addV2Strategy(desired, map[string]any{
		"cooldown":  nil,
		"max_count": map[string]any{"unit": "day", "value": float64(8)},
	})
	values := compileV2Parameters(desired, "development")
	raw, ok := values["ad_strategies_config"].(string)
	if !ok {
		t.Fatalf("strategy parameter = %#v", values["ad_strategies_config"])
	}
	var payload struct {
		Version           int    `json:"version"`
		DefaultStrategyID string `json:"default_strategy_id"`
		Strategies        map[string]struct {
			Allowlist []string                  `json:"allowlist_client_ids"`
			Policies  map[string]map[string]any `json:"frequency_policies"`
		} `json:"strategies"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatal(err)
	}
	strategy := payload.Strategies["balanced"]
	policy := strategy.Policies["interstitial_main"]
	if payload.Version != 1 || payload.DefaultStrategyID != "balanced" || len(strategy.Allowlist) != 1 || strategy.Allowlist[0] != "interstitial_main" {
		t.Fatalf("strategy payload = %#v", payload)
	}
	if policy["cooldown"] != nil || policy["interval"] != nil || policy["max_count"].(map[string]any)["value"] != float64(8) {
		t.Fatalf("effective strategy policy = %#v", policy)
	}
}

func TestCompileV2ParametersCustomParameters(t *testing.T) {
	desired := v2CompileFixture(t)
	desired["custom_parameters"] = []any{
		map[string]any{"id": "feature_flag", "fields": map[string]any{"key": "feature_flag", "value_type": "boolean", "value": true}},
		map[string]any{"id": "welcome_text", "fields": map[string]any{"key": "welcome_text", "value_type": "string", "value": "hello"}},
		map[string]any{"id": "min_version", "fields": map[string]any{"key": "min_version", "value_type": "number", "value": 12.5}},
		map[string]any{"id": "rollout_rules", "fields": map[string]any{"key": "rollout_rules", "value_type": "json", "value": map[string]any{"regions": []any{"cn", "us"}, "enabled": true}}},
	}
	values := compileV2Parameters(desired, "development")
	if values["feature_flag"] != true || values["welcome_text"] != "hello" || values["min_version"] != 12.5 {
		t.Fatalf("custom parameter values = %#v", values)
	}
	if got := values["rollout_rules"]; got != `{"enabled":true,"regions":["cn","us"]}` {
		t.Fatalf("custom JSON value = %#v", got)
	}

	encoded, err := json.Marshal(desired)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := MergeFirebaseTemplate([]byte(`{"parameters":{}}`), encoded, []RemoteParameterChange{{ParameterKey: "feature_flag", Managed: true}, {ParameterKey: "welcome_text", Managed: true}, {ParameterKey: "min_version", Managed: true}, {ParameterKey: "rollout_rules", Managed: true}}, "mobile-ad-monetization/v2", "development")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Parameters map[string]struct {
			DefaultValue struct {
				Value string `json:"value"`
			} `json:"defaultValue"`
			ValueType string `json:"valueType"`
		} `json:"parameters"`
	}
	if err := json.Unmarshal(merged, &document); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]struct{ value, valueType string }{
		"feature_flag":  {"true", "BOOLEAN"},
		"welcome_text":  {"hello", "STRING"},
		"min_version":   {"12.5", "NUMBER"},
		"rollout_rules": {`{"enabled":true,"regions":["cn","us"]}`, "JSON"},
	} {
		got := document.Parameters[key]
		if got.DefaultValue.Value != want.value || got.ValueType != want.valueType {
			t.Fatalf("%s Firebase parameter = %#v", key, got)
		}
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

func TestCompileV2SharedStrategyFixtureIsDeterministic(t *testing.T) {
	configuration := v2CompileFixtureFile(t, "strategy-valid.json")
	first := compileV2Parameters(configuration, "development")["ad_strategies_config"]
	reordered := v2CompileClone(t, configuration)
	strategies := reordered["ad_strategies"].([]any)
	strategies[0], strategies[1] = strategies[1], strategies[0]
	second := compileV2Parameters(reordered, "development")["ad_strategies_config"]
	if first != second {
		t.Fatalf("strategy payload changed after record reordering:\nfirst:  %s\nsecond: %s", first, second)
	}
	var payload struct {
		Version           int    `json:"version"`
		DefaultStrategyID string `json:"default_strategy_id"`
		Strategies        map[string]struct {
			Allowlist []string                  `json:"allowlist_client_ids"`
			Policies  map[string]map[string]any `json:"frequency_policies"`
		} `json:"strategies"`
	}
	if err := json.Unmarshal([]byte(first.(string)), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Version != 1 || payload.DefaultStrategyID != "balanced" || len(payload.Strategies) != 2 {
		t.Fatalf("strategy payload = %#v", payload)
	}
	if got := payload.Strategies["balanced"].Allowlist; len(got) != 1 || got[0] != "AD-PDF-001" {
		t.Fatalf("client allowlist = %#v", got)
	}
	if _, exists := payload.Strategies["balanced"].Policies["AD-PDF-001"]; !exists {
		t.Fatalf("effective frequency policy must use client_id as key: %#v", payload.Strategies["balanced"].Policies)
	}
	if _, exists := payload.Strategies["balanced"].Policies["interstitial_main"]; exists {
		t.Fatalf("effective frequency policy leaked Conflow entity ID: %#v", payload.Strategies["balanced"].Policies)
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
		RemoteSnapshot:  remote.Snapshot{Status: "available", RemoteETag: "etag-v2", Parameters: compileV2Parameters(baseline, "development"), Summary: &remote.Summary{}},
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

func TestBuildV2LayoutParameterKeyRenameShowsDeleteAddAndHighRisk(t *testing.T) {
	baseline := v2CompileFixture(t)
	desired := v2CompileClone(t, baseline)
	desired["remote_config_layouts"].([]any)[0].(map[string]any)["fields"].(map[string]any)["placements_parameter_key"] = "ad_placements_config_v2"
	built, err := Build(Input{
		EnvironmentID: "development", PackRef: "mobile-ad-monetization/v2", Baseline: baseline, Desired: desired, ValidationReady: true,
		RemoteSnapshot: remote.Snapshot{Status: "available", Parameters: compileV2Parameters(baseline, "development"), Summary: &remote.Summary{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	changes := map[string]string{}
	for _, change := range built.Plan.RemoteParameterChanges {
		changes[change.ParameterKey] = change.ChangeKind
	}
	if len(built.Plan.RemoteParameterChanges) != 2 || len(changes) != 2 || changes["ad_placements_config"] != "deleted" || changes["ad_placements_config_v2"] != "added" {
		t.Fatalf("layout remote changes = %#v", built.Plan.RemoteParameterChanges)
	}
	if !hasRisk(built.Plan, "remote_parameter_key_changed") || hasRisk(built.Plan, "managed_parameter_deleted") {
		t.Fatalf("layout rename risks = %#v", built.Plan.RiskItems)
	}
}

func TestBuildV2MapsFrequencyChangeToPolicyAndStrategyParameters(t *testing.T) {
	baseline := v2CompileFixture(t)
	addV2Strategy(baseline, map[string]any{"cooldown": map[string]any{"unit": "seconds", "value": float64(45)}})
	desired := v2CompileClone(t, baseline)
	desired["frequency_policies"].([]any)[0].(map[string]any)["fields"].(map[string]any)["max_count"] = map[string]any{"unit": "day", "value": float64(8)}
	built, err := Build(Input{
		EnvironmentID: "development", PackRef: "mobile-ad-monetization/v2", Baseline: baseline, Desired: desired, ValidationReady: true,
		RemoteSnapshot: remote.Snapshot{Status: "available", RemoteETag: "etag-v2", Parameters: compileV2Parameters(baseline, "development"), Summary: &remote.Summary{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, change := range built.Plan.RemoteParameterChanges {
		seen[change.ParameterKey] = true
	}
	if !seen["ad_frequency_policies_config"] || !seen["ad_strategies_config"] {
		t.Fatalf("remote parameter mapping = %#v", built.Plan.RemoteParameterChanges)
	}
}

func TestBuildV2StrategyRiskUsesEffectiveFrequency(t *testing.T) {
	baseline := v2CompileFixture(t)
	addV2Strategy(baseline, map[string]any{"cooldown": map[string]any{"unit": "minutes", "value": float64(5)}})
	t.Run("tightening is medium", func(t *testing.T) {
		desired := v2CompileClone(t, baseline)
		strategyOverride(desired)["cooldown"] = map[string]any{"unit": "minutes", "value": float64(10)}
		built, err := Build(Input{EnvironmentID: "development", PackRef: "mobile-ad-monetization/v2", Baseline: baseline, Desired: desired, ValidationReady: true, RemoteSnapshot: remote.Snapshot{Status: "available", Parameters: compileV2Parameters(baseline, "development"), Summary: &remote.Summary{}}})
		if err != nil {
			t.Fatal(err)
		}
		if !hasRisk(built.Plan, "ad_strategy_changed") || hasRisk(built.Plan, "ad_strategy_relaxed") {
			t.Fatalf("tightening risks = %#v", built.Plan.RiskItems)
		}
	})
	t.Run("disabling constraint is high", func(t *testing.T) {
		desired := v2CompileClone(t, baseline)
		strategyOverride(desired)["cooldown"] = nil
		built, err := Build(Input{EnvironmentID: "development", PackRef: "mobile-ad-monetization/v2", Baseline: baseline, Desired: desired, ValidationReady: true, RemoteSnapshot: remote.Snapshot{Status: "available", Parameters: compileV2Parameters(baseline, "development"), Summary: &remote.Summary{}}})
		if err != nil {
			t.Fatal(err)
		}
		if !hasRisk(built.Plan, "ad_strategy_relaxed") {
			t.Fatalf("relaxing risks = %#v", built.Plan.RiskItems)
		}
	})
}

func TestBuildV2PlacementClientIDChangeAffectsStrategyAndIsHighRisk(t *testing.T) {
	baseline := v2CompileFixture(t)
	addV2Strategy(baseline, map[string]any{})
	desired := v2CompileClone(t, baseline)
	desired["placements"].([]any)[0].(map[string]any)["fields"].(map[string]any)["client_id"] = "AD-PDF-001"
	built, err := Build(Input{
		EnvironmentID: "development", PackRef: "mobile-ad-monetization/v2", Baseline: baseline, Desired: desired, ValidationReady: true,
		RemoteSnapshot: remote.Snapshot{Status: "available", Parameters: compileV2Parameters(baseline, "development"), Summary: &remote.Summary{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, change := range built.Plan.RemoteParameterChanges {
		seen[change.ParameterKey] = true
	}
	if !seen["ad_placements_config"] || !seen["ad_strategies_config"] {
		t.Fatalf("client ID remote parameter mapping = %#v", built.Plan.RemoteParameterChanges)
	}
	if !hasRisk(built.Plan, "ad_strategy_placement_identity_changed") {
		t.Fatalf("client ID risk = %#v", built.Plan.RiskItems)
	}
}

func TestBuildV2NonDefaultStrategyDeletionIsHighRisk(t *testing.T) {
	baseline := v2CompileFixtureFile(t, "strategy-valid.json")
	desired := v2CompileClone(t, baseline)
	desired["ad_strategies"] = desired["ad_strategies"].([]any)[:1]
	built, err := Build(Input{
		EnvironmentID: "development", PackRef: "mobile-ad-monetization/v2", Baseline: baseline, Desired: desired, ValidationReady: true,
		RemoteSnapshot: remote.Snapshot{Status: "available", Parameters: compileV2Parameters(baseline, "development"), Summary: &remote.Summary{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasRisk(built.Plan, "ad_strategy_deleted") || hasRisk(built.Plan, "managed_parameter_deleted") {
		t.Fatalf("strategy deletion risks = %#v", built.Plan.RiskItems)
	}
}

func TestBuildV2SkipsRemoteChangesForDescriptionOnlyEdit(t *testing.T) {
	baseline := v2CompileFixture(t)
	desired := v2CompileClone(t, baseline)
	desired["frequency_policies"].([]any)[0].(map[string]any)["fields"].(map[string]any)["description"] = "Only the entity description changed"
	parameters := compileV2Parameters(baseline, "development")
	var frequencyPayload any
	if err := json.Unmarshal([]byte(parameters["ad_frequency_policies_config"].(string)), &frequencyPayload); err != nil {
		t.Fatal(err)
	}
	parameters["ad_frequency_policies_config"] = frequencyPayload
	built, err := Build(Input{
		EnvironmentID: "development", PackRef: "mobile-ad-monetization/v2", Baseline: baseline, Desired: desired, ValidationReady: true,
		RemoteSnapshot: remote.Snapshot{Status: "available", RemoteETag: "etag-v2", Parameters: parameters, Summary: &remote.Summary{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Plan.SemanticChanges) == 0 || len(built.Plan.RemoteParameterChanges) != 0 {
		t.Fatalf("description-only plan = %#v", built.Plan)
	}
	if hasRisk(built.Plan, "frequency_policy_changed") {
		t.Fatalf("description-only plan has frequency risk: %#v", built.Plan.RiskItems)
	}
}

func TestBuildV2DescriptionEditProjectsNoRemoteChangeEvenWhenParameterDiffers(t *testing.T) {
	baseline := v2CompileFixture(t)
	desired := v2CompileClone(t, baseline)
	policy := desired["frequency_policies"].([]any)[0].(map[string]any)["fields"].(map[string]any)
	policy["description"] = "描述与真实值同时变化"
	policy["cooldown"] = map[string]any{"unit": "seconds", "value": float64(240)}
	parameters := compileV2Parameters(baseline, "development")
	var frequencyPayload any
	if err := json.Unmarshal([]byte(parameters["ad_frequency_policies_config"].(string)), &frequencyPayload); err != nil {
		t.Fatal(err)
	}
	parameters["ad_frequency_policies_config"] = frequencyPayload
	built, err := Build(Input{
		EnvironmentID: "development", PackRef: "mobile-ad-monetization/v2", Baseline: baseline, Desired: desired, ValidationReady: true,
		RemoteSnapshot: remote.Snapshot{Status: "available", RemoteETag: "etag-v2", Parameters: parameters, Summary: &remote.Summary{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Only the cooldown edit feeds the compiled output: exactly one remote
	// change, and it must not be attributed to the description node.
	if len(built.Plan.RemoteParameterChanges) != 1 {
		t.Fatalf("remote changes = %#v", built.Plan.RemoteParameterChanges)
	}
	if built.Plan.RemoteParameterChanges[0].ChangeKind != "updated" {
		t.Fatalf("change kind = %s", built.Plan.RemoteParameterChanges[0].ChangeKind)
	}
	for _, change := range built.Plan.SemanticChanges {
		if change.FieldPath == "/description" && len(change.RemoteParameterNodeIDs) != 0 {
			t.Fatalf("description change projected remote nodes: %#v", change)
		}
	}
}

func TestBuildV2AddsRemoteParameterForNewEntityWhenKeyIsAbsent(t *testing.T) {
	baseline := v2CompileFixture(t)
	desired := v2CompileClone(t, baseline)
	desired["feature_switches"] = append(desired["feature_switches"].([]any), map[string]any{"id": "new_switch", "fields": map[string]any{"key": "new_entity_switch", "default_value": true}})
	built, err := Build(Input{
		EnvironmentID: "development", PackRef: "mobile-ad-monetization/v2", Baseline: baseline, Desired: desired, ValidationReady: true,
		RemoteSnapshot: remote.Snapshot{Status: "available", RemoteETag: "etag-v2", Parameters: compileV2Parameters(baseline, "development"), Summary: &remote.Summary{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range built.Plan.RemoteParameterChanges {
		if change.ParameterKey == "new_entity_switch" {
			if change.ChangeKind != "added" || change.AfterSummary != summary(true) {
				t.Fatalf("new entity remote change = %#v", change)
			}
			return
		}
	}
	t.Fatalf("new entity remote change is missing: %#v", built.Plan.RemoteParameterChanges)
}

func TestBuildV2CustomParameterAdoptionIsHighRisk(t *testing.T) {
	baseline := v2CompileFixture(t)
	desired := v2CompileClone(t, baseline)
	desired["custom_parameters"] = []any{map[string]any{"id": "min_version", "fields": map[string]any{"key": "min_version", "value_type": "number", "value": 12.5}}}
	built, err := Build(Input{
		EnvironmentID: "development", PackRef: "mobile-ad-monetization/v2", Baseline: baseline, Desired: desired, ValidationReady: true,
		RemoteSnapshot: remote.Snapshot{Status: "available", RemoteETag: "etag-v2", Parameters: map[string]any{"min_version": "10"}, Summary: &remote.Summary{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasRisk(built.Plan, "custom_parameter_adopted") {
		t.Fatalf("custom parameter adoption risk is missing: %#v", built.Plan.RiskItems)
	}
	for _, change := range built.Plan.RemoteParameterChanges {
		if change.ParameterKey == "min_version" && change.Managed && change.ChangeKind == "updated" {
			return
		}
	}
	t.Fatalf("custom parameter is not in managed remote changes: %#v", built.Plan.RemoteParameterChanges)
}

func TestBuildV2CustomParameterDeletionIsHighRiskAndRemovesFirebaseParameter(t *testing.T) {
	baseline := v2CompileFixture(t)
	baseline["custom_parameters"] = []any{map[string]any{"id": "min_version", "fields": map[string]any{"key": "min_version", "value_type": "number", "value": 12.5}}}
	desired := v2CompileClone(t, baseline)
	desired["custom_parameters"] = []any{}
	built, err := Build(Input{
		EnvironmentID: "development", PackRef: "mobile-ad-monetization/v2", Baseline: baseline, Desired: desired, ValidationReady: true,
		RemoteSnapshot: remote.Snapshot{Status: "available", RemoteETag: "etag-v2", Parameters: map[string]any{"min_version": "12.5"}, Summary: &remote.Summary{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasRisk(built.Plan, "custom_parameter_deleted") || hasRisk(built.Plan, "managed_parameter_deleted") {
		t.Fatalf("custom parameter deletion risks = %#v", built.Plan.RiskItems)
	}
	encoded, err := json.Marshal(desired)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := MergeFirebaseTemplate([]byte(`{"parameters":{"min_version":{"defaultValue":{"value":"12.5"},"valueType":"NUMBER"}}}`), encoded, built.Plan.RemoteParameterChanges, "mobile-ad-monetization/v2", "development")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Parameters map[string]any `json:"parameters"`
	}
	if err := json.Unmarshal(merged, &document); err != nil {
		t.Fatal(err)
	}
	if _, exists := document.Parameters["min_version"]; exists {
		t.Fatalf("deleted custom parameter remains in Firebase template: %#v", document.Parameters)
	}
}

func v2CompileFixture(t *testing.T) map[string]any {
	return v2CompileFixtureFile(t, "minimal-valid.json")
}

func v2CompileFixtureFile(t *testing.T, name string) map[string]any {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "mobile-ad-monetization", "v2", name))
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

func addV2Strategy(configuration map[string]any, override map[string]any) {
	configuration["ad_strategy_settings"] = []any{map[string]any{"id": "default", "fields": map[string]any{"parameter_key": "ad_strategies_config", "payload_version": float64(1), "default_strategy_id": "balanced"}}}
	configuration["ad_strategies"] = []any{map[string]any{"id": "balanced", "fields": map[string]any{
		"description":                "Balanced strategy",
		"placement_rule_mode":        "allowlist",
		"allowlist_placement_ids":    []any{"interstitial_main"},
		"frequency_policy_overrides": map[string]any{"interstitial_main": override},
	}}}
}

func strategyOverride(configuration map[string]any) map[string]any {
	strategy := configuration["ad_strategies"].([]any)[0].(map[string]any)
	fields := strategy["fields"].(map[string]any)
	overrides := fields["frequency_policy_overrides"].(map[string]any)
	return overrides["interstitial_main"].(map[string]any)
}
