# Spec 010：Firebase 发布与并发保护

> 状态：待实现  
> 依赖：Spec 008、009

## 目标

使用已确认 Plan、Firebase ETag 与幂等键发布完整合并模板，确保重复点击、远端并发修改或草稿变化不会产生不确定结果。

## 范围

- 发布请求引用 `plan_id`，重新校验草稿 revision、source digest 和 remote ETag。
- 合并受管理参数，保留未管理参数和已建模条件。
- 发布前再次调用 Provider validate。
- `Idempotency-Key` 去重和冲突检测。
- 根据环境与风险级别要求确认信息；Production 默认需要明确确认。
- 发布 Operation、成功版本 metadata 和失败诊断。

## 发布请求与确认合同

`POST /api/v1/environments/{environment_id}/releases` 必须带 `Idempotency-Key`，请求 body 为：

```json
{
  "plan_id": "plan_01J...",
  "expected_draft_revision": 13,
  "expected_remote_etag": "etag-remote-57",
  "confirmation": {
    "acknowledged": true,
    "environment_id": "production",
    "acknowledged_risk_item_ids": ["risk_01J..."]
  }
}
```

`environment_id` 在 Plan 的 `confirmation_requirements.environment_id_requirement=required` 时必填且必须等于路径环境；策略放宽为低风险勾选确认时可以省略。`acknowledged_risk_item_ids` 始终为数组，服务端按集合比较而非顺序比较，不能接受未知 ID；`acknowledged` 满足一般确认，不替代风险逐项确认。

服务端在创建发布 Operation 前按固定顺序验证：路径环境与 Plan 环境 -> Plan `status=ready` 和未过期 -> snapshot token/草稿 revision/source digest -> remote ETag -> readiness 与 blocking reasons -> `confirmation_requirements` -> Idempotency-Key。客户端传入的风险等级、环境 kind 或“已确认”文本均不是权威输入。重复键且规范化请求完全相同返回同一 Operation；同键不同 payload 返回 `409 idempotency_conflict`。

### 项目级确认策略（UI-API-004 / UI-API-019）

推荐将策略放入 Project 资源：`project.release_confirmation_policy.production_low_risk_mode`，枚举为 `environment_id` 或 `acknowledgement`，默认 `environment_id`。高风险的 `required_risk_item_ids` 与任何 blocking 规则不受该选项放宽；non-production 仍要求一般 `acknowledged=true`，但默认不要求输入环境 ID。Project GET/bootstrap 返回策略，项目 PUT 通过 manifest ETag 修改它。

备选方案 A 是扩展每个 `Environment.publish`，允许同一项目的 Production 环境配置不同强度；代价是维护者已收口的“项目级配置”被悄然拆散，UI 必须解释多处策略。备选方案 B 是独立 `/release-confirmation-policy` 资源；代价是新增 revision 域并使发布策略与项目设置脱节。推荐 Project 字段，代价是未来确有环境差异时需要迁移到更细粒度配置；本 ADR 草稿将此作为维护者终审点。现有 `Environment.publish.requires_confirmation` 保持兼容性展示字段，但其语义固定为“该环境发布需要一般确认”，不得再表达确认强度或覆盖项目策略。

## 远端 ETag 冲突恢复（UI-API-018）

当 Plan 的 remote ETag 与 Firebase 当前 ETag 不同，服务端不得发送更新请求，返回 `412 remote_etag_mismatch`。错误 payload 必须包含 `plan_id`、`expected_remote_etag`、`current_remote`（最新 `remote_etag`、`version`、`observed_at`、`summary`）与 `rebuild`（`required=true`、`plan_endpoint`、`reason_code=remote_etag_changed`）。`current_remote.summary` 仅包含参数数、受管理参数数、条件值计数和内容 digest 等脱敏摘要；需要字段级展示时 UI 读取 Spec 009 projection。

这与本地 `revision_mismatch`、`source_revision_mismatch`、`plan_invalidated` 完全分离：远端冲突没有 `current_revision`、DraftView 或 manifest ETag，UI 不得把新 remote ETag 作为本地写入的 `If-Match`。恢复路径是重新读取 projection/可选 pull 后重新 POST `:plan`，不能复用旧 Plan 或自动重试发布。

## API

- `POST /api/v1/environments/{environment_id}/releases`
- `GET /api/v1/operations/{operation_id}`

发布请求至少包含 `plan_id`、`expected_draft_revision`、`expected_remote_etag` 和 confirmation。

## CLI

- `conflow publish --environment <id> --plan <plan_id> --confirm`
- 非交互环境要求显式 `--idempotency-key`；不得默认跳过确认。

## 非范围

- 定时发布、多人审批和云端审批服务。
- `If-Match: *` 或通用 force publish。

## 共享合同 fixture

`testdata/contracts/mobile-ad-monetization/v1/plan-risk-operation-rollback.json` 覆盖主发布、`remote_etag_mismatch` 的无写入重建路径，以及提交后传输不确定时的 `remote_state=unknown`。任何 retry 实现都必须以该状态为准，而不是把网络错误一律显示为未发布。

## 验收

- 远端 ETag 变化返回 `412 remote_etag_mismatch`，且未发送更新请求。
- 相同幂等键和请求返回同一结果；相同键不同请求返回冲突。
- 未管理参数保留，未建模条件值阻断。
- 发布失败不写成功审计，不改变本地 source。
- 服务端只以 Plan `confirmation_requirements` 和 Project 策略验证确认；低风险策略放宽不会遗漏一般确认，高风险逐项确认从不被放宽。
- `remote_etag_mismatch` 包含最新脱敏远端摘要和明确重建路径，且不会混入任何 draft/manifest revision 字段。
