# UI 任务流与状态矩阵

> 归属：Spec 012。本目录是 UI 结构设计的事实源之一：原型和实现不得与这里的状态矩阵冲突；发现冲突先改文档再改图。

状态矩阵不是后端合同。凡流程依赖服务端行为，必须引用已合并的 Spec / OpenAPI，或登记到 [UI 假设与后端合同缺口](../contract-gaps.md)；缺少记录时先补合同，不让 UI 与后端分别猜测。

## 文件

| 文件 | 内容 |
|---|---|
| [pm-main-path.md](pm-main-path.md) | PM 端到端主路径：修改频控 → 校验 → Plan → 发布 Production |
| [01-project-environment-switch.md](01-project-environment-switch.md) | 项目加载与环境切换 |
| [02-placement-editing.md](02-placement-editing.md) | 广告位与频控编辑 |
| [03-validation.md](03-validation.md) | 校验与发布就绪度 |
| [04-plan-diff.md](04-plan-diff.md) | Plan 构建与语义 Diff |
| [05-production-release.md](05-production-release.md) | Production 发布与回滚入口 |

## 状态矩阵约定

每个任务流覆盖八类状态：**入口、正常、空、加载、错误、冲突、危险确认、成功**。约定：

- 「错误」区分*可行动错误*（绑定字段/实体，给出下一步）与*系统错误*（重试与诊断入口）；不允许只出 toast。
- 「冲突」指并发或外部修改导致的 `412 revision_mismatch` / 远端 ETag 变化，UI 必须提供重载与对比入口，禁止静默覆盖。
- 「危险确认」只出现在删除、Production 发布、回滚和条件值冲突等减速流程；普通保存不加确认。
- 状态命名与 [HTTP API 规范](../../http-api.md) 的错误码对应；矩阵中引用错误码时以 OpenAPI 为准。

## 术语原则

面向 PM 的界面文案不出现 Pack / Adapter / Provider / revision 等实现术语；对应的用户语言在各矩阵「文案基调」小节中定义。

技术字段保持 `baseline`、`environment_override`、`draft` 等稳定命名；PM 文案统一映射为「通用值」「此环境专属值」「未发布修改」。组件不得自行创建同义词。
