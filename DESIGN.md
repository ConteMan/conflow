# Conflow UI Design Direction

> 状态：探索基线（2026-07-10）。本文定义产品体验方向和设计流程，不是最终视觉稿。UI 结构或视觉方向发生显著变化时，同步更新本文和对应 UI Spec。

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

## 3. 初始信息架构假设

```text
项目 / 环境栏
├─ Overview        项目、环境、连接状态、最近发布
├─ Configuration   按 Pack 展示业务实体
│  ├─ Placements
│  ├─ Frequency Policies
│  ├─ Feature Switches
│  └─ Environment Bindings
├─ Validation       问题、引用、文档和规则结果
├─ Plan & Diff      业务 diff、Firebase diff、风险与影响
└─ Releases         发布、历史、回滚与审计
```

该结构是原型起点，不作为实现定论。UI Spec 012 必须验证 PM 是否能在不理解 Pack / Adapter 术语的情况下完成主流程。

## 4. 视觉方向假设

- 基础气质：专业、安静、工具感；不模仿 Firebase Console 的高密度后台样式。
- 色彩：中性灰为主；品牌色用于导航与主操作；黄色/红色只表达风险与阻断。
- 信息密度：列表适中，编辑区宽松；高级 JSON / diff 可切换高密度模式。
- 布局：桌面优先，最小支持 1280px 主流程；窄屏用于查看与轻量修改，不承诺移动端发布体验。
- 动效：只用于状态过渡、层级和操作反馈，不用装饰性动效掩盖延迟。

## 5. 工具组合与产物

| 阶段 | 建议工具 | 产物 | 事实源 |
|---|---|---|---|
| 流程与状态 | Markdown / Mermaid | 用户任务流、状态矩阵 | UI Spec |
| 低保真探索 | Pencil | 可编辑原型源文件 | `docs/design/ui/prototypes/` |
| 评审证据 | Pencil 导出 PNG/PDF | 带日期的评审快照 | `docs/design/ui/reviews/` |
| 方向收口 | `DESIGN.md` | 原则、信息架构、视觉方向 | 本文件 |
| 实现 | React + shadcn/ui + Base UI | 可运行界面 | `web/` |
| 视觉验收 | 浏览器截图与状态清单 | 桌面/窄屏/异常态证据 | 对应 Spec |

Pencil 不是强制依赖。若它不能稳定版本化、导出或复用组件，则保留其探索产物，最终合同仍是 UI Spec、`DESIGN.md` 和实现截图。

## 6. 原型评审流程

1. 为一个完整用户任务写状态矩阵：入口、正常、空、加载、错误、冲突、危险确认和成功。
2. 在 Pencil 中至少探索两个结构方向，不只改颜色和圆角。
3. 用真实长度数据验证：长项目名、20+ 广告位、多条校验错误和大 diff。
4. 评审 PM 主路径：新建项目、修改频控、处理校验、查看计划、发布生产。
5. 在 `docs/design/ui/reviews/YYYY-MM-DD-<topic>.md` 记录选择、放弃项和原因。
6. 收口后更新 `DESIGN.md` 与 UI Spec，再进入 React 实现。

## 7. 组件策略

- shadcn/ui 组件源码进入仓库后视为本项目代码，修改需保持可访问性和一致 API。
- Base UI 用于 Dialog、Popover、Select、Combobox、Menu、Tabs、Tooltip 等交互原语。
- 业务组件围绕 `EnvironmentBadge`、`ValidationSummary`、`RiskIndicator`、`SemanticDiff`、`PublishConfirmation` 等稳定语义建立。
- 第一阶段不建设独立设计系统包；在 `web/src/components/ui` 与 `web/src/components/domain` 中演进，出现第二个前端消费者后再评估抽包。

## 8. 当前待探索问题

- 项目与环境切换应在全局顶栏、侧栏，还是组合选择器中？
- 编辑广告位使用数据表 + 抽屉，还是列表 + 独立详情页？
- “基线值 / 环境覆盖 / 远端当前值”如何同时展示而不制造三列表格负担？
- 语义 diff 与原始 Firebase diff 是并列标签，还是逐层展开？
- Production 发布确认需要输入环境名、确认短语，还是结合风险等级动态升级？
