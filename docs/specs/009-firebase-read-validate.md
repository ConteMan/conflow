# Spec 009：Firebase Provider 认证、拉取与预校验

> 状态：待实现  
> 依赖：Spec 002、005、008

## 目标

实现 Firebase Remote Config Provider 的只读与安全预校验能力，在任何发布前取得当前模板、ETag、版本和默认值事实。

## 范围

- Provider 接口：connect/status、pull、validate、capabilities。
- 本地认证资料只保存引用；支持的认证方式和凭据存储需在实现 PR 的安全说明中明确。
- 拉取当前模板、ETag 和版本 metadata；完整模板作为受保护快照保存。
- 调用 Firebase `validate_only`，映射上游错误为稳定领域错误。
- 检测受管理参数中的条件值、未知参数和模板结构差异。
- Operation 资源和 SSE 进度基础。

## API

- `GET /api/v1/environments/{environment_id}/provider`
- `POST /api/v1/environments/{environment_id}/provider:connect`
- `POST /api/v1/environments/{environment_id}/remote:pull`
- `POST /api/v1/environments/{environment_id}/remote:validate`
- `GET /api/v1/operations/{operation_id}`
- `GET /api/v1/events?operation_id=<id>`

## CLI

- `conflow provider status --environment <id>`
- `conflow pull --environment <id>`
- `conflow remote validate --environment <id> --plan <plan_id>`

## 非范围

- 发布、回滚和写入 Git。
- Firebase A/B Testing、Personalization 和 Analytics。

## 验收

- fake Provider 覆盖认证成功/失败、ETag、超时、上游 4xx/5xx 和取消。
- 未建模条件值生成 blocking 诊断，不会被合并器静默删除。
- 日志、API 和快照 metadata 不泄露 token；完整快照不作为普通 API 响应返回。
- SSE 断线后可通过 Operation 轮询获得最终状态。
