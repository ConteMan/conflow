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

## 验收

- 远端 ETag 变化返回 `412 remote_etag_mismatch`，且未发送更新请求。
- 相同幂等键和请求返回同一结果；相同键不同请求返回冲突。
- 未管理参数保留，未建模条件值阻断。
- 发布失败不写成功审计，不改变本地 source。
