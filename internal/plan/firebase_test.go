package plan

import (
	"encoding/json"
	"testing"
)

func TestMergeFirebaseTemplatePreservesUnmanagedAndConditionalValues(t *testing.T) {
	remote := []byte(`{"conditions":[{"name":"country"}],"parameters":{"ad_frequency_inter_global_cap":{"defaultValue":{"value":"30000"},"conditionalValues":{"country":{"value":"60000"}}},"unmanaged":{"defaultValue":{"value":"keep"}}}}`)
	desired := []byte(`{"frequency_policies":[{"id":"inter_global_cap","fields":{"cooldown_ms":120000}}]}`)
	merged, err := MergeFirebaseTemplate(remote, desired, []RemoteParameterChange{{ParameterKey: "ad_frequency_inter_global_cap", Managed: true}}, "mobile-ad-monetization/v1", "")
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(merged, &document); err != nil {
		t.Fatal(err)
	}
	parameters := document["parameters"].(map[string]any)
	managed := parameters["ad_frequency_inter_global_cap"].(map[string]any)
	if got := managed["defaultValue"].(map[string]any)["value"]; got != "120000" {
		t.Fatalf("managed value=%v", got)
	}
	if got := managed["conditionalValues"].(map[string]any)["country"].(map[string]any)["value"]; got != "60000" {
		t.Fatalf("condition=%v", got)
	}
	if got := parameters["unmanaged"].(map[string]any)["defaultValue"].(map[string]any)["value"]; got != "keep" {
		t.Fatalf("unmanaged=%v", got)
	}
}

func TestMergeFirebaseTemplateRenamesV2LayoutParameterWithoutRewritingOthers(t *testing.T) {
	desired := v2CompileFixture(t)
	layout := desired["remote_config_layouts"].([]any)[0].(map[string]any)["fields"].(map[string]any)
	layout["placements_parameter_key"] = "ad_placements_config_v2"
	encoded, err := json.Marshal(desired)
	if err != nil {
		t.Fatal(err)
	}
	remote := []byte(`{"parameters":{"ad_placements_config":{"defaultValue":{"value":"old"}},"unmanaged":{"defaultValue":{"value":"keep"}}}}`)
	merged, err := MergeFirebaseTemplate(remote, encoded, []RemoteParameterChange{
		{ParameterKey: "ad_placements_config", ChangeKind: "deleted", Managed: true},
		{ParameterKey: "ad_placements_config_v2", ChangeKind: "added", Managed: true},
	}, "mobile-ad-monetization/v2", "development")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Parameters map[string]any `json:"parameters"`
	}
	if err := json.Unmarshal(merged, &document); err != nil {
		t.Fatal(err)
	}
	if _, exists := document.Parameters["ad_placements_config"]; exists {
		t.Fatalf("old layout parameter remains: %#v", document.Parameters)
	}
	if _, exists := document.Parameters["ad_placements_config_v2"]; !exists {
		t.Fatalf("new layout parameter is missing: %#v", document.Parameters)
	}
	if got := document.Parameters["unmanaged"].(map[string]any)["defaultValue"].(map[string]any)["value"]; got != "keep" {
		t.Fatalf("unmanaged parameter = %#v", got)
	}
	if len(document.Parameters) != 2 {
		t.Fatalf("unexpected parameters were rewritten: %#v", document.Parameters)
	}
}
