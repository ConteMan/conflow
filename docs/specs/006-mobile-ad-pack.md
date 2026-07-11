# Spec 006：移动广告配置包领域模型

> 状态：待实现  
> 依赖：Spec 003、004

## 目标

实现首个内置 Pack `mobile-ad-monetization/v1`，让 PM 以业务实体管理广告位、频控、开关和环境绑定。

## 实体

- `placement`：稳定 ID、placement、类型、启用开关、network mode、频控引用、加载超时、缓存、fallback。
- `frequency_policy`：cooldown、interval、max count、shift count、positions。
- `feature_switch`：稳定配置 ID、key、默认值、风险和回滚方式。
- `unit_binding`：环境、平台、unit ID 引用和配置状态。

支持的首版广告类型：`app_open`、`interstitial`、`native`。新增类型必须升级 Pack schema。

## 实体资源合同

### 挂载位置与分层理由

实体资源挂在 `/api/v1/drafts/{environment_id}` 之下，不增加广告专用顶层路径，也不创建独立于 Draft 的实体存储。理由如下：

- 实体读写必须以 `environment_id` 作为分层解析视角；它不把共享 baseline 变成环境私有数据。
- 创建、替换和删除必须写入 `baseline` 或当前环境的 `environment_override` replacement，复用 Spec 004 的 presence、source revision、项目级 draft revision、`If-Match` 与 typed `412`。
- Pack 将每个实体类型声明为一个数组字段；数组按 Spec 004 整体替换。因此服务端在实体写入时从目标 resolved collection 构造完整 replacement，再执行单实体变更。删除某个实体不会发明 tombstone 或第四种合并规则。
- `unit_binding` collection 可以按环境覆盖；`placement`、`frequency_policy`、`feature_switch` 的稳定 ID、key 和实体类型不得按环境覆盖。具体允许字段仍由 Pack schema 声明。

该选择只是已接受 ADR-005 的实体化应用，不改变草稿层或并发域，因此不新增 ADR。

### 稳定实体引用

所有跨资源引用、诊断和删除冲突使用不透明字符串 `entity_ref`：

```text
entity:<pack_ref>:<entity_type>:<entity_id>
```

例如 `entity:mobile-ad-monetization/v1:frequency_policy:inter_global_cap`。`pack_ref`、`entity_type` 与 `entity_id` 均由服务端返回；客户端只比较、保存和展示完整字符串，不拆分或据此推断业务语义。`entity_id` 在同一 Pack 的同一 `entity_type` 内稳定且不可变；改 ID 是创建新实体并迁移引用的显式业务操作，不是 `PUT` 的副作用。

实体路径为：

- `GET /api/v1/drafts/{environment_id}/entities?entity_type=...`
- `POST /api/v1/drafts/{environment_id}/entities`
- `GET /api/v1/drafts/{environment_id}/entities/{entity_type}/{entity_id}`
- `PUT /api/v1/drafts/{environment_id}/entities/{entity_type}/{entity_id}`
- `DELETE /api/v1/drafts/{environment_id}/entities/{entity_type}/{entity_id}`
- `GET /api/v1/drafts/{environment_id}/entities/{entity_type}/{entity_id}/referenced-by`

读响应使用 `EntityView`，固定返回 `entity_ref`、`entity_type`、`entity_id`、`source`、`draft`、`resolved`、`effective`、`origin`、`source_revision`。四个实体值都使用 `{"present":false}` / `{"present":true,"value":{...}}` presence union；`effective` 必须存在。列表按 `(entity_type, entity_id)` 稳定排序，引用者按 `entity_ref` 稳定排序。

创建、替换与删除都必须携带 `If-Match`（项目级 draft ETag）、`expected_source_revision` 和 `write_scope`。创建与替换的 request body 还携带完整 `entity`；路径中的 type/ID 与 body 不一致为 `400 invalid_request`。这些操作按 Spec 004 固定顺序先检查前置条件和两个 revision，再检查 Pack structural schema，最后检查实体类型、ID、不可变 ID 与业务规则。成功操作无论结果是否与原 resolved collection 相同，均递增一次项目级 draft revision。

### 引用查询与受限删除

`referenced-by` 返回当前环境有效配置中指向目标实体的 `referenced_by[]`；每项固定为 `entity_ref`、`entity_type`、`entity_id`、`path`。`path` 是引用者实体内的 RFC 6901 JSON Pointer。该响应只描述当前有效图，不把跨环境推测结果混入列表。

Pack `deletion_policy=restrict` 的实体删除前必须检查当前有效引用图。仍存在引用时返回 `409 entity_referenced`，响应 `error.references[]` 使用和查询相同的 `EntityReference` DTO，且必须非空；前端不得解析 `message`。删除已不存在的实体返回 `404 entity_not_found`。`cascade` 或 `allow` 的具体行为仍由 Pack 明确声明，v1 的频控策略使用 `restrict`。

## 范围

- 字段 schema、中文 UI metadata、默认值、ID / key 命名约束。
- 基线与环境字段边界：unit binding 必须按环境；稳定 ID 与 placement 不允许环境覆盖。
- Pack-neutral 实体 CRUD、稳定实体引用、引用查询和 typed 受限删除冲突。
- Provider-neutral 编译中间模型；Firebase 参数编译留给 Spec 008/009。
- 建立版本化、机器可读的领域 contract fixture：`testdata/contracts/mobile-ad-monetization/v1/entities.json`。Go golden tests、API tests 与 UI E2E 直接复用，`docs/design/ui/prototypes/fixtures.md` 只保留为人类可读说明。

## 非范围

- Banner、实验、竞价策略、收益分析和广告 SDK 管理。
- Firebase 发布和 Git JSON 兼容。
- 实体引用变更的 Plan、风险或跨环境影响分析；由 Spec 008 定义。

## 实现门槛

- 本 contract-only PR 不改 Go Handler、Draft Store 或 React 运行时代码。实现时必须把 entity collection 编码映射到 Spec 004 的完整目标 replacement，不能新建旁路实体存储。
- `internal/packs` 的 `EntityMetadata`、`EntitySchema` 与未来实体运行时 DTO 必须能表达本 Spec 的 type、稳定 ID、deletion policy 和 environment override 约束；Pack 不得导入 Source 或 Provider。

## 验收

- 共享 contract fixture 表达 24 个 placement、5 个共享频控、6 个 feature switch 与 3 个环境 x 2 个平台的绑定矩阵；实体 key 以 fixture 为唯一事实源。
- UI schema 不要求 PM 直接填写长 JSON。
- 删除被引用频控策略前返回稳定、非空的 `references[]`，而不是仅返回 message。
- 实体读取与写入严格复用 Spec 004 的分层、presence、ETag、source revision 和 typed `412` 模型。
- Pack schema、OpenAPI、生成 TypeScript 类型和 contract fixture 同步；运行时实现完成前本 Spec 保持“待实现”。
