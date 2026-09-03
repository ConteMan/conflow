package plan

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/ConteMan/conflow/internal/remote"
)

func TestBuildV2ReconcilesManagedParametersWithoutLocalChanges(t *testing.T) {
	desired := v2CompileFixture(t)
	addV2Strategy(desired, map[string]any{})
	layout := desired["remote_config_layouts"].([]any)[0].(map[string]any)["fields"].(map[string]any)
	layout["mediation_strategy_parameter_key"] = "ad_network_attribution_routing_config"
	desired["network_settings"].([]any)[0].(map[string]any)["fields"].(map[string]any)["mediation_strategy"] = "bidding"
	custom := make([]any, 0, 31)
	for index := 0; index < 31; index++ {
		key := fmt.Sprintf("feature_%02d", index+1)
		custom = append(custom, map[string]any{"id": key, "fields": map[string]any{"key": key, "value_type": "boolean", "value": index%2 == 0}})
	}
	desired["custom_parameters"] = custom
	beforeBuild, err := json.Marshal(desired)
	if err != nil {
		t.Fatal(err)
	}
	compiled := compileV2Parameters(desired, "development")
	if len(compiled) != 37 {
		t.Fatalf("compiled parameters = %d, want 37: %#v", len(compiled), compiled)
	}
	remoteParameters := map[string]any{"ad_placements_config": compiled["ad_placements_config"]}
	built, err := Build(v2ReconcileInput(desired, desired, remoteParameters))
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Plan.SemanticChanges) != 36 || len(built.Plan.RemoteParameterChanges) != 36 {
		t.Fatalf("reconcile counts: semantic=%d remote=%d", len(built.Plan.SemanticChanges), len(built.Plan.RemoteParameterChanges))
	}
	seen := map[string]bool{}
	for _, change := range built.Plan.SemanticChanges {
		if change.ChangeKind != "managed_remote_drift" || len(change.RemoteParameterNodeIDs) != 1 {
			t.Fatalf("unexpected synthetic change: %#v", change)
		}
	}
	for _, change := range built.Plan.RemoteParameterChanges {
		if change.ChangeKind != "added" || len(change.CausedBySemanticChangeIDs) != 1 || seen[change.ParameterKey] {
			t.Fatalf("unexpected remote addition: %#v", change)
		}
		seen[change.ParameterKey] = true
	}
	if !strings.Contains(string(built.Artifacts["review.md"]), "Managed remote drift: 36") {
		t.Fatalf("review markdown = %s", built.Artifacts["review.md"])
	}
	remoteTemplate, err := json.Marshal(map[string]any{"parameters": map[string]any{"ad_placements_config": map[string]any{"defaultValue": map[string]any{"value": compiled["ad_placements_config"]}}}})
	if err != nil {
		t.Fatal(err)
	}
	merged, err := MergeFirebaseTemplate(remoteTemplate, beforeBuild, built.Plan.RemoteParameterChanges, "mobile-ad-monetization/v2", "development")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Parameters map[string]any `json:"parameters"`
	}
	if err := json.Unmarshal(merged, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Parameters) != 37 {
		t.Fatalf("merged parameters = %d, want 37", len(document.Parameters))
	}
	afterBuild, err := json.Marshal(desired)
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeBuild) != string(afterBuild) {
		t.Fatal("plan build mutated the local desired configuration")
	}
}

func TestBuildV2FirstPublishUsesAdditionsWithoutDeleteAddPairs(t *testing.T) {
	desired := v2CompileFixture(t)
	built, err := Build(v2ReconcileInput(map[string]any{}, desired, map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Plan.RemoteParameterChanges) != len(compileV2Parameters(desired, "development")) {
		t.Fatalf("remote additions = %#v", built.Plan.RemoteParameterChanges)
	}
	for _, change := range built.Plan.SemanticChanges {
		if change.ChangeKind == "deleted" || change.ChangeKind == "managed_remote_drift" {
			t.Fatalf("first publish contains an unexpected change: %#v", change)
		}
	}
	for _, change := range built.Plan.RemoteParameterChanges {
		if change.ChangeKind != "added" || len(change.CausedBySemanticChangeIDs) == 0 {
			t.Fatalf("first publish remote change = %#v", change)
		}
	}
}

func TestBuildV2ManagedReconcileStates(t *testing.T) {
	desired := v2CompileFixture(t)
	compiled := compileV2Parameters(desired, "development")
	tests := []struct {
		name           string
		parameters     map[string]any
		wantKind       string
		wantDriftRisk  string
		wantUnmanaged  bool
		wantRemoteSize int
	}{
		{name: "fully synchronized", parameters: copyParameters(compiled), wantRemoteSize: 0},
		{name: "managed parameter missing", parameters: withoutParameter(compiled, "ads_enabled"), wantKind: "added", wantDriftRisk: "medium", wantRemoteSize: 1},
		{name: "managed parameter drifted", parameters: withParameter(compiled, "ads_enabled", false), wantKind: "updated", wantDriftRisk: "high", wantRemoteSize: 1},
		{name: "unmanaged parameter preserved", parameters: withParameter(compiled, "external_flag", "keep"), wantUnmanaged: true, wantRemoteSize: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			built, err := Build(v2ReconcileInput(desired, desired, test.parameters))
			if err != nil {
				t.Fatal(err)
			}
			if len(built.Plan.RemoteParameterChanges) != test.wantRemoteSize {
				t.Fatalf("remote changes = %#v", built.Plan.RemoteParameterChanges)
			}
			if test.wantKind != "" {
				change := built.Plan.RemoteParameterChanges[0]
				if change.ParameterKey != "ads_enabled" || change.ChangeKind != test.wantKind {
					t.Fatalf("remote change = %#v", change)
				}
				risk, found := riskFor(built.Plan, "managed_remote_drift")
				if !found || risk.Severity != test.wantDriftRisk || !risk.AcknowledgementRequired {
					t.Fatalf("drift risk = %#v", risk)
				}
			}
			if hasRisk(built.Plan, "unmanaged_remote_parameter_preserved") != test.wantUnmanaged {
				t.Fatalf("unmanaged risks = %#v", built.Plan.RiskItems)
			}
		})
	}
}

