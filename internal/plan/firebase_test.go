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
