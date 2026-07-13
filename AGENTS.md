# AGENTS.md — Conflow 工作指引

Conflow 是一个面向产品、运营和研发的本地 ConfigOps 工具：Go 单二进制同时提供 CLI 和本地 Web GUI，用配置包、源适配器和发布适配器安全管理 Remote Config。

## 语言

- 与维护者的对话、任务清单、设计文档、ADR、Spec、GitHub Issue 标题与正文：中文。
- 代码、代码注释、PR 正文与模板：英文；提交信息：中文。
- README 与 CONTRIBUTING 保持中英文镜像；中文为默认入口。

## Issue 跟踪

问题、设计差距和改进需求统一在 **GitHub Issues** 记录，不在仓库文件中维护。

### Label 含义

| Label | 用途 |
|-------|------|
| `bug` | 功能行为与预期不符 |
| `design-gap` | 实现与 UI 设计稿（`.pen` 文件）不一致 |
| `enhancement` | 改进或新功能，范围可在现有 Spec 内消化 |
| `spec-needed` | 需要新建 Spec 才能动工的大型需求 |

### 工作流

- **发现问题** → 先搜索是否已有 Issue，重复则评论不新建。
- **新建 Issue** → 标题和正文使用中文；正文描述现象、期望行为、影响范围和参考资料，必要时附截图或命令输出；打对应 Label；影响大的挂 Milestone。
- **开始处理** → 在 PR 描述中写 `Closes #N`，合并后 Issue 自动关闭。
- **Spec-needed** → 先由维护者拍板方向，再写 Spec，再实现；不允许跳过 Spec 直接实现大型功能。
- **设计差距** → 分两类：实现未覆盖设计（补代码）；实现超前设计（更新设计稿）。两者都在 Issue 中标注类别。

## 入项阅读顺序

1. [docs/design/architecture.md](docs/design/architecture.md)
2. [docs/design/config-model.md](docs/design/config-model.md)
3. [docs/decisions/README.md](docs/decisions/README.md)
4. [docs/roadmap.md](docs/roadmap.md)
5. [docs/specs/README.md](docs/specs/README.md)

## 硬规则

1. 文档先行：架构、配置模型、公开 CLI 或 API 变化必须同步设计文档或 ADR。
2. 线上 Firebase 是发布目标与审计对象，不是未受控的唯一事实源；禁止静默覆盖条件值。
3. 配置包定义业务规则；源适配器只负责读写格式；发布适配器只负责目标平台交互，禁止跨层耦合。
4. 所有本地 HTTP 服务默认只监听 `127.0.0.1`；认证凭据不得写入仓库、日志或发布产物。
5. React 前端使用 Vite、TypeScript、Tailwind、shadcn/ui（Base UI primitives）。构建产物必须嵌入 Go 二进制，运行时不得依赖 Node。
6. 新增直接依赖需写入 ADR 或 Spec 的选型说明，保持依赖最少。
7. 提交前必须通过 `make check`；CI 红灯不得合并。
8. Commit 使用中文 Conventional Commits；改动较多时在提交正文列举主要内容；一个 commit 只做一个逻辑变更。

## 目录

- `cmd/conflow/`：二进制入口与前端资产同步工具。
- `internal/cli/`：Cobra 命令。
- `internal/project/`：项目清单、环境和校验。
- `internal/packs/`：配置包注册表与业务规则。
- `internal/source/`：Git / 托管文件等源适配器。
- `internal/provider/`：Firebase 等发布适配器。
- `internal/server/`：本地 HTTP API 与 Web UI 托管。
- `internal/webui/`：嵌入式前端构建产物。
- `web/`：React 源码。
- `docs/`：设计、ADR、Spec、路线图。
