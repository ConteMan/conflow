# Spec 008：构建计划、语义 Diff 与风险分析

> 状态：已实现  
> 依赖：Spec 005、007

## 目标

把当前草稿编译成可审阅、不可变的 Plan，回答“会改什么、影响谁、风险多大”，并作为发布的唯一输入。

## 范围

- Pack 编译为 provider-neutral desired configuration。
- 项目基线、环境覆盖、当前远端快照三方差异。
- 业务语义 diff、原始参数 diff、引用影响范围和风险等级。
- 不可变 `plan_id`、草稿 revision、source digest、remote ETag、生成时间和过期时间。
- review JSON / Markdown 与 Provider 输入 artifact。
- 计划过期、草稿改变或远端 ETag 改变时自动失效。

## 合同决策：Plan 构建与生命周期

`POST /api/v1/drafts/{environment_id}:plan` 始终返回 `202 Accepted` 和 `Operation`。Operation 成功后，其 `result` 指向唯一的 `plan` 资源；客户端以 `GET /operations/{operation_id}` 轮询恢复，以 SSE 作为进度增强。即使实现发现本地编译和远端快照均已命中缓存，也不得改为同步返回 Plan。

推荐这一方案。Plan 同时编排 DraftView 快照、完整校验、新鲜远端读取/快照、Pack 编译和风险分析；同步与异步按耗时切换会让 UI 无法可靠恢复中断的“正在构建”状态，也会使同一请求在网络波动时具有不同响应形状。备选方案是本地阶段同步、远端阶段单独 `remote:pull` 后再同步建 Plan；它降低短路径延迟，但会把 UI-API-014 的三阶段编排和失败恢复泄漏给前端，因此不采用。

Plan 是不可变资源，包含以下生命周期字段：

- `status`：`ready`、`preview_only`、`invalidated`、`expired`。只有 `ready` 可作为发布输入；`preview_only` 是远端不可达时完成的本地语义预览，明确不可发布。
- `snapshot_token`：不透明令牌，绑定 `draft_revision`、`source_digest`、`remote_etag` 或远端不可用状态、Pack ref 和编译输入内容；客户端不得解析或构造。
- `draft_revision`、`source_digest`、`remote_etag`：创建 Plan 时捕获的独立快照。`remote_etag` 在 `preview_only` 时为 `null`，不能伪造旧值。
- `expires_at`：UTC RFC 3339；到期后状态为 `expired`。实现的 TTL 可以配置，但 v1 默认 15 分钟，不能由客户端延长。
- `invalidation_reason`：仅在 `invalidated` 或 `expired` 时存在，固定为 `draft_revision_changed`、`source_digest_changed`、`remote_etag_changed`、`remote_snapshot_unavailable`、`ttl_expired` 或 `provider_capability_changed`。

服务端在 Plan GET、发布前检查和远端快照更新时重新比较这些前置条件；不能只信任客户端提交的 `expected_*`。草稿/source 变化与远端 ETag 变化是不同的错误域：前两者返回 `409 plan_invalidated`，远端 ETag 在发布前置检查中返回专用 `412 remote_etag_mismatch`。

### 远端不可达的预览

远端拉取失败、认证不可用或没有受保护远端快照时，Plan Operation 可以成功生成 `preview_only` Plan，前提是本地 DraftView 与 Pack 编译可完成。它必须包含 `remote_snapshot.status=unavailable`、结构化 `remote_snapshot.unavailable_reason`，以及未与远端比较的明确范围；不会产生 Provider 输入 artifact，不能发布，也不能被重新标为 `ready`。用户恢复连接后必须重新构建 Plan。

推荐保留这种 non-publishable preview：它让 PM 在离线或上游故障中审阅业务语义，同时把“能看”与“可发布”分离。备选方案是远端不可达即让 Plan Operation 失败；代价是把本地配置审阅完全耦合到 Firebase 可用性，并使 UI 无法区分本地问题与远端故障。

## Plan 读取模型

Plan DTO 必须可直接渲染“业务变更 -> 受影响实体 -> 远端参数”的逐层展开树，不允许 UI 根据 raw Firebase diff 推导影响关系。稳定字段如下：

