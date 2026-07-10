# ADR-004：前后端通过版本化 HTTP API 交互

> 状态：已接受（2026-07-10）

## 背景

Conflow 的 React GUI 与 CLI 必须复用同一套 Go 领域能力。GUI 需要稳定、可测试的交互契约，而领域模型还会随着配置包、源适配器和发布适配器扩展。

## 决策

- 本地 Web GUI 只通过同源 `/api/v1` HTTP API 调用 Go 服务，不直接读取文件或复制领域规则。
- HTTP 契约以 [`api/openapi.yaml`](../../api/openapi.yaml) 为机器可读事实源，以 [`docs/design/http-api.md`](../design/http-api.md) 解释语义和安全边界。
- API 使用 JSON、`snake_case` 字段、稳定错误码、资源修订号和 HTTP `ETag` / `If-Match` 乐观并发控制。
- 长任务统一暴露 Operation 资源；事件流使用同源 SSE。发布与回滚请求必须带幂等键。
- CLI 调用 Go 应用服务，不通过本地 HTTP 回环调用；CLI 与 HTTP Handler 共享用例层和领域错误映射。

## 理由

- OpenAPI 能让前后端在实现前评审同一份契约，并支持生成 TypeScript 类型与契约测试。
- 版本化路径和稳定错误码能防止 UI 依赖 Go 内部结构或错误文本。
- 乐观并发与幂等键可以覆盖多标签页、文件外部修改、远端 ETag 变化和重复发布等高风险场景。

## 后果

- 新增或改变公开端点时，必须在同一 PR 先更新 OpenAPI、设计文档或对应 Spec。
- Go DTO 与领域实体分离；前端不得直接镜像未声明的 Go struct。
- 当前 `GET /api/v1/health` 保持最小裸响应，作为健康检查特例；其他业务端点使用统一 envelope。
