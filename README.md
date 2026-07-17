# Conflow

**中文** | [English](README.en.md)

> 本地优先的 ConfigOps 工作台：通过业务表单、校验、语义差异和受控发布管理 Firebase Remote Config。

Conflow 是一个 Go 单二进制工具，同时提供 CLI 和本地 Web GUI。它将远程配置表达为广告位、频控策略、功能开关、自定义参数等业务对象，而不是让团队直接编辑 Firebase 控制台里的长 JSON——配置的唯一事实源保存在你的本地工作区（可纳入 Git 管理），Firebase 只是发布目标和审计对象。

**核心能力**

- **业务化编辑**：以领域表单维护配置实体，字段带校验、说明和引用完整性检查
- **发布计划**：每次发布前生成语义差异与风险分析，精确到字段级变更和受影响的远端参数
- **受控发布**：ETag 并发保护、validate-only 预检、生产环境显式确认，杜绝手滑
- **审计与回滚**：不可变发布记录，任意成功发布可一键回滚并回验
- **多环境**：每个环境绑定独立 Firebase 项目，业务配置跨环境单源、发布节奏各自独立
- **本地优先**：数据在本地工作区，凭据永不入库；无服务端依赖，单二进制开箱即用

## 安装

**macOS（Homebrew）**

```sh
brew install ConteMan/tap/conflow
```

**Windows（Scoop）**

```sh
scoop bucket add conflow https://github.com/ConteMan/homebrew-tap
scoop install conflow
```

**直接下载**

从 [GitHub Releases](https://github.com/ConteMan/conflow/releases/latest) 下载对应平台的 tar.gz/zip，解压后将 `conflow` 放入 `$PATH`。

> macOS 未签名提示：直接下载的二进制会被 Gatekeeper 标记。首次运行前执行一次：
> ```sh
> xattr -dr com.apple.quarantine conflow
> ```
> Homebrew 安装会自动处理，无需手动操作。

**更新**

```sh
conflow update          # 更新（直接安装方式）
brew upgrade conflow    # Homebrew
scoop update conflow    # Scoop
```

## 快速开始

```sh
# 交互式创建项目工作区。默认创建 development 和 production 两个环境；
# Firebase 项目 ID 可稍后在界面中补齐。
conflow init --dir ./my-app-config

# 启动本地 Web GUI（只监听 127.0.0.1）
conflow serve --workspace ./my-app-config
```

打开终端输出的本地地址即可开始配置。自动化场景可使用非交互模式：

```sh
conflow init --non-interactive --dir ./my-app-config \
  --project-id my-app --project-name "My App" --json
```

## 连接 Firebase

在环境管理或概览页的 Firebase 连接卡中填写项目 ID 与服务账号 JSON 路径，或使用 CLI：

```sh
conflow provider connect --workspace ./my-app-config \
  --environment development --path "$HOME/.config/conflow/firebase.json"

conflow pull --workspace ./my-app-config --environment development
```

服务账号 JSON 永远保留在本机路径，Conflow 只在已忽略的 `.conflow/` 本地状态中保存路径引用。连接会先校验文件存在、可读、JSON 格式、`type=service_account` 和必填字段；任一步失败都不会写入或覆盖已有引用。远端连通性检查在 `pull` 时进行。

**不要把服务账号 JSON、访问令牌或绝对凭据路径提交到仓库或写入日志。**

## 日常工作流

配置变更遵循固定的受控流程，GUI 与 CLI 语义一致：

```
编辑配置 → 校验 → 构建发布计划（语义差异 + 风险）→ 发布 → 发布后回验
```

```sh
conflow validate  --workspace . --environment development   # 校验
conflow plan      --workspace . --environment development   # 构建不可变发布计划
conflow publish   --workspace . --environment development   # 发布（生产环境需显式确认）
conflow release list --workspace . --environment development # 审计记录
conflow rollback  --workspace . --environment development --release <id> --confirm --idempotency-key <key>
```

其他常用命令：`conflow import`（跨工作区配置导入）、`conflow source`（配置源检查）、`conflow environment`（环境管理）。所有命令支持 `--json` 输出稳定的自动化信封，适合 CI 集成；完整说明见 `conflow --help`。

## 开发者指南

以下内容面向参与 Conflow 本身开发的贡献者。

**环境要求**：Go 1.25+、Node.js 24+（仅开发构建使用，发布物是单一 Go 二进制）。

```sh
git clone https://github.com/ConteMan/conflow.git
cd conflow
make bootstrap     # 安装依赖
make check         # 完整检查：合同、构建、Go 测试、e2e

# 常用目标
make web-dev       # Vite 开发服务器
make web-build     # 构建 React UI 并同步为 Go 嵌入资源
make test          # Go 测试
make check-ci      # CI 检查集（不含 e2e）
```

前端使用 React、TypeScript、Tailwind 与 Base UI primitives；表格基于 TanStack Table。接口契约以 `api/openapi.yaml` 为准，改动后运行 `make api-generate` 同步 TypeScript 类型。功能开发以 [实现规格（Spec）](docs/specs/README.md) 为单元，一份 Spec 一个聚焦 PR。

**文档**

- [架构总览](docs/design/architecture.md)
- [配置模型](docs/design/config-model.md)
- [前后端 HTTP API 规范](docs/design/http-api.md)
- [实现规格清单](docs/specs/README.md)
- [UI 设计方向与原型流程](DESIGN.md)
- [架构决策记录](docs/decisions/README.md)
- [路线图](docs/roadmap.md)
- [贡献指南](CONTRIBUTING.md)

## 许可证

[MIT](LICENSE)
