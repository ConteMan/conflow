# 架构总览

> 状态：已定稿（2026-07-10）。变更公开边界、分层或运行模型前必须更新本文或新增 ADR。

## 定位

Conflow 是本地优先的 ConfigOps 工作台，不是 Firebase 控制台替代品，也不是任意 JSON 的低代码编辑器。

它把“可发布配置”表达成可验证的业务实体，并用受控流程发布到 Firebase Remote Config 等目标。首个业务域是移动广告配置：广告位、频控策略、广告开关和环境 unit 绑定。

## 运行模型

```text
conflow serve
  ├─ Go HTTP API，默认仅监听 127.0.0.1
  ├─ 嵌入式 React 静态资源
  ├─ 项目 / 环境 / 配置包服务
  └─ 源适配器与发布适配器

conflow validate / plan / publish
  └─ 复用完全相同的 Go 内核，无需浏览器
```

React 只负责展示、输入和调用本地 API；业务校验、模板构建和发布安全检查都在 Go 中执行。Node 只参与开发期前端构建，发布二进制通过 `go:embed` 携带前端资源。

前后端通过版本化的同源 HTTP API 交互，契约见 [http-api.md](http-api.md) 和仓库根的 [`api/openapi.yaml`](../../api/openapi.yaml)。CLI 不通过 HTTP 回环调用，而是与 HTTP Handler 复用相同的 Go 应用服务。

## 四层模型

```text
Config Pack
  定义业务实体、表单元数据、不可放宽规则、编译器与风险模型
       ↓
Project + Environment
  定义具体 App、dev/staging/prod 覆盖、发布策略和凭据引用
       ↓
Source Adapter
  定义从 Git JSON、托管文件或导入模板读取 / 写回的方式
       ↓
Provider Adapter
  定义如何与 Firebase 等发布目标拉取、校验、发布和回滚
```

源文件不是业务规则。业务规则属于 Config Pack；源适配器只负责格式转换与持久化。

## 包边界

| 包 | 职责 |
|---|---|
| `cmd/conflow` | 主程序入口 |
| `cmd/embedui` | 将 Vite 产物同步为可嵌入资源 |
| `internal/cli` | `init`、`serve`、`validate` 等 Cobra 命令 |
| `internal/project` | 项目清单、环境、基础校验 |
| `internal/packs` | 版本化、纯声明式的配置包契约、内置注册表与未来编译 / 校验 / 语义 diff 边界 |
| `internal/source` | 源适配器接口与实现 |
| `internal/provider` | 发布适配器接口与实现 |
| `internal/server` | 本地 HTTP API 与静态 UI 托管 |
| `internal/webui` | `go:embed` 资源 |
| `web` | React + Vite 源码 |

## 安全边界

- HTTP 只绑定 loopback；禁止默认暴露到局域网。
- 凭据进入系统钥匙串或环境变量；项目清单只保存引用名。
- 发布必须确认目标项目、环境、计划摘要与 ETag。
- 受管理 Firebase 参数出现未建模条件值时，默认阻止发布。
- Firebase 当前模板是发布基线与审计快照，不自动成为业务配置的唯一事实源。
