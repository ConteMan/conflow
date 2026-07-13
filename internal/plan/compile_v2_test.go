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
	first := compileV2Parameters(desired)
	second := compileV2Parameters(desired)
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
	values := compileV2Parameters(v2CompileFixture(t))
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
	values := compileV2Parameters(desired)
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
	values := compileV2Parameters(v2CompileFixture(t))
	var payload struct {
		Version    int `json:"version"`
		Placements []struct {
			ClientID     string `json:"client_id"`
			UnitBindings []struct {
				EnvironmentID string `json:"environment_id"`
				Platform      string `json:"platform"`
				Network       string `json:"network"`
				UnitIDRef     string `json:"unit_id_ref"`
			} `json:"unit_bindings"`
		} `json:"placements"`
	}
	if err := json.Unmarshal([]byte(values["ad_placements_config"].(string)), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Version != 2 || len(payload.Placements) != 1 || payload.Placements[0].ClientID != "interstitial_main" {
		t.Fatalf("placement payload = %#v", payload)
	}
	if len(payload.Placements[0].UnitBindings) != 1 || payload.Placements[0].UnitBindings[0].Network != "admob" {
		t.Fatalf("placement bindings = %#v", payload.Placements[0].UnitBindings)
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
	merged, err := MergeFirebaseTemplate(remoteTemplate, encoded, changes, "mobile-ad-monetization/v2")
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
