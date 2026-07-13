package validation

import (
	"fmt"
	"sort"
	"strings"
)

func validateV2(input Input) []Diagnostic {
	layouts := records(input.Effective, "remote_config_layouts")
	networkSettings := records(input.Effective, "network_settings")
	switches := records(input.Effective, "feature_switches")
	policies := records(input.Effective, "frequency_policies")
	placements := records(input.Effective, "placements")
	bindings := records(input.Effective, "unit_bindings")
	diagnostics := []Diagnostic{}

	layout, layoutOK := v2Singleton(layouts)
	if !layoutOK {
		diagnostics = append(diagnostics, diagnostic("remote_config_layout_not_singleton", "/remote_config_layouts/default/id", SeverityBlocking, entityRef(input.PackRef, "remote_config_layout", "default"), "远端配置布局必须且只能有一个 default 实体", "保留一个 ID 为 default 的远端配置布局。"))
	}
	network, networkOK := v2Singleton(networkSettings)
	if !networkOK {
		diagnostics = append(diagnostics, diagnostic("network_settings_not_singleton", "/network_settings/default/id", SeverityBlocking, entityRef(input.PackRef, "network_settings", "default"), "网络设置必须且只能有一个 default 实体", "保留一个 ID 为 default 的网络设置。"))
	}

	parameterKeys := map[string]string{}
	if layoutOK {
		for _, field := range []string{"active_network_parameter_key", "frequency_policies_parameter_key", "placements_parameter_key"} {
			value, ok := layout.Fields[field].(string)
			if !ok || strings.TrimSpace(value) == "" {
				diagnostics = append(diagnostics, v2Diagnostic(input, "layout_key_empty", "remote_config_layout", "remote_config_layouts", layout.ID, field, SeverityError, "远端参数键不能为空", "填写一个非空的远端参数键。"))
				continue
			}
			diagnostics = v2AddParameterKeyDiagnostic(input, diagnostics, parameterKeys, value, "remote_config_layout", "remote_config_layouts", layout.ID, field)
		}
		if value, ok := layout.Fields["mediation_strategy_parameter_key"].(string); ok && strings.TrimSpace(value) != "" {
			diagnostics = v2AddParameterKeyDiagnostic(input, diagnostics, parameterKeys, value, "remote_config_layout", "remote_config_layouts", layout.ID, "mediation_strategy_parameter_key")
		}
	}

	switchIDs := ids(switches)
	for _, featureSwitch := range switches {
		key, ok := featureSwitch.Fields["key"].(string)
		if !ok || strings.TrimSpace(key) == "" {
			diagnostics = append(diagnostics, v2Diagnostic(input, "feature_switch_key_empty", "feature_switch", "feature_switches", featureSwitch.ID, "key", SeverityError, "功能开关键不能为空", "填写稳定的功能开关键。"))
			continue
		}
		if previous, exists := parameterKeys[key]; exists {
			diagnostics = append(diagnostics, v2Diagnostic(input, "parameter_key_conflict", "feature_switch", "feature_switches", featureSwitch.ID, "key", SeverityBlocking, fmt.Sprintf("远端参数键与 %s 冲突", previous), "为冲突的参数选择不同的键。"))
		} else {
			parameterKeys[key] = "/feature_switches/" + featureSwitch.ID + "/key"
		}
	}
	seenSwitchKeys := map[string]bool{}
	for _, featureSwitch := range switches {
		key, _ := featureSwitch.Fields["key"].(string)
		if strings.TrimSpace(key) == "" {
			continue
		}
		if seenSwitchKeys[key] {
			diagnostics = append(diagnostics, v2Diagnostic(input, "feature_switch_key_duplicate", "feature_switch", "feature_switches", featureSwitch.ID, "key", SeverityError, "功能开关键在项目内重复", "为功能开关选择唯一的键。"))
			continue
		}
		seenSwitchKeys[key] = true
	}

	policyIDs := ids(policies)
	for _, policy := range policies {
		for _, field := range []string{"cooldown", "interval", "max_count", "shift_count"} {
			value := policy.Fields[field]
			if value == nil {
				continue
			}
			valid := false
			code, message := "", ""
			switch field {
			case "cooldown":
				valid, code, message = validDuration(value), "duration_invalid", "冷却时间必须是合法 Duration"
			case "interval":
				valid, code, message = validInterval(value), "interval_invalid", "展示间隔必须是合法 Interval"
			case "max_count":
				valid, code, message = validCountLimit(value), "count_limit_invalid", "次数上限必须是合法 CountLimit"
			case "shift_count":
				valid, code, message = validShiftLimit(value), "shift_limit_invalid", "分时上限必须是合法 ShiftLimit"
			}
			if !valid {
				diagnostics = append(diagnostics, v2Diagnostic(input, code, "frequency_policy", "frequency_policies", policy.ID, field, SeverityError, message, "按字段类型填写合法单位和整数值。"))
			}
		}
	}

	placementIDs := ids(placements)
	clientIDs := map[string]bool{}
	for _, placement := range placements {
		pathCollection := "placements"
		ref := entityRef(input.PackRef, "placement", placement.ID)
		if switchID, ok := placement.Fields["enabled_switch_id"].(string); !ok || !switchIDs[switchID] {
			diagnostics = append(diagnostics, diagnostic("reference_not_found", "/"+pathCollection+"/"+placement.ID+"/enabled_switch_id", SeverityError, ref, "广告位引用的功能开关不存在", "选择一个存在的功能开关。"))
		}
		frequencyType, typeOK := placement.Fields["frequency_policy_type"].(string)
		if !typeOK || (frequencyType != "preset" && frequencyType != "custom") {
			diagnostics = append(diagnostics, diagnostic("freq_type_invalid", "/"+pathCollection+"/"+placement.ID+"/frequency_policy_type", SeverityError, ref, "频控类型必须是 preset 或 custom", "选择 preset 或 custom。"))
		} else if frequencyType == "preset" {
			policyID, ok := placement.Fields["frequency_policy_id"].(string)
			if !ok || strings.TrimSpace(policyID) == "" || !policyIDs[policyID] || placement.Fields["custom_frequency_policy"] != nil {
				diagnostics = append(diagnostics, diagnostic("preset_custom_exclusive", "/"+pathCollection+"/"+placement.ID+"/frequency_policy_id", SeverityError, ref, "预设频控必须且只能引用一个频控策略", "设置 frequency_policy_id 并清空 custom_frequency_policy。"))
			}
		} else {
			if placement.Fields["custom_frequency_policy"] == nil || placement.Fields["frequency_policy_id"] != nil {
				diagnostics = append(diagnostics, diagnostic("preset_custom_exclusive", "/"+pathCollection+"/"+placement.ID+"/custom_frequency_policy", SeverityError, ref, "自定义频控必须且只能设置 custom_frequency_policy", "设置 custom_frequency_policy 并清空 frequency_policy_id。"))
			}
		}
		if cacheTTL := placement.Fields["cache_ttl"]; cacheTTL != nil {
			if !validDuration(cacheTTL) {
				diagnostics = append(diagnostics, diagnostic("duration_invalid", "/"+pathCollection+"/"+placement.ID+"/cache_ttl", SeverityError, ref, "缓存有效期必须是合法 Duration", "按 Duration 格式填写缓存有效期。"))
			}
		}
		if timeout, ok := integer(placement.Fields["load_timeout_ms"]); !ok || timeout < 1 {
			diagnostics = append(diagnostics, diagnostic("load_timeout_out_of_range", "/"+pathCollection+"/"+placement.ID+"/load_timeout_ms", SeverityError, ref, "广告加载超时必须大于等于 1 毫秒", "填写大于等于 1 的整数毫秒值。"))
		}
		clientID, ok := placement.Fields["client_id"].(string)
		if !ok || strings.TrimSpace(clientID) == "" {
			diagnostics = append(diagnostics, diagnostic("client_id_empty", "/"+pathCollection+"/"+placement.ID+"/client_id", SeverityError, ref, "客户端 ID 不能为空", "填写稳定的客户端 ID。"))
		} else if clientIDs[clientID] {
			diagnostics = append(diagnostics, diagnostic("client_id_duplicate", "/"+pathCollection+"/"+placement.ID+"/client_id", SeverityError, ref, "客户端 ID 在项目内重复", "为广告位选择唯一的客户端 ID。"))
		} else {
			clientIDs[clientID] = true
		}
	}

	bindingByKey := map[string]record{}
	for _, binding := range bindings {
		ref := entityRef(input.PackRef, "unit_binding", binding.ID)
		path := "/unit_bindings/" + binding.ID
		placementID, placementOK := binding.Fields["placement_id"].(string)
		if !placementOK || !placementIDs[placementID] {
			diagnostics = append(diagnostics, diagnostic("reference_not_found", path+"/placement_id", SeverityError, ref, "广告单元绑定引用的广告位不存在", "选择一个存在的广告位。"))
		}
		if binding.Fields["environment_id"] != input.EnvironmentID {
			diagnostics = append(diagnostics, diagnostic("unit_binding_environment_mismatch", path+"/environment_id", SeverityError, ref, "广告单元绑定属于其他环境", "将绑定写入与其 environment_id 一致的环境覆盖。"))
		}
		key := v2BindingKey(binding)
		if _, exists := bindingByKey[key]; exists {
			diagnostics = append(diagnostics, diagnostic("unit_binding_composite_key_duplicate", path+"/network", SeverityBlocking, ref, "广告单元绑定复合键在项目内重复", "为绑定选择唯一的 placement、environment、platform 和 network 组合。"))
		} else {
			bindingByKey[key] = binding
		}
		status, _ := binding.Fields["status"].(string)
		if status == "configured" && binding.Fields["unit_id_ref"] == nil {
			diagnostics = append(diagnostics, diagnostic("unit_binding_ref_missing", path+"/unit_id_ref", SeverityError, ref, "configured 绑定必须提供 unit_id_ref", "填写 unit_id_ref，或将状态改为 missing。"))
		}
		if status == "missing" && binding.Fields["unit_id_ref"] != nil {
			diagnostics = append(diagnostics, diagnostic("unit_binding_ref_unexpected", path+"/unit_id_ref", SeverityError, ref, "missing 绑定的 unit_id_ref 必须为 null", "清空 unit_id_ref，或将状态改为 configured。"))
		}
	}

	if input.EnvironmentKind == "production" && networkOK {
		platforms := v2StringSlice(network.Fields["platforms"])
		activeNetwork, _ := network.Fields["active_network"].(string)
		missing := []v2RequiredBinding{}
		for _, placement := range placements {
			for _, platform := range platforms {
				key := v2BindingTuple(placement.ID, input.EnvironmentID, platform, activeNetwork)
				binding, exists := bindingByKey[key]
				if !exists || binding.Fields["status"] != "configured" || binding.Fields["unit_id_ref"] == nil {
					missing = append(missing, v2RequiredBinding{placementID: placement.ID, platform: platform, network: activeNetwork, binding: binding, exists: exists})
				}
			}
		}
		for _, missingBinding := range missing {
			id := "missing_" + input.EnvironmentID + "_" + missingBinding.platform + "_" + missingBinding.placementID
			ref := ""
			if missingBinding.exists {
				id = missingBinding.binding.ID
				ref = entityRef(input.PackRef, "unit_binding", id)
			}
			diagnostics = append(diagnostics, diagnostic("production_unit_binding_missing", "/unit_bindings/"+id+"/unit_id_ref", SeverityWarning, ref, "Production 广告位缺少所需的广告单元绑定", "为当前网络和平台配置一个 configured unit binding。"))
		}
		if len(missing) > 0 {
			diagnostics = append(diagnostics, diagnostic("production_enabled_placements_unbound", "/unit_bindings/production/status", SeverityBlocking, "", "Production 存在未完成的广告单元绑定", "为每个广告位、平台和当前网络配置 unit binding。"))
		}
	}

	SortDiagnostics(diagnostics)
	return diagnostics
}

