# Conflow

**中文** | [English](README.en.md)

> 本地优先的 ConfigOps 工作台：通过业务表单、校验、差异和受控发布管理应用配置。

Conflow 是一个 Go 单二进制工具，同时提供 CLI 和本地 Web GUI。它将配置表达为广告位、频控策略和功能开关等业务对象，而不是让团队直接复制 Firebase Remote Config 的长 JSON。

## 快速开始

```sh
git clone https://github.com/ConteMan/conflow.git
cd conflow
make bootstrap
make check

# 交互式创建项目、首个环境，并可选择稍后填写 Firebase 项目 ID
go run ./cmd/conflow init --dir ./examples/photo-editor

# 自动化场景必须显式提供创建参数；缺少必填参数会以 exit 64 结束
go run ./cmd/conflow init --non-interactive --dir ./examples/photo-editor \
  --project-id photo-editor --project-name "Photo Editor" \
  --environment-id development --environment-name Development \
  --environment-kind development --provider-project-id photo-editor-dev

go run ./cmd/conflow serve --workspace ./examples/photo-editor
```

打开终端输出的本地地址。概览页可创建更多环境；Firebase 项目 ID 可先留空，但连接或拉取前必须在环境管理中补齐。

## 连接 Firebase

服务账号 JSON 永远保留在本机路径，Conflow 只在已忽略的 `.conflow/` 本地状态中保存路径引用。GUI 的 Firebase 连接卡会在提交后清空输入，并仅显示 `…/firebase.json` 一类脱敏尾部。

```sh
go run ./cmd/conflow provider connect --workspace ./examples/photo-editor \
  --environment development --path "$HOME/.config/conflow/firebase.json"

go run ./cmd/conflow pull --workspace ./examples/photo-editor --environment development
```

不要把服务账号 JSON、访问令牌或绝对凭据路径提交到仓库或写入日志。

## 开发

```sh
make web-dev       # Vite 开发服务器
make web-build     # 构建 React UI 并同步为 Go 嵌入资源
make test
make check
```

前端使用 React、TypeScript、Tailwind 与 shadcn/ui 的 Base UI primitives；Node 只参与开发构建，发布物仍是单一 Go 二进制。

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
