package draft

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

type layeringFixture struct {
	Pack struct {
		Ref      string         `json:"ref"`
		Defaults map[string]any `json:"defaults"`
		Fields   []struct {
			Path                       string `json:"path"`
			Nullable                   bool   `json:"nullable"`
			EnvironmentOverrideAllowed bool   `json:"environment_override_allowed"`
			MinLength                  *int   `json:"min_length"`
		} `json:"fields"`
	} `json:"pack"`
	Environments []Environment `json:"environments"`
	Source       struct {
		Revision             string                    `json:"revision"`
		Baseline             map[string]any            `json:"baseline"`
		EnvironmentOverrides map[string]map[string]any `json:"environment_overrides"`
	} `json:"source"`
	Scenarios []fixtureScenario `json:"scenarios"`
}

type fixtureScenario struct {
	ID    string `json:"id"`
	Given struct {
		DraftRevision  uint64 `json:"draft_revision"`
		SourceRevision string `json:"source_revision"`
		Drafts         struct {
			Baseline             *map[string]any           `json:"baseline"`
			EnvironmentOverrides map[string]map[string]any `json:"environment_overrides"`
		} `json:"drafts"`
	} `json:"given"`
	Operation struct {
		Method        string         `json:"method"`
		Action        string         `json:"action"`
		EnvironmentID string         `json:"environment_id"`
		IfMatch       string         `json:"if_match"`
		Body          map[string]any `json:"body"`
	} `json:"operation"`
	Expected map[string]any `json:"expected"`
}

func TestLayeringContractFixture(t *testing.T) {
	fixture := loadLayeringFixture(t)
	schema := Schema{PackRef: fixture.Pack.Ref, Defaults: fixture.Pack.Defaults}
	for _, field := range fixture.Pack.Fields {
		typeName := typeAt(fixture.Pack.Defaults, field.Path)
		schema.Fields = append(schema.Fields, Field{Path: field.Path, Type: typeName, Nullable: field.Nullable, EnvironmentOverrideAllowed: field.EnvironmentOverrideAllowed, MinLength: field.MinLength})
	}

	for _, scenario := range fixture.Scenarios {
		t.Run(scenario.ID, func(t *testing.T) {
			source := SourceSnapshot{Revision: fixture.Source.Revision, Baseline: fixture.Source.Baseline, EnvironmentOverrides: fixture.Source.EnvironmentOverrides}
			if scenario.Given.SourceRevision != "" {
				source.Revision = scenario.Given.SourceRevision
			}
			state := State{Revision: scenario.Given.DraftRevision, EnvironmentOverrides: scenario.Given.Drafts.EnvironmentOverrides}
			if state.Revision == 0 {
				state.Revision = 1
			}
			if scenario.Given.Drafts.Baseline != nil {
				state.BaselinePresent = true
				state.Baseline = *scenario.Given.Drafts.Baseline
			}
			store := NewMemory(source, state)
			wantStatus := int(scenario.Expected["status"].(float64))

			if scenario.Operation.Method == "GET" {
				view, revision, err := store.View(schema, fixture.Environments, scenario.Operation.EnvironmentID)
				if err != nil {
					t.Fatal(err)
				}
				if wantStatus != 200 {
					t.Fatalf("GET expected status %d", wantStatus)
				}
				assertFixtureView(t, scenario.Expected, view, revision)
				return
			}
			if scenario.Operation.IfMatch == "" {
				// HTTP validates this before request decoding; covered by handler tests.
				if wantStatus != 428 {
					t.Fatalf("missing If-Match fixture status = %d", wantStatus)
				}
				return
			}
			expectedRevision, err := ParseRevision(scenario.Operation.IfMatch)
			if err != nil {
				t.Fatal(err)
			}
			body := scenario.Operation.Body
			expectedSourceRevision, _ := body["expected_source_revision"].(string)
			scope, _ := body["write_scope"].(string)
			var configuration json.RawMessage
			if raw, exists := body["configuration"]; exists {
				configuration, err = json.Marshal(raw)
				if err != nil {
					t.Fatal(err)
				}
			}
			action := "put"
			if scenario.Operation.Action != "" {
				action = scenario.Operation.Action
			}
			view, revision, err := store.Mutate(schema, fixture.Environments, scenario.Operation.EnvironmentID, Mutation{ExpectedRevision: expectedRevision, ExpectedSourceRevision: expectedSourceRevision, Scope: scope, Action: action, Configuration: configuration})
			switch wantStatus {
			case 200:
				if err != nil {
					t.Fatalf("mutation failed: %v", err)
				}
				assertFixtureView(t, scenario.Expected, view, revision)
			case 412:
				var conflict *ConflictError
				if !errors.As(err, &conflict) {
					t.Fatalf("error = %v, want conflict", err)
				}
				if conflict.Code != scenario.Expected["error_code"] || conflict.CurrentRevision != uint64(scenario.Expected["current_revision"].(float64)) || conflict.CurrentSourceRevision != scenario.Expected["current_source_revision"] || conflict.ConflictScope != scenario.Expected["conflict_scope"] {
					t.Fatalf("conflict = %#v", conflict)
				}
				assertFixtureView(t, scenario.Expected["current_state_subset"].(map[string]any), conflict.CurrentState, conflict.CurrentRevision)
			case 422:
				var validation *ValidationError
				if !errors.As(err, &validation) {
					t.Fatalf("error = %v, want validation", err)
				}
				assertDetails(t, scenario.Expected, validation.Details)
			default:
				t.Fatalf("unsupported fixture status %d", wantStatus)
			}
		})
	}
}

func TestStorePersistsExplicitEmptyReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".conflow", "draft.json")
	source := SourceSnapshot{Revision: "src-1", Baseline: map[string]any{}, EnvironmentOverrides: map[string]map[string]any{}}
	schema := Schema{PackRef: "fixture-config/v1", Defaults: map[string]any{}, Fields: []Field{}}
	environments := []Environment{{ID: "development", Name: "Development", Kind: "development"}}
	store, err := Open(path, func() (SourceSnapshot, error) { return cloneSource(source), nil })
	if err != nil {
		t.Fatal(err)
	}
	view, revision, err := store.Mutate(schema, environments, "development", Mutation{ExpectedRevision: 1, ExpectedSourceRevision: "src-1", Scope: ScopeBaseline, Action: "reset"})
	if err != nil {
		t.Fatal(err)
	}
	if revision != 2 || !view.Baseline.Draft.Present || len(view.Baseline.Draft.Value) != 0 {
		t.Fatalf("reset state = %#v, revision = %d", view, revision)
	}

	reopened, err := Open(path, func() (SourceSnapshot, error) { return cloneSource(source), nil })
	if err != nil {
		t.Fatal(err)
	}
	view, revision, err = reopened.View(schema, environments, "development")
	if err != nil {
		t.Fatal(err)
	}
	if revision != 2 || !view.Baseline.Draft.Present || len(view.Baseline.Draft.Value) != 0 {
		t.Fatalf("reopened state = %#v, revision = %d", view, revision)
	}
}

func assertFixtureView(t *testing.T, expected map[string]any, view View, revision uint64) {
	t.Helper()
	if want, exists := expected["etag"]; exists && FormatRevision(revision) != want {
		t.Fatalf("etag = %s, want %s", FormatRevision(revision), want)
	}
	if want, exists := expected["draft_revision"]; exists && revision != uint64(want.(float64)) {
		t.Fatalf("revision = %d, want %v", revision, want)
	}
	if want, exists := expected["dirty"]; exists && view.Dirty != want {
		t.Fatalf("dirty = %v, want %v", view.Dirty, want)
	}
	if want, exists := expected["dirty_scopes"]; exists && !jsonEqual(view.DirtyScopes, want) {
		t.Fatalf("dirty scopes = %#v, want %#v", view.DirtyScopes, want)
	}
	if want, exists := expected["affected_environment_ids"]; exists {
		got := make([]string, len(view.AffectedEnvironments))
		for index, environment := range view.AffectedEnvironments {
			got[index] = environment.ID
		}
		if !jsonEqual(got, want) {
			t.Fatalf("affected = %#v, want %#v", got, want)
		}
	}
	if want, exists := expected["baseline_layer_draft"]; exists && !jsonEqual(view.Baseline.Draft, want) {
		t.Fatalf("baseline draft = %#v, want %#v", view.Baseline.Draft, want)
	}
	if want, exists := expected["baseline_layer_resolved"]; exists && !jsonEqual(view.Baseline.Resolved, want) {
		t.Fatalf("baseline resolved = %#v, want %#v", view.Baseline.Resolved, want)
	}
	if want, exists := expected["baseline_resolved_source"]; exists && want.(bool) && !jsonEqual(view.Baseline.Resolved, view.Baseline.Source) {
		t.Fatalf("baseline resolved = %#v, source = %#v", view.Baseline.Resolved, view.Baseline.Source)
	}
	if want, exists := expected["development_effective"]; exists && !jsonEqual(view.Effective, want) {
		t.Fatalf("effective = %#v, want %#v", view.Effective, want)
	}
	if want, exists := expected["development_effective_subset"]; exists && !containsJSON(view.Effective, want) {
		t.Fatalf("effective = %#v, missing subset %#v", view.Effective, want)
	}
	if states, exists := expected["field_states"].(map[string]any); exists {
		for path, raw := range states {
			assertField(t, view, path, raw.(map[string]any))
		}
	}
	if origins, exists := expected["origins"].(map[string]any); exists {
		for path, raw := range origins {
			assertField(t, view, path, map[string]any{"origin": raw})
		}
	}
	if field, exists := expected["field"].(map[string]any); exists {
		assertField(t, view, field["path"].(string), field)
	}
}

