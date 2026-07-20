# Spec 011：发布审计、默认值下载与回滚

> 状态：已实现  
> 依赖：Spec 010

## 目标

为每次发布保存可追踪证据，并支持基于 Firebase 版本的受控回滚和 Android 等客户端默认值导出。

## 范围

- Release 记录：项目、环境、Pack、操作者、时间、source digest、plan digest、远端前后版本和 ETag。
- 发布前/后模板摘要与语义 diff；敏感数据脱敏。
- Firebase 版本列表与回滚；回滚本身创建新的 Release 记录。
- 下载 XML / JSON / plist 默认值。JSON 与 plist 带来源版本 metadata；XML 与 Firebase 后台导出格式保持一致，不嵌入 Conflow metadata。
- 审计保留策略、导出和损坏检测。

## Release 历史与审计资源

`GET /environments/{environment_id}/releases` 返回可分页、按 `created_at` 倒序的 Release 摘要；`GET .../releases/{release_id}` 返回完整脱敏审计记录。Release 固定包含 `release_id`、`environment_id`、`kind`（`publish|rollback`）、`outcome`（`succeeded|failed`）、`created_at`、`completed_at`、`plan_id`（发布时）、`rollback_of_release_id`（回滚时）、`source_digest`、`plan_digest`、`remote_before`、`remote_after`、`semantic_summary`、`risk_summary` 和 `operation_id`。

`remote_before` / `remote_after` 只保存版本、ETag、时间和内容摘要；完整模板、凭据、token、敏感默认值不进入 Release API 或普通审计产物。失败发布也留下 `outcome=failed` 的审计记录，记录结构化 failure 与 `remote_state`，但绝不能伪装为成功 Release 或拥有虚构的 `remote_after`。

每次成功回滚都创建新的 `kind=rollback` Release，`rollback_of_release_id` 仅指向被恢复的成功 Release；它不是修改旧记录、也不是删除发布历史。

## 回滚 preview、请求与确认（UI-API-021）

回滚采用两步 read model，避免把“选择历史记录”误当成对当前线上事实的授权：

1. `POST /environments/{environment_id}/releases/{release_id}:rollback-preview` 创建 `operation_type=rollback_preview`，读取当前远端并构建不可变 preview；成功 Operation result 指向 `rollback_preview`。
2. `GET /environments/{environment_id}/releases/{release_id}/rollback-preview` 返回 `rollback_preview_id`、目标 Release/版本、当前远端摘要、`expected_remote_etag`、语义/远端差异、风险 items、blocking reasons、confirmation requirements、`status` 与 `expires_at`。它与 Plan 一样具有 TTL；状态为 `ready|invalidated|expired`。
3. `POST /environments/{environment_id}/releases/{release_id}:rollback` 必须带 `Idempotency-Key` 和 body：`rollback_preview_id`、`expected_remote_etag`、以及与发布同形的 `confirmation`（`acknowledged`、可选 `environment_id`、`acknowledged_risk_item_ids`）。它返回 `202 Operation`，成功 result 指向新 Release。

服务端逐项校验 preview 的环境/目标 release、状态/TTL、当前 remote ETag、项目策略、风险确认和幂等键。回滚不得接受 `force`，不得使用旧发布时的确认结果，也不得因为目标为历史成功版本而跳过当前高风险风险项。若 remote ETag 已变，返回与发布相同的 `412 remote_etag_mismatch` 恢复模型，并要求重新建 preview。

## API

- `GET /api/v1/environments/{environment_id}/releases`
- `GET /api/v1/environments/{environment_id}/releases/{release_id}`
- `POST /api/v1/environments/{environment_id}/releases/{release_id}:rollback-preview`
- `GET /api/v1/environments/{environment_id}/releases/{release_id}/rollback-preview`
- `POST /api/v1/environments/{environment_id}/releases/{release_id}:rollback`
- `GET /api/v1/environments/{environment_id}/defaults?format=xml|json|plist`

XML 响应使用 Firebase 客户端默认值结构：根节点为无属性的 `defaults`，每个参数使用 `entry` 下的 `key` 与 `value` 文本子元素。参数按 key 稳定排序，并进行 XML 文本转义；不得通过根节点属性或额外 entry 注入 Conflow metadata。来源版本和 digest 继续由 JSON 与 plist 格式提供。

## CLI

- `conflow release list/show`
- `conflow rollback --environment <id> --release <release_id> --confirm`
- `conflow defaults download --environment <id> --format xml`

## 非范围

- 长期云端审计仓库和组织级合规报表。
- 自动把默认值提交到客户端仓库。

## 共享合同 fixture

`testdata/contracts/mobile-ad-monetization/v1/plan-risk-operation-rollback.json` 的 `rollback-preview-and-new-release` 断言 preview 的当前 ETag/确认要求、rollback Operation 和新 `kind=rollback` Release 对目标 Release 的稳定链接。失败审计和未知远端状态也由同一文件的发布场景表达。

## 验收

- 发布、失败发布和回滚的审计语义不同且可检索。
- 回滚仍受 ETag、幂等和 Production 确认保护。
- 默认值文件与指定远端版本一致；JSON 与 plist 可通过内嵌 digest 验证，XML 与 Firebase 后台导出格式兼容。
- 删除本地敏感凭据不影响历史审计可读性。
- 回滚 preview 显示目标版本、当前远端基线、差异、风险与服务端确认要求；只有未失效 preview 能提交回滚。
- 成功回滚生成新的 `kind=rollback` Release 并链接目标 Release；失败回滚保留失败审计与 Operation failure，不改写旧 Release。
