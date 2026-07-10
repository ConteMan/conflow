# UI 原型源文件

> 归属：Spec 012。

## 文件

| 文件 | 内容 |
|---|---|
| `explorations.pen` | Pencil 原型源文件：方向 A（A1/A2）、方向 B（B1/B2/B2b）、窄屏（N1/N2）共 7 屏 |
| [fixtures.md](fixtures.md) | 真实规模参考数据（24 广告位 / 5 频控 / 6 开关 / 3 环境 / 诊断样例 / 大 diff 场景） |

**命名基准**：`fixtures.md` 是实体 key 的事实源。`explorations.pen` 的首轮帧早于 fixtures 定稿，锚点实体（`app_open_cold_start`、`inter_global_cap`、`native_scroll_gap`、`compress_complete_interstitial_legacy_campaign`、`use_amazon_bidding`、`ads_enabled_legacy`）与 fixtures 一致，其余广告位命名存在差异；后续补帧和 React 实现一律以 fixtures.md 为准，首轮帧不返工。

## 画布结构

- 第一行：方向 A —— A1 配置数据表 + 频控编辑抽屉（Staging）、A2 发布计划 + Production 确认弹层。
- 第二行：方向 B —— B1 侧栏主从 + 广告位详情页、B2 发布计划逐层展开树、B2b 独立确认步骤页。
- 第三行：窄屏 —— N1 配置列表（只读为主）、N2 详情（仅轻量编辑）。
- 第四行（评审收口后补帧，采用混合结构 + fixtures 命名）—— C1 发布成功页（含回滚入口与发布历史）、C2 发布计划远端冲突态（ETag 失效阻断）、C3 保存冲突（412）弹层。

## 工具限制记录（Spec 012 要求）

- `.pen` 为 Pencil 专有加密格式：**Git 只能整文件版本化，无法 diff / merge**。多人同时编辑同一 `.pen` 会产生二进制冲突，按「单人持锁编辑 + PNG 导出评审」协作。
- 评审证据以导出 PNG 为准，存放于 `../reviews/<date>-assets/`；`.pen` 源文件仅作为可继续编辑的底稿。
- 结论性内容不落在 `.pen` 内：所有取舍与状态语义以 `flows/`、评审记录和 `DESIGN.md` 为事实源，截图不替代 API / 业务契约。
- Pencil 支持组件复用与变量（本文件已用色彩/字体 token），但不承诺与 React 实现共享 token 定义；实现阶段以 `web/` 中的 Tailwind 配置为准。
