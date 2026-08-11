package validation

import (
	"testing"

	"github.com/ConteMan/conflow/internal/packs"
)

func TestValidatePackRulesRejectsInvalidV2EntityID(t *testing.T) {
	configuration := v2ValidationFixture(t)
	configuration["placements"].([]any)[0].(map[string]any)["id"] = "Illegal-ID!!"
	diagnostics := Validate(v2ValidationInput(t, configuration))
	requirePackDiagnostic(t, diagnostics, Diagnostic{
		Code:      "entity_id_invalid",
		Path:      "/placements/Illegal-ID!!",
		Severity:  SeverityError,
		EntityRef: "entity:mobile-ad-monetization/v2:placement:Illegal-ID!!",
	})
	if readiness := ReadinessFor(diagnostics); readiness != ReadinessBlocked {
		t.Fatalf("readiness = %q, want blocked", readiness)
	}
}

func TestValidatePackRulesRejectsInvalidV2Enum(t *testing.T) {
	configuration := v2ValidationFixture(t)
	v2RecordFields(t, configuration, "placements", "interstitial_main")["ad_type"] = "banner_not_in_enum"
	diagnostics := Validate(v2ValidationInput(t, configuration))
	requirePackDiagnostic(t, diagnostics, Diagnostic{
		Code:      "field_value_not_allowed",
		Path:      "/placements/interstitial_main/ad_type",
		Severity:  SeverityError,
		EntityRef: "entity:mobile-ad-monetization/v2:placement:interstitial_main",
	})
	if readiness := ReadinessFor(diagnostics); readiness != ReadinessBlocked {
		t.Fatalf("readiness = %q, want blocked", readiness)
	}
}

func TestValidatePackRulesRejectsInvalidAdStrategyID(t *testing.T) {
	configuration := v2ValidationFixture(t)
	addV2ValidationStrategy(configuration)
	invalidID := "Illegal-Strategy!!"
	configuration["ad_strategies"].([]any)[0].(map[string]any)["id"] = invalidID
	v2RecordFields(t, configuration, "ad_strategy_settings", "default")["default_strategy_id"] = invalidID
	diagnostics := Validate(v2ValidationInput(t, configuration))
	requirePackDiagnostic(t, diagnostics, Diagnostic{
		Code:      "entity_id_invalid",
		Path:      "/ad_strategies/" + invalidID,
		Severity:  SeverityError,
		EntityRef: "entity:mobile-ad-monetization/v2:ad_strategy:" + invalidID,
	})
	if readiness := ReadinessFor(diagnostics); readiness != ReadinessBlocked {
		t.Fatalf("readiness = %q, want blocked", readiness)
	}
}

func TestValidatePackRulesApplyToV1(t *testing.T) {
	effective := map[string]any{"feature_switches": []any{map[string]any{
		"id":     "valid_switch",
		"fields": map[string]any{"key": "valid_switch", "default_value": true, "risk_level": "critical", "rollback_method": "disable"},
	}}}
	diagnostics := Validate(Input{
		PackRef: "mobile-ad-monetization/v1", Definition: testPackDefinition(t, "mobile-ad-monetization/v1"),
		EnvironmentID: "development", EnvironmentKind: "development", Effective: effective,
	})
	requirePackDiagnostic(t, diagnostics, Diagnostic{
		Code:      "field_value_not_allowed",
		Path:      "/feature_switches/valid_switch/risk_level",
		Severity:  SeverityError,
		EntityRef: "entity:mobile-ad-monetization/v1:feature_switch:valid_switch",
	})
	if readiness := ReadinessFor(diagnostics); readiness != ReadinessBlocked {
		t.Fatalf("readiness = %q, want blocked", readiness)
	}
}

func testPackDefinition(t *testing.T, packRef string) packs.Definition {
	t.Helper()
	definition, _, err := packs.BuiltinRegistry().Resolve(packRef)
	if err != nil {
		t.Fatal(err)
	}
	return definition
}

func requirePackDiagnostic(t *testing.T, diagnostics []Diagnostic, want Diagnostic) {
	t.Helper()
	for _, item := range diagnostics {
		if item.Code == want.Code && item.Path == want.Path && item.Severity == want.Severity && item.EntityRef == want.EntityRef {
			return
		}
	}
	t.Fatalf("missing diagnostic %#v in %#v", want, diagnostics)
}
