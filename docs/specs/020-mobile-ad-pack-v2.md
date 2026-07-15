# Spec 020：移动广告配置包 v2 与聚合参数编译

> 状态：待实现  
> 依赖：Spec 006、008、011  
> 关联：GitHub Issue #41、#37、#33、#36

## 目标

新增内置 `mobile-ad-monetization/v2`，在不改变 Conflow 通用四层边界的前提下，以可复用业务实体表达广告开关、网络设置、频控策略、广告位和多维单元绑定，并确定性编译为独立开关参数与版本化聚合参数。

## 方向决策

1. v2 是新的 Pack 版本，不在 `mobile-ad-monetization/v1` 引用下静默改变字段类型或语义。
2. Pack 定义业务 schema、规范化、校验、编译、语义 Diff、风险和迁移规则；Source 只负责格式读写，Provider 只负责目标平台交互。
3. 实体与远端参数不是一一对应关系。Pack 可以把多个业务实体聚合为一个受管参数，但聚合只改变交付形态，不改变实体 ID、引用关系或业务语义。
4. 项目在 Pack 业务配置中声明聚合参数 key；通用代码不写入项目名、固定项目 key 或项目特例。
5. v1 到 v2 只通过显式迁移计划执行；无法无损转换时阻断并列出人工决策项，不按字段缺失、声明顺序或 Provider 默认值猜测。

## 领域类型

### Duration

```json
{"unit":"seconds","value":180}
```

- `unit`：`seconds`、`minutes`、`hours`、`days`。
- `value`：正整数。
- 字段显式为 `null` 表示该时间约束关闭；字段缺失不等价于 `null`。
- `0` 不作为关闭约束的别名，避免与缺失和 `null` 混用。

### Interval

```json
{"unit":"items","value":3}
```

- `unit`：`seconds`、`minutes`、`hours`、`days`、`items`。
- `value`：正整数。
- 时间单位沿用 `Duration` 的范围；`items` 表示客户端定义的离散项目间隔，不由 Conflow 执行运行时计数。
- 显式 `null` 表示不启用间隔约束。

### CountLimit

```json
{"unit":"day","value":4}
```

- `unit`：`session`、`day`。
- `value`：非负整数；`0` 表示对应窗口内不允许展示。
- 显式 `null` 表示不启用次数上限。

### ShiftLimit

```json
{"am":2,"pm":2}
```

- `am`、`pm` 均为非负整数，使用客户端本地时区解释。
- `0` 表示对应时段内不允许展示。
- 显式 `null` 表示不启用分时上限。
- v2 不引入任意时段表达式或规则语言；需要自定义时段时升级 Pack schema。

## 实体模型

### `remote_config_layout`

项目基线必须且只能包含一个 ID 为 `default` 的布局实体：

| 字段 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `active_network_parameter_key` | string | 否 | 当前网络的独立参数 key |
| `mediation_strategy_parameter_key` | string | 是 | 聚合策略参数 key；未使用时为 `null` |
| `frequency_policies_parameter_key` | string | 否 | 频控策略聚合 JSON 参数 key |
| `placements_parameter_key` | string | 否 | 广告位主数据聚合 JSON 参数 key |
| `payload_version` | integer | 否 | 聚合 JSON 顶层 `version`，v2 默认 `2` |

四类 key 必须满足目标 Remote Config 的参数命名约束，并与所有 `feature_switch.key` 全局唯一。布局属于 Pack 业务配置，不写入项目 manifest，也不由 Source 或 Provider 推导。

### `feature_switch`

沿用 v1 的稳定实体 ID、`key`、`default_value`、`risk_level` 和 `rollback_method`。每个开关编译为独立 Boolean 受管参数，不嵌入广告位或频控聚合 JSON。

> 维护者修订（2026-07-15）：新增可空 `description` 字段用于用途说明，仅存于基线，不参与编译输出。

### `network_settings`

项目基线必须且只能包含一个 ID 为 `default` 的网络设置实体：

| 字段 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `active_network` | string | 否 | 当前生效网络或聚合平台的稳定 ID |
| `mediation_strategy` | enum | 是 | `hybrid`、`bidding`、`waterfall`；未使用时为 `null` |
| `platforms` | string[] | 否 | 项目要求发布就绪检查覆盖的平台集合 |

