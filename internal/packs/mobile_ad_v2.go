package packs

func mobileAdV2Definition() Definition {
	minimumOne := 1.0
	defaultEntity := func(name, collection, label, description string, overrides []string) EntityMetadata {
		metadata := entityMetadata(name, collection, label, description, `^default$`, DeletionPolicyRestrict, overrides)
		metadata.IDRule.MinLength = 7
		metadata.IDRule.MaxLength = 7
		return metadata
	}

	return Definition{
		Metadata: Metadata{
			Name:         "mobile-ad-monetization",
			Version:      "v2",
			Description:  "Versioned contract for mobile advertising configuration with structured frequency controls.",
			Capabilities: []string{"entities", "environment_overrides"},
			EntityTypes: []EntityMetadata{
				defaultEntity("remote_config_layout", "remote_config_layouts", "远端配置布局", "定义广告聚合参数布局。", nil),
				entityMetadata("feature_switch", "feature_switches", "功能开关", "定义稳定的广告功能开关。", `^[a-z][a-z0-9_]{0,62}$`, DeletionPolicyRestrict, nil),
				defaultEntity("network_settings", "network_settings", "网络设置", "定义当前广告网络和聚合策略。", []string{"active_network", "mediation_strategy"}),
				entityMetadata("frequency_policy", "frequency_policies", "频控策略", "定义结构化广告展示频率限制。", `^[a-z][a-z0-9_]{0,62}$`, DeletionPolicyRestrict, nil),
				entityMetadata("placement", "placements", "广告位", "定义稳定广告展示位置。", `^[a-z][a-z0-9_]{0,62}$`, DeletionPolicyRestrict, nil),
				entityMetadata("unit_binding", "unit_bindings", "广告单元绑定", "定义环境、平台和网络对应的广告单元。", `^[a-z][a-z0-9_]{0,127}$`, DeletionPolicyRestrict, []string{"placement_id", "environment_id", "platform", "network", "unit_id_ref", "status"}),
			},
		},
		Schema: Schema{
			Version: 2,
			Entities: []EntitySchema{
				{Name: "remote_config_layout", Fields: []FieldSchema{
					field("active_network_parameter_key", FieldTypeString, true, false, `""`, "当前网络参数键", "当前网络独立参数的键。", "input", "布局", 0, FieldValidation{}),
					field("mediation_strategy_parameter_key", FieldTypeString, true, true, "null", "聚合策略参数键", "聚合策略参数的键。", "input", "布局", 1, FieldValidation{}),
					field("frequency_policies_parameter_key", FieldTypeString, true, false, `""`, "频控参数键", "频控策略聚合参数的键。", "input", "布局", 2, FieldValidation{}),
					field("placements_parameter_key", FieldTypeString, true, false, `""`, "广告位参数键", "广告位聚合参数的键。", "input", "布局", 3, FieldValidation{}),
					field("payload_version", FieldTypeInteger, true, false, "2", "负载版本", "聚合 JSON 的顶层版本。", "number", "布局", 4, FieldValidation{}),
				}},
				{Name: "feature_switch", Fields: []FieldSchema{
					field("key", FieldTypeString, true, false, `""`, "开关键", "稳定且可读的配置键。", "input", "基础", 0, FieldValidation{}),
					field("default_value", FieldTypeBoolean, true, false, "false", "默认值", "未覆盖时使用的开关值。", "switch", "基础", 1, FieldValidation{}),
					field("risk_level", FieldTypeString, true, false, `"medium"`, "风险等级", "变更该开关的业务风险。", "select", "治理", 2, enum("low", "medium", "high")),
					field("rollback_method", FieldTypeString, true, false, `""`, "回滚方式", "发生风险时的标准回滚操作。", "input", "治理", 3, FieldValidation{}),
				}},
				{Name: "network_settings", Fields: []FieldSchema{
					field("active_network", FieldTypeString, true, false, `""`, "当前网络", "当前生效网络或聚合平台的稳定 ID。", "input", "网络", 0, FieldValidation{}),
					field("mediation_strategy", FieldTypeString, true, true, "null", "聚合策略", "广告网络的请求策略。", "select", "网络", 1, enum("hybrid", "bidding", "waterfall")),
					field("platforms", FieldTypeArray, true, false, "[]", "平台", "发布就绪检查覆盖的平台集合。", "tags", "网络", 2, FieldValidation{}),
				}},
				{Name: "frequency_policy", Fields: []FieldSchema{
					field("cooldown", FieldTypeObject, true, true, "null", "冷却时间", "两次成功展示之间的最短时间。", "duration", "频控", 0, FieldValidation{}),
					field("interval", FieldTypeObject, true, true, "null", "展示间隔", "时间或离散项目间隔。", "interval", "频控", 1, FieldValidation{}),
					field("max_count", FieldTypeObject, true, true, "null", "次数上限", "session 或 day 的次数上限。", "count_limit", "频控", 2, FieldValidation{}),
					field("shift_count", FieldTypeObject, true, true, "null", "分时上限", "本地上午和下午的次数上限。", "shift_limit", "频控", 3, FieldValidation{}),
					field("positions", FieldTypeArray, true, true, "null", "适用位置", "固定业务位置集合。", "tags", "频控", 4, FieldValidation{}),
				}},
				{Name: "placement", Fields: []FieldSchema{
					field("client_id", FieldTypeString, true, false, `""`, "客户端 ID", "编译到客户端聚合 JSON 的稳定 ID。", "input", "基础", 0, FieldValidation{}),
					field("key", FieldTypeString, true, false, `""`, "广告位键", "面向业务的稳定键。", "input", "基础", 1, FieldValidation{}),
					field("ad_type", FieldTypeString, true, false, `"interstitial"`, "广告类型", "支持开屏、插屏和原生广告。", "select", "基础", 2, enum("app_open", "interstitial", "native")),
					field("enabled_switch_id", FieldTypeReference, true, false, `""`, "启用开关", "引用 feature_switch 实体 ID。", "feature_switch_ref", "投放", 3, FieldValidation{}),
					field("frequency_policy_type", FieldTypeString, true, false, `"preset"`, "频控类型", "使用预设或自定义频控策略。", "select", "投放", 4, enum("preset", "custom")),
					field("network_mode", FieldTypeString, false, true, `"admob"`, "广告链路", "覆盖全局 ad_network_mode；空表示继承全局配置。", "select", "投放", 5, enum("admob", "max")),
					field("frequency_policy_id", FieldTypeReference, true, true, "null", "频控策略", "引用 frequency_policy 实体 ID。", "select", "投放", 5, FieldValidation{}),
					field("custom_frequency_policy", FieldTypeObject, true, true, "null", "自定义频控", "使用与频控策略相同的结构。", "object", "投放", 6, FieldValidation{}),
					field("load_timeout_ms", FieldTypeInteger, true, false, "4000", "加载超时（毫秒）", "加载广告的最长等待时间。", "number", "投放", 7, FieldValidation{Minimum: &minimumOne}),
					field("cache_policy", FieldTypeString, true, false, `"memory"`, "缓存策略", "广告素材缓存位置。", "select", "投放", 8, enum("memory", "disk", "none")),
					field("cache_ttl", FieldTypeObject, true, true, "null", "缓存有效期", "缓存的有效期。", "duration", "投放", 9, FieldValidation{}),
					field("fallback_behavior", FieldTypeString, true, false, `"continue"`, "兜底行为", "广告不可用时的业务动作。", "select", "投放", 10, enum("continue", "skip_slot", "show_empty_safe")),
				}},
				{Name: "unit_binding", Fields: []FieldSchema{
					field("placement_id", FieldTypeReference, true, false, `""`, "广告位", "引用 placement 实体 ID。", "select", "绑定", 0, FieldValidation{}),
					field("environment_id", FieldTypeString, true, false, `""`, "环境", "绑定所属的项目环境。", "input", "绑定", 1, FieldValidation{}),
					field("platform", FieldTypeString, true, false, `""`, "平台", "客户端平台稳定 ID。", "input", "绑定", 2, FieldValidation{}),
					field("network", FieldTypeString, true, false, `""`, "网络", "广告网络或聚合平台稳定 ID。", "input", "绑定", 3, FieldValidation{}),
					field("unit_id_ref", FieldTypeString, true, true, "null", "广告单元引用", "广告网络单元 ID 或其安全引用。", "input", "绑定", 4, FieldValidation{}),
					field("status", FieldTypeString, true, false, `"configured"`, "配置状态", "单元 ID 是否已经配置。", "select", "绑定", 5, enum("configured", "missing")),
				}},
			},
			Migrations: []SchemaMigration{},
		},
	}
}
