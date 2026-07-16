# Spec 027：通用自定义 Remote Config 参数

> 状态：待实现  
> 依赖：Spec 020（v2 聚合模型与编译）、Spec 024（实体变更基线）  
> 关联：ad_network_mode 讨论引出的架构缺口——当前全 Pack 化架构下新增任意参数需改代码发版

## 目标

用户可以在界面上新增、编辑、删除自定义 Firebase Remote Config 参数（boolean / string / number / json），经受控流程（校验 → plan → 发布 → 审计 → 回滚）直达 Firebase，无需修改 Pack 代码或发布 Conflow 新版本。

## 背景

当前每个发布参数都必须由 Pack 领域实体显式编译而来（开关、网络模式、两个聚合 JSON）。加一个普通业务参数（如 `min_supported_version`）需要改 schema、编译器、校验与 UI 并发版，成本不成比例。既有"未管理参数"机制只保护远端已有参数不被覆盖，不提供创建与编辑能力。

## 范围

### 实体模型

`mobile-ad-monetization/v2` 新增实体类型 `custom_parameter`（集合 `custom_parameters`，删除策略 restrict）：

| 字段 | 类型 | 必填 | 可空 | 说明 |
|---|---|---|---|---|
| `key` | string | 是 | 否 | Firebase 参数 key，同时作为实体 ID（键即标识符）；命名约束与目标 Remote Config 一致 |
| `value_type` | enum | 是 | 否 | `boolean`、`string`、`number`、`json` |
| `value` | 任意 | 是 | 否 | 与 `value_type` 匹配的参数值 |
| `description` | string | 否 | 是 | 用途说明，仅存基线，不参与编译 |

- **基线共享，不开环境覆盖**：值跨环境一致，发布节奏按环境独立（与整体提升模型一致）。未来出现真实的按环境差异需求时，按 `network_settings` 先例通过 overrides 白名单开放，另行修订。
- `value_type` 创建后不可修改（改类型 = 删除重建，避免类型漂移的隐式语义）。

### 校验

- `key` 满足 Remote Config 参数命名约束，且与以下受管 key 全局唯一：全部 `feature_switch.key`、`remote_config_layout` 声明的聚合参数 key 与 `active_network_parameter_key` / `mediation_strategy_parameter_key`、其他 `custom_parameter.key`。冲突为 blocking。
- `value` 与 `value_type` 匹配：boolean 为布尔、number 为有限数值、json 为合法 JSON（对象或数组）、string 为字符串。不匹配为 blocking。

### 编译与发布

- 每个 `custom_parameter` 编译为一个独立受管参数：key 为实体 `key`，Firebase `valueType` 按 `boolean→BOOLEAN`、`string→STRING`、`number→NUMBER`、`json→JSON` 映射；`defaultValue.value` 为字符串序列化（boolean 输出 `true`/`false`，json 输出规范化紧凑 JSON，number 输出十进制字符串）。
- **纳管路径**：`key` 与远端"未管理参数"同名时不在本地阻断，而是在 plan 中生成 high 风险项（"发布后该参数由 Conflow 接管，远端当前值将被覆盖"），要求显式确认。这是把存量手工参数收编进受控源的正规通道。
- 语义 diff 与风险：新增 / 修改 custom_parameter 至少 medium；删除（参数从远端移除）至少 high。参与实体变更基线（Spec 024）与发布审计，回滚按既有合同恢复。

### UI

- 配置页新增「自定义参数」tab：DataTable 列表（key + 描述、类型、值摘要、未发布修改、操作），复用键即标识符、描述占位、编辑抽屉（Drawer 原语 + SelectField）等既有惯例。
- 值编辑器按类型切换：boolean 用开关、number 用数字输入、string 用文本输入、json 用多行文本 + 合法性即时校验。
- 抽屉内展示纳管提示：key 命中远端未管理参数时给出说明文案。

### API / CLI

- 复用既有草稿实体 CRUD 端点（`entity_type=custom_parameter`），无新增专用端点；OpenAPI 的实体类型枚举与示例同步。
- CLI `validate` / `plan` / `publish` 自然覆盖，无行为变更。

## 非范围

- 环境级参数值差异（本版不开 overrides）。
- Firebase 条件值（conditional values）的创建与编辑（既有保护合同不变）。
- 批量导入自定义参数（Spec 021 导入合同的扩展另行考虑）。
- v1 Pack 不加此实体。

## 验收

- UI 新建 string / json 自定义参数 → 校验通过 → plan 显示新增参数与风险 → dev 发布成功 → Firebase 模板出现对应参数且 `valueType` 正确 → 回滚后参数从远端消失。
- key 与 feature_switch 或聚合参数 key 冲突时校验 blocking；value 与类型不匹配 blocking。
- key 命中远端未管理参数时 plan 生成 high 风险项并要求确认，确认后发布接管该参数。
- 与 Spec 024 基线联动：发布后列表 `unchanged`，修改值后标记 `已修改`。
- `make check` 通过；Go 单测覆盖四种类型的编译序列化、key 冲突校验、纳管风险生成。
