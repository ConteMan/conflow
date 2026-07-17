package plan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ConteMan/conflow/internal/entities"
	"github.com/ConteMan/conflow/internal/remote"
)

func TestLargeDiffFixtureBuildsTraversableStablePlan(t *testing.T) {
	baseline := loadEntities(t)
	scenario := loadPlanScenario(t)
	baseLayer := clone(t, baseline)
	for _, replacement := range scenario.EntityReplacements {
		for field, value := range replacement.Fields {
			setField(baseLayer, collectionFor(replacement.EntityType), replacement.EntityID, field, value)
		}
	}
	desired := clone(t, baseLayer)
	for _, replacement := range scenario.EnvironmentOverrideReplacements {
		for field, value := range replacement.Fields {
			setField(desired, collectionFor(replacement.EntityType), replacement.EntityID, field, value)
		}
	}
	input := Input{EnvironmentID: "staging", EnvironmentKind: "staging", PackRef: "mobile-ad-monetization/v1", DraftRevision: 13, SourceDigest: "sha256:source-fixture-1", Baseline: baseline, BaseLayer: baseLayer, Desired: desired, ValidationReady: true, Now: time.Date(2026, 7, 11, 10, 0, 4, 0, time.UTC), RemoteSnapshot: remote.Snapshot{Status: "available", RemoteETag: "etag-remote-57", Version: "57", Summary: &remote.Summary{ParameterCount: 42, ManagedParameterCount: 18, ContentDigest: "sha256:remote-fixture-57"}}}
	built, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(built.Plan.SemanticChanges); got != scenario.DirectChangesCount {
		t.Fatalf("semantic changes = %d, want %d", got, scenario.DirectChangesCount)
	}
	if got := len(built.Plan.AffectedEntities); got != scenario.AffectedEntitiesCount {
		t.Fatalf("affected entities = %d, want %d", got, scenario.AffectedEntitiesCount)
	}
	if built.Plan.Status != "ready" || built.Plan.RemoteETag == nil {
		t.Fatalf("plan is not ready: %#v", built.Plan)
	}
	if len(built.Plan.ArtifactMetadata) != 3 {
		t.Fatalf("artifacts = %d, want 3", len(built.Plan.ArtifactMetadata))
	}
	nodes := map[string]bool{}
	for _, a := range built.Plan.AffectedEntities {
		nodes[a.NodeID] = true
	}
	for _, r := range built.Plan.RemoteParameterChanges {
		nodes[r.NodeID] = true
	}
	for _, change := range built.Plan.SemanticChanges {
		for _, id := range append(append([]string{}, change.AffectedEntityNodeIDs...), change.RemoteParameterNodeIDs...) {
			if !nodes[id] {
				t.Fatalf("change %s points to missing node %s", change.NodeID, id)
			}
		}
	}
	var globalCap *SemanticChange
	for index := range built.Plan.SemanticChanges {
		change := &built.Plan.SemanticChanges[index]
		if change.DirectEntityRef == "entity:mobile-ad-monetization/v1:frequency_policy:inter_global_cap" && change.FieldPath == "/cooldown_ms" {
			globalCap = change
			break
		}
	}
	if globalCap == nil || globalCap.BeforeSummary != "30 seconds" || globalCap.AfterSummary != "120 seconds" || len(globalCap.AffectedEntityNodeIDs) != 10 || len(globalCap.RemoteParameterNodeIDs) != 1 {
		t.Fatalf("global cap evidence = %#v", globalCap)
	}
	remoteNode := globalCap.RemoteParameterNodeIDs[0]
	var remoteChange *RemoteParameterChange
	for index := range built.Plan.RemoteParameterChanges {
		change := &built.Plan.RemoteParameterChanges[index]
		if change.NodeID == remoteNode {
			remoteChange = change
			break
		}
	}
	if remoteChange == nil || remoteChange.ParameterKey != "ad_frequency_inter_global_cap" || remoteChange.BeforeSummary != "30 seconds" || remoteChange.AfterSummary != "120 seconds" || len(remoteChange.AffectedEntityNodeIDs) != 10 {
		t.Fatalf("global cap remote evidence = %#v", remoteChange)
	}
	if built.Plan.Severity != scenario.Severity || len(built.Plan.RiskItems) != 1 || built.Plan.RiskItems[0].ReasonCode != scenario.RiskReasonCode {
		t.Fatalf("risk result = %#v", built.Plan)
	}
	artifacts := map[string]bool{}
	for _, artifact := range built.Plan.ArtifactMetadata {
		artifacts[artifact.ArtifactName] = true
	}
	for _, name := range scenario.ArtifactNames {
		if !artifacts[name] {
			t.Fatalf("fixture artifact %q is missing", name)
		}
	}
	input.Now = input.Now.Add(5 * time.Second)
	again, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	if built.Plan.ContentDigest != again.Plan.ContentDigest {
		t.Fatalf("digest changed: %s != %s", built.Plan.ContentDigest, again.Plan.ContentDigest)
	}
}