func TestBuildV2AggregatesMultipleSemanticCausesByParameterKey(t *testing.T) {
	baseline := v2CompileFixture(t)
	desired := v2CompileClone(t, baseline)
	policy := desired["frequency_policies"].([]any)[0].(map[string]any)["fields"].(map[string]any)
	policy["cooldown"] = map[string]any{"unit": "seconds", "value": float64(60)}
	policy["max_count"] = map[string]any{"unit": "day", "value": float64(8)}
	built, err := Build(v2ReconcileInput(baseline, desired, compileV2Parameters(baseline, "development")))
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Plan.RemoteParameterChanges) != 1 {
		t.Fatalf("remote changes = %#v", built.Plan.RemoteParameterChanges)
	}
	change := built.Plan.RemoteParameterChanges[0]
	if change.ParameterKey != "ad_frequency_policies_config" || change.ChangeKind != "updated" || len(change.CausedBySemanticChangeIDs) != 2 {
		t.Fatalf("aggregate remote change = %#v", change)
	}
	for _, semantic := range built.Plan.SemanticChanges {
		if semantic.ChangeKind == "managed_remote_drift" {
			t.Fatalf("local changes must remain the causes: %#v", semantic)
		}
	}
}

func TestBuildV2FeatureSwitchKeyChangeCausesPlacementAggregateUpdate(t *testing.T) {
	baseline := v2CompileFixture(t)
	desired := v2CompileClone(t, baseline)
	desired["feature_switches"].([]any)[0].(map[string]any)["fields"].(map[string]any)["key"] = "ads_enabled_v2"
	built, err := Build(v2ReconcileInput(baseline, desired, compileV2Parameters(baseline, "development")))
	if err != nil {
		t.Fatal(err)
	}
	remoteByKey := map[string]RemoteParameterChange{}
	for _, change := range built.Plan.RemoteParameterChanges {
		remoteByKey[change.ParameterKey] = change
	}
	for _, key := range []string{"ads_enabled", "ads_enabled_v2", "ad_placements_config"} {
		change, found := remoteByKey[key]
		if !found || len(change.CausedBySemanticChangeIDs) == 0 {
			t.Fatalf("remote change %q lacks its local cause: %#v", key, change)
		}
	}
	for _, semantic := range built.Plan.SemanticChanges {
		if semantic.ChangeKind == "managed_remote_drift" {
			t.Fatalf("feature switch key change must explain every affected parameter: %#v", semantic)
		}
	}
}

func TestReconcileV2DoesNotDeleteHistoricalKeyWithoutExplicitCause(t *testing.T) {
	desired := v2CompileFixture(t)
	baseline := v2CompileClone(t, desired)
	baseline["custom_parameters"] = []any{map[string]any{"id": "legacy", "fields": map[string]any{"key": "legacy_managed", "value_type": "string", "value": "old"}}}
	planModel := Plan{}
	changes := reconcileV2ManagedParameters(v2ReconcileInput(baseline, desired, compileV2Parameters(baseline, "development")), nil, &planModel)
	if len(changes) != 0 || len(planModel.RemoteParameterChanges) != 0 {
		t.Fatalf("historical key was deleted without a local cause: semantic=%#v remote=%#v", changes, planModel.RemoteParameterChanges)
	}
}

func TestBuildV2RemoteConditionsRemainBlocking(t *testing.T) {
	desired := v2CompileFixture(t)
	input := v2ReconcileInput(desired, desired, compileV2Parameters(desired, "development"))
	input.RemoteSnapshot.Summary = &remote.Summary{HasUnmodeledConditions: true}
	built, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	if !hasRisk(built.Plan, "unmodeled_remote_condition") || !hasBlockingReason(built.Plan, "unmodeled_remote_condition") || built.Plan.Severity != "blocking" {
		t.Fatalf("conditional values did not block the plan: %#v", built.Plan)
	}
}

func v2ReconcileInput(baseline, desired, parameters map[string]any) Input {
	return Input{
		EnvironmentID: "development", PackRef: "mobile-ad-monetization/v2", Baseline: baseline, Desired: desired, ValidationReady: true,
		RemoteSnapshot: remote.Snapshot{Status: "available", RemoteETag: "etag-v2", Parameters: parameters, Summary: &remote.Summary{}},
	}
}

func copyParameters(parameters map[string]any) map[string]any {
	result := make(map[string]any, len(parameters))
	for key, value := range parameters {
		result[key] = value
	}
	return result
}

func withoutParameter(parameters map[string]any, removed string) map[string]any {
	result := copyParameters(parameters)
	delete(result, removed)
	return result
}

func withParameter(parameters map[string]any, key string, value any) map[string]any {
	result := copyParameters(parameters)
	result[key] = value
	return result
}
