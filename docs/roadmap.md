# Roadmap

> GitHub Issues / Milestones 跟踪任务状态；本文维护 v1 边界、Spec 编排与里程碑完成标准。修改 v1 In / Out 或 Spec 依赖需要维护者确认。

## v1 产品闭环

```text
创建项目与环境
  → 选择移动广告 Pack
  → 编辑广告位、频控、开关和环境 unit
  → 保存到 Managed File 或 Git JSON
  → 校验、生成语义 Diff 与发布 Plan
  → 拉取并预校验 Firebase 模板
  → 带 ETag 和幂等键发布
  → 下载默认值、审计和回滚
```

CLI 与 React GUI 必须复用同一 Go 应用服务；前后端契约以 [HTTP API 规范](design/http-api.md) 与 [`api/openapi.yaml`](../api/openapi.yaml) 为准。

## 里程碑

| 里程碑 | Specs | 交付内容 | 完成标准 |
|---|---|---|---|
| M1 基础骨架 | [001](specs/001-foundation.md) | Go CLI、本地 HTTP、嵌入 React、CI、GoReleaser | `init` / `validate` / `serve` 可用，已完成 |
| M2 配置核心 | [002–005](specs/README.md) | 项目环境、API 基础、Pack 契约、草稿分层、Managed File | 新 App 不写代码即可创建项目、环境、草稿并安全保存 |
| M3 广告与计划 | [006–008](specs/README.md) | 移动广告实体、校验、语义 Diff、风险和 Plan | PDF Launcher 等价 fixture 可表达、校验并生成可审阅计划 |
| M4 Firebase 闭环 | [009–011](specs/README.md) | 认证、拉取、预校验、发布、默认值、审计和回滚 | 测试 Firebase 项目完成 ETag 保护的发布与回滚 |
| M5 PM Web 体验 | [012–015](specs/README.md) | Pencil 原型探索、设计基线、项目壳、编辑器、发布 UI | PM 不接触 JSON 完成主流程；异常与冲突 E2E 通过 |
| M6 Git、自动化与分发 | [016–019](specs/README.md) | Git JSON 迁移、审阅工作流、CLI/CI、签名和更新 | 既有仓可接入，三平台 release 可验证安装与升级 |

M5 的设计探索不等待 M4 完成：Spec 012 在 Spec 006 领域合同稳定后启动；Spec 013 可与 Firebase Provider 并行。生产发布 UI 仍必须等待 Spec 009–011 的真实 API。

## 近期执行顺序

1. Spec 002、003 与 Spec 012 已完成；混合模式、完整设计系统和 Spec 013–015 页面基线已冻结，后续按已合并合同实现，不重新开启无边界视觉探索。
2. 先关闭 [`UI-API-001`](design/ui/contract-gaps.md#spec-013-开工门槛) 与 `UI-API-002`，再实现 Spec 013 的 Production 状态和冲突交互；应用壳的路由与布局可先并行。
3. 先合并 Spec 004 contract-only PR，再实现草稿分层；随后 Spec 005、006 可并行。
4. Spec 006/007 的合同与共享 fixture 合并后实现领域校验，并关闭 Spec 014 的合同门槛。
5. Spec 008–011 与对应 contract-only PR 形成 Plan、Firebase、发布和回滚闭环；Spec 014 可按依赖并行。
6. Spec 015 合并完整 PM 发布体验；Spec 016–018 补既有仓与自动化。
7. Spec 019 在 v1 功能冻结后完成正式分发。

## v1 In

- 单进程、单 workspace；项目内支持多个环境。
- 内置 `mobile-ad-monetization/v1` Pack。
- Managed File 与一个可配置 Git JSON Source Adapter。
- Firebase Remote Config client template：拉取、合并、预校验、发布、版本、默认值和回滚。
- 业务校验、语义 Diff、风险分析、不可变 Plan 和本地审计。
- 本地 React GUI 与功能对齐的 CLI / JSON 自动化输出。
- macOS、Linux、Windows 单二进制发布。

## v1 Out

- 云端多租户托管、组织账号、多人审批和实时协作。
- 通用低代码 Pack / 字段 / 规则编辑器。
- 任意脚本、CEL / Rego 或用户插件执行。
- Firebase A/B Testing、Personalization、Analytics 和服务端模板。
- 一次性支持多云 Provider、配置市场和远程插件下载。
- 定时生产发布、静默自动更新、强制更新和遥测。

## v1 总验收

- 新 App 可通过 GUI 创建项目、dev/staging/production，配置并发布移动广告 Remote Config。
- 既有 PDF Launcher 等价 JSON 可导入、round-trip、评审并安全发布。
- 多标签页、源文件外部修改和 Firebase ETag 变化均不会静默覆盖。
- 生产发布有确认、幂等、审计、默认值下载和可验证回滚。
- GUI、CLI 和 API 对相同输入产生相同诊断、Plan digest 和发布结果。
- `make check`、契约检查、关键 E2E 和三平台 release 验证全部通过。