func TestPreviewOnlyHasNoProviderArtifact(t *testing.T) {
	built, err := Build(Input{EnvironmentID: "development", PackRef: "mobile-ad-monetization/v1", DraftRevision: 1, SourceDigest: "src", Desired: map[string]any{}, Baseline: map[string]any{}, ValidationReady: true, RemoteSnapshot: remote.Snapshot{Status: "unavailable", UnavailableReason: remote.SnapshotMissing}})
	if err != nil {
		t.Fatal(err)
	}
	if built.Plan.Status != "preview_only" || !hasRisk(built.Plan, "remote_snapshot_unavailable") || !hasRisk(built.Plan, "remote_baseline_missing") {
		t.Fatalf("preview result = %#v", built.Plan)
	}
	if _, ok := built.Artifacts["provider-input.json"]; ok {
		t.Fatal("preview only created provider artifact")
	}
}

func TestRiskRules(t *testing.T) {
	available := func(parameters map[string]any) remote.Snapshot {
		return remote.Snapshot{Status: "available", RemoteETag: "etag-1", Parameters: parameters, Summary: &remote.Summary{}}
	}
	tests := []struct {
		name, reason, severity string
		input                  Input
	}{
		{"shared frequency policy", "shared_frequency_policy_relaxed", "high", Input{PackRef: "pack/v1", Baseline: configuration("frequency_policies", record("cap", "cooldown_ms", 30000)), Desired: configuration("frequency_policies", record("cap", "cooldown_ms", 120000)), RemoteSnapshot: available(nil), ValidationReady: true}},
		{"global feature switch", "global_feature_switch_changed", "high", Input{PackRef: "pack/v1", EnvironmentKind: "production", Baseline: configuration("feature_switches", record("switch", "default_value", false)), Desired: configuration("feature_switches", record("switch", "default_value", true)), RemoteSnapshot: available(nil), ValidationReady: true}},
		{"production network mode", "production_network_mode_changed", "high", Input{PackRef: "pack/v1", EnvironmentKind: "production", Baseline: configuration("placements", record("placement", "network_mode", "hybrid")), Desired: configuration("placements", record("placement", "network_mode", "bidding")), RemoteSnapshot: available(nil), ValidationReady: true}},
		{"unit binding", "unit_binding_changed", "medium", Input{PackRef: "pack/v1", Baseline: configuration("unit_bindings", record("binding", "unit_id_ref", "unit-a")), Desired: configuration("unit_bindings", record("binding", "unit_id_ref", "unit-b")), RemoteSnapshot: available(nil), ValidationReady: true}},
		{"managed parameter deleted", "managed_parameter_deleted", "blocking", Input{PackRef: "pack/v1", Baseline: configuration("frequency_policies", record("cap", "cooldown_ms", 30000)), Desired: configuration("frequency_policies"), RemoteSnapshot: available(map[string]any{"ad_frequency_cap": 30000}), ValidationReady: true}},
		{"remote baseline missing", "remote_baseline_missing", "blocking", Input{PackRef: "pack/v1", Baseline: map[string]any{}, Desired: map[string]any{}, RemoteSnapshot: remote.Snapshot{Status: "unavailable", UnavailableReason: remote.SnapshotMissing}, ValidationReady: true}},
		{"unmodeled remote condition", "unmodeled_remote_condition", "blocking", Input{PackRef: "pack/v1", Baseline: map[string]any{}, Desired: map[string]any{}, RemoteSnapshot: remote.Snapshot{Status: "available", RemoteETag: "etag-1", Summary: &remote.Summary{HasUnmodeledConditions: true}}, ValidationReady: true}},
		{"validation not ready", "validation_not_ready", "blocking", Input{PackRef: "pack/v1", Baseline: map[string]any{}, Desired: map[string]any{}, RemoteSnapshot: available(nil), ValidationReady: false}},
		{"remote snapshot unavailable", "remote_snapshot_unavailable", "blocking", Input{PackRef: "pack/v1", Baseline: map[string]any{}, Desired: map[string]any{}, RemoteSnapshot: remote.Snapshot{Status: "unavailable", UnavailableReason: remote.ProviderUnavailable}, ValidationReady: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			built, err := Build(test.input)
			if err != nil {
				t.Fatal(err)
			}
			risk, ok := riskFor(built.Plan, test.reason)
			if !ok {
				t.Fatalf("risk %q missing from %#v", test.reason, built.Plan.RiskItems)
			}
			if risk.Severity != test.severity {
				t.Fatalf("risk %q severity = %q, want %q", test.reason, risk.Severity, test.severity)
			}
			if isBlockingReason(test.reason) && !hasBlockingReason(built.Plan, test.reason) {
				t.Fatalf("blocking reason %q missing from %#v", test.reason, built.Plan.BlockingReasons)
			}
		})
	}
}

