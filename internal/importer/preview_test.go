package importer_test

import (
	"testing"
	"time"

	"github.com/ConteMan/conflow/internal/importer"
)

func newTestBundle() importer.ImportBundle {
	return importer.ImportBundle{
		FormatVersion: importer.FormatVersion,
		PackRef:       "mobile-ad-monetization/v2",
		SchemaVersion: 2,
		CreatedAt:     time.Now().UTC(),
		Entities: map[string][]importer.BundleEntity{
			"feature_switch": {
				{ID: "enable_splash", Fields: map[string]any{"key": "enable_splash", "default_value": true}},
				{ID: "enable_banner", Fields: map[string]any{"key": "enable_banner", "default_value": false}},
			},
			"frequency_policy": {
				{ID: "global_cap", Fields: map[string]any{"cooldown": 30}},
			},
		},
	}
}

func TestPreview_ConflictReplace(t *testing.T) {
	bundle := newTestBundle()

	// Draft already has enable_splash (same ID as bundle entity)
	draftEntities := map[string][]string{
		"feature_switch": {"enable_splash"},
	}

	result, err := importer.Preview(bundle, draftEntities, 1, importer.ConflictReplace)
	if err != nil {
		t.Fatalf("Preview returned error: %v", err)
	}

	// enable_splash is in draft → should be replaced
	// enable_banner is new → should be added
	// global_cap is new type with no draft entities → should be added
	assertEntityAction(t, result.EntityPlan.ToAdd, "feature_switch", "enable_banner")
	assertEntityAction(t, result.EntityPlan.ToAdd, "frequency_policy", "global_cap")
	assertEntityAction(t, result.EntityPlan.ToReplace, "feature_switch", "enable_splash")
	assertNone(t, result.EntityPlan.ToSkip)

	// enable_splash is referenced by both draft and bundle — no ToKeep
	assertNone(t, result.EntityPlan.ToKeep)

	if result.PreviewToken == "" {
		t.Error("expected non-empty preview token")
	}
	if result.ExpiresAt.Before(time.Now()) {
		t.Error("expires_at should be in the future")
	}
	if len(result.Risks) == 0 {
		t.Error("expected at least one risk when ToReplace is non-empty")
	}
	if result.ConflictMode != importer.ConflictReplace {
		t.Errorf("expected conflict_mode replace, got %q", result.ConflictMode)
	}
}

func TestPreview_ConflictMerge(t *testing.T) {
	bundle := newTestBundle()

	// Draft already has enable_splash
	draftEntities := map[string][]string{
		"feature_switch": {"enable_splash"},
	}

	result, err := importer.Preview(bundle, draftEntities, 1, importer.ConflictMerge)
	if err != nil {
		t.Fatalf("Preview returned error: %v", err)
	}

	// enable_splash is in draft → skipped (merge keeps existing)
	// enable_banner is new → added
	// global_cap is new → added
	assertEntityAction(t, result.EntityPlan.ToAdd, "feature_switch", "enable_banner")
	assertEntityAction(t, result.EntityPlan.ToAdd, "frequency_policy", "global_cap")
	assertEntityAction(t, result.EntityPlan.ToSkip, "feature_switch", "enable_splash")
	assertNone(t, result.EntityPlan.ToReplace)

	// enable_splash remains in draft but is skipped, not replaced
	if len(result.Risks) != 0 {
		t.Errorf("expected no risks for merge mode, got %v", result.Risks)
	}
}

func TestPreview_ConflictSkip(t *testing.T) {
	bundle := newTestBundle()

	// Draft has one feature_switch entity — so the whole type is skipped.
	// frequency_policy type has no draft entities → added.
	draftEntities := map[string][]string{
		"feature_switch": {"existing_switch"},
	}

	result, err := importer.Preview(bundle, draftEntities, 1, importer.ConflictSkip)
	if err != nil {
		t.Fatalf("Preview returned error: %v", err)
	}

	// Both feature_switch bundle entities must be skipped (type has draft entries)
	assertEntityAction(t, result.EntityPlan.ToSkip, "feature_switch", "enable_splash")
	assertEntityAction(t, result.EntityPlan.ToSkip, "feature_switch", "enable_banner")

	// frequency_policy has no draft entries → added
	assertEntityAction(t, result.EntityPlan.ToAdd, "frequency_policy", "global_cap")

	// existing_switch is in draft but not in bundle → ToKeep
	assertEntityAction(t, result.EntityPlan.ToKeep, "feature_switch", "existing_switch")

	assertNone(t, result.EntityPlan.ToReplace)
}

