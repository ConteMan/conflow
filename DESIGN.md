# Conflow UI Design Direction

> 状态：实现基线（2026-07-10）。本文定义产品体验、视觉系统和实现约束；UI 结构或视觉方向发生显著变化时，同步更新本文、对应 UI Spec 和最终 Pencil 事实源。
> 首轮结构探索与维护者评审已完成，混合结构已采纳；第二阶段已形成完整设计系统与 Spec 013–015 页面基线。任务流见 [docs/design/ui/flows/](docs/design/ui/flows/)，页面清单见 [screen-inventory.md](docs/design/ui/screen-inventory.md)，最终设计见 [`conflow-ui.pen`](docs/design/ui/prototypes/conflow-ui.pen)，收口记录见 [完整 UI 系统评审](docs/design/ui/reviews/2026-07-10-complete-ui-system.md)。

## 1. 设计目标

Conflow 的核心用户是需要安全修改配置的 PM、运营和研发。界面首先降低误操作和认知负担，其次才追求信息密度与效率。

用户应始终知道：

- 当前在哪个项目和环境；
- 正在修改基线还是环境覆盖；
- 改动尚未保存、已保存、已校验还是已发布；
- 本次变更影响哪些业务对象和远端参数；
- 出错后如何修复或回滚。

## 2. 体验原则

1. **环境优先可见**：项目与环境选择常驻；Production 使用持续、克制但明确的危险提示。
2. **业务对象优先**：PM 编辑广告位、频控和开关，不直接编辑 Firebase 长 JSON。
3. **渐进披露**：常用字段默认展示；RC key、源映射、原始 JSON 和 Provider 细节进入高级区。
4. **修改可解释**：每次保存、校验和发布都能回答“改了什么、影响谁、风险是什么”。
5. **错误可行动**：错误绑定具体字段或实体，并给出下一步；不只显示 toast。
6. **危险操作减速**：删除、生产发布、回滚和条件值冲突使用独立确认流程。
7. **键盘与可访问性**：表格、表单、Dialog、Combobox 和命令操作满足键盘使用；Base UI 负责原语，业务组件负责语义。

## 3. 信息架构（2026-07-10 评审采纳）

```text
全局顶栏（项目 / 环境组合选择器 + 水平导航 + 未发布修改徽标；Production 时整栏变调）
├─ 概览            项目、环境、连接状态、最近发布
├─ 配置            按实体类型 tab 切换的数据表（广告位 / 频控策略 / 功能开关 / 环境绑定）
│  ├─ 广告位编辑    独立详情页（分区表单 + 环境绑定矩阵 + 字段级三值 caption）
│  └─ 频控策略编辑  右侧抽屉（保持数据表可见，展示受影响广告位）
├─ 校验            问题按实体分组、锚点跳转（入口形态待 Spec 007 定：常驻底栏 vs 独立页）
├─ 发布计划        逐层展开树：业务变更 → 受影响广告位 → 最终写入远端参数
└─ 发布记录        发布历史、审计与回滚入口
    └─ Production 发布：独立步骤页（审阅 → 确认风险 → 执行），非 modal
```

结构来源与放弃项见 [2026-07-10 评审记录](docs/design/ui/reviews/2026-07-10-structure-directions.md)。UI Spec 012 的验收前提不变：PM 在不理解 Pack / Adapter 术语的情况下完成主流程。

## 4. 视觉系统

- 基础气质：专业、安静、工具感；中性表面和清晰网格承载信息，不模仿 Firebase Console 的高密度后台样式。
- 色彩：`#F7F7F5` 页面背景、`#FFFFFF` 表面、`#181A19` 主文字、`#2656D8` 主操作；黄色和红色只表达风险、阻断或 Production 边界。
- 字体：中文 UI 使用 Noto Sans SC，key、ETag、revision、Operation ID 和 diff 使用 JetBrains Mono；React 实现可使用等价的可分发字体栈，但层级和字重不变。
- 尺寸：4px 间距基线，32 / 40px 控件，48px 默认表格行，4 / 6px 圆角；阴影只用于 Dialog、Drawer 和覆盖层。
- 信息密度：列表适中，编辑区宽松；高级远端参数在语义树内逐层展开，不与业务 diff 争夺首屏。
- 布局：桌面画板 1440px，主流程最小支持 1280px；窄屏 390px 用于查看和轻量修改，不提供发布、回滚或复杂绑定编辑。
- 动效：只用于状态过渡、层级和操作反馈，不用装饰性动效掩盖延迟。

## 5. 工具组合与产物