func TestRuntimeEntityShapeReadsSemanticDiffFields(t *testing.T) {
	baseline := configuration("frequency_policies", record("inter_global_cap", "cooldown_ms", 30000))
	desired := clone(t, baseline)
	setField(desired, "frequency_policies", "inter_global_cap", "cooldown_ms", 120000)
	built, err := Build(Input{
		EnvironmentID: "development", PackRef: "mobile-ad-monetization/v1", Baseline: baseline, Desired: desired,
		RemoteSnapshot: remote.Snapshot{Status: "available", RemoteETag: "etag-1", Summary: &remote.Summary{}}, ValidationReady: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var cooldown *SemanticChange
	for index := range built.Plan.SemanticChanges {
		change := &built.Plan.SemanticChanges[index]
		if change.DirectEntityRef == "entity:mobile-ad-monetization/v1:frequency_policy:inter_global_cap" && change.FieldPath == "/cooldown_ms" {
			cooldown = change
			break
		}
	}
	if cooldown == nil || cooldown.BeforeSummary != "30 seconds" || cooldown.AfterSummary != "120 seconds" {
		t.Fatalf("cooldown semantic diff = %#v", cooldown)
	}
	if !hasRisk(built.Plan, "shared_frequency_policy_relaxed") {
		t.Fatalf("runtime shape did not produce shared frequency risk: %#v", built.Plan.RiskItems)
	}
}

func TestV1FieldChangesKeepFieldParameterProjection(t *testing.T) {
	baseline := configuration("frequency_policies", record("inter_global_cap", "cooldown_ms", 30000))
	desired := clone(t, baseline)
	setField(desired, "frequency_policies", "inter_global_cap", "cooldown_ms", 120000)
	built, err := Build(Input{
		EnvironmentID: "development", PackRef: "mobile-ad-monetization/v1", Baseline: baseline, Desired: desired, ValidationReady: true,
		RemoteSnapshot: remote.Snapshot{Status: "available", RemoteETag: "etag-1", Parameters: map[string]any{"ad_frequency_inter_global_cap": 30000}, Summary: &remote.Summary{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range built.Plan.RemoteParameterChanges {
		if change.ParameterKey == "ad_frequency_inter_global_cap" {
			if change.ChangeKind != "updated" || change.BeforeSummary != summary(30000) || change.AfterSummary != summary(120000) {
				t.Fatalf("v1 remote change = %#v", change)
			}
			return
		}
	}
	t.Fatalf("v1 remote change is missing: %#v", built.Plan.RemoteParameterChanges)
}

func TestSnapshotTokenIsOpaqueAndBoundToSnapshotState(t *testing.T) {
	input := Input{EnvironmentID: "development", PackRef: "pack/v1", DraftRevision: 17, SourceDigest: "sha256:source-secret", Baseline: map[string]any{}, Desired: map[string]any{"key": "desired-value"}, ValidationReady: true, Now: time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC), RemoteSnapshot: remote.Snapshot{Status: "available", RemoteETag: "etag-secret", Summary: &remote.Summary{}}}
	first, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"17", "source-secret", "etag-secret", "desired-value"} {
		if strings.Contains(first.Plan.SnapshotToken, value) {
			t.Fatalf("snapshot token exposes %q: %s", value, first.Plan.SnapshotToken)
		}
	}
	input.RemoteSnapshot = remote.Snapshot{Status: "unavailable", UnavailableReason: remote.ProviderUnauthorized}
	second, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	if first.Plan.SnapshotToken == second.Plan.SnapshotToken {
		t.Fatalf("snapshot token did not bind remote state: %s", first.Plan.SnapshotToken)
	}
}

func hasRisk(p Plan, reason string) bool {
	_, ok := riskFor(p, reason)
	return ok
}

func riskFor(p Plan, reason string) (RiskItem, bool) {
	for _, item := range p.RiskItems {
		if item.ReasonCode == reason {
			return item, true
		}
	}
	return RiskItem{}, false
}

func hasBlockingReason(p Plan, reason string) bool {
	for _, item := range p.BlockingReasons {
		if item.ReasonCode == reason {
			return true
		}
	}
	return false
}

func isBlockingReason(reason string) bool {
	return map[string]bool{"managed_parameter_deleted": true, "remote_baseline_missing": true, "unmodeled_remote_condition": true, "validation_not_ready": true, "remote_snapshot_unavailable": true}[reason]
}

func configuration(collection string, records ...map[string]any) map[string]any {
	values := make([]any, len(records))
	for index := range records {
		values[index] = records[index]
	}
	return map[string]any{collection: values}
}

func record(id, field string, value any) map[string]any {
	return map[string]any{"id": id, "fields": map[string]any{field: value}}
}

func TestArtifactsRedactCredentialLikeValues(t *testing.T) {
	built, err := Build(Input{EnvironmentID: "development", PackRef: "mobile-ad-monetization/v1", DraftRevision: 1, SourceDigest: "src", Desired: map[string]any{"api_token": "do-not-persist"}, Baseline: map[string]any{}, ValidationReady: true, RemoteSnapshot: remote.Snapshot{Status: "available", RemoteETag: "etag"}})
	if err != nil {
		t.Fatal(err)
	}
	for name, artifact := range built.Artifacts {
		if string(artifact) == "" {
			continue
		}
		if contains := string(artifact); contains != "" && strings.Contains(contains, "do-not-persist") {
			t.Fatalf("%s contains a token", name)
		}
	}
}

func loadEntities(t *testing.T) map[string]any {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "mobile-ad-monetization", "v1", "entities.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Entities map[string]any `json:"entities"`
	}
	if err := json.Unmarshal(content, &fixture); err != nil {
		t.Fatal(err)
	}
	// Contract fixtures remain flat for compatibility; the Plan engine only
	// receives the nested runtime shape persisted by Draft entity CRUD.
	return entities.AdaptFlatFixture(fixture.Entities)
}

type fixtureReplacement struct {
	EntityType string         `json:"entity_type"`
	EntityID   string         `json:"entity_id"`
	Fields     map[string]any `json:"fields"`
}
type planScenario struct {
	EntityReplacements              []fixtureReplacement `json:"entity_replacements"`
	EnvironmentOverrideReplacements []fixtureReplacement `json:"environment_override_replacements"`
	DirectChangesCount              int                  `json:"direct_changes_count"`
	AffectedEntitiesCount           int                  `json:"affected_entities_count"`
	Severity                        string               `json:"severity"`
	RiskItems                       []struct {
		ReasonCode string `json:"reason_code"`
	} `json:"risk_items"`
	ArtifactMetadata []struct {
		ArtifactName string `json:"artifact_name"`
	} `json:"artifact_metadata"`
	RiskReasonCode string
	ArtifactNames  []string
}

func loadPlanScenario(t *testing.T) planScenario {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "mobile-ad-monetization", "v1", "plan-risk-operation-rollback.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Scenarios []struct {
			ID           string       `json:"id"`
			Input        planScenario `json:"input"`
			ExpectedPlan planScenario `json:"expected_plan"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(content, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range fixture.Scenarios {
		if scenario.ID == "large-diff-inter-global-cap-30-to-120" {
			scenario.Input.DirectChangesCount = scenario.ExpectedPlan.DirectChangesCount
			scenario.Input.AffectedEntitiesCount = scenario.ExpectedPlan.AffectedEntitiesCount
			scenario.Input.Severity = scenario.ExpectedPlan.Severity
			if len(scenario.ExpectedPlan.RiskItems) > 0 {
				scenario.Input.RiskReasonCode = scenario.ExpectedPlan.RiskItems[0].ReasonCode
			}
			for _, artifact := range scenario.ExpectedPlan.ArtifactMetadata {
				scenario.Input.ArtifactNames = append(scenario.Input.ArtifactNames, artifact.ArtifactName)
			}
			return scenario.Input
		}
	}
	t.Fatal("main plan fixture is missing")
	return planScenario{}
}
func collectionFor(entityType string) string {
	return map[string]string{"frequency_policy": "frequency_policies", "feature_switch": "feature_switches", "placement": "placements", "unit_binding": "unit_bindings"}[entityType]
}
func clone(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(content, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func setField(configuration map[string]any, collection, id, field string, value any) {
	for _, raw := range configuration[collection].([]any) {
		record := raw.(map[string]any)
		if record["id"] == id {
			record["fields"].(map[string]any)[field] = value
			return
		}
	}
}
