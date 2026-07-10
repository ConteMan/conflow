# 维护与发版

## 日常质量门禁

提交前运行：

```sh
make check
git diff --check
```

`make check` 会构建 React UI、同步 Go 嵌入资源、确认嵌入资源已提交，运行 `gofmt`、Go tests、`go vet`、二进制构建和本地 Playwright E2E。

GitHub Actions 运行 `make check-ci`：覆盖除 Playwright 以外的同一组门禁，不安装 Chromium 或系统浏览器依赖。涉及 UI、API client 或交互状态的 PR 必须在本地运行完整 `make check`，并在 PR 中记录 E2E 结果。

## 版本与变更记录

- 版本遵循 Semantic Versioning，Git tag 格式为 `vX.Y.Z`。
- 用户可见变更写入 `CHANGELOG.md` 的 `Unreleased`；发版时移入对应版本和日期。
- 破坏性 CLI、配置包或公开 HTTP API 变化须增加 ADR 或更新既有 ADR 的取代决策。

## 发版流程

1. 确认 `main` 的 CI 为绿。
2. 更新 `CHANGELOG.md`、双语 README（如有用户可见变化）和相关文档。
3. 本地运行 `make check`。
4. 创建并推送带签名策略由维护者决定的 `vX.Y.Z` tag。
5. GitHub Actions 使用 GoReleaser 构建 macOS、Linux、Windows 的 archive、校验和与 GitHub Release。
6. 在干净机器或对应平台验证 `conflow version`、`init`、`validate`、`serve`。

## 更新策略

M1 不实现二进制自更新。用户通过 GitHub Releases 下载新 archive；后续在供应链签名、校验和验证和平台安装路径明确后，再单独设计包管理器与 `conflow upgrade`。自动更新不能绕过用户确认。

## 前端资产

`web/` 是源代码，`internal/webui/assets/` 是必须提交的生成物。React 依赖不会随发布物分发；GoReleaser 构建的二进制内嵌已同步资产。