func v2Singleton(values []record) (record, bool) {
	if len(values) != 1 || values[0].ID != "default" {
		return record{}, false
	}
	return values[0], true
}

func v2Diagnostic(input Input, code, entityType, collection, entityID, field, severity, message, suggestion string) Diagnostic {
	return diagnostic(code, "/"+collection+"/"+entityID+"/"+field, severity, entityRef(input.PackRef, entityType, entityID), message, suggestion)
}

func v2AddParameterKeyDiagnostic(input Input, diagnostics []Diagnostic, keys map[string]string, key, entityType, collection, entityID, field string) []Diagnostic {
	if previous, exists := keys[key]; exists {
		return append(diagnostics, v2Diagnostic(input, "parameter_key_conflict", entityType, collection, entityID, field, SeverityBlocking, fmt.Sprintf("远端参数键与 %s 冲突", previous), "为冲突的参数选择不同的键。"))
	}
	keys[key] = "/" + collection + "/" + entityID + "/" + field
	return diagnostics
}

func validDuration(value any) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	_, unitOK := object["unit"].(string)
	return unitOK && allowedString(object["unit"], "seconds", "minutes", "hours", "days") && positiveInteger(object["value"])
}

func validInterval(value any) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	return allowedString(object["unit"], "seconds", "minutes", "hours", "days", "items") && positiveInteger(object["value"])
}

func validCountLimit(value any) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	return allowedString(object["unit"], "session", "day") && nonNegativeInteger(object["value"])
}

func validShiftLimit(value any) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	return nonNegativeInteger(object["am"]) && nonNegativeInteger(object["pm"])
}

func positiveInteger(value any) bool {
	integer, ok := integer(value)
	return ok && integer > 0
}

func nonNegativeInteger(value any) bool {
	integer, ok := integer(value)
	return ok && integer >= 0
}

func v2BindingKey(binding record) string {
	placementID, _ := binding.Fields["placement_id"].(string)
	environmentID, _ := binding.Fields["environment_id"].(string)
	platform, _ := binding.Fields["platform"].(string)
	network, _ := binding.Fields["network"].(string)
	return v2BindingTuple(placementID, environmentID, platform, network)
}

func v2BindingTuple(placementID, environmentID, platform, network string) string {
	return strings.Join([]string{placementID, environmentID, platform, network}, "\x00")
}

func v2StringSlice(value any) []string {
	values := []string{}
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				values = append(values, text)
			}
		}
	case []string:
		values = append(values, typed...)
	}
	sort.Strings(values)
	return values
}

type v2RequiredBinding struct {
	placementID string
	platform    string
	network     string
	binding     record
	exists      bool
}
