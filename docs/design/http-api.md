# 前后端 HTTP API 交互规范

> 状态：已定稿（2026-07-10）。本规范约束 Conflow React GUI 与 Go 本地服务；机器可读契约维护在 [`api/openapi.yaml`](../../api/openapi.yaml)。

## 1. 目标与边界

API 服务于本地 GUI，不是公共云 API。它仍按公开契约维护，因为：

- 前后端独立构建，需要在实现前确认字段和错误语义；
- 同一用户可能打开多个标签页，或同时通过编辑器、Git、CLI 修改项目；
- 发布 Firebase 属于高风险写操作，需要并发保护、幂等和完整审计；
- 配置包会增加实体类型，但不应破坏项目、草稿、校验、计划、发布等稳定工作流。

CLI 不调用 HTTP。CLI 与 HTTP Handler 共同调用 Go 应用服务，保证校验、计划和发布语义一致。

## 2. 契约事实源

| 内容 | 事实源 |
|---|---|
| 路径、方法、请求和响应 schema | `api/openapi.yaml` |
| 分层、错误、并发、安全和演进语义 | 本文 |
| 单个功能的范围与验收 | `docs/specs/*.md` |
| 业务字段、规则和 UI 元数据 | 对应 Config Pack |

API 变更顺序固定为：

1. 更新对应 Spec 和 OpenAPI；
2. 评审破坏性、并发和安全影响；
3. 实现 Go Handler / DTO / 用例；
4. 更新或生成前端 TypeScript 类型；
5. 增加 Handler、契约和前端交互测试。

不得先让前端猜测 JSON，再倒推 Go 实现。

UI flows 或原型依赖服务端行为时，必须先登记并关闭 [`ui/contract-gaps.md`](ui/contract-gaps.md) 中的对应缺口。UI Spec 开工前使用 contract-only PR 合并路径、DTO、错误恢复、状态机和 fixture；该 PR 不混入 Handler 或 React 实现。后端尚未实现时，前端只能基于已合并 OpenAPI 与同源 contract fixture mock，不得另建一套手写响应。

## 3. 基础约定

- 基础路径：`/api/v1`。
- 编码：UTF-8 JSON；请求必须使用 `Content-Type: application/json`。
- JSON 字段：`snake_case`，与 Firebase key 和现有 Go/YAML 模型保持一致。
- 时间：UTC RFC 3339，例如 `2026-07-10T06:30:00Z`。
- ID：客户端视为不透明字符串；不从 ID 推导业务含义。环境风险身份只读取 `environment.kind`，不得从 ID、名称或 `publish.requires_confirmation` 推断。
- 数值单位必须写入字段名或结构，例如 `load_timeout_ms`、`cooldown: {unit, value}`。
- API 默认 `Cache-Control: no-store`。
- 未声明字段：服务端拒绝写请求中的未知字段；读响应新增可选字段视为向后兼容。

## 4. 响应 envelope

除健康检查、文件下载和 SSE 外，成功响应统一为：

```json
{
  "data": {},
  "meta": {
    "request_id": "req_01J...",
    "revision": 12
  }
}
```

- `data` 是资源或资源集合。
- `meta.request_id` 用于日志与问题定位。
- 可修改资源返回 `meta.revision`，同时响应头带 `ETag: "12"`。
- Pack 查询返回只读的 registry revision 与对应 ETag；它和项目 manifest revision 属于不同 revision 域，不得用于项目写请求的 `If-Match`。
- 集合分页时，`meta` 增加 `next_cursor`；没有下一页时省略，不返回空字符串。

健康检查保留最小响应：

```json
{"status":"ok","project_id":"photo-editor"}
```

## 5. 错误格式

```json
{
  "error": {
    "code": "validation_failed",
    "message": "配置存在 2 个问题",
    "request_id": "req_01J...",
    "details": [
      {
        "path": "/placements/AD-PDF-001/frequency_policy_id",
        "code": "reference_not_found",
        "message": "频控策略不存在"
      }
    ]
  }
}
```

前端根据 `error.code`、`details[].code` 和 `details[].path` 决定交互；不得解析 `message` 文本。

