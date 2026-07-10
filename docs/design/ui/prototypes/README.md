# UI 原型源文件

> 归属：Spec 012。

## 文件

| 文件 | 内容 |
|---|---|
| `explorations.pen` | 历史探索：方向 A、方向 B、窄屏与首轮收口状态；不再作为实现依据 |
| `conflow-ui.pen` | 最终事实源：混合模式、设计 token、27 个 reusable 组件、Spec 013–015 页面与窄屏状态 |
| [fixtures.md](fixtures.md) | 真实规模参考数据（24 广告位 / 5 频控 / 6 开关 / 3 环境 / 诊断样例 / 大 diff 场景） |

**命名基准**：当前由 `fixtures.md` 维护实体 key；Spec 006 建立结构化 contract fixture 后，以其为可执行事实源。`explorations.pen` 的首轮帧早于 fixtures 定稿，锚点实体（`app_open_cold_start`、`inter_global_cap`、`native_scroll_gap`、`compress_complete_interstitial_legacy_campaign`、`use_amazon_bidding`、`ads_enabled_legacy`）与 fixtures 一致，其余命名和“14 处修改”等统计属于历史探索数据；后续补帧和 React 实现一律以共享 contract fixture 为准，首轮帧不返工。

## 最终画布结构

`conflow-ui.pen` 从左到右排列：

1. Foundations、基础组件、ConfigOps 业务组件。
2. Spec 013：项目概览、环境管理、Production 壳、服务不可达、manifest 冲突和通用状态板。
3. Spec 014：广告位表格、详情、频控抽屉、开关与绑定、保存冲突和删除阻断。
4. Spec 015：校验、Plan、失效、Production 发布三阶段、成功、历史、回滚和安全异常状态。
5. Narrow：配置列表、轻量修改和折叠菜单；不含发布流程。

完整节点与截图映射见 [完整 UI 系统评审](../reviews/2026-07-10-complete-ui-system.md)。

## 工具限制记录（Spec 012 要求）

- `.pen` 为 Pencil 专有加密格式：**Git 只能整文件版本化，无法 diff / merge**。多人同时编辑同一 `.pen` 会产生二进制冲突，按「单人持锁编辑 + PNG 导出评审」协作。
- 评审证据以导出 PNG 与多页 PDF 为准，存放于 `../reviews/2026-07-10-complete-ui-system/`；`.pen` 源文件用于后续组件和页面迭代。
- 结论性内容不落在 `.pen` 内：所有取舍与状态语义以 `flows/`、评审记录和 `DESIGN.md` 为事实源，截图不替代 API / 业务契约。
- Pencil 变量提供 React token 的命名与视觉基线，但不直接生成 Tailwind 配置；实现阶段在 `web/` 显式映射并通过浏览器截图回归验证。
