# Spec 002：项目、环境与 API 基础

> 状态：已实现（2026-07-10）
> 依赖：Spec 001

> 合同增补（2026-07-10）：API `0.4.0` 已定义环境身份与 manifest 冲突快照；本 PR 只锁定合同，Go 运行时对齐由后续实现 PR 完成。

## 目标

让 Go 应用服务、CLI 和 React GUI 使用同一套项目/环境用例，并建立业务 API 的统一 envelope、错误映射、revision 与安全中间件。

## 范围

- 引入应用服务层，Handler 和 CLI 不直接编排文件读写。
- 读取、创建、更新项目资料与环境；环境 ID 创建后不可原地改名。
- 环境使用显式 `name` 与 `kind`；`kind` 是 Production 风险身份的服务端事实，创建后与 ID 一样不可修改。
- Project 资源承载项目级发布确认策略；它与环境身份、风险分析和 Release 资源使用不同的职责边界。
- 项目清单外部变化检测和单调 revision。
- API request ID、`Cache-Control: no-store`、JSON 未知字段拒绝、错误 envelope。
- loopback、Host、Origin 和 Content-Type 安全校验。
- `serve --address` 在 Spec 002 仅接受 loopback host。
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

## 实现说明

- 前端类型由 `openapi-typescript` 7.13.0 从 `api/openapi.yaml` 生成。它只作为前端开发依赖，用于消除手写 TypeScript DTO 漂移；不会进入 Go 二进制运行时。
- 项目清单是 Spec 002 的单一 revision 域：项目、环境集合和单个环境修改共享 manifest revision。
- `Environment.kind` 固定为 `development`、`staging`、`production`、`custom`；允许多个同类环境。UI 不得从环境 ID、名称、Provider project ID 或确认开关推断类别。
- manifest revision 冲突返回原子的 `current_state`（项目 + 环境集合）；`current_revision` 与响应头 ETag 属于同一 Store 快照。该状态供 UI 对照，不授权自动重试或覆盖。
- 本合同在稳定 v1 发布前直接纳入 manifest version 1；实验性旧清单必须显式补齐 `name` 与 `kind`，不做隐式类别推断。
- 合同增补（待实现）：`Project.release_confirmation_policy.production_low_risk_mode` 是 `environment_id|acknowledgement`，默认 `environment_id`。为保留既有 manifest/API 客户端兼容性，读响应可以省略该字段来表示默认值，写请求省略则保持当前策略；新建或迁移后的 manifest 应显式写入默认值。它只允许放宽低风险 Production Plan 的环境 ID 输入要求；一般确认、高风险逐项确认和 blocking 规则仍由 Spec 008/010 的服务端权威结果决定。该字段与 Project 资料共用 manifest revision / ETag 域。`Environment.publish.requires_confirmation` 保留为兼容性字段，只表示一般确认是否适用，不能表达强度或覆盖项目策略。

## 示例

读取 GUI 启动上下文：

```http
GET /api/v1/bootstrap
Host: 127.0.0.1:9010
```

```json
{
  "data": {
    "project": {
      "id": "photo-editor",
      "name": "Photo Editor",
      "pack_ref": "mobile-ad-monetization/v1",
      "source_type": "managed-file",
      "release_confirmation_policy": {
        "production_low_risk_mode": "environment_id"
      }
    },
    "environments": [
      {
        "id": "development",
        "name": "Development",
        "kind": "development",
        "provider": {"type": "firebase-remote-config", "project_id": "photo-editor-dev"},
        "publish": {"requires_confirmation": false}
      }
    ],
    "capabilities": {"project_edit": true, "environment_manage": true}
  },
  "meta": {"request_id": "req_01J...", "revision": 1}
}
```

修改项目资料：

```http
PUT /api/v1/project
Content-Type: application/json
If-Match: "1"

{"id":"photo-editor","name":"Photo Editor Pro"}
```

旧 revision 修改项目时的冲突响应：

```http
HTTP/1.1 412 Precondition Failed
ETag: "2"
Content-Type: application/json
```

```json
{
  "error": {
    "code": "revision_mismatch",
    "message": "项目已被其他操作修改，请查看最新内容",
    "request_id": "req_01J...",
    "current_revision": 2,
    "current_state": {
      "project": {
        "id": "photo-editor",
        "name": "Photo Editor from Git",
        "pack_ref": "mobile-ad-monetization/v1",
        "source_type": "managed-file"
      },
      "environments": [
        {
          "id": "development",
          "name": "Development",
          "kind": "development",
          "provider": {"type": "firebase-remote-config", "project_id": "photo-editor-dev"},
          "publish": {"requires_confirmation": false}
        }
      ]
    }
  }
}
```

## 验收

- OpenAPI 覆盖全部端点，并生成可被 React 引用的类型。
- Environment 读写合同要求显式 `name` / `kind`；Production 状态只由 `kind` 判定。
- Handler 测试覆盖 CRUD、未知字段、重复环境、revision 冲突、非法 Origin 和非 loopback Host。
- 外部修改 `.conflow/project.yaml` 后，旧 revision 的写请求返回 `412 revision_mismatch`；响应 ETag、`current_revision` 与原子 `current_state` 相互一致。
- CLI 现有 `init` / `validate` 行为不回归，`make check` 通过。