| HTTP | 稳定错误码示例 | 用途 |
|---|---|---|
| `400` | `invalid_request`、`malformed_json` | 请求结构无法解析 |
| `403` | `invalid_origin` | 浏览器来源不允许 |
| `404` | `project_not_found`、`entity_not_found`、`validation_not_found`、`pack_not_found`、`pack_version_not_found` | 资源不存在 |
| `409` | `state_conflict`、`entity_referenced`、`operation_in_progress` | 当前状态不允许操作；`entity_referenced` 必须携带 typed references[] |
| `412` | `revision_mismatch`、`source_revision_mismatch`、`remote_etag_mismatch` | 本地 revision、源 revision 或远端 ETag 已变化 |
| `415` | `unsupported_media_type` | 修改请求未使用 JSON Content-Type |
| `422` | `validation_failed`、`rule_violation`、`schema_incompatible` | 请求结构有效，但配置结构、业务规则不通过或客户端不能消费所请求 schema |
| `428` | `precondition_required` | 修改请求缺少 `If-Match` |
| `502` | `provider_error`、`provider_unauthorized` | 上游 Provider 调用失败 |
| `503` | `provider_unavailable` | 上游暂时不可用 |

Go 内部错误文本、文件绝对路径、凭据和上游响应正文不得直接返回给前端。

## 6. 乐观并发

所有可修改资源都有单调递增的本地 `revision`：

1. `GET` 返回 `ETag: "12"`；
2. `PUT` / `DELETE` 必须发送 `If-Match: "12"`；
3. 修订号不匹配返回 `412 revision_mismatch`，同时返回当前 revision 和该 revision 下的权威当前状态；
4. 前端使用本地输入与 `current_state` 展示冲突差异，不自动覆盖；用户确认重做后才能携带新 ETag 再次保存。

项目资料与环境共享 manifest revision 域。它们的所有写端点在 `412` 中返回同一次 Store 快照产生的 `current_state.project` 与 `current_state.environments`，响应头 `ETag` 与 `error.current_revision` 必须匹配该快照。不得先读取 revision、再通过另一次读取拼装状态。完整响应结构以 OpenAPI 的 `ManifestRevisionMismatchResponse` 为准。

草稿使用独立于 manifest 的项目级 draft revision 域。项目共享 baseline replacement 与各环境的 environment override replacement 的任何写入，都会递增同一个 draft revision；不能按环境维护互不相知的 ETag。`environment_id` 只是 DraftView 的读取和写入视角。草稿写请求还必须在请求体提交独立的 `expected_source_revision`，禁止把 Source Adapter revision 填入 `If-Match`。

草稿并发冲突固定为：

- `revision_mismatch`：`If-Match` 不是当前项目级 draft revision；
- `source_revision_mismatch`：draft revision 匹配，但 `expected_source_revision` 不是当前源快照；
- 两者都返回 `current_revision`、`current_source_revision`、`conflict_scope` 与当前环境视角的完整 `current_state`；`conflict_scope` 是失败请求试图写入的 scope，不是服务端猜测的并发变更来源；
- 错误响应的 ETag、两个 revision 与 current state 必须来自同一次锁内或事务内读取；
- 校验顺序必须先比较 draft revision，再比较 source revision，最后执行 Pack structural validation。

Firebase 的 ETag 是远端并发令牌，必须单独保存为 `remote_etag`。本地 revision 与远端 ETag 不得混用。

## 7. 修改语义

- `POST`：创建资源或触发有明确副作用的动作。
- `PUT`：完整替换一个可编辑资源，要求 `If-Match`。
- `DELETE`：删除资源，要求 `If-Match` 和引用检查。
- v1 不使用通用 JSON Merge Patch / JSON Patch；字段级保存由前端提交完整业务实体，降低空值与删除语义歧义。
- 危险操作不使用布尔 `force=true` 绕过规则；需要例外时必须由 Pack 明确定义可审计的 override。

字段语义：

- 缺失字段表示“未提供或使用默认值”；
- `null` 只在 schema 明确允许时表示“显式无值”；
- 空数组表示“明确为空集合”；
- 敏感值只返回 `configured: true/false` 和引用名，不回显真实内容。

### 7.1 草稿定向替换

草稿 API 使用 targeted replacement，完整合同见 [Spec 004](../specs/004-draft-layering.md) 与 [ADR-005](../decisions/ADR-005-targeted-draft-layer.md)：

```text
resolved baseline = draft.baseline ?? source.baseline
resolved environment override = draft.environment_override ?? source.environment_override
effective = Pack defaults < resolved baseline < resolved environment override
```

