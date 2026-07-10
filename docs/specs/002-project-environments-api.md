# Spec 002：项目、环境与 API 基础

> 状态：待实现  
> 依赖：Spec 001

## 目标

让 Go 应用服务、CLI 和 React GUI 使用同一套项目/环境用例，并建立业务 API 的统一 envelope、错误映射、revision 与安全中间件。

## 范围

- 引入应用服务层，Handler 和 CLI 不直接编排文件读写。
- 读取、创建、更新项目资料与环境；环境 ID 创建后不可原地改名。
- 项目清单外部变化检测和单调 revision。
- API request ID、`Cache-Control: no-store`、JSON 未知字段拒绝、错误 envelope。
- loopback、Host、Origin 和 Content-Type 安全校验。
- OpenAPI 契约校验与 TypeScript 类型生成流程；新增前端开发依赖必须在本 Spec 实现说明中记录版本与理由。

## API

- `GET /api/v1/bootstrap`
- `GET /api/v1/project`
- `PUT /api/v1/project`
- `GET /api/v1/environments`
- `POST /api/v1/environments`
- `GET /api/v1/environments/{environment_id}`
- `PUT /api/v1/environments/{environment_id}`
- `DELETE /api/v1/environments/{environment_id}`

写请求遵循 [HTTP API 规范](../design/http-api.md) 的 `If-Match` 和错误约定。

## 非范围

- Pack 业务字段、草稿配置、Firebase 网络调用。
- 多项目同时加载；一个 `serve` 进程仍对应一个 workspace。

## 验收

- OpenAPI 覆盖全部端点，并生成可被 React 引用的类型。
- Handler 测试覆盖 CRUD、未知字段、重复环境、revision 冲突、非法 Origin 和非 loopback Host。
- 外部修改 `.conflow/project.yaml` 后，旧 revision 的写请求返回 `412 revision_mismatch`。
- CLI 现有 `init` / `validate` 行为不回归，`make check` 通过。
