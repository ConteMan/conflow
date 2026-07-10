# Roadmap

GitHub Issues / Milestones 跟踪具体工作；本文维护范围边界。

## M1：基础骨架

- Go CLI：`version`、`init`、`serve`、`validate`。
- Project / Environment 清单和基础 Pack 注册表。
- React + Vite + Tailwind + Base UI 技术基础。
- 文档、ADR、CI、GoReleaser、公开开源治理。

## M2：移动广告配置包

- 广告位、频控策略、开关、环境 unit 绑定。
- 业务表单、引用关系、影响范围和风险提示。
- `plan` 与可审阅构建产物。

## M3：Git 与 Firebase

- 既有 Git JSON 源适配器。
- Firebase 模板拉取、合并、ETag、`validate_only`、发布和回滚。
- 默认值下载、发布审计与条件值阻断。

## M4：工作流完善

- Git 分支 / 提交 / PR 辅助。
- 更完整的配置包 SDK、迁移和文档适配器。
- 发布前后证据与通知扩展。
- 已签名分发与自更新策略。

## 不在 v1

- 云端多租户托管服务。
- 通用低代码字段 / 规则编辑器。
- 任意脚本执行或用户自定义代码。
- 多云 Provider 的一次性全覆盖。
- 替代 Firebase Console 的实验、个性化或 Analytics 能力。