- `PUT /drafts/{environment_id}` 的 `write_scope` 为 `baseline` 或 `environment_override`，`configuration` 完整替换该目标草稿。
- `POST ...:reset` 安装显式 `{}` replacement；`POST ...:discard` 移除 replacement。两者都必须携带 `write_scope` 与 `expected_source_revision`。
- draft layer 使用 `{"present":false}` / `{"present":true,"value":{...}}` union，因此显式 `{}` 不会在序列化时退化为 missing。
- field state 扁平返回 `pack_default`、`baseline`、`draft_baseline`、`environment_override`、`draft_environment_override` 与 `effective`；每个值使用相同的布尔判别 presence union，`present:true` 的 value 可以是 Pack schema 允许的 JSON `null`。前端不得从嵌套状态自行拆出 source 与 draft。
- field state 的 `origin` 固定为 `pack_default`、`baseline`、`draft_baseline`、`environment_override` 或 `draft_environment_override`，明确区分 source 与 draft；Pack 当前要求字段提供 default，不使用 `missing` origin。
- 对象递归合并；标量替换；数组整体替换；字段缺失回退；空对象不清除低层对象。
- RFC 6901 JSON Pointer 是 field state 和 structural error 的稳定寻址格式。
- `dirty` / `dirty_scopes` 只汇总共享 baseline 与当前 `environment_id` 的 override replacement；其他环境 override 不进入当前视角，但其写入仍递增项目级 draft revision。
- `affected_environments` 只归因于当前视角展开的共享 baseline 与当前环境 replacement：比较保留它们和仅移除它们（其他环境草稿保持不变）时的 effective value，按项目环境顺序返回；dirty 但 effective 未变化的环境不计入。
- 每个通过前置条件与校验的 PUT/reset/discard 都递增一次 draft revision，包括内容相同和 discard-missing 的 no-op 写入。

`PUT` 结构校验失败返回 typed `422 validation_failed`，每条 detail 必须包含稳定 `code`（`invalid_config_shape`、`field_type_mismatch`、`required_field_missing`、`value_not_allowed`、`explicit_null_forbidden` 或 `environment_override_forbidden`）、`path`、`scope` 与 `message`，并按 `(scope, path, code)` 排序。该错误只覆盖 schema 结构、`nullable` 与 environment override 权限；引用、领域规则与 readiness 留给 Spec 006/007。reset/discard 没有 configuration，跳过此步骤且不声明不可达的 `422`。

草稿写入的错误优先级固定为：Origin / Content-Type → 解析并校验 `If-Match` → 解码请求与拒绝未知字段 → 资源存在性 → 捕获原子快照 → draft revision → source revision → structural validation → 提交。尤其不能先解码一个无 ETag 请求，再根据请求内容返回其他错误。

### 7.2 Pack-neutral 实体资源

Spec 006 的 Pack-neutral 实体资源位于 `/drafts/{environment_id}/entities`，因为实体是目标 Draft layer 的业务表达，而非独立于 baseline / environment override 的第四份配置。实体 CRUD 的 `write_scope`、`expected_source_revision`、项目级 draft ETag 与 `412` 顺序完全复用 7.1；`environment_id` 仍只是读取和写入视角。

- 实体引用固定为不透明的 `entity:<pack_ref>:<entity_type>:<entity_id>` 字符串。前端不得拆分它来推断类型或名称。
- `GET /drafts/{environment_id}/entities` 与单实体 `GET` 返回 `source`、`draft`、`resolved`、`effective` 的 presence state；列表按 `(entity_type, entity_id)` 排序。
- `POST`、`PUT` 与 `DELETE` 都要求 `If-Match`、`expected_source_revision` 和 `write_scope`。服务端在指定目标层的完整实体 collection replacement 内执行单实体变更；数组整体替换规则保持不变。
- `GET .../referenced-by` 返回当前环境有效图中的 `referenced_by[]`，每项包含稳定 `entity_ref`、`entity_type`、`entity_id` 与引用者内 RFC 6901 `path`。
- Pack `deletion_policy=restrict` 的删除若仍有有效引用，返回 `409 entity_referenced`，其中 `error.current_revision` 与 ETag 来自同一快照，且非空 `error.references[]` 是唯一供 UI 决策的引用清单。不得要求前端解析 `message`；不得用 `force=true` 绕过。

### 7.3 完整校验结果

Spec 007 的 `POST /drafts/{environment_id}:validate` 对一次捕获的 DraftView 运行完整校验并存储 `ValidationResult`；`GET /drafts/{environment_id}/diagnostics` 返回该环境最近一次结果，尚无结果时返回 `404 validation_not_found`。

- `ValidationResult` 固定包含 `validated_draft_revision`、`validated_at`、`status`（`fresh|stale`）、`readiness`（`ready|blocked`）与按稳定顺序返回的非 null `diagnostics[]`。
- 任意项目级 draft revision 变化会使旧结果为 `stale`，但旧结果仍可读；stale 结果不能授权后续发布。
- `Diagnostic` 与 structural error 共享 `code`、RFC 6901 `path` 和可选 `entity_ref`。structural error 另有 `scope`，只覆盖 schema / nullable / overrideability；完整诊断才表达引用、领域、环境和 readiness。
- severity 是闭合枚举：`info` 映射 UI“建议”，`warning` 映射“警告”，`error` 与 `blocking` 均映射“阻断”。任一 `error` 或 `blocking` 使 readiness 为 `blocked`；CLI 分别返回 1 或 2，存在 blocking 时优先 2。

