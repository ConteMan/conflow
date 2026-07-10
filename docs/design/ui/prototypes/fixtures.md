# Spec 012 UI 原型真实规模 Fixture

> 场景：类 PDF Launcher 的移动应用广告配置包 `mobile-ad-monetization/v1`。本数据用于原型、评审和后续实现的共享参考；它刻意保留禁用项、长 key、未引用策略与不完整绑定，以覆盖真实操作场景。本文是人类可读视图；Spec 006 将建立结构化 contract fixture 作为 Go golden tests、API tests 和 UI E2E 的共同可执行事实源。

## 1. placement（24 条）

字段约定：网络模式为 `waterfall`、`bidding` 或 `hybrid`；缓存策略为 `none`、`memory` 或 `disk`。频控策略引用均指向本文件的 [frequency_policy](#2-frequency_policy5-条)。

| 稳定 ID | placement key | 类型 | 启用状态 | 网络模式 | 频控策略引用 | 加载超时 | 缓存策略 | fallback 行为 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| <a id="placement-app_open_cold_start"></a>`ad_app_open_001` | `app_open_cold_start` | `app_open` | 启用 | `hybrid` | `app_open_session_gate` | 4,000 ms | `memory` | 未加载完成时直接进入首页 |
| <a id="placement-app_open_warm_resume"></a>`ad_app_open_002` | `app_open_warm_resume` | `app_open` | 启用 | `bidding` | `app_open_session_gate` | 3,000 ms | `memory` | 继续恢复上次文档 |
| <a id="placement-app_open_after_document_share"></a>`ad_app_open_003` | `app_open_after_document_share` | `app_open` | 停用 | `waterfall` | `app_open_session_gate` | 5,000 ms | `none` | 跳过展示并返回分享完成页 |
| <a id="placement-app_open_return_from_settings"></a>`ad_app_open_004` | `app_open_return_from_settings` | `app_open` | 启用 | `hybrid` | `app_open_session_gate` | 3,500 ms | `memory` | 返回设置页，不重试 |
| <a id="placement-app_open_new_user_onboarding"></a>`ad_app_open_005` | `app_open_new_user_onboarding` | `app_open` | 启用 | `waterfall` | `app_open_session_gate` | 4,500 ms | `disk` | 进入引导下一步 |
| <a id="placement-app_open_background_return"></a>`ad_app_open_006` | `app_open_background_return` | `app_open` | 停用 | `hybrid` | `app_open_session_gate` | 3,000 ms | `memory` | 恢复后台前页面 |
| <a id="placement-app_open_library_refresh"></a>`ad_app_open_007` | `app_open_library_refresh` | `app_open` | 启用 | `waterfall` | `app_open_session_gate` | 4,000 ms | `memory` | 刷新本地文档列表 |
| <a id="placement-interstitial_open_document"></a>`ad_interstitial_001` | `interstitial_open_document` | `interstitial` | 启用 | `hybrid` | `inter_global_cap` | 5,000 ms | `memory` | 直接打开文档 |
| <a id="placement-interstitial_close_document"></a>`ad_interstitial_002` | `interstitial_close_document` | `interstitial` | 启用 | `waterfall` | `inter_global_cap` | 4,000 ms | `none` | 关闭文档并回到库 |
| <a id="placement-interstitial_export_pdf"></a>`ad_interstitial_003` | `interstitial_export_pdf` | `interstitial` | 启用 | `bidding` | `inter_global_cap` | 6,000 ms | `disk` | 继续导出 PDF |
| <a id="placement-interstitial_print_complete"></a>`ad_interstitial_004` | `interstitial_print_complete` | `interstitial` | 启用 | `hybrid` | `inter_global_cap` | 5,000 ms | `memory` | 显示打印完成提示 |
| <a id="placement-interstitial_merge_complete"></a>`ad_interstitial_005` | `interstitial_merge_complete` | `interstitial` | 启用 | `waterfall` | `inter_global_cap` | 5,000 ms | `memory` | 打开合并结果 |
| <a id="placement-interstitial_split_complete"></a>`ad_interstitial_006` | `interstitial_split_complete` | `interstitial` | 启用 | `waterfall` | `inter_global_cap` | 5,000 ms | `memory` | 显示拆分文件列表 |
| <a id="placement-interstitial_compress_complete"></a>`ad_interstitial_007` | `interstitial_compress_complete` | `interstitial` | 启用 | `hybrid` | `inter_global_cap` | 6,000 ms | `disk` | 显示压缩结果 |
| <a id="placement-compress_complete_interstitial_legacy_campaign"></a>`ad_interstitial_008` | `compress_complete_interstitial_legacy_campaign` | `interstitial` | 停用 | `waterfall` | `inter_global_cap` | 5,000 ms | `none` | 保留旧活动结果页，不发起请求 |
| <a id="placement-interstitial_scan_complete"></a>`ad_interstitial_009` | `interstitial_scan_complete` | `interstitial` | 启用 | `bidding` | `inter_global_cap` | 5,500 ms | `memory` | 打开扫描件预览 |
| <a id="placement-interstitial_sign_complete"></a>`ad_interstitial_010` | `interstitial_sign_complete` | `interstitial` | 启用 | `hybrid` | `inter_global_cap` | 5,000 ms | `memory` | 进入签名完成页 |
| <a id="placement-native_home_feed_top"></a>`ad_native_001` | `native_home_feed_top` | `native` | 启用 | `hybrid` | `native_scroll_gap` | 2,500 ms | `memory` | 隐藏广告槽并收起间距 |
| <a id="placement-native_library_list_inline"></a>`ad_native_002` | `native_library_list_inline` | `native` | 启用 | `waterfall` | `native_scroll_gap` | 2,500 ms | `memory` | 继续显示文档列表 |
| <a id="placement-native_recent_documents"></a>`ad_native_003` | `native_recent_documents` | `native` | 启用 | `bidding` | `native_scroll_gap` | 3,000 ms | `disk` | 显示最近文档，不预留空位 |
| <a id="placement-native_search_result_inline"></a>`ad_native_004` | `native_search_result_inline` | `native` | 启用 | `hybrid` | `native_scroll_gap` | 2,500 ms | `memory` | 保留搜索结果连续滚动 |
| <a id="placement-native_document_tools_panel"></a>`ad_native_005` | `native_document_tools_panel` | `native` | 停用 | `waterfall` | `high_intent_tool_cap` | 2,000 ms | `none` | 隐藏工具面板广告槽 |
| <a id="placement-native_document_scan_result_recommendation"></a>`ad_native_006` | `native_document_scan_result_recommendation` | `native` | 启用 | `hybrid` | `native_scroll_gap` | 3,000 ms | `memory` | 仅显示扫描建议卡片 |
| <a id="placement-native_subscription_offer"></a>`ad_native_007` | `native_subscription_offer` | `native` | 启用 | `bidding` | `native_scroll_gap` | 3,500 ms | `disk` | 展示订阅入口 |

统计：`app_open` 7 条、`interstitial` 10 条、`native` 7 条；停用 4 条；长度不少于 30 个字符的 key 为 3 条。`inter_global_cap` 恰由 10 个 placement 引用。

## 2. frequency_policy（5 条）

| <a id="frequency-policy"></a>策略 key | cooldown | interval | max count | shift count | positions | 引用 placement 数 | 原型用途 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| <a id="frequency-policy-inter_global_cap"></a>`inter_global_cap` | 30 秒 | 300 秒 | 3 次 | 1 次 | `open_document, export_pdf, complete_action` | 10 | 共享频控编辑抽屉与受影响实体清单 |
| <a id="frequency-policy-app_open_session_gate"></a>`app_open_session_gate` | 30 秒 | 1,800 秒 | 2 次 | 0 次 | `cold_start, warm_resume, return_from_settings` | 7 | PM 主路径将其冷却时间从 30 秒提高到 120 秒 |
| <a id="frequency-policy-native_scroll_gap"></a>`native_scroll_gap` | 0 秒 | 90 秒 | 4 次 | 2 次 | `feed_top, list_inline, search_inline` | 6 | 信息流滚动间隔 |
| <a id="frequency-policy-legacy_campaign_cap"></a>`legacy_campaign_cap` | 600 秒 | 3,600 秒 | 1 次 | 0 次 | `compress_complete` | 0 | 未引用提示与安全删除场景 |
| <a id="frequency-policy-high_intent_tool_cap"></a>`high_intent_tool_cap` | 180 秒 | 900 秒 | 2 次 | 1 次 | `document_tools_panel` | 1 | 工具面板的低频展示节流 |

说明：`legacy_campaign_cap` 是唯一未引用策略，用于“未引用”提示与安全删除场景。

## 3. feature_switch（6 条）

| <a id="feature-switch"></a>key | 默认值 | 风险等级 | 回滚方式描述 |
| --- | --- | --- | --- |
| <a id="feature-switch-use_amazon_bidding"></a>`use_amazon_bidding` | `false` | 高 | 关闭开关并重新生成 Plan；已发布时对目标环境发布 `false`，回退到 waterfall。 |
| <a id="feature-switch-ads_enabled_legacy"></a>`ads_enabled_legacy` | `false` | 高 | 保持为 `false`；如需紧急回滚，移除旧客户端环境覆盖并执行 Production 发布确认。 |
| <a id="feature-switch-enable_native_preload"></a>`enable_native_preload` | `true` | 中 | 设为 `false` 后清除本地内存缓存，下一次启动生效。 |
| <a id="feature-switch-show_subscription_offer"></a>`show_subscription_offer` | `true` | 中 | 设为 `false` 并发布；客户端保留订阅入口但不展示广告位。 |
| <a id="feature-switch-enable_ad_debug_overlay"></a>`enable_ad_debug_overlay` | `false` | 低 | 设为 `false`；无需迁移或清理远端数据。 |
| <a id="feature-switch-defer_app_open_until_consent"></a>`defer_app_open_until_consent` | `true` | 高 | 设为 `true` 并发布，强制在用户同意后才请求 app open。 |

## 4. unit_binding（3 个环境 × iOS / Android）

每个单元格表示一条绑定，格式为“`unit ID 引用` · 配置状态”。绑定稳定 ID 按 `ub_<environment>_<platform>_<placement_stable_id>` 推导；环境可覆盖 unit ID，不能覆盖 placement 的稳定 ID 或 key。`configured` 为已配置，`missing` 为缺少绑定。

| placement 稳定 ID | dev iOS | dev Android | staging iOS | staging Android | production iOS | production Android |
| --- | --- | --- | --- | --- | --- | --- |
| `ad_app_open_001` | `ios_dev_001` · configured | `android_dev_001` · configured | `ios_stg_001` · configured | `android_stg_001` · configured | `ios_prod_001` · configured | `android_prod_001` · configured |
| `ad_app_open_002` | `ios_dev_002` · configured | `android_dev_002` · configured | `ios_stg_002` · configured | `android_stg_002` · configured | `ios_prod_002` · configured | `android_prod_002` · configured |
| `ad_app_open_003` | `ios_dev_003` · configured | `android_dev_003` · configured | `ios_stg_003` · configured | `android_stg_003` · configured | `ios_prod_003` · configured | `android_prod_003` · configured |
| `ad_app_open_004` | `ios_dev_004` · configured | `android_dev_004` · configured | `ios_stg_004` · configured | `android_stg_004` · configured | `ios_prod_004` · configured | `android_prod_004` · configured |
| `ad_app_open_005` | `ios_dev_005` · configured | `android_dev_005` · configured | `ios_stg_005` · configured | `android_stg_005` · configured | `ios_prod_005` · configured | `android_prod_005` · configured |
| `ad_app_open_006` | `ios_dev_006` · configured | `android_dev_006` · configured | `ios_stg_006` · configured | `android_stg_006` · configured | `ios_prod_006` · configured | `android_prod_006` · configured |
| `ad_app_open_007` | `ios_dev_007` · configured | `android_dev_007` · configured | `ios_stg_007` · configured | `android_stg_007` · configured | `ios_prod_007` · configured | `android_prod_007` · configured |
| `ad_interstitial_001` | `ios_dev_008` · configured | `android_dev_008` · configured | `ios_stg_008` · configured | `android_stg_008` · configured | `ios_prod_008` · configured | `android_prod_008` · configured |
| `ad_interstitial_002` | `ios_dev_009` · configured | `android_dev_009` · configured | `ios_stg_009` · configured | `android_stg_009` · configured | `ios_prod_009` · configured | `android_prod_009` · configured |
| `ad_interstitial_003` | `ios_dev_010` · configured | `android_dev_010` · configured | `ios_stg_010` · configured | `android_stg_010` · configured | `ios_prod_010` · configured | `android_prod_010` · configured |
| `ad_interstitial_004` | `ios_dev_011` · configured | `android_dev_011` · configured | `ios_stg_011` · configured | `android_stg_011` · configured | `ios_prod_011` · configured | `android_prod_011` · configured |
| `ad_interstitial_005` | `ios_dev_012` · configured | `android_dev_012` · configured | `ios_stg_012` · configured | `android_stg_012` · configured | `ios_prod_012` · configured | `android_prod_012` · configured |
| `ad_interstitial_006` | `ios_dev_013` · configured | `android_dev_013` · configured | `ios_stg_013` · configured | `android_stg_013` · configured | `ios_prod_013` · configured | `android_prod_013` · configured |
| `ad_interstitial_007` | `ios_dev_014` · configured | `android_dev_014` · configured | `ios_stg_014` · configured | `android_stg_014` · configured | `ios_prod_014` · configured | `android_prod_014` · configured |
| `ad_interstitial_008` | `ios_dev_015` · configured | `android_dev_015` · configured | `ios_stg_015` · configured | `android_stg_015` · configured | `ios_prod_015` · configured | `android_prod_015` · configured |
| `ad_interstitial_009` | `ios_dev_016` · configured | `android_dev_016` · configured | `ios_stg_016` · configured | `android_stg_016` · configured | `ios_prod_016` · configured | `android_prod_016` · configured |
| `ad_interstitial_010` | `ios_dev_017` · configured | `android_dev_017` · configured | `ios_stg_017` · configured | `android_stg_017` · configured | `ios_prod_017` · configured | `android_prod_017` · configured |
| `ad_native_001` | `ios_dev_018` · configured | `android_dev_018` · configured | `ios_stg_018` · configured | `android_stg_018` · configured | `ios_prod_018` · configured | `android_prod_018` · configured |
| `ad_native_002` | `ios_dev_019` · configured | `android_dev_019` · configured | `ios_stg_019` · configured | `android_stg_019` · configured | `ios_prod_019` · configured | `android_prod_019` · configured |
| `ad_native_003` | `ios_dev_020` · configured | `android_dev_020` · configured | `ios_stg_020` · configured | `android_stg_020` · configured | `ios_prod_020` · configured | `android_prod_020` · configured |
| `ad_native_004` | `ios_dev_021` · configured | `android_dev_021` · configured | `ios_stg_021` · configured | `android_stg_021` · configured | `ios_prod_021` · configured | `android_prod_021` · configured |
| `ad_native_005` | `ios_dev_022` · configured | `android_dev_022` · configured | `ios_stg_022` · configured | `android_stg_022` · configured | `ios_prod_022` · configured | `android_prod_022` · configured |
| <a id="binding-production-ad_native_006"></a>`ad_native_006` | `ios_dev_023` · configured | `android_dev_023` · configured | `ios_stg_023` · configured | `android_stg_023` · configured | `—` · missing | `—` · missing |
| <a id="binding-production-ad_native_007"></a>`ad_native_007` | `ios_dev_024` · configured | `android_dev_024` · configured | `ios_stg_024` · configured | `android_stg_024` · configured | `—` · missing | `—` · missing |

统计：共 24 × 3 × 2 = 144 个预期绑定；已配置 140 个，Production 中 `ad_native_006` 与 `ad_native_007` 两个 placement 均缺少 iOS、Android 绑定。

## 5. 诊断样例（9 条）

| 级别 | 类型 | 实体锚点 | 可行动的中文文案 |
| --- | --- | --- | --- |
| 错误 | 字段错误 | [placement `app_open_cold_start` 的加载超时](#placement-app_open_cold_start) | 加载超时必须介于 1,000 ms 与 10,000 ms 之间；请将值调整到允许范围后重新校验。 |
| 错误 | 字段错误 | [placement `native_subscription_offer` 的 network mode](#placement-native_subscription_offer) | `native` 广告位不能使用未知网络模式 `auction`; 请选择 `waterfall`、`bidding` 或 `hybrid`。 |
| 错误 | 字段错误 | [feature switch `use_amazon_bidding` 的默认值](#feature-switch-use_amazon_bidding) | 默认值必须是布尔值；请将字符串 `enabled` 改为 `true` 或 `false`。 |
| 错误 | 引用完整性 | [frequency policy `inter_global_cap`](#frequency-policy-inter_global_cap) | 无法删除频控策略：仍被 10 个广告位引用。请先在受影响广告位中替换或移除该引用。 |
| 错误 | 引用完整性 | [placement `native_home_feed_top`](#placement-native_home_feed_top) | 频控策略 `native_scroll_gap` 不存在或已被删除；请选择一个有效的频控策略。 |
| 警告 | 缺失绑定 | [Production 的 `ad_native_006` 绑定](#binding-production-ad_native_006) | Production 缺少 `native_document_scan_result_recommendation` 的 iOS 与 Android unit ID。该广告位不会发布，请补齐绑定或停用广告位。 |
| 警告 | 缺失绑定 | [Production 的 `ad_native_007` 绑定](#binding-production-ad_native_007) | Production 缺少 `native_subscription_offer` 的 iOS 与 Android unit ID。若计划发布订阅入口，请先配置两个平台的 unit ID。 |
| 警告 | 未引用 | [frequency policy `legacy_campaign_cap`](#frequency-policy-legacy_campaign_cap) | 此频控策略当前未被任何广告位引用；删除前请确认不需要为旧活动回滚保留它。 |
| 阻断 | 发布就绪度 | [Production 绑定矩阵](#4-unit_binding3-个环境--ios--android) | Production 发布被阻断：2 个启用的广告位缺少平台绑定。补齐绑定或将对应广告位停用后，才能生成可发布 Plan。 |

## 6. 大 diff 场景（5 项直接修改 / 10 个受影响实体）

场景：运营将共享频控 `inter_global_cap` 的 cooldown 从 **30 秒**提高到 **120 秒**；此修改会波及其引用的 10 个 interstitial 广告位。同时翻转 3 个功能开关，并为 staging 新增 1 个环境覆盖。

| 变更类别 | 业务侧变更清单 | 影响 |
| --- | --- | --- |
| 共享频控（1 处） | [`inter_global_cap`](#frequency-policy-inter_global_cap) 的 `cooldown`：30 秒 → 120 秒 | 波及 10 个 interstitial placement；Plan 应逐层列出全部引用者。 |
| 受影响广告位（10 条） | `interstitial_open_document`、`interstitial_close_document`、`interstitial_export_pdf`、`interstitial_print_complete`、`interstitial_merge_complete`、`interstitial_split_complete`、`interstitial_compress_complete`、`compress_complete_interstitial_legacy_campaign`、`interstitial_scan_complete`、`interstitial_sign_complete` | 业务配置引用不变，但运行时频控结果改变；其中 1 条为停用旧活动，用于展示“受影响但不写入”的状态。 |
| 功能开关（3 处） | `use_amazon_bidding`：`false` → `true`；`enable_native_preload`：`true` → `false`；`show_subscription_offer`：`true` → `false` | 同时展示高、中、中风险标签与对应回滚说明。 |
| 环境覆盖（1 处） | 新增 staging 覆盖：`use_amazon_bidding = false` | 仅影响 staging；基线启用竞价，staging 保持 waterfall，用于三值 caption 与环境覆盖新增态。 |

直接修改共 5 项：1 个共享频控、3 个功能开关和 1 个 staging 环境覆盖；共享频控另外产生 10 个受影响广告位。远端参数数量级约 **12 个 Firebase 参数**：10 个引用影响在编译后按有效广告位聚合为约 8 个 placement 参数，3 个功能开关与 1 个 staging 覆盖各形成 1 个参数变更。该数量是 UI 评审用的编译结果 fixture，不构成 Firebase 参数命名或编译契约。