`active_network` 与 `mediation_strategy` 是正交概念。任何一方都不得从另一方、空值或 Provider 默认值推导。`platforms` 非空、去重并稳定排序，只定义配置完整性范围，不表示 Provider 能力。

### `frequency_policy`

| 字段 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `cooldown` | Duration | 是 | 两次成功展示之间的最短时间 |
| `interval` | Interval | 是 | 时间或离散项目间隔 |
| `max_count` | CountLimit | 是 | session/day 次数上限 |
| `shift_count` | ShiftLimit | 是 | 本地上午/下午次数上限 |
| `positions` | string[] | 是 | 固定位置集合；`null` 表示不使用位置约束 |

五个字段都必须显式存在，值可以为 `null`。全为 `null` 是合法的“无额外频控”策略，不等价于实体缺失。数组去重并按规范化顺序编译；源文件中的数组顺序不产生业务语义。

> 维护者修订（2026-07-15）：新增可空 `description` 字段用于用途说明，仅存于基线，不参与编译输出。

### `placement`

| 字段 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `client_id` | string | 否 | 编译到客户端聚合 JSON 的稳定 ID |
| `key` | string | 否 | 业务触发位置 |
| `ad_type` | enum | 否 | `app_open`、`interstitial`、`native` |
| `enabled_switch_id` | reference | 否 | 引用 `feature_switch` |
| `frequency_policy_type` | enum | 否 | `preset` 或 `custom` |
| `frequency_policy_id` | reference | 是 | `preset` 时引用 `frequency_policy` |
| `custom_frequency_policy` | object | 是 | `custom` 时使用与频控策略相同的结构 |
| `load_timeout_ms` | integer | 否 | 广告加载超时；正整数毫秒 |
| `cache_policy` | enum | 否 | `memory`、`disk`、`none` |
| `cache_ttl` | Duration | 是 | 缓存有效期；不启用时为 `null` |
| `fallback_behavior` | string | 否 | 广告不可用时的业务动作 |

`client_id` 在同一项目内唯一，允许与 Conflow 的 snake_case 实体 ID 不同。`preset` 必须且只能设置 `frequency_policy_id`；`custom` 必须且只能设置 `custom_frequency_policy`。广告位不保存可变的 unit ID，也不把网络供应商字段嵌进基础实体。

> 维护者修订（2026-07-15）：
> - 新增可空 `description` 字段用于用途说明，仅存于基线，不参与编译输出；
> - `network_mode`（Spec 022 新增）调整为可空 enum（`admob`、`max`），空表示继承全局 `ad_network_mode`；
> - `fallback_behavior` 收敛为 enum：`continue`、`skip_slot`、`show_empty_safe`。

### `unit_binding`

| 字段 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `placement_id` | reference | 否 | 引用 `placement` |
| `environment_id` | string | 否 | 目标环境 |
| `platform` | string | 否 | 客户端平台稳定 ID |
| `network` | string | 否 | 广告网络或聚合平台稳定 ID |
| `unit_id_ref` | string | 是 | 单元 ID 或安全引用 |
| `status` | enum | 否 | `configured`、`missing` |

唯一键固定为 `(placement_id, environment_id, platform, network)`。v2 不支持维度通配；同一唯一键出现多条记录是 blocking 错误，不能以文件顺序决定生效项。`configured` 要求 `unit_id_ref` 非空，`missing` 要求其为 `null`。

## 分层规则

- `remote_config_layout`、`feature_switch`、`frequency_policy`、`placement` 的稳定身份和结构位于项目基线。
- `network_settings` 可以按环境覆盖 `active_network` 与 `mediation_strategy`。
- `unit_binding` 按环境覆盖，并由自身 `environment_id` 与当前读取环境一致性校验。
- v2 不新增配置层、通配继承或 Provider 私有覆盖优先级；继续使用 Spec 004 的 baseline / environment override 定向替换模型。

## 编译合同

### Provider-neutral managed parameter

Pack compiler 输出稳定排序的 managed parameter 集合，每项至少包含：

- `parameter_key`
- `value_type`：`boolean`、`string`、`json`
- `value`
- `source_entity_refs[]`
- `content_digest`