| 阶段 | 建议工具 | 产物 | 事实源 |
|---|---|---|---|
| 流程与状态 | Markdown / Mermaid | 用户任务流、状态矩阵 | UI Spec |
| 结构探索 | Pencil | A / B 方向历史证据 | `explorations.pen` |
| 实现基线 | Pencil | token、reusable 组件、完整页面与状态 | `conflow-ui.pen` |
| 评审证据 | Pencil 导出 PNG/PDF | 带日期的评审快照 | `docs/design/ui/reviews/` |
| 方向收口 | `DESIGN.md` | 原则、信息架构、视觉方向 | 本文件 |
| 契约对齐 | Spec + OpenAPI + contract fixture | UI 假设与后端合同缺口、已合并合同 | [`docs/design/ui/contract-gaps.md`](docs/design/ui/contract-gaps.md) |
| 实现 | React + shadcn/ui + Base UI | 可运行界面 | `web/` |
| 视觉验收 | 浏览器截图与状态清单 | 桌面/窄屏/异常态证据 | 对应 Spec |

`conflow-ui.pen` 是视觉与交互层级的事实源，`explorations.pen` 仅保留历史决策证据。Pencil 文件不可文本 diff，采用单人持锁编辑与 PNG/PDF 评审；API、风险、来源、Plan 生命周期和 Operation 语义仍以 Spec、OpenAPI 和结构化 fixture 为合同。

## 6. 原型评审流程

1. 为一个完整用户任务写状态矩阵：入口、正常、空、加载、错误、冲突、危险确认和成功。
2. 结构方向发生变化时先在独立探索文件验证，不直接污染最终事实源。
3. 用真实长度数据验证：长项目名、20+ 广告位、多条校验错误和大 diff。
4. 评审 PM 主路径：新建项目、修改频控、处理校验、查看计划、发布生产。
5. 在 `docs/design/ui/reviews/YYYY-MM-DD-<topic>.md` 记录选择、放弃项、合同占位和原因，并导出对应 PNG/PDF。
6. 收口后更新 `DESIGN.md` 与 UI Spec，将所有后端行为假设映射到已合并合同或缺口 ID。
7. 合并对应 contract-only PR 后，再进入 React 与 Go 实现。

## 7. 组件策略

- shadcn/ui 组件源码进入仓库后视为本项目代码，修改需保持可访问性和一致 API。
- Base UI 用于 Dialog、Popover、Select、Combobox、Menu、Tabs、Tooltip 等交互原语。
- 基础组件覆盖 Button、IconButton、Input、Select、Tabs、Badge、Alert、Toggle、Checkbox、Dialog、Drawer 和 Stepper。
- 业务组件围绕 `AppTopBar`、`ProjectEnvironmentSelector`、`DraftIndicator`、`ValidationSummaryBar`、`FieldOriginCaption`、`RiskTag`、`BindingMatrix`、`AffectedEntitiesList`、`SemanticDiffTree`、`PublishConfirmation`、`OperationProgress` 和 `RollbackPreview` 建立。
- 同一语义必须实例化 reusable 组件；页面不得自行绘制第二套 Production、风险、来源或 Plan 状态。
- 第一阶段不建设独立设计系统包；在 `web/src/components/ui` 与 `web/src/components/domain` 中演进，出现第二个前端消费者后再评估抽包。

## 8. 已定项与实现门槛

首轮结构评审和第二阶段完整设计（均 2026-07-10）后的状态；依据与放弃原因见两份评审记录：

- 项目与环境切换应在全局顶栏、侧栏，还是组合选择器中？——**已确认**：全局顶栏中的组合选择器；Production 用顶栏整体变调而非切换弹窗。
- 编辑广告位使用数据表 + 抽屉，还是列表 + 独立详情页？——**已确认**：浏览用数据表；广告位编辑用详情页；频控策略（被引用实体）编辑用抽屉。
- “基线值 / 环境覆盖 / 远端当前值”如何同时展示而不制造三列表格负担？——**已确认**：放弃三列并排，改字段级 caption（「通用值 · 未发布（线上为 X）」）+ Plan 页完整对照。
- 语义 diff 与原始 Firebase diff 是并列标签，还是逐层展开？——**已确认**：逐层展开树（业务变更 → 受影响广告位 → 最终写入参数），放弃并列 tab。
- Production 发布确认需要输入环境名、确认短语，还是结合风险等级动态升级？——**已确认**：独立步骤页 + 输入环境 ID + 高风险项逐项勾选，强度随风险等级动态升级；放弃固定确认短语。确认强度为**项目级配置项**：默认始终输入环境 ID，高频发布场景可在项目配置中放宽为「低风险 Plan 勾选确认」。

- 校验入口已确认使用独立页，同时保留全局摘要条和字段锚点；不采用常驻重型底栏。
- 窄屏只展示查看与轻量修改；当前画板仅示意启用开关。Spec 004 未允许的字段必须降级为只读，不能因为原型存在输入样式就开放写入。
- Staging 发布成功复用成功页组件，主操作为查看记录；当 Production 可用且合同允许时，次操作可引导到 Production 重新生成 Plan，不复用 Staging Plan。
- 目前没有未决的结构或视觉方向。实现仍受 `contract-gaps.md` 的合同门槛约束；014/015 不得在前端重算来源、风险、影响面、就绪度、Plan 新鲜度或 Operation 远端状态。