func TestPreview_EmptyDraft(t *testing.T) {
	bundle := newTestBundle()
	draftEntities := map[string][]string{}

	result, err := importer.Preview(bundle, draftEntities, 5, importer.ConflictReplace)
	if err != nil {
		t.Fatalf("Preview returned error: %v", err)
	}

	// All bundle entities should be added (draft is empty)
	if len(result.EntityPlan.ToAdd) != 3 {
		t.Errorf("expected 3 ToAdd entities, got %d", len(result.EntityPlan.ToAdd))
	}
	assertNone(t, result.EntityPlan.ToReplace)
	assertNone(t, result.EntityPlan.ToSkip)
	assertNone(t, result.EntityPlan.ToKeep)
}

func TestPreview_UnitBindingIgnored(t *testing.T) {
	bundle := importer.ImportBundle{
		FormatVersion: importer.FormatVersion,
		PackRef:       "mobile-ad-monetization/v2",
		SchemaVersion: 2,
		CreatedAt:     time.Now().UTC(),
		Entities: map[string][]importer.BundleEntity{
			"unit_binding": {
				{ID: "binding_1", Fields: map[string]any{"placement_id": "p1"}},
			},
		},
	}

	result, err := importer.Preview(bundle, map[string][]string{}, 1, importer.ConflictReplace)
	if err != nil {
		t.Fatalf("Preview returned error: %v", err)
	}

	// unit_binding is never imported
	assertNone(t, result.EntityPlan.ToAdd)
	assertNone(t, result.EntityPlan.ToReplace)
}

func TestPreview_DecisionsRequired_PassedThrough(t *testing.T) {
	bundle := importer.ImportBundle{
		FormatVersion: importer.FormatVersion,
		PackRef:       "mobile-ad-monetization/v2",
		SchemaVersion: 2,
		CreatedAt:     time.Now().UTC(),
		Entities:      map[string][]importer.BundleEntity{},
		DecisionsRequired: []importer.DecisionRequired{
			{Key: "network_settings.default.active_network", Reason: "需要选择广告网络", Hint: "admob"},
		},
	}

	result, err := importer.Preview(bundle, map[string][]string{}, 1, importer.ConflictReplace)
	if err != nil {
		t.Fatalf("Preview returned error: %v", err)
	}

	if len(result.DecisionsRequired) != 1 {
		t.Errorf("expected 1 DecisionRequired, got %d", len(result.DecisionsRequired))
	}
	if result.DecisionsRequired[0].Key != "network_settings.default.active_network" {
		t.Errorf("unexpected DecisionRequired key: %s", result.DecisionsRequired[0].Key)
	}
}

func TestPreview_InvalidFormatVersion(t *testing.T) {
	bundle := importer.ImportBundle{
		FormatVersion: 99,
		Entities:      map[string][]importer.BundleEntity{},
	}
	_, err := importer.Preview(bundle, map[string][]string{}, 1, importer.ConflictReplace)
	if err == nil {
		t.Error("expected error for unsupported format version")
	}
}

// assertEntityAction checks that the given action list contains an entity of
// the given type and ID.
func assertEntityAction(t *testing.T, actions []importer.EntityAction, entityType, id string) {
	t.Helper()
	for _, a := range actions {
		if a.EntityType == entityType && a.ID == id {
			return
		}
	}
	t.Errorf("expected entity_type=%q id=%q in actions %v", entityType, id, actions)
}

// assertNone checks that the given action list is empty.
func assertNone(t *testing.T, actions []importer.EntityAction) {
	t.Helper()
	if len(actions) > 0 {
		t.Errorf("expected empty action list, got %v", actions)
	}
}
