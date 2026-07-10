# Conflow

**中文** | [English](README.en.md)

> 本地优先的 ConfigOps 工作台：用业务表单、校验、差异和安全发布流程管理应用配置。

Conflow 是一个 Go 单二进制工具，同时提供 CLI 和本地 Web GUI。它让 PM、运营和研发以广告位、频控策略、功能开关等业务对象管理配置，而不是直接复制 Firebase Remote Config 的长 JSON。

## 核心能力

- **项目与环境**：一个 App 下管理开发、测试、生产环境及其发布保护。
- **配置包**：用受版本管理的业务模型定义字段、规则、风险和编译方式。
- **源适配器**：兼容 Git JSON、Conflow 托管文件和导入迁移；Git 仍可作为事实源。
- **发布适配器**：以 Firebase Remote Config 为首个目标，使用模板合并、ETag、预校验和回滚记录。
- **本地 GUI + CLI**：浏览器完成日常操作，CLI 可进入 Git、CI 和自动化流程。

## 当前状态

项目处于 M2 配置核心阶段：基础骨架、项目/环境 API 与版本化 Config Pack 契约已完成。首个配置包为 `mobile-ad-monetization/v1`，首个发布目标为 Firebase Remote Config。详细范围见 [路线图](docs/roadmap.md)。

## 快速开始

```sh
git clone https://github.com/ConteMan/conflow.git
cd conflow
make bootstrap
make check

# 创建一个本地管理项目，然后打开 GUI
go run ./cmd/conflow init --dir ./examples/photo-editor
go run ./cmd/conflow serve --workspace ./examples/photo-editor
```

## 开发

```sh
make web-dev       # Vite 开发服务器
make web-build     # 构建 React UI 并同步为 Go 嵌入资源
make test
make check
```

前端使用 React、TypeScript、Tailwind 与 shadcn/ui 的 Base UI primitives；Node 只用于开发构建，最终发布物是单一 Go 二进制。

## 文档

- [架构总览](docs/design/architecture.md)
- [配置模型](docs/design/config-model.md)
- [前后端 HTTP API 规范](docs/design/http-api.md)
- [UI 设计方向与原型流程](DESIGN.md)
- [架构决策记录](docs/decisions/README.md)
- [路线图](docs/roadmap.md)
- [贡献指南](CONTRIBUTING.md)

## 许可证

[MIT](LICENSE)
