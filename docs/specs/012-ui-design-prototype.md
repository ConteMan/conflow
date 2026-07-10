# Spec 012：UI 设计探索与设计基线

> 状态：已实现  
> 依赖：Spec 002、003、006

## 目标

在实现完整 GUI 前验证 PM 主路径和信息架构，形成可评审、可版本化的设计基线，解决以往“直接实现后仍不满意”的问题。

首轮已经完成两个结构方向的探索并采纳混合模式。第二阶段将该结论扩展为完整页面、异常状态和 reusable 设计系统，作为 Spec 013–015 的实现基线；范围见 [`screen-inventory.md`](../design/ui/screen-inventory.md)。

第二阶段于 2026-07-10 完成：最终 Pencil 文件包含 Foundations、27 个 reusable 组件、Spec 013–015 的 22 个桌面画板和 3 个窄屏画板；评审资产包含 28 张 PNG 与一份 28 页 PDF。

## 范围

- 维护根目录 [`DESIGN.md`](../../DESIGN.md)。
- 为项目/环境切换、广告位编辑、校验、Plan 和 Production 发布建立状态矩阵。
- 使用 Pencil 或等价工具探索至少两个结构方向；工具选择需记录导出和版本化限制。
- 使用真实规模 fixture：20+ 广告位、多环境、多条诊断和大 diff。
- 桌面主流程与窄屏只读/轻量编辑探索。
- 评审记录：选择、放弃项、原因、未决问题和后续验证方式。
- shadcn/ui + Base UI 组件清单和业务组件边界。
- 基于已采纳混合模式建立唯一最终 Pencil 事实源，旧探索文件仅保留为决策证据。
- 建立 token、基础组件、应用壳组件和 ConfigOps 业务组件，并在页面中通过实例复用。
- 覆盖 Spec 013–015 的桌面关键页面、安全边界状态与窄屏查看 / 轻量修改页面。

## 产物

- `docs/design/ui/flows/`
- `docs/design/ui/prototypes/`
- `docs/design/ui/reviews/YYYY-MM-DD-<topic>.md`
- `docs/design/ui/screen-inventory.md`
- `docs/design/ui/prototypes/conflow-ui.pen`
- 更新后的 `DESIGN.md`

## 非范围

- 完整视觉品牌、营销站和移动端发布体验。
- 在原型工具中复制最终业务规则。

## 验收

- PM 能在不理解 Pack / Adapter 术语的情况下说明如何修改频控并发布到 Production。
- 每个主流程包含加载、空、错误、冲突、危险确认和成功状态。
- 至少两个方向在结构上有真实差异，并有明确取舍记录。
- 原型结论同步回 UI Specs，未以截图替代 API 或业务契约。
- 最终文件以混合结构为唯一方向，组件可复用且主要页面通过布局检查与截图评审。
- Production、保存冲突、Plan 失效、Operation 远端状态不确定和回滚均有独立视觉证据。
- fixture 不足处明确标记合同占位，前端不得根据原型自行推导风险、来源、影响面或生命周期。
