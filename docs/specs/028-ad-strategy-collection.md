# Spec 028：通用广告策略集合

> 状态：已实现
> 依赖：Spec 020、024、026
> 关联：GitHub Issue #69；`conflow-ad-strategy-requirements.md`

## 目标

让产品和运营在 Conflow 中维护一组可复用广告策略，按广告位选择生效范围并覆盖基础频控，最终确定性编译为一个版本化 Remote Config JSON 参数。

## 方向决策

1. Pack ref 保持 `mobile-ad-monetization/v2`，schema 从 2 升为 3；现有 v2 项目不配置广告策略时继续合法且不新增远端参数。
2. 基础 `frequency_policy` 仍是广告位的默认频控；策略只保存稀疏覆盖。覆盖字段缺失表示继承，显式 `null` 表示关闭该约束。
3. 默认策略和被设置引用的策略禁止删除；其他策略删除至少为 high 风险。
4. 广告策略使用“列表 + 独立详情页”，设置与列表在同一 tab；窄屏只读，不提供策略编辑和发布。

## 实体模型

### `ad_strategy_settings`

启用广告策略能力时，项目基线必须且只能包含一个 ID 为 `default` 的设置实体；设置和策略集合同时缺失是合法的兼容状态：

| 字段 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `parameter_key` | string | 否 | 广告策略聚合参数 key |
| `payload_version` | integer | 否 | 聚合 JSON 顶层版本，默认 `1` |
| `default_strategy_id` | reference | 是 | 默认策略；`null` 表示客户端不启用默认策略 |

该实体不可按环境覆盖。`parameter_key` 必须与所有 Pack 受管参数 key 全局唯一。

### `ad_strategy`

| 字段 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `description` | string | 是 | 用途说明，不参与编译 |
| `placement_rule_mode` | enum | 否 | 本版固定为 `allowlist` |
| `allowlist_placement_ids` | reference[] | 否 | 可使用该策略的广告位 ID；去重并稳定排序 |
| `frequency_policy_overrides` | object | 否 | key 为广告位 ID，value 为稀疏频控覆盖对象 |

`frequency_policy_overrides` 的每个 key 必须同时出现在 `allowlist_placement_ids`。覆盖对象只允许 `cooldown`、`interval`、`max_count`、`shift_count`、`positions`；字段缺失表示继承广告位最终基础频控，显式 `null` 表示关闭该约束，非空值复用 Spec 020 的领域类型。

## 引用合同

Pack schema 增加声明式引用规则，至少支持：

- `scalar`：字段值引用单个实体；
- `array_items`：数组每一项引用实体；
- `object_keys`：对象 key 引用实体。

现有 `placement.enabled_switch_id`、`placement.frequency_policy_id`、`unit_binding.placement_id` 与本 Spec 的三类引用统一消费该元数据。结构校验、`referenced-by`、受限删除和 UI 选择器不得继续各自硬编码引用关系。

## 编译合同

仅当 `ad_strategy_settings/default` 存在且至少有一个 `ad_strategy` 时输出策略参数。输出值：

```json
{
  "version": 1,
  "default_strategy_id": "balanced",
  "strategies": {
    "balanced": {
      "placement_rule_mode": "allowlist",
      "allowlist_client_ids": ["AD-PDF-001"],
      "frequency_policies": {
        "AD-PDF-001": {
          "cooldown": {"unit": "minutes", "value": 5},
          "interval": null,
          "max_count": {"unit": "day", "value": 4},
          "shift_count": null,
          "positions": null
        }
      }
    }
  }
}
```

- 聚合对象按策略 ID、广告位 `client_id` 和字段名稳定排序，输出紧凑规范 JSON。
- allowlist 从 Conflow 广告位 ID 映射为客户端 `client_id`；缺失、重复或无法映射均 blocking。
- `frequency_policies` 输出每个 allowlist 广告位的最终有效频控：先解析广告位的 preset/custom 基础策略，再应用稀疏覆盖。
- 参数来源包含设置实体、全部策略实体及被引用广告位/频控实体；同一广告位变更可以同时影响 placements 与 strategies 两个远端参数。
- 现有 v2 配置没有设置和策略实体时不输出策略参数，不产生删除或纳管风险。

## 校验、Diff 与风险

- singleton、参数 key、引用、allowlist/override 一致性、覆盖字段白名单与领域值执行 blocking 校验。
- 默认策略变更、策略生效广告位集合扩大、有效频控放宽、参数 key 或 payload version 变化至少为 high。
- 频控收紧和普通策略内容修改至少为 medium；非默认且未被引用策略删除至少为 high。
- 风险比较基于覆盖前后的最终有效频控，不以“字段是否为 null”或原始对象形状猜测。
- 远端同名未管理参数允许受控纳管，但 Plan 必须产生 high 风险并要求确认；未知 payload version、未建模条件值和受管参数意外删除继续 blocking。

## 导入与兼容

- schema 2 的 v2 文件读取为合法输入；内存中补齐空的策略集合，不写回源文件，直到用户首次保存策略相关实体。
- Spec 021 的导入预览/应用支持两个新集合，继续使用同 ID 冲突策略、revision 与 source revision 保护。
- 不从现有 placements、频控或远端 JSON 自动猜测并生成策略。

## UI

- 配置页新增「广告策略」tab；页面顶部为策略设置卡，下面为可搜索/筛选列表。
- 列表展示策略 ID、描述、适用广告位数量、覆盖数量、默认标记、未发布修改与操作。
- 新建/编辑进入独立详情页；分区展示基础信息、广告位 allowlist、逐广告位频控继承与覆盖、影响摘要和高级源映射。
- 继承态、显式关闭态和自定义值必须有文本标识；不能只用颜色或空输入框表达。
- 删除默认策略或被引用策略时展示引用清单并阻断；允许删除时明确 high 风险将在 Plan 中要求确认。
- 1440px、1280px 完成编辑流视觉验收；390px 只读展示策略摘要和“请使用桌面端编辑”说明。

## API / CLI

- 复用 Pack-neutral 草稿实体 CRUD、引用查询、校验、Plan、导入、发布与回滚端点。
- OpenAPI `EntityType` 与 Pack schema 引用元数据同步；无新增广告策略专用端点。
- CLI `validate`、`plan`、`publish` 与 `import` 自然覆盖，无新增专用命令。

## 非范围

- 任意规则语言、动态优先级、AB 实验、受众、收益优化或自动选策。
- 按环境维护不同策略集合。
- 从远端未知 JSON 反向生成可编辑策略。
- 窄屏策略编辑或发布。

## 验收

- 现有 schema 2 v2 fixture 读取、校验、编译结果不变且不新增策略参数。
- 新建两个策略、设置默认策略、选择广告位并覆盖频控后，校验通过且生成确定性 JSON；记录顺序变化不改变 digest。
- 缺失引用、override 不在 allowlist、未知覆盖字段、重复 `client_id`、key 冲突均 blocking。
- Plan 能从基础频控或广告位 `client_id` 变化追踪到策略参数，并按最终有效频控判定放宽/收紧风险。
- 默认/被引用策略删除被阻断；其他策略删除生成 high 风险确认。
- 导入 round-trip、API contract、Go 单测、UI E2E、1440/1280/390 视觉验收和 `make check` 全部通过后状态改为“已实现”。
