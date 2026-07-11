# ADR-006：Plan Operation 与项目级确认策略

> 状态：提议（待维护者终审，2026-07-11）

## 背景

Plan 同时需要远端读取、Pack 编译、语义差异、风险和确认要求。UI 的 Plan 入口会自动触发构建，且页面刷新、SSE 断开或 Firebase 短暂不可用时仍必须能恢复权威状态。维护者已收口 Production 确认强度为项目级设置：默认输入环境 ID，低风险高频发布可放宽为勾选确认，高风险项不能跳过。

## 候选方案

1. `POST :plan` 始终异步，返回 Operation；确认策略放入 Project。
2. `POST :plan` 在本地编译快时同步返回、需要远端时才返回 Operation；确认策略放入每个 Environment。
3. UI 先显式 pull，再同步构建 Plan；确认策略单独成为一个配置资源。

## 推荐决策

推荐方案 1：`POST :plan` 无条件 `202 + Operation`，成功 result 指向不可变 Plan；GET Operation 是断线/刷新恢复权威，SSE 只增强。Plan 在远端不可达时可以是 `preview_only`，但永远不可发布。风险、阻断项和 confirmation requirements 一律由服务端计算。

确认策略作为 `Project.release_confirmation_policy.production_low_risk_mode` 与 Project 共用 manifest revision；默认 `environment_id`，仅可放宽低风险 Production Plan 的环境 ID 输入。高风险逐项确认、一般确认和 blocking rules 仍由 Plan requirement 决定。

## 理由

- 单一响应形状消除“缓存快时同步、网络慢时异步”的 UI 状态分叉，并保留后台恢复能力。
- Project 归属符合已收口的产品交互，也避免多个 Production 环境产生难以解释的确认强度差异。
- `preview_only` 保持本地审阅价值，但不把远端不可达伪装为可发布事实。

## 若维护者改选的代价

- 选择方案 2 会引入同步/异步双状态机，前端、CLI 和测试需各自处理两种结果，且 Operation 历史不完整。
- 选择方案 3 会把 Plan 的远端依赖编排交给 UI，CLI/GUI 容易出现不同重试路径；独立策略资源还会额外增加 revision 域与冲突界面。
- 将策略放到 Environment 能支持未来差异，但推翻“项目级”已收口决策，并增加 PM 对当前规则来源的理解成本。

## 后果

- 实现必须为 Plan、remote pull/validate、publish、rollback preview/rollback 使用同一 Operation 读模型和结构化 failure。
- `Environment.publish.requires_confirmation` 不再承担确认强度含义，保持兼容展示；以后若移除需显式 API major/迁移决策。
- 本 ADR 不授权运行时实现；维护者终审后，Spec 008–011 实现 PR 才能落地。
