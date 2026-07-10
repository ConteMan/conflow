# 参与贡献 Conflow

**中文** | [English](CONTRIBUTING.en.md)

Conflow 采用文档先行的开发方式：[`docs/`](docs/README.md) 是架构与范围的事实源，[`AGENTS.md`](AGENTS.md) 是编码入口。

## 开发环境

- Go 1.25+
- Node.js 24+ 与 npm
- Playwright Chromium（`make bootstrap` 自动安装，仅用于前端 E2E）

```sh
make bootstrap
make check
```

## 规则

- 从 `main` 创建 `feat/<name>`、`fix/<name>` 或 `docs/<name>` 分支。
- 改变架构、配置 schema、公开 CLI / HTTP API 时，同一 PR 更新设计文档或 ADR。
- 用户可见变更登记到 [CHANGELOG.md](CHANGELOG.md) 的 `Unreleased`。
- 提交遵循 Conventional Commits，使用英文，例如 `feat(cli): add project validation`。
- 合并前必须通过 `make check`。

## 许可证

提交贡献即表示同意以 [MIT](LICENSE) 授权。