- `plan_id`、`environment_id`、`status`、生命周期快照字段和 `content_digest`。
- `semantic_changes[]`：每项有稳定不透明的 `node_id`、`change_kind`、`summary`、`direct_entity_ref`、`field_path`、`before` / `after` 的脱敏展示值、`affected_entity_ids` 和下级 `affected_entity_node_ids` / `remote_parameter_node_ids`。数组按 `node_id` 稳定排序。
- `affected_entities[]`：每项有稳定 `node_id`、`entity_ref`、`entity_type`、`entity_id`、`impact_kind`、`caused_by_semantic_change_ids`；它表达服务端/Pack 判定的派生影响，不与直接变更混计。
- `remote_parameter_changes[]`：每项有稳定 `node_id`、`parameter_key`、`change_kind`、`before_summary`、`after_summary`、`caused_by_semantic_change_ids`、`affected_entity_node_ids` 和 `managed`。值摘要不得包含凭据、token 或未脱敏敏感值。
- `artifact_metadata[]`：`artifact_name`、`media_type`、`content_digest`、`size_bytes`、`sensitive` 与 `available`。artifact 元数据进入 content digest；时间、本机路径、Operation ID 不进入。
- `remote_snapshot`：远端快照版本/ETag 的脱敏摘要，或不可用原因；完整 Firebase 模板永不作为普通 Plan 响应返回。

`semantic_changes`、`affected_entities` 与 `remote_parameter_changes` 的 `node_id` 在同一 Plan 内稳定，跨不同 Plan 不保证相同；UI 只能将其作为 React key、展开状态和确认指向，不能反推领域规则。

## 风险与确认权威

风险是 Pack 与发布策略共同计算的服务端权威结果，UI 只能展示、收集确认和提交 ID，不得按 diff、环境名或中文文案重算。

- `severity` 是闭合枚举 `low`、`medium`、`high`、`blocking`；Plan 汇总风险为所有 risk item 的最高级别。
- `risk_items[]` 每项包含稳定不透明 `risk_item_id`、`severity`、稳定 snake_case `reason_code`、中文 `summary`、可选 `entity_ref`、`semantic_change_ids`、`remote_parameter_node_ids` 与 `acknowledgement_required`。
- `blocking_reasons[]` 是不可发布原因，包含稳定 `reason_code`、`summary`、可选关联风险项/节点 ID；它必须非 null，且 `status=ready` 时为空数组。
- `confirmation_requirements` 是发布请求的服务端输入合同：`requires_acknowledgement`、`environment_id_requirement`（`required|not_required`）、`required_risk_item_ids[]` 和 `policy_source`。它由环境 kind、项目级确认策略与 risk items 计算；`blocking` risk item 绝不能以确认绕过。

v1 的稳定 `reason_code` 初集为 `shared_frequency_policy_relaxed`、`global_feature_switch_changed`、`production_network_mode_changed`、`unit_binding_changed`、`managed_parameter_deleted`、`unmodeled_remote_condition`、`remote_baseline_missing`、`validation_not_ready` 与 `remote_snapshot_unavailable`。新增 code 只能追加，已发布 code 不改义。

## API

- `POST /api/v1/drafts/{environment_id}:plan`
- `GET /api/v1/plans/{plan_id}`
- `GET /api/v1/plans/{plan_id}/artifacts/{artifact_name}`

## CLI

- `conflow plan --environment <id> [--format text|json]`
- `conflow plan ... --output <dir>`

## 共享合同 fixture

`testdata/contracts/mobile-ad-monetization/v1/plan-risk-operation-rollback.json` 的 `large-diff-inter-global-cap-30-to-120` 是本 Spec 的主场景。它以基础 fixture 的 `inter_global_cap.cooldown_ms=30000` 为起点，断言 120000 的 replacement 产生 5 项直接变更、10 个受影响广告位、稳定树节点、风险项和 artifact metadata；后续 Go golden tests、API tests 与 UI E2E 必须直接复用，不能重写为截图或 Markdown 常量。

## 风险示例

- 低：说明文字或无行为影响的 metadata。
- 中：单广告位关闭、unit binding 调整。
- 高：共享频控降低、全局开关、生产网络切换。
- 阻断：删除被引用策略、覆盖未建模条件值、远端基线缺失。

## 验收

- 180 秒改为 300 秒时，语义 diff 展示受影响广告位而非只展示 JSON 数字。
- 相同输入生成相同内容 digest；时间和本机路径不进入 digest。
- 修改草稿或拉取新远端模板后旧 plan 无法发布。
- artifact 不包含凭据或真实 token。
- `POST :plan` 无论缓存命中与否都返回 Operation；页面刷新后仅靠 GET Operation 能取得 Plan 或结构化失败。
- `large-diff-inter-global-cap-30-to-120` fixture 产生 5 项直接变更、10 个受影响实体，`inter_global_cap` 的 `30 -> 120` 业务语义和远端参数证据通过稳定节点链接完整可遍历。
- UI 不解析 message、不计算受影响实体或风险即可渲染树、风险清单和确认要求；`preview_only` 永远不能成为发布输入。