该中间模型不包含 Firebase ETag、条件表达式、凭据或 HTTP 字段。Plan 使用它建立“业务变更 → 受影响实体 → 远端参数”节点；Provider 只负责把已编译参数写入目标模板。

### 输出规则

1. 每个 `feature_switch` 输出一个 Boolean 参数，key 为实体的 `key`。
2. `network_settings.active_network` 输出到布局声明的独立 String 参数。
3. `mediation_strategy_parameter_key` 非空时，`network_settings.mediation_strategy` 输出为独立 String 参数；两者一空一非空是校验错误。
4. 所有 `frequency_policy` 编译为一个 JSON 参数：

   ```json
   {"version":2,"policies":{"policy_id":{}}}
   ```

5. 所有 `placement` 与当前环境匹配的 `unit_binding` 编译为一个 JSON 参数：

   ```json
   {"version":2,"placements":[]}
   ```

6. 聚合 JSON 使用规范化字段顺序；实体按稳定 ID 排序，位置数组去重排序。空白、对象键顺序和源文件记录顺序不影响 `content_digest`。
7. JSON 值必须使用 JSON 编码，不得使用语言默认的对象字符串表示。
8. 同一业务实体可以影响多个 managed parameter，但每个参数必须完整列出 `source_entity_refs[]`，供语义 Diff、风险和审计追踪。

### 远端保护

- 聚合参数是 Pack 管理的完整值，不执行未声明的局部 JSON 合并。
- 远端聚合 JSON 含未知版本、未知必填字段或无法映射的顶层内容时，Plan 为 blocking，禁止重建后覆盖。
- 受管参数存在未建模条件值时沿用 Spec 008–010 的 blocking 规则；默认值更新不得删除或改写条件值。
- 未管理参数、未选择的受管参数、条件定义和 ETag 必须保持现有保护语义。
- validate-only 与正式发布必须使用同一份不可变 Provider artifact。

## 校验与引用规则

- 所有实体 ID、`client_id`、参数 key 和复合 binding 键必须唯一。
- `enabled_switch_id`、`frequency_policy_id`、`placement_id` 必须命中有效实体。
- 当前环境下，每个 placement、`network_settings.platforms[]` 与 `active_network` 的组合都必须有唯一 unit binding；生产环境缺失为 blocking，非生产环境按 Pack 风险规则报告。
- `preset/custom` 联合字段、nullable 字段、Duration/Interval 单位和数值范围执行 structural validation。
- 删除被引用实体继续使用 `deletion_policy=restrict` 和 typed `references[]`。
- 编译后参数 key 冲突、JSON 值无法确定性生成或 binding 存在同优先级歧义均为 blocking，不提供 `force=true` 绕过。

## 语义 Diff 与风险

语义 Diff 基于规范化业务模型和编译产物，不比较 YAML/JSON 排版。至少区分：

- 时间、间隔、次数或分时限制放宽/收紧；
- 共享频控变更及其受影响广告位；
- 开关默认值变化；
- active network 与 mediation strategy 的独立变化；
- binding 新增、缺失、替换及影响的 environment / platform / network；
- 聚合参数 key、payload version 或客户端 ID 变化；
- 仅表示变化而业务值等价的 no-op。

频控放宽、全局开关打开、生产网络切换、binding 生效范围扩大、参数 key 重命名和 payload version 变化至少为 high；未知聚合版本、引用缺失、binding 歧义、未建模条件值和无法无损迁移为 blocking。

## v1 到 v2 迁移合同

- v1 项目继续按 `mobile-ad-monetization/v1` 读取和发布；注册 v2 不改变其行为。
- 迁移先生成不可变预览，包含源/目标 Pack ref、manifest/source revision、字段映射、生成实体、删除字段、风险和 `decisions_required[]`。
- `cooldown_ms`、`interval_ms` 在能整除 1000 时迁移为 seconds，并可进一步规范化为能精确表达的较大单位；不能以 v2 单位精确表达时加入人工决策项，不允许舍入。迁移后的业务时长必须等价。
- v1 `network_mode` 仅迁移为 `mediation_strategy`；不得据此猜测 `active_network`。
- v1 unit binding 缺少 network 时必须由用户提供 network 映射，不能从 unit ID 文本推断。
- v1 placement 的布尔 `enabled` 不能自动替代 `enabled_switch_id`；无法建立唯一开关引用时加入人工决策项。
- 迁移只产生 v2 草稿和 manifest/source 变更预览；确认、revision 和 source digest 任一变化后旧预览失效。
- 应用失败不得留下半迁移 manifest、source 或 draft。Git JSON 项目不尝试写回无法表达 v2 的旧格式；跨 workspace 导入由独立规格处理。
- 不做 v1/v2 双写，也不把已迁移项目自动降级为 v1。回滚恢复迁移前完整本地快照和原 Pack ref。

