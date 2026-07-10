# Conflow UI Screen Inventory

> 状态：已完成（2026-07-10）。本清单定义 `conflow-ui.pen` 的页面、状态和组件覆盖；实现仍以 Spec、OpenAPI 与结构化 fixture 为合同。

## 设计事实源

- `prototypes/explorations.pen`：保留首轮 A / B 结构探索，不再追加最终页面。
- `prototypes/conflow-ui.pen`：混合模式的完整设计系统与实现基线。
- `reviews/2026-07-10-complete-ui-system/`：从最终事实源导出的评审快照。

## 画布分区

| 分区 | 内容 | 验收重点 |
|---|---|---|
| 00 Foundations | 颜色、字体、间距、圆角、尺寸与风险语义 | 变量命名可映射到 React token；黄色和红色只表示风险或阻断 |
| 01 Components | 基础组件、应用壳组件和 ConfigOps 业务组件 | reusable；状态与尺寸稳定；可访问文本不只依赖颜色 |
| 02 Spec 013 | 项目概览、环境管理、Production 壳、服务不可达、manifest 冲突 | 项目 / 环境持续可见；冲突可对照且可行动 |
| 03 Spec 014 | 配置表格、广告位详情、频控抽屉、开关与绑定、保存冲突、删除阻断 | 业务对象优先；字段来源可解释；受影响对象可追溯 |
| 04 Spec 015 | 校验中心、Plan、Plan 失效、Production 发布、执行进度、成功、历史、回滚 | 风险由服务端权威数据呈现；发布和回滚有明确减速点 |
| 05 Narrow | 配置列表、详情轻量修改、折叠菜单 | 可查看与轻量修改；不提供窄屏发布流程 |

## 页面清单

### Spec 013：项目与环境壳

1. `013 / Project Overview`：项目、环境、配置来源、Firebase 连接和最近发布。
2. `013 / Environment Management`：环境列表、创建 / 编辑面板和不可变字段说明。
3. `013 / Production Shell`：Production 顶栏变调、环境文本标签、未发布修改和校验状态。
4. `013 / Service Unavailable`：API 不可达、重试和本地服务恢复提示。
5. `013 / Manifest Conflict`：`412` 本地修改与服务端当前值对照。

加载、无项目、无环境、只读和通用错误使用 `013 / State Board` 的组件状态，不重复制作整屏。

### Spec 014：领域编辑器

1. `014 / Placement Table`：24 条广告位、筛选、状态、类型、频控和未发布修改。
2. `014 / Placement Detail`：分区表单、字段来源 caption、环境绑定矩阵和高级信息。
3. `014 / Frequency Drawer`：`inter_global_cap` 与 10 个引用广告位。
4. `014 / Switches & Bindings`：功能开关与 unit binding 两个 tab 的稳定布局。
5. `014 / Save Conflict`：`412` 本地草稿 / 服务端当前值字段级对照。
6. `014 / Referenced Delete`：删除被引用频控的阻断确认。

字段即时错误、保存中、保存成功和空筛选结果收进同一页面的状态区域或组件状态板。

### Spec 015：校验、Plan、发布与回滚

1. `015 / Validation Center`：9 条诊断、按实体分组、字段锚点和重新校验。
2. `015 / Release Plan`：5 项直接修改、10 个受影响实体和约 12 个远端参数的逐层树。
3. `015 / Plan Stale`：草稿 revision 或 Firebase ETag 变化导致的失效与重建入口。
4. `015 / Production Review`：独立步骤页的审阅阶段。
5. `015 / Production Confirm`：输入环境 ID、高风险项逐项确认和最终发布。
6. `015 / Operation Progress`：阶段进度、幂等操作 ID 和远端状态。
7. `015 / Release Success`：版本、校验摘要、审计入口和回滚入口。
8. `015 / Release History`：发布记录、操作者、环境、结果和详情。
9. `015 / Rollback Preview`：回滚目标、反向语义 diff、风险和确认。

非 Production 发布确认使用通用 Dialog 变体；远端不可达、条件值阻断与 Operation 失败使用状态板独立验收。

### 窄屏

1. `Narrow / Configuration List`：卡片式可扫读列表、搜索与筛选。
2. `Narrow / Placement Quick Edit`：只展示经 Spec 004 明确允许的轻量字段；未冻结字段以只读呈现。
3. `Narrow / Navigation Menu`：项目 / 环境、主导航和 Production 文本状态。

## 组件清单

基础组件：`Button`、`IconButton`、`Input`、`Select`、`Tabs`、`Badge`、`Alert`、`Tooltip`、`Table`、`Dialog`、`Drawer`、`Stepper`、`Skeleton`、`EmptyState`、`ErrorState`。

业务组件：`AppTopBar`、`ProjectEnvironmentSelector`、`EnvironmentBadge`、`DraftIndicator`、`ValidationSummaryBar`、`EntityStatusChip`、`RiskTag`、`FieldOriginCaption`、`BindingMatrix`、`AffectedEntitiesList`、`SemanticDiffTree`、`RemoteParamDiff`、`PublishConfirmation`、`OperationProgress`、`ReleaseHistory`、`RollbackPreview`。

## Fixture 与合同边界

- 广告位、频控、开关、绑定、诊断和 Plan 使用 `fixtures.md` 已冻结的数据与统计。
- 项目资料、Provider、Operation、发布历史和回滚缺少结构化 fixture 的字段标记为“契约占位”，不得在 React 中据此自定义后端语义。
- 风险、来源、校验就绪度、影响面、Plan 生命周期与 Operation 远端状态均由服务端 DTO 提供，前端不重算。
- Pencil 只锁定信息层级、文案意图、组件状态和布局，不替代 OpenAPI。

## 第二阶段验收

- 混合结构是唯一实现基线；最终文件不再保留相互竞争的 A / B 方向。
- 桌面关键页面使用 1440px 画板，并在 1280px 内容约束下保持可用；窄屏使用 390px 画板。
- Production、保存冲突、Plan 失效、条件值阻断、Operation 不确定和回滚均有独立视觉证据。
- 组件通过 `ref` 复用；同一语义不在页面内重新绘制不同版本。
- 导出 PNG 与多页 PDF，评审记录说明已定项、合同占位和实现门槛。
