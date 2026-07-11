package validation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type overlayFixture struct {
	Entities struct {
		Placements        []map[string]any `json:"placements"`
		FrequencyPolicies []map[string]any `json:"frequency_policies"`
		FeatureSwitches   []map[string]any `json:"feature_switches"`
	} `json:"entities"`
	UnitBindingMatrix struct {
		Rows []struct {
			PlacementID string                     `json:"placement_id"`
			Production  map[string]json.RawMessage `json:"production"`
		} `json:"rows"`
	} `json:"unit_binding_matrix"`
}

type overlaysFixture struct {
	SeverityContract map[string]struct {
		CLIExitCode int `json:"cli_exit_code"`
	} `json:"severity_contract"`
	Scenarios []struct {
		ID            string `json:"id"`
		EnvironmentID string `json:"environment_id"`
		Overlay       struct {
			EntityReplacements []struct {
				EntityType string         `json:"entity_type"`
				EntityID   string         `json:"entity_id"`
				Fields     map[string]any `json:"fields"`
			} `json:"entity_replacements"`
			DeleteAttempts []string `json:"delete_attempts"`
		} `json:"overlay"`
		Expected struct {
			Readiness   string `json:"readiness"`
			CLIExitCode int    `json:"cli_exit_code"`
			Diagnostics []struct {
				Code      string `json:"code"`
				Path      string `json:"path"`
				Severity  string `json:"severity"`
				EntityRef string `json:"entity_ref"`
			} `json:"diagnostics"`
		} `json:"expected"`
	} `json:"scenarios"`
}

func TestValidationOverlaysGolden(t *testing.T) {
	base, overlays := loadFixtures(t)
	var scenario *struct {
		ID            string `json:"id"`
		EnvironmentID string `json:"environment_id"`
		Overlay       struct {
			EntityReplacements []struct {
				EntityType string         `json:"entity_type"`
				EntityID   string         `json:"entity_id"`
				Fields     map[string]any `json:"fields"`
			} `json:"entity_replacements"`
			DeleteAttempts []string `json:"delete_attempts"`
		} `json:"overlay"`
		Expected struct {
			Readiness   string `json:"readiness"`
			CLIExitCode int    `json:"cli_exit_code"`
			Diagnostics []struct {
				Code      string `json:"code"`
				Path      string `json:"path"`
				Severity  string `json:"severity"`
				EntityRef string `json:"entity_ref"`
			} `json:"diagnostics"`
		} `json:"expected"`
	}
	for index := range overlays.Scenarios {
		if overlays.Scenarios[index].ID == "nine-human-fixture-diagnostics" {
			scenario = &overlays.Scenarios[index]
			break
		}
	}
	if scenario == nil {
		t.Fatal("nine-human-fixture-diagnostics scenario is missing")
	}

	effective := effectiveFixture(base, scenario.EnvironmentID)
	for _, replacement := range scenario.Overlay.EntityReplacements {
		replaceFixtureEntity(effective, replacement.EntityType, replacement.EntityID, replacement.Fields)
	}
	deletes := make([]RestrictedDelete, 0, len(scenario.Overlay.DeleteAttempts))
	for _, reference := range scenario.Overlay.DeleteAttempts {
		parts := strings.Split(reference, ":")
		if len(parts) != 4 {
			t.Fatalf("invalid delete attempt %q", reference)
		}
		deletes = append(deletes, RestrictedDelete{EntityType: parts[2], EntityID: parts[3]})
	}
	diagnostics := Validate(Input{PackRef: "mobile-ad-monetization/v1", EnvironmentID: scenario.EnvironmentID, EnvironmentKind: "production", Effective: effective, RestrictedDeletes: deletes})
	if got := ReadinessFor(diagnostics); got != scenario.Expected.Readiness {
		t.Fatalf("readiness = %q, want %q", got, scenario.Expected.Readiness)
	}
	if got := ExitCodeFor(diagnostics); got != scenario.Expected.CLIExitCode {
		t.Fatalf("exit code = %d, want %d", got, scenario.Expected.CLIExitCode)
	}
	if len(diagnostics) != len(scenario.Expected.Diagnostics) {
		t.Fatalf("diagnostic count = %d, want %d: %#v", len(diagnostics), len(scenario.Expected.Diagnostics), diagnostics)
	}
	for index, expected := range scenario.Expected.Diagnostics {
		got := diagnostics[index]
		if got.Code != expected.Code || got.Path != expected.Path || got.Severity != expected.Severity || got.EntityRef != expected.EntityRef {
			t.Fatalf("diagnostic %d = %#v, want %#v", index, got, expected)
		}
	}
	for severity, contract := range overlays.SeverityContract {
		if got := ExitCodeFor([]Diagnostic{{Severity: severity}}); got != contract.CLIExitCode {
			t.Fatalf("%s exit code = %d, want %d", severity, got, contract.CLIExitCode)
		}
	}
}

func TestResultStoreStaleAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".conflow", "validation-results.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	result := Result{EnvironmentID: "production", ValidatedDraftRevision: 12, Readiness: ReadinessReady, Diagnostics: []Diagnostic{}}
	if err := store.Save(result); err != nil {
		t.Fatal(err)
	}
	stale, err := store.Get("production", 13)
	if err != nil || stale.Status != StatusStale || stale.ValidatedDraftRevision != 12 {
		t.Fatalf("stale result = %#v, err = %v", stale, err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := reopened.Get("production", 12)
	if err != nil || fresh.Status != StatusFresh || fresh.Diagnostics == nil {
		t.Fatalf("fresh result = %#v, err = %v", fresh, err)
	}
}

func loadFixtures(t *testing.T) (overlayFixture, overlaysFixture) {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "contracts", "mobile-ad-monetization", "v1")
	baseContent, err := os.ReadFile(filepath.Join(root, "entities.json"))
	if err != nil {
		t.Fatal(err)
	}
	overlayContent, err := os.ReadFile(filepath.Join(root, "validation-overlays.json"))
	if err != nil {
		t.Fatal(err)
	}
	var base overlayFixture
	var overlays overlaysFixture
	if err := json.Unmarshal(baseContent, &base); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(overlayContent, &overlays); err != nil {
		t.Fatal(err)
	}
	return base, overlays
}

func effectiveFixture(base overlayFixture, environmentID string) map[string]any {
	effective := map[string]any{
		"placements":         recordsAsAny(base.Entities.Placements),
		"frequency_policies": recordsAsAny(base.Entities.FrequencyPolicies),
		"feature_switches":   recordsAsAny(base.Entities.FeatureSwitches),
		"unit_bindings":      []any{},
	}
	bindings := make([]any, 0, len(base.UnitBindingMatrix.Rows)*2)
	for _, row := range base.UnitBindingMatrix.Rows {
		for _, platform := range []string{"ios", "android"} {
			var unitID any
			_ = json.Unmarshal(row.Production[platform], &unitID)
			bindings = append(bindings, map[string]any{
				"id": "ub_" + environmentID + "_" + platform + "_" + row.PlacementID, "placement_id": row.PlacementID,
				"environment_id": environmentID, "platform": platform, "unit_id_ref": unitID, "status": "configured",
			})
		}
	}
	effective["unit_bindings"] = bindings
	return effective
}

func recordsAsAny(records []map[string]any) []any {
	result := make([]any, len(records))
	for index, record := range records {
		copy := make(map[string]any, len(record))
		for key, value := range record {
			copy[key] = value
		}
		result[index] = copy
	}
	return result
}

func replaceFixtureEntity(effective map[string]any, entityType, entityID string, fields map[string]any) {
	collections := map[string]string{"placement": "placements", "frequency_policy": "frequency_policies", "feature_switch": "feature_switches"}
	for _, value := range effective[collections[entityType]].([]any) {
		record := value.(map[string]any)
		if record["id"] != entityID {
			continue
		}
		for key, field := range fields {
			record[key] = field
		}
		return
	}
}
