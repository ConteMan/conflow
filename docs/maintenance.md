# 维护与发版

## 日常质量门禁

提交前运行：

```sh
make check
git diff --check
```

`make check` 会构建 React UI、同步 Go 嵌入资源、确认嵌入资源已提交，运行 `gofmt`、Go tests、`go vet`、二进制构建和本地 Playwright E2E。

GitHub Actions 运行 `make check-ci`：覆盖除 Playwright 以外的同一组门禁，不安装 Chromium 或系统浏览器依赖。涉及 UI、API client 或交互状态的 PR 必须在本地运行完整 `make check`，并在 PR 中记录 E2E 结果。

## CI 自动化示例

CI 应使用 `--json` 读取稳定的 `data` / `error.code` envelope，并根据退出码处理失败：`0` 成功、`1` 校验失败、`2` 阻断校验、`3` 冲突、`4` Provider 失败、`64` 使用错误。凭据只通过 GitHub Secrets 注入为环境变量或由项目配置引用的文件路径，不能放入命令行参数、日志或构建产物。

只校验配置，不访问发布接口：

```yaml
name: validate
on: [pull_request]
jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: {go-version-file: go.mod}
      - run: go run ./cmd/conflow validate --workspace . --environment production --json
```

生成可供人工审阅的 Plan artifact；此工作流不发布：

```yaml
name: plan
on: [pull_request]
jobs:
  plan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: {go-version-file: go.mod}
      - run: go run ./cmd/conflow plan --workspace . --environment production --json --output plan-artifacts
      - uses: actions/upload-artifact@v4
        with: {name: conflow-plan, path: plan-artifacts}
```

发布仅在 GitHub Environment 的人工 approval 后执行。凭据引用由部署前的受控步骤写入项目配置；敏感值来自 GitHub Secrets，不作为 CLI 参数传入：

```yaml
name: publish-after-approval
on:
  workflow_dispatch:
    inputs:
      plan_id:
        required: true
        type: string
jobs:
  publish:
    environment: production
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: {go-version-file: go.mod}
      - name: Configure credential reference
        env:
          FIREBASE_CREDENTIALS_JSON: ${{ secrets.FIREBASE_CREDENTIALS_JSON }}
        run: |
          install -d -m 700 "$RUNNER_TEMP/conflow" .conflow/provider
          printf '%s' "$FIREBASE_CREDENTIALS_JSON" > "$RUNNER_TEMP/conflow/firebase.json"
          chmod 600 "$RUNNER_TEMP/conflow/firebase.json"
          printf 'credentials_path: %s\n' "$RUNNER_TEMP/conflow/firebase.json" > .conflow/provider/production.yaml
      - name: Publish reviewed plan
        env:
          CONFLOW_IDEMPOTENCY_KEY: ${{ github.run_id }}-${{ github.run_attempt }}
          PLAN_ID: ${{ inputs.plan_id }}
        run: go run ./cmd/conflow publish --workspace . --environment production --plan "$PLAN_ID" --confirm --idempotency-key "$CONFLOW_IDEMPOTENCY_KEY" --json
```

上述示例刻意不提供默认的自动 Production 发布触发器；`environment: production` 必须在 GitHub repository settings 中配置 required reviewers。

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