## 8. 幂等与发布

以下请求必须带 `Idempotency-Key`：

- 创建发布；
- 回滚；
- 写回 Git 或生成提交；
- 可能重复触发外部副作用的 Provider 操作。

相同用户、环境、动作和幂等键在保留期内只能产生一个结果。请求体不同但复用键时返回 `409 idempotency_conflict`。

发布请求必须引用已经生成且未过期的 `plan_id`，并提交：

- `expected_draft_revision`；
- `expected_remote_etag`；
- `confirmation.environment_id`；
- 对高风险发布要求的确认短语或二次确认状态。

服务端发布前重新校验上述值；前端确认不能代替服务端检查。

## 9. 长任务与事件

拉取远端、生成计划、发布、回滚和 Git 操作可能成为长任务。此类请求返回 `202 Accepted`：

```json
{
  "data": {
    "operation_id": "op_01J...",
    "status": "pending"
  },
  "meta": {"request_id": "req_01J..."}
}
```

- `GET /api/v1/operations/{operation_id}` 获取权威状态。
- `GET /api/v1/events?operation_id=...` 使用 Server-Sent Events 提供进度增强。
- SSE 断开不影响任务；前端回退轮询 Operation。
- 状态固定为 `pending`、`running`、`succeeded`、`failed`、`cancelled`。
- 进度事件不包含凭据、原始 token、完整 Firebase 模板或敏感路径。

## 10. 端点资源图

以下是 v1 目标端点族；只有进入对应 Spec 并写入 OpenAPI 后才算已实现契约。

| 端点族 | 用途 | Spec |
|---|---|---|
| `/health`、`/bootstrap` | 运行状态、GUI 启动上下文、能力发现 | 001、002 |
| `/project`、`/environments` | 项目和环境 CRUD | 002 |
| `/packs` | Pack 列表、版本 metadata 与声明式表单 schema；schema 查询可带客户端支持的 `schema_version` | 003 |
| `/drafts/{environment_id}`、`/drafts/{environment_id}/entities` | 草稿、分层实体 CRUD、引用查询、环境覆盖 | 004、005、006 |
| `/drafts/{environment_id}:validate` | 完整校验 | 007 |
| `/drafts/{environment_id}:plan` | 构建、语义 diff、风险和影响范围 | 008 |
| `/environments/{environment_id}/remote:*` | Provider 连接、拉取和验证 | 009 |
| `/environments/{environment_id}/releases` | 发布、历史和回滚 | 010、011 |
| `/operations`、`/events` | 长任务状态与进度 | 009、010 |

## 11. 本地服务安全

- v1 只允许监听 loopback；如未来支持非 loopback，必须先新增认证与安全 ADR，不能只增加命令行开关。
- 不启用通配 CORS；GUI 与 API 同源。
- 服务端校验 `Host`；写请求校验 `Origin` 和 JSON `Content-Type`。
- 浏览器提交不了的 API 仍必须在服务端鉴权 Provider 凭据和校验发布权限。
- API 日志记录 request ID、端点、状态和耗时；不记录请求体、凭据、unit ID 原值或完整远端模板。
- GUI 不把 Provider token 放入 localStorage、sessionStorage 或 URL。

## 12. 前端状态规则

- 远端服务状态由 TanStack Query 一类服务端状态层管理的必要性，在 UI Spec 中单独评审；不得先引入全局状态库。
- 表单编辑状态保留在页面或领域表单层，服务端 revision 是保存冲突的最终依据。
- 前端展示加载、空状态、失败、过期、冲突、只读和危险确认七类通用状态。
- `bootstrap.capabilities` 是服务端权威能力；字段为 `false` 时前端必须进入对应只读状态，不得先发送写请求再依赖错误恢复。当前可写的本地项目返回 `true`，后续只读 Source Adapter 可返回 `false`。
- UI 可以乐观更新低风险本地字段，但发布、删除、回滚和 Provider 操作不得使用不可恢复的乐观成功提示。

## 13. 测试门禁

- OpenAPI 必须通过 lint 和 schema 校验。
- Go Handler 使用 `httptest` 覆盖成功、未知字段、错误映射、revision 冲突和安全头。
- 前端 API client 使用固定 fixture 覆盖错误码与字段路径，不 mock 领域规则。
- 每个端点 Spec 必须提供至少一个请求/响应示例和可执行验收。
- CI 检查 OpenAPI 与生成的 TypeScript 类型无漂移；生成物由对应 Spec 决定是否提交。