func assertField(t *testing.T, view View, path string, expected map[string]any) {
	t.Helper()
	state := fieldState(view, path)
	if state == nil {
		t.Fatalf("missing field state %s", path)
	}
	if want, exists := expected["origin"]; exists && state.Origin != want {
		t.Fatalf("%s origin = %s, want %v", path, state.Origin, want)
	}
	if want, exists := expected["is_environment_overridden"]; exists && state.IsEnvironmentOverridden != want {
		t.Fatalf("%s overridden = %v, want %v", path, state.IsEnvironmentOverridden, want)
	}
	if want, exists := expected["source_revision"]; exists && state.SourceRevision != want {
		t.Fatalf("%s source revision = %s, want %v", path, state.SourceRevision, want)
	}
	for name, got := range map[string]any{"pack_default": state.PackDefault, "baseline": state.Baseline, "draft_baseline": state.DraftBaseline, "environment_override": state.EnvironmentOverride, "draft_environment_override": state.DraftEnvironmentOverride, "effective": state.Effective} {
		if want, exists := expected[name]; exists && !jsonEqual(got, want) {
			t.Fatalf("%s %s = %#v, want %#v", path, name, got, want)
		}
	}
}

func assertDetails(t *testing.T, expected map[string]any, details []StructuralError) {
	t.Helper()
	if order, exists := expected["detail_order"].([]any); exists {
		if len(order) != len(details) {
			t.Fatalf("details = %#v, want %d", details, len(order))
		}
		for index, raw := range order {
			assertDetail(t, details[index], raw.(map[string]any))
		}
		return
	}
	for _, raw := range expected["detail_subset"].([]any) {
		if !slices.ContainsFunc(details, func(detail StructuralError) bool { return detailMatches(detail, raw.(map[string]any)) }) {
			t.Fatalf("details %#v missing %#v", details, raw)
		}
	}
}

func assertDetail(t *testing.T, detail StructuralError, expected map[string]any) {
	t.Helper()
	if !detailMatches(detail, expected) {
		t.Fatalf("detail = %#v, want %#v", detail, expected)
	}
	if detail.Message == "" {
		t.Fatal("detail message is empty")
	}
}
func detailMatches(detail StructuralError, expected map[string]any) bool {
	return detail.Code == expected["code"] && detail.Path == expected["path"] && detail.Scope == expected["scope"]
}
func fieldState(view View, path string) *FieldState {
	for index := range view.FieldStates {
		if view.FieldStates[index].Path == path {
			return &view.FieldStates[index]
		}
	}
	return nil
}

func containsJSON(value, subset any) bool {
	object, objectOK := value.(map[string]any)
	expected, expectedOK := subset.(map[string]any)
	if objectOK && expectedOK {
		for key, expectedValue := range expected {
			actual, exists := object[key]
			if !exists || !containsJSON(actual, expectedValue) {
				return false
			}
		}
		return true
	}
	return jsonEqual(value, subset)
}

func typeAt(defaults map[string]any, path string) string {
	value := valueAt(defaults, path).Value
	switch value.(type) {
	case bool:
		return "boolean"
	case string:
		return "string"
	case float64:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "string"
	}
}

func loadLayeringFixture(t *testing.T) layeringFixture {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "contracts", "drafts", "v1", "layering.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture layeringFixture
	if err := json.Unmarshal(content, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}
