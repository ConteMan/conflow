# Spec 007：校验、引用完整性与发布就绪度

> 状态：待实现  
> 依赖：Spec 006

## 目标

将字段、引用、业务和环境校验统一为稳定诊断模型，供 CLI、API 和 GUI 共用。

## 校验层级

1. 字段：必填、类型、枚举、命名、范围。
2. 引用：开关、频控、广告位和环境绑定存在且类型匹配。
3. 业务：权限前禁弹、失败不阻断、类型与频控兼容、危险阈值。
4. 环境：Production unit、Provider、凭据引用和禁止覆盖字段。
5. 发布就绪度：未解决错误、未建模条件值、远端基线和计划状态。

## 与 Spec 004 写入校验的边界

Spec 004 的 `PUT /drafts/{environment_id}` 与 Spec 006 的实体写入只执行写入前 structural validation：请求 JSON、未知字段、Pack schema shape、字段 JSON 类型、`nullable`、静态 enum/range 以及 environment override 权限。它们失败时不写入草稿，返回 `422 validation_failed`；不产生 ValidationResult，也不更新上次完整校验记录。

本 Spec 的完整校验在已接受的 DraftView 快照上运行，覆盖引用完整性、实体跨字段规则、业务规则、环境完整性和 readiness。它可报告原始 source 或导入数据中已经存在的问题，因此不会假设每个问题都能先经过写入 API。完整校验不会自动修改草稿。

两类结果共享稳定字段：

- `code`：程序可判断的稳定 snake_case code。
- `path`：RFC 6901 JSON Pointer；指向整个实体时为实体 collection 中该实体的路径。
- `entity_ref`：可定位到单一业务实体时使用 Spec 006 的稳定引用；Pack-neutral structural error 或根级问题可省略。

structural detail 另外带 `scope`，完整诊断另外带 severity、fix suggestion 和文档链接。readiness 只由完整校验产生；写入成功、写入失败或单条 structural error 都不得暗示可发布。

## 校验结果合同

完整校验端点为：

- `POST /api/v1/drafts/{environment_id}:validate`：对请求开始时捕获的 DraftView / project draft revision 运行同步完整校验，并存储结果。
- `GET /api/v1/drafts/{environment_id}/diagnostics`：返回该环境最近一次完整校验结果；没有记录时返回 `404 validation_not_found`。

两者的成功响应都是 `ValidationResult`：

```json
{
  "data": {
    "environment_id": "production",
    "validated_draft_revision": 12,
    "validated_at": "2026-07-11T09:30:00Z",
    "status": "fresh",
    "readiness": "blocked",
    "diagnostics": []
  },
  "meta": {"request_id": "req_01J...", "revision": 12}
}
```

- `validated_draft_revision` 是开始验证时的项目级 draft revision，不是 manifest revision 或 source revision。
- `validated_at` 是完成验证的 UTC RFC 3339 时间。
- `status` 固定为 `fresh` 或 `stale`。任一 baseline 或环境 override 写入使该项目的 draft revision 改变后，既有结果继续可读但必须返回 `stale`；客户端不得把 stale 结果当作可发布授权。
- `readiness` 固定为 `ready` 或 `blocked`，由本次诊断 severity 计算。`stale` 是结果新鲜度，不是 readiness 的第三个值。
- `diagnostics` 必须非 null，按 `(severity_rank, entity_ref, path, code)` 稳定排序，其中 blocking、error、warning、info 的 rank 依次递增。

每条 `Diagnostic` 包含 `code`、`path`、`severity`、中文 `message`、可选 `entity_ref`、`fix_suggestion` 和可选 `documentation_url`。message 只供展示；CLI、GUI 和测试只依赖稳定字段。

## Severity、CLI 与 readiness 映射

severity 是闭合枚举，语义不得由 Pack 或 UI 自定义：

| severity | CLI exit code | readiness | UI 分类 |
|---|---:|---|---|
| `info` | 0 | 不受影响 | 建议 |
| `warning` | 0 | 不受影响 | 警告 |
| `error` | 1（若没有 `blocking`） | `blocked` | 阻断 |
| `blocking` | 2（只要存在至少一条） | `blocked` | 阻断 |

`conflow validate --environment <id> [--json]` 的退出码取结果中最高严重度：没有 `error` / `blocking` 为 `0`，存在 `error` 且没有 `blocking` 为 `1`，存在任意 `blocking` 为 `2`。warning 不改变退出码。UI 必须把 `error` 与 `blocking` 都展示为“阻断”，不得将 `error` 降级成警告或依据中文 message 重算 readiness。

`status=stale` 时，UI 可以展示上次结果及时间，但发布前的 readiness 必须视为不可用；后续 Plan / publish 合同将以 fresh result 为前置条件。Spec 007 不提前定义远端、Plan 或 Provider 结果的最终发布工作流。

## 结构化领域 fixture

`testdata/contracts/mobile-ad-monetization/v1/validation-overlays.json` 是对同版本 `entities.json` 的场景 overlay。每个场景通过可复放的实体 mutation / 删除尝试描述输入，并断言完整的稳定诊断 subset、readiness 与 CLI exit code。它至少覆盖人读 fixture 的 9 条诊断，包括字段、引用、缺失绑定、未引用和发布阻断；Go golden tests、API tests 与 UI E2E 使用同一文件，不能解析 Markdown。

## 非范围

- 自动修改配置。
- 执行任意用户脚本或 CEL/Rego 规则。
- Plan、远端读取或实际发布前置条件的运行时实现；分别由 Spec 008–010 定义。

## 验收

- 同一 fixture 经 CLI 和 API 得到相同的 `code`、`path`、`entity_ref`、severity、readiness 与 exit code。
- structural error 不写入草稿也不制造 ValidationResult；完整校验诊断不替代写入 API 的 `422`。
- Production 缺少 unit binding、删除被引用策略、非法广告类型均有稳定诊断；删除 API 的 `409 entity_referenced` 另返回 typed references。
- 草稿写入后旧 ValidationResult 可读取且明确为 stale，新的验证结果绑定触发时的 draft revision。
- UI 不解析 message 即可定位实体、字段、阻断/警告/建议和 readiness；运行时实现完成前本 Spec 保持“待实现”。
