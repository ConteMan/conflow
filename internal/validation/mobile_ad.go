package validation

import (
	"fmt"
	"sort"

	"github.com/ConteMan/conflow/internal/entities"
)

// Input is the captured DraftView data needed for a complete validation run.
// RestrictedDeletes is supplied by callers that validate a pending restricted
// deletion; ordinary full validation leaves it empty.
type Input struct {
	PackRef           string
	EnvironmentID     string
	EnvironmentKind   string
	Effective         map[string]any
	RestrictedDeletes []RestrictedDelete
}

type RestrictedDelete struct {
	EntityType string
	EntityID   string
}

// Validate validates the currently supported mobile advertising Pack. Other
// Pack versions have no domain rules until their Pack-specific validator exists.
func Validate(input Input) []Diagnostic {
	if input.PackRef != "mobile-ad-monetization/v1" {
		return []Diagnostic{}
	}
	placements := records(input.Effective, "placements")
	policies := records(input.Effective, "frequency_policies")
	switches := records(input.Effective, "feature_switches")
	bindings := records(input.Effective, "unit_bindings")
	policyIDs := ids(policies)
	placementIDs := ids(placements)
	diagnostics := make([]Diagnostic, 0)

	for _, placement := range placements {
		ref := entityRef(input.PackRef, "placement", placement.ID)
		path := "/placements/" + placement.ID
		if timeout, ok := integer(placement.Fields["load_timeout_ms"]); !ok || timeout < 1000 || timeout > 60000 {
			diagnostics = append(diagnostics, diagnostic("load_timeout_out_of_range", path+"/load_timeout_ms", SeverityError, ref, "广告加载超时必须在 1000 至 60000 毫秒之间", "将加载超时调整到 1000 至 60000 毫秒之间。"))
		}
		if !allowedString(placement.Fields["ad_type"], "app_open", "interstitial", "native") {
			diagnostics = append(diagnostics, diagnostic("ad_type_not_allowed", path+"/ad_type", SeverityError, ref, "广告类型不受当前配置包支持", "选择 app_open、interstitial 或 native。"))
		}
		if !allowedString(placement.Fields["network_mode"], "hybrid", "bidding", "waterfall") {
			diagnostics = append(diagnostics, diagnostic("network_mode_not_allowed", path+"/network_mode", SeverityError, ref, "广告网络模式不受当前配置包支持", "选择 hybrid、bidding 或 waterfall。"))
		}
		policyID, ok := placement.Fields["frequency_policy_id"].(string)
		if !ok || !policyIDs[policyID] {
			diagnostics = append(diagnostics, diagnostic("reference_not_found", path+"/frequency_policy_id", SeverityError, ref, "广告位引用的频控策略不存在", "选择一个存在的频控策略，或先创建该策略。"))
		}
	}

	for _, featureSwitch := range switches {
		if _, ok := featureSwitch.Fields["default_value"].(bool); !ok {
			ref := entityRef(input.PackRef, "feature_switch", featureSwitch.ID)
			diagnostics = append(diagnostics, diagnostic("feature_switch_default_not_boolean", "/feature_switches/"+featureSwitch.ID+"/default_value", SeverityError, ref, "功能开关默认值必须是布尔值", "将默认值设置为 true 或 false。"))
		}
	}

	policyUsage := make(map[string]int, len(policies))
	for _, placement := range placements {
		if policyID, ok := placement.Fields["frequency_policy_id"].(string); ok && policyIDs[policyID] {
			policyUsage[policyID]++
		}
	}
	for _, policy := range policies {
		if policyUsage[policy.ID] == 0 {
			ref := entityRef(input.PackRef, "frequency_policy", policy.ID)
			diagnostics = append(diagnostics, diagnostic("frequency_policy_unused", "/frequency_policies/"+policy.ID, SeverityWarning, ref, "频控策略未被任何广告位使用", "关联至少一个广告位，或删除不再需要的频控策略。"))
		}
	}

	for _, deletion := range input.RestrictedDeletes {
		if deletion.EntityType != "frequency_policy" {
			continue
		}
		for _, placement := range placements {
			if placement.Fields["frequency_policy_id"] == deletion.EntityID {
				ref := entityRef(input.PackRef, "frequency_policy", deletion.EntityID)
				diagnostics = append(diagnostics, diagnostic("frequency_policy_still_referenced", "/frequency_policies/"+deletion.EntityID, SeverityError, ref, "频控策略仍被广告位引用，不能删除", "先迁移所有引用该策略的广告位，再删除策略。"))
				break
			}
		}
	}

	for _, binding := range bindings {
		ref := entityRef(input.PackRef, "unit_binding", binding.ID)
		path := "/unit_bindings/" + binding.ID
		placementID, ok := binding.Fields["placement_id"].(string)
		if !ok || !placementIDs[placementID] {
			diagnostics = append(diagnostics, diagnostic("reference_not_found", path+"/placement_id", SeverityError, ref, "广告单元绑定引用的广告位不存在", "选择一个存在的广告位。"))
		}
		if binding.Fields["environment_id"] != input.EnvironmentID {
			diagnostics = append(diagnostics, diagnostic("unit_binding_environment_mismatch", path+"/environment_id", SeverityError, ref, "广告单元绑定属于其他环境", "将绑定写入与其 environment_id 一致的环境覆盖。"))
		}
	}

	if input.EnvironmentKind == "production" {
		missing := productionMissingBindings(input, placements, bindings)
		for _, binding := range missing {
			// A placement-level production warning avoids duplicate iOS/Android
			// messages while the aggregate blocker still covers both platforms.
			if binding.platform != "ios" {
				continue
			}
			ref := entityRef(input.PackRef, "unit_binding", binding.id)
			diagnostics = append(diagnostics, diagnostic("production_unit_binding_missing", "/unit_bindings/"+binding.id, SeverityWarning, ref, "Production 广告位缺少 iOS unit ID", "配置 Production iOS unit ID，或停用该广告位。"))
		}
		if len(missing) > 0 {
			diagnostics = append(diagnostics, diagnostic("production_enabled_placements_unbound", "/unit_bindings", SeverityBlocking, "", "Production 存在已启用但未完成 unit binding 的广告位", "为所有已启用广告位配置 Production iOS 和 Android unit ID，或停用这些广告位。"))
		}
	}

	SortDiagnostics(diagnostics)
	return diagnostics
}

