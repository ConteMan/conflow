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
| `404` | `project_not_found`、`entity_not_found`、`pack_not_found`、`pack_version_not_found` | 资源不存在 |
| `409` | `state_conflict`、`operation_in_progress` | 当前状态不允许操作 |
| `412` | `revision_mismatch`、`remote_etag_mismatch` | `If-Match` 或远端 ETag 已变化 |
| `415` | `unsupported_media_type` | 修改请求未使用 JSON Content-Type |
| `422` | `validation_failed`、`rule_violation`、`schema_incompatible` | 请求结构有效，但业务规则不通过或客户端不能消费所请求 schema |
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
| `/drafts/{environment_id}` | 草稿、实体 CRUD、环境覆盖 | 004、005、006 |
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
- UI 可以乐观更新低风险本地字段，但发布、删除、回滚和 Provider 操作不得使用不可恢复的乐观成功提示。

## 13. 测试门禁

- OpenAPI 必须通过 lint 和 schema 校验。
- Go Handler 使用 `httptest` 覆盖成功、未知字段、错误映射、revision 冲突和安全头。
- 前端 API client 使用固定 fixture 覆盖错误码与字段路径，不 mock 领域规则。
- 每个端点 Spec 必须提供至少一个请求/响应示例和可执行验收。
- CI 检查 OpenAPI 与生成的 TypeScript 类型无漂移；生成物由对应 Spec 决定是否提交。
