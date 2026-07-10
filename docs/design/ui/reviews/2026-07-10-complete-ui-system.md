# 2026-07-10 混合模式完整 UI 系统评审

## 结论

首轮 A / B 探索已经完成结构取舍，本轮不再比较方向。`conflow-ui.pen` 以已采纳混合模式为唯一实现基线：全局顶栏、配置数据表、广告位独立详情页、频控右侧抽屉、逐层 Plan 树和 Production 独立步骤页。

最终文件已移除继承的探索节点；`explorations.pen` 继续保留历史证据。所有新增根画板已通过 Pencil `snapshot_layout` 深度检查，无重叠、裁切或塌缩问题。

## 设计系统

- Token：18 个颜色语义、2 个字体、4px 间距基线、32 / 40px 控件、48px 表格行和 4 / 6px 圆角。
- reusable 组件共 27 个，覆盖 Button、IconButton、Input、Select、Tabs、Badge、Alert、Toggle、Checkbox 和 ConfigOps 业务组件。
- 业务组件：AppTopBar、项目 / 环境选择、草稿与校验状态、字段来源、风险、影响列表、语义 Diff、发布步骤、确认和 Operation 进度。
- 页面只实例化稳定语义；Production、风险、来源、Plan 新鲜度和 Operation 状态不允许出现页面私有版本。

## 页面覆盖

| 分区 | 画板 | Pencil 节点 |
|---|---:|---|
| Foundations 与组件 | 3 | `T9InxA`、`c5mXC`、`w36tfp` |
| Spec 013 | 6 | `nS9y1`、`DIPQ2`、`bIxRb`、`QIF1u`、`n4z1c`、`Mn6dG` |
| Spec 014 | 6 | `dQQdZ`、`Xxn5w`、`Ro8Gd`、`T6NMK6`、`NFlwv`、`aiAiO` |
| Spec 015 | 10 | `iGMW2`、`L29xT`、`n1vFO`、`s8iJ5h`、`VDpHC`、`Isgyt`、`C56qu`、`d9pM3`、`FBDyH`、`U74oJf` |
| Narrow | 3 | `RXyEj`、`XKi9h`、`YZjZm` |

详细页面与状态语义见 [`screen-inventory.md`](../screen-inventory.md)。

## Fixture 使用

- 配置表格使用 24 个广告位的真实规模，首屏展示 13 条并明确总量、停用量和未发布状态。
- 频控抽屉使用 `inter_global_cap` 的 10 个引用者；删除阻断显示引用迁移入口。
- 校验中心使用 9 条诊断的分组与详情；Plan 固定为 5 项直接修改、10 个受影响实体和约 12 个远端参数。
- `use_amazon_bidding` 承担高风险 Production 确认场景；`ad_native_006`、`ad_native_007` 承担缺失绑定场景。

项目资料、Provider、Operation、发布历史和回滚仍缺少完整结构化 fixture。画板中的这部分数据是明确的合同占位，用于锁定层级和组件状态，不授权前端定义 DTO 或业务语义。

## 已定交互

- Production 使用顶栏整体变调和文本标签，不只依赖颜色。
- 校验使用独立页、全局摘要和字段锚点，不建设常驻重型底栏。
- 保存 `412` 必须展示本地值与服务端当前值；不能只返回或只显示错误码。
- Plan 绑定草稿 revision 和 Firebase ETag；任一变化都使旧 Plan 只读失效。
- Production 发布按审阅、确认风险、执行三步进行；高风险项逐项确认，并按项目策略输入环境 ID。
- Operation 失败必须呈现 `remote_state`；`unknown` 时先核对远端，不提供盲目重试。
- 窄屏只支持查看与 Spec 004 明确允许的轻量修改，不提供发布、回滚或复杂绑定编辑。

## 评审资产

- 可编辑源文件：[`conflow-ui.pen`](../prototypes/conflow-ui.pen)
- PNG：[`2026-07-10-complete-ui-system/png/`](2026-07-10-complete-ui-system/png/)
- 28 页 PDF：[`conflow-ui-review.pdf`](2026-07-10-complete-ui-system/conflow-ui-review.pdf)

PNG 文件名使用 Pencil 节点 ID，可直接通过上方页面覆盖表定位。PDF 按 Foundations、Spec 013、Spec 014、Spec 015、Narrow 排序。

## 实现门槛

- Spec 013 可直接按当前设计与已合并合同实现。
- Spec 014 必须等待草稿分层、领域写入、字段来源和诊断合同；原型不得代替 `UI-API-005` 至 `UI-API-013`。
- Spec 015 必须等待服务端权威的 Plan、风险、Operation、发布历史和回滚合同。
- React 实现完成后，用同一结构化 fixture 运行桌面 1440px、最小支持宽度 1280px 和窄屏 390px 的截图回归。
