package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestValidationHandlersStoreAndMarkResultsStale(t *testing.T) {
	handler, _ := newTestHandler(t)
	missing := executeRequest(t, handler, http.MethodGet, "/api/v1/drafts/production/diagnostics", "", nil)
	assertDraftError(t, missing, http.StatusNotFound, "validation_not_found")

	validated := executeRequest(t, handler, http.MethodPost, "/api/v1/drafts/production:validate", "", nil)
	if validated.Code != http.StatusOK || validated.Header().Get("ETag") != `"1"` {
		t.Fatalf("validate = %d %s", validated.Code, validated.Body.String())
	}
	var response struct {
		Data validationResultDTO `json:"data"`
		Meta responseMeta        `json:"meta"`
	}
	decodeResponse(t, validated, &response)
	if response.Data.EnvironmentID != "production" || response.Data.ValidatedDraftRevision != 1 || response.Data.Status != "fresh" || response.Data.Readiness != "ready" || response.Data.Diagnostics == nil || response.Meta.Revision != 1 {
		t.Fatalf("validation response = %#v", response)
	}

	current := getDraftForTest(t, handler, "production")
	body := []byte(`{"expected_source_revision":"` + current.SourceRevision + `","write_scope":"baseline","configuration":{"placements":[]}}`)
	updated := executeRequest(t, handler, http.MethodPut, "/api/v1/drafts/production", `"1"`, body)
	if updated.Code != http.StatusOK || updated.Header().Get("ETag") != `"2"` {
		t.Fatalf("draft write = %d %s", updated.Code, updated.Body.String())
	}
	stale := executeRequest(t, handler, http.MethodGet, "/api/v1/drafts/production/diagnostics", "", nil)
	if stale.Code != http.StatusOK || stale.Header().Get("ETag") != `"2"` {
		t.Fatalf("diagnostics = %d %s", stale.Code, stale.Body.String())
	}
	decodeResponse(t, stale, &response)
	if response.Data.Status != "stale" || response.Data.ValidatedDraftRevision != 1 || response.Meta.Revision != 2 {
		t.Fatalf("stale response = %#v", response)
	}
}

func TestValidationHandlerUnknownEnvironment(t *testing.T) {
	handler, _ := newTestHandler(t)
	response := executeRequest(t, handler, http.MethodPost, "/api/v1/drafts/missing:validate", "", nil)
	assertDraftError(t, response, http.StatusNotFound, "environment_not_found")
}

func TestValidationHandlerMatchesOrderedFixtureDiagnostics(t *testing.T) {
	handler, _ := newTestHandler(t)
	fixture := loadValidationFixture(t)
	configureFixtureDraft(t, handler, fixture)

	response := executeRequest(t, handler, http.MethodPost, "/api/v1/drafts/production:validate", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("validate = %d %s", response.Code, response.Body.String())
	}
	var result struct {
		Data validationResultDTO `json:"data"`
	}
	decodeResponse(t, response, &result)
	if result.Data.Readiness != fixture.Readiness {
		t.Fatalf("readiness = %q, want %q", result.Data.Readiness, fixture.Readiness)
	}
	// The fixture's restrict-delete diagnostic belongs to the delete operation;
	// :validate receives only a DraftView and has no pending deletion context.
	expected := make([]fixtureDiagnostic, 0, len(fixture.Diagnostics)-1)
	for _, diagnostic := range fixture.Diagnostics {
		if diagnostic.Code != "frequency_policy_still_referenced" {
			expected = append(expected, diagnostic)
		}
	}
	if len(result.Data.Diagnostics) != len(expected) {
		t.Fatalf("diagnostic count = %d, want %d", len(result.Data.Diagnostics), len(expected))
	}
	for index, expected := range expected {
		got := result.Data.Diagnostics[index]
		if got.Code != expected.Code || got.Path != expected.Path || got.Severity != expected.Severity || got.EntityRef != expected.EntityRef {
			t.Fatalf("diagnostic %d = %#v, want %#v", index, got, expected)
		}
	}
}

type validationFixture struct {
	Placements        []any
	FrequencyPolicies []any
	FeatureSwitches   []any
	Bindings          []any
	Replacements      []fixtureReplacement
	Readiness         string
	Diagnostics       []fixtureDiagnostic
}

type fixtureReplacement struct {
	EntityType string         `json:"entity_type"`
	EntityID   string         `json:"entity_id"`
	Fields     map[string]any `json:"fields"`
}

