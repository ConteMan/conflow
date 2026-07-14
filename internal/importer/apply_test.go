package importer_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ConteMan/conflow/internal/importer"
)

func bundleWithDecisions() importer.ImportBundle {
	return importer.ImportBundle{
		FormatVersion: importer.FormatVersion,
		PackRef:       "mobile-ad-monetization/v2",
		SchemaVersion: 2,
		CreatedAt:     time.Now().UTC(),
		Entities: map[string][]importer.BundleEntity{
			"feature_switch": {
				{ID: "enable_splash", Fields: map[string]any{"default_value": true}},
			},
		},
		DecisionsRequired: []importer.DecisionRequired{
			{Key: "feature_switch.enable_splash.risk_level", Reason: "需要指定风险等级", Hint: "low/medium/high"},
		},
	}
}

func TestApply_ValidToken_DecisionsMerged(t *testing.T) {
	bundle := bundleWithDecisions()

	// Generate a valid token for revision 7.
	token := importer.GenerateToken(7)

	result, err := importer.Apply(importer.ApplyInput{
		Bundle:       bundle,
		PreviewToken: token,
		Decisions: []importer.ImportDecision{
			{Key: "feature_switch.enable_splash.risk_level", Value: "low"},
		},
		ConflictMode:  importer.ConflictReplace,
		DraftRevision: 7,
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	entities, ok := result.PreparedEntities["feature_switch"]
	if !ok || len(entities) == 0 {
		t.Fatal("expected feature_switch entities in PreparedEntities")
	}

	riskLevel, _ := entities[0].Fields["risk_level"].(string)
	if riskLevel != "low" {
		t.Errorf("expected risk_level=low after decision merge, got %q", riskLevel)
	}

	// Original default_value should still be present
	if entities[0].Fields["default_value"] != true {
		t.Error("expected default_value=true to be preserved after decision merge")
	}
}

func TestApply_TokenExpired(t *testing.T) {
	bundle := newTestBundle()

	// A token with an expired ExpiresAt cannot be created via GenerateToken (it
	// generates tokens in the future). We create an obviously invalid token
	// string to trigger the expired path.
	_, err := importer.Apply(importer.ApplyInput{
		Bundle:        bundle,
		PreviewToken:  "aW52YWxpZA==", // base64("invalid") — won't parse correctly
		DraftRevision: 1,
	})

	if !errors.Is(err, importer.ErrTokenExpired) {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}
}

func TestApply_RevisionChanged(t *testing.T) {
	bundle := newTestBundle()
	token := importer.GenerateToken(3) // token for revision 3

	_, err := importer.Apply(importer.ApplyInput{
		Bundle:        bundle,
		PreviewToken:  token,
		DraftRevision: 5, // draft has advanced to 5
	})

	if !errors.Is(err, importer.ErrRevisionChanged) {
		t.Errorf("expected ErrRevisionChanged, got %v", err)
	}
}

func TestApply_MissingDecisions(t *testing.T) {
	bundle := bundleWithDecisions() // has one DecisionsRequired
	token := importer.GenerateToken(1)

	// No decisions supplied at all
	_, err := importer.Apply(importer.ApplyInput{
		Bundle:        bundle,
		PreviewToken:  token,
		Decisions:     nil,
		DraftRevision: 1,
	})

	if err == nil {
		t.Fatal("expected error for missing decisions, got nil")
	}
}

func TestApply_PartialDecisions(t *testing.T) {
	bundle := importer.ImportBundle{
		FormatVersion: importer.FormatVersion,
		PackRef:       "mobile-ad-monetization/v2",
		SchemaVersion: 2,
		CreatedAt:     time.Now().UTC(),
		Entities:      map[string][]importer.BundleEntity{},
		DecisionsRequired: []importer.DecisionRequired{
			{Key: "a.b.c", Reason: "reason1"},
			{Key: "d.e.f", Reason: "reason2"},
		},
	}
	token := importer.GenerateToken(1)

	// Only one of two required decisions provided
	_, err := importer.Apply(importer.ApplyInput{
		Bundle:       bundle,
		PreviewToken: token,
		Decisions: []importer.ImportDecision{
			{Key: "a.b.c", Value: "val1"},
			// d.e.f is missing
		},
		DraftRevision: 1,
	})

	if err == nil {
		t.Fatal("expected error for missing decision d.e.f, got nil")
	}
}

func TestApply_UnitBindingExcluded(t *testing.T) {
	bundle := importer.ImportBundle{
		FormatVersion: importer.FormatVersion,
		PackRef:       "mobile-ad-monetization/v2",
		SchemaVersion: 2,
		CreatedAt:     time.Now().UTC(),
		Entities: map[string][]importer.BundleEntity{
			"unit_binding": {
				{ID: "binding_1", Fields: map[string]any{"placement_id": "p1"}},
			},
			"feature_switch": {
				{ID: "flag_1", Fields: map[string]any{"default_value": true}},
			},
		},
	}
	token := importer.GenerateToken(2)

	result, err := importer.Apply(importer.ApplyInput{
		Bundle:        bundle,
		PreviewToken:  token,
		DraftRevision: 2,
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if _, ok := result.PreparedEntities["unit_binding"]; ok {
		t.Error("unit_binding must not appear in PreparedEntities")
	}
	if _, ok := result.PreparedEntities["feature_switch"]; !ok {
		t.Error("feature_switch should appear in PreparedEntities")
	}
}

func TestApply_NoDecisionsRequired(t *testing.T) {
	bundle := newTestBundle() // no DecisionsRequired
	token := importer.GenerateToken(10)

	result, err := importer.Apply(importer.ApplyInput{
		Bundle:        bundle,
		PreviewToken:  token,
		Decisions:     nil, // no decisions needed
		DraftRevision: 10,
	})
	if err != nil {
		t.Fatalf("Apply returned error for bundle without decisions: %v", err)
	}

	if len(result.PreparedEntities) == 0 {
		t.Error("expected PreparedEntities to be non-empty")
	}
}

func TestApply_InputNotMutated(t *testing.T) {
	bundle := newTestBundle()
	token := importer.GenerateToken(1)
	originalField := bundle.Entities["feature_switch"][0].Fields["default_value"]

	_, err := importer.Apply(importer.ApplyInput{
		Bundle:        bundle,
		PreviewToken:  token,
		DraftRevision: 1,
		Decisions: []importer.ImportDecision{
			{Key: "feature_switch.enable_splash.default_value", Value: false},
		},
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	// Original bundle entity must not be mutated
	if bundle.Entities["feature_switch"][0].Fields["default_value"] != originalField {
		t.Error("Apply must not mutate the input bundle entities")
	}
}