迁移 API / CLI 的公开合同在实现前与配置包选择能力一并补入 OpenAPI；至少提供 preview/apply 两阶段、`If-Match`、expected source revision 和结构化人工决策输入。本 Spec 不以临时命令或直接改 manifest 代替正式合同。

## 共享 fixture 与测试

新增 `testdata/contracts/mobile-ad-monetization/v2/`，至少包含：

- 最小合法 v2；
- 多环境、平台、网络、广告位、频控和 binding 的完整场景；
- Duration/Interval/null/零次数边界；
- preset/custom、引用缺失和复合键冲突；
- 自定义聚合 key、key 冲突、未知 payload version；
- 可无损迁移、需要人工决策、无法迁移和重复迁移；
- 默认值与条件值并存、未管理参数保留、ETag 变化；
- 发布前后及回滚后的完整 Provider artifact。

同一 fixture 被 Pack golden tests、Plan tests、API tests 和 UI E2E 复用。必须覆盖：

- `parse → render → parse` 语义等价；
- `v1 → migration preview → v2` 与预期 fixture 等价；
- `pull → plan → validate-only → publish → pull` 受管参数等价；
- 回滚后默认值、条件值和未管理参数与发布前快照一致；
- 空 Diff 不调用 publish，同一 Plan 重复提交保持幂等。

## API / CLI

- 现有 `/packs` 与 Pack schema API 返回 v1、v2 两个独立版本。
- 现有 draft/entity/validate/plan/release API 保持 Pack-neutral，通过 `pack_ref` 和 schema 驱动 v2。
- 迁移 preview/apply 的端点、DTO、错误码和 CLI 在 contract-only 实现阶段同步加入 `api/openapi.yaml`、Go DTO 与生成 TypeScript 类型。

## 非范围

- 从其他 workspace、导出文件或历史格式导入配置；由独立导入规格负责。
- 动态脚本、第三方 Pack 下载、通用低代码 Pack 编辑器或项目专属 Pack 代码。
- 运行时广告决策引擎、收益优化、实验、受众或归因系统。
- 任意时段表达式、任意 binding 通配规则或 Provider 私有竞价参数。
- 在 Source / Provider 中实现业务默认值、频控算法、网络选择或项目特例。

## 实现门槛

- 先合并 contract-only PR：Spec、配置模型、OpenAPI、Pack schema、迁移 DTO 和共享 fixture 同步，不混入 Handler 或 React 实现。
- `mobile-ad-monetization/v1` 的 schema、编译结果、fixture 和行为测试保持不变。
- Pack compiler、validator、semantic differ、migrator 不得导入 `internal/source` 或 `internal/provider`。
- 不新增运行用户脚本或动态下载代码，不引入新的直接依赖。

## 验收

- v2 schema 可表达开关、网络设置、可空频控、广告位和 environment / platform / network 单元绑定。
- 相同规范化输入稳定生成独立开关参数、网络参数和两个版本化聚合 JSON；源文件排序变化不改变 digest。
- UI 通过 schema 驱动字段控件，用户不直接编辑聚合 JSON 或手填毫秒裸值。
- Plan 能从单个业务字段追踪到受影响实体和聚合参数，并正确计算风险与确认要求。
- 聚合 key 冲突、非法联合字段、引用缺失、binding 歧义、未知远端版本和未建模条件值均明确阻断。
- v1 项目行为不变；迁移预览可重复、可审计，人工决策未完成时不能应用。
- validate-only、ETag、幂等、发布后复核、审计和回滚继续复用 Spec 009–011 的安全边界。
- OpenAPI、Go DTO、生成 TypeScript 类型、共享 fixture 和文档一致，`make check` 通过后状态改为“已实现”。