type fixtureDiagnostic struct {
	Code      string `json:"code"`
	Path      string `json:"path"`
	Severity  string `json:"severity"`
	EntityRef string `json:"entity_ref"`
}

func loadValidationFixture(t *testing.T) validationFixture {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "contracts", "mobile-ad-monetization", "v1")
	entitiesContent, err := os.ReadFile(filepath.Join(root, "entities.json"))
	if err != nil {
		t.Fatal(err)
	}
	overlaysContent, err := os.ReadFile(filepath.Join(root, "validation-overlays.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entities struct {
		Entities struct {
			Placements        []any `json:"placements"`
			FrequencyPolicies []any `json:"frequency_policies"`
			FeatureSwitches   []any `json:"feature_switches"`
		} `json:"entities"`
		UnitBindingMatrix struct {
			Rows []struct {
				PlacementID string         `json:"placement_id"`
				Production  map[string]any `json:"production"`
			} `json:"rows"`
		} `json:"unit_binding_matrix"`
	}
	var overlays struct {
		Scenarios []struct {
			ID      string `json:"id"`
			Overlay struct {
				EntityReplacements []fixtureReplacement `json:"entity_replacements"`
			} `json:"overlay"`
			Expected struct {
				Readiness   string              `json:"readiness"`
				Diagnostics []fixtureDiagnostic `json:"diagnostics"`
			} `json:"expected"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(entitiesContent, &entities); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(overlaysContent, &overlays); err != nil {
		t.Fatal(err)
	}
	fixture := validationFixture{Placements: entities.Entities.Placements, FrequencyPolicies: entities.Entities.FrequencyPolicies, FeatureSwitches: entities.Entities.FeatureSwitches}
	for _, row := range entities.UnitBindingMatrix.Rows {
		for _, platform := range []string{"ios", "android"} {
			fixture.Bindings = append(fixture.Bindings, map[string]any{
				"id": "ub_production_" + platform + "_" + row.PlacementID, "placement_id": row.PlacementID,
				"environment_id": "production", "platform": platform, "unit_id_ref": row.Production[platform], "status": "configured",
			})
		}
	}
	for _, scenario := range overlays.Scenarios {
		if scenario.ID == "nine-human-fixture-diagnostics" {
			fixture.Replacements = scenario.Overlay.EntityReplacements
			fixture.Readiness = scenario.Expected.Readiness
			fixture.Diagnostics = scenario.Expected.Diagnostics
			return fixture
		}
	}
	t.Fatal("nine-human-fixture-diagnostics scenario is missing")
	return validationFixture{}
}

func configureFixtureDraft(t *testing.T, handler http.Handler, fixture validationFixture) {
	t.Helper()
	for _, replacement := range fixture.Replacements {
		collection := map[string][]any{
			"placement":        fixture.Placements,
			"frequency_policy": fixture.FrequencyPolicies,
			"feature_switch":   fixture.FeatureSwitches,
		}[replacement.EntityType]
		for _, value := range collection {
			record := value.(map[string]any)
			if record["id"] != replacement.EntityID {
				continue
			}
			for field, replacementValue := range replacement.Fields {
				record[field] = replacementValue
			}
		}
	}
	sourceRevision := getDraftForTest(t, handler, "production").SourceRevision
	baseline := map[string]any{
		"expected_source_revision": sourceRevision,
		"write_scope":              "baseline",
		"configuration": map[string]any{
			"placements": fixture.Placements, "frequency_policies": fixture.FrequencyPolicies, "feature_switches": fixture.FeatureSwitches,
		},
	}
	body, err := json.Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}
	updated := executeRequest(t, handler, http.MethodPut, "/api/v1/drafts/production", `"1"`, body)
	if updated.Code != http.StatusOK {
		t.Fatalf("baseline write = %d %s", updated.Code, updated.Body.String())
	}
	override := map[string]any{
		"expected_source_revision": sourceRevision,
		"write_scope":              "environment_override",
		"configuration":            map[string]any{"unit_bindings": fixture.Bindings},
	}
	body, err = json.Marshal(override)
	if err != nil {
		t.Fatal(err)
	}
	updated = executeRequest(t, handler, http.MethodPut, "/api/v1/drafts/production", `"2"`, body)
	if updated.Code != http.StatusOK {
		t.Fatalf("override write = %d %s", updated.Code, updated.Body.String())
	}
}
