package packs

import "encoding/json"

// BuiltinRegistry contains Pack declarations compiled into the Conflow binary.
func BuiltinRegistry() *Registry {
	return MustNewRegistry(mobileAdDefinition(), mobileAdV2Definition())
}

func mobileAdDefinition() Definition {
	minimumOne := 1.0
	minimumZero := 0.0
	return Definition{
		Metadata: Metadata{
			Name:         "mobile-ad-monetization",
			Version:      "v1",
			Description:  "Versioned contract for mobile advertising configuration.",
			Capabilities: []string{"entities", "environment_overrides"},
			EntityTypes: []EntityMetadata{
				entityMetadata("placement", "placements", "广告位", "定义稳定广告展示位置。", `^[a-z][a-z0-9_]{0,62}$`, DeletionPolicyRestrict, nil),
				entityMetadata("frequency_policy", "frequency_policies", "频控策略", "定义广告展示频率限制。", `^[a-z][a-z0-9_]{0,62}$`, DeletionPolicyRestrict, nil),
				entityMetadata("feature_switch", "feature_switches", "功能开关", "定义稳定的广告功能开关。", `^[a-z][a-z0-9_]{0,62}$`, DeletionPolicyRestrict, nil),
				entityMetadata("unit_binding", "unit_bindings", "广告单元绑定", "定义环境和平台对应的广告单元。", `^[a-z][a-z0-9_]{0,127}$`, DeletionPolicyRestrict, []string{"placement_id", "environment_id", "platform", "unit_id_ref", "status"}),
			},
		},
		Schema: Schema{
			Version: 1,
			Entities: []EntitySchema{
				{Name: "placement", Fields: []FieldSchema{
					field("key", FieldTypeString, true, false, `""`, "广告位键", "面向业务的稳定键。", "input", "基础", 0, FieldValidation{MinLength: intPointer(1)}),
					field("ad_type", FieldTypeString, true, false, `"interstitial"`, "广告类型", "支持开屏、插屏和原生广告。", "select", "基础", 1, enum("app_open", "interstitial", "native")),
					field("enabled", FieldTypeBoolean, true, false, "true", "启用", "是否允许展示该广告位。", "switch", "基础", 2, FieldValidation{}),
					field("network_mode", FieldTypeString, true, false, `"hybrid"`, "广告网络模式", "广告网络的请求策略。", "select", "投放", 3, enum("hybrid", "bidding", "waterfall")),
					field("frequency_policy_id", FieldTypeReference, true, false, `""`, "频控策略", "引用 frequency_policy 实体 ID。", "select", "投放", 4, FieldValidation{MinLength: intPointer(1)}),
					field("load_timeout_ms", FieldTypeInteger, true, false, "4000", "加载超时（毫秒）", "加载广告的最长等待时间。", "number", "投放", 5, FieldValidation{Minimum: &minimumOne}),
					field("cache_policy", FieldTypeString, true, false, `"memory"`, "缓存策略", "广告素材缓存位置。", "select", "投放", 6, enum("memory", "disk", "none")),
					field("fallback_behavior", FieldTypeString, true, false, `"continue"`, "兜底行为", "广告不可用时继续的业务动作。", "input", "投放", 7, FieldValidation{MinLength: intPointer(1)}),
				}},
				{Name: "frequency_policy", Fields: []FieldSchema{
					field("cooldown_ms", FieldTypeInteger, true, false, "0", "冷却时间（毫秒）", "两次展示之间的最短间隔。", "number", "频控", 0, FieldValidation{Minimum: &minimumZero}),
					field("interval_ms", FieldTypeInteger, true, false, "60000", "统计窗口（毫秒）", "频次限制的统计窗口。", "number", "频控", 1, FieldValidation{Minimum: &minimumOne}),
					field("max_count", FieldTypeInteger, true, false, "1", "窗口最大次数", "统计窗口内允许展示的最大次数。", "number", "频控", 2, FieldValidation{Minimum: &minimumOne}),
					field("shift_count", FieldTypeInteger, true, false, "0", "偏移次数", "达到阈值前的偏移次数。", "number", "频控", 3, FieldValidation{Minimum: &minimumZero}),
					field("positions", FieldTypeArray, true, false, "[]", "适用位置", "适用该策略的业务位置。", "tags", "频控", 4, FieldValidation{}),
				}},
				{Name: "feature_switch", Fields: []FieldSchema{
					field("key", FieldTypeString, true, false, `""`, "开关键", "稳定且可读的配置键。", "input", "基础", 0, FieldValidation{MinLength: intPointer(1)}),
					field("default_value", FieldTypeBoolean, true, false, "false", "默认值", "未覆盖时使用的开关值。", "switch", "基础", 1, FieldValidation{}),
					field("risk_level", FieldTypeString, true, false, `"medium"`, "风险等级", "变更该开关的业务风险。", "select", "治理", 2, enum("low", "medium", "high")),
					field("rollback_method", FieldTypeString, true, false, `"disable"`, "回滚方式", "发生风险时的标准回滚操作。", "input", "治理", 3, FieldValidation{MinLength: intPointer(1)}),
				}},
				{Name: "unit_binding", Fields: []FieldSchema{
					field("placement_id", FieldTypeReference, true, false, `""`, "广告位", "引用 placement 实体 ID。", "select", "绑定", 0, FieldValidation{MinLength: intPointer(1)}),
					field("environment_id", FieldTypeString, true, false, `""`, "环境", "绑定所属的项目环境。", "input", "绑定", 1, FieldValidation{MinLength: intPointer(1)}),
					field("platform", FieldTypeString, true, false, `"ios"`, "平台", "移动端目标平台。", "select", "绑定", 2, enum("ios", "android")),
					field("unit_id_ref", FieldTypeString, true, true, "null", "广告单元引用", "广告网络单元 ID 或其安全引用。", "input", "绑定", 3, FieldValidation{}),
					field("status", FieldTypeString, true, false, `"configured"`, "配置状态", "单元 ID 是否已经配置。", "select", "绑定", 4, enum("configured", "missing")),
				}},
			},
			Migrations: []SchemaMigration{},
		},
	}
}

func entityMetadata(name, collection, label, description, pattern string, deletion DeletionPolicy, overrides []string) EntityMetadata {
	return EntityMetadata{Name: name, Collection: collection, Label: label, Description: description, IDRule: IDRule{Pattern: pattern, MinLength: 1, MaxLength: 128}, DeletionPolicy: deletion, EnvironmentOverrideFields: overrides}
}

func field(name string, fieldType FieldType, required, nullable bool, defaultValue, label, description, control, group string, order int, validation FieldValidation) FieldSchema {
	return FieldSchema{Name: name, Type: fieldType, Required: required, Nullable: nullable, Default: json.RawMessage(defaultValue), Sensitivity: SensitivityPublic, UI: FieldUI{Label: label, Description: description, Control: control, Group: group, Order: order}, Validation: validation}
}

func enum(values ...string) FieldValidation {
	result := FieldValidation{Enum: make([]json.RawMessage, len(values))}
	for index, value := range values {
		encoded, _ := json.Marshal(value)
		result.Enum[index] = encoded
	}
	return result
}

func intPointer(value int) *int { return &value }