type record = entities.Record

func records(configuration map[string]any, collection string) []record {
	result := entities.Records(configuration, collection)
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func ids(values []record) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value.ID] = true
	}
	return result
}

func integer(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), typed == float64(int64(typed))
	default:
		return 0, false
	}
}

func allowedString(value any, allowed ...string) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	for _, candidate := range allowed {
		if text == candidate {
			return true
		}
	}
	return false
}

func diagnostic(code, path, severity, entityRef, message, suggestion string) Diagnostic {
	return Diagnostic{Code: code, Path: path, Severity: severity, EntityRef: entityRef, Message: message, FixSuggestion: suggestion}
}

func entityRef(packRef, entityType, id string) string {
	return fmt.Sprintf("entity:%s:%s:%s", packRef, entityType, id)
}

type missingBinding struct {
	id       string
	platform string
}

func productionMissingBindings(input Input, placements []record, bindings []record) []missingBinding {
	bindingByPlacementPlatform := make(map[string]record, len(bindings))
	for _, binding := range bindings {
		placementID, _ := binding.Fields["placement_id"].(string)
		platform, _ := binding.Fields["platform"].(string)
		bindingByPlacementPlatform[placementID+"\x00"+platform] = binding
	}
	missing := make([]missingBinding, 0)
	for _, placement := range placements {
		if enabled, _ := placement.Fields["enabled"].(bool); !enabled {
			continue
		}
		for _, platform := range []string{"ios", "android"} {
			binding, found := bindingByPlacementPlatform[placement.ID+"\x00"+platform]
			if !found {
				missing = append(missing, missingBinding{id: "ub_" + input.EnvironmentID + "_" + platform + "_" + placement.ID, platform: platform})
				continue
			}
			unitID, configured := binding.Fields["unit_id_ref"].(string)
			if !configured || unitID == "" || binding.Fields["status"] == "missing" {
				missing = append(missing, missingBinding{id: binding.ID, platform: platform})
			}
		}
	}
	return missing
}
