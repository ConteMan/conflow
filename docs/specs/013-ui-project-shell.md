# Spec 013：Web 应用壳、项目与环境管理 UI

> 状态：已实现（2026-07-10）
> 依赖：Spec 002、003、012

## 目标

交付稳定的 React 应用壳、API client 和项目/环境管理界面，为后续领域编辑器提供统一状态与错误处理。

## 结构前提（Spec 012 评审收口，2026-07-10）

- 全局壳采用顶栏结构：项目/环境组合选择器 + 水平主导航 + 全局「未发布修改」徽标常驻。
- 切换到 Production 不弹确认框；顶栏整体变调（危险色条 + 环境徽标）作为持续风险标识，不只依赖颜色。
- 窄屏（<1280px）导航折叠；窄屏定位为查看与轻量修改，不承载发布流程。
- 结构依据：DESIGN.md §3 与 docs/design/ui/reviews/2026-07-10-structure-directions.md。

## 合同前置

- [`UI-API-001`](../design/ui/contract-gaps.md#spec-013-开工门槛) 与 `UI-API-002` 的合同已定义：消费 `Environment.name/kind` 与 `ManifestRevisionMismatchResponse`，不猜测 Production 身份或 `412` 恢复语义。
- `UI-API-003` 在本 Spec 只实现 draft 状态槽，不展示伪造的 dirty 数据；真实状态由 Spec 004 合同接入。
- `UI-API-004` 的项目级确认策略可以延后至 Spec 015，但本 Spec 不创建临时字段或本地存储。

## 范围

- 路由、全局布局、项目/环境常驻选择和 Production 风险标识。
- bootstrap、项目资料、环境 CRUD、Provider 状态占位和 Pack 能力展示。
- 类型安全 API client；服务端状态缓存库如需引入，必须在本 Spec 实现说明中论证。
- 通用 loading、empty、error、stale、conflict 和只读状态。
- request ID 可复制；错误码映射，不解析后端 message。
- 键盘导航、焦点恢复、颜色对比和窄屏降级。

## API

消费 Spec 002/003 端点；不直接读取 `.conflow` 文件。

## 非范围

- 广告位编辑、语义 diff 和发布。
- 独立设计系统包、主题市场和插件 UI。

## 实现选型

- 状态管理使用 React 内置 state / hook；本阶段只有单一 manifest revision 域，不引入服务端状态缓存库。
- `lucide-react` 作为直接运行时依赖，统一提供按钮和状态图标，避免维护手绘 SVG。
- `@playwright/test` 作为直接开发依赖，在浏览器中运行完整 React 应用并拦截 HTTP 合同，覆盖 bootstrap、Production、环境写入、typed `412` 和断线恢复；不以组件快照替代交互验收。
- E2E 使用 Playwright 管理的 Chromium，`make bootstrap` 安装本地浏览器。为控制公共 GitHub Actions 的安装时间与资源消耗，CI 运行不含浏览器的 `make check-ci`；涉及 UI、API client 或交互状态的变更必须在本地运行完整 `make check` 并记录结果。

## 验收

- Playwright 或等价 E2E 覆盖新建环境、修改项目、revision 冲突和 API 失败恢复。
- Production 环境在所有主页面持续可见，不只依赖颜色。
- 断开 Go API 时有可行动错误页，并显示 request ID（如有）。
- 前端构建产物同步进 Go 二进制，`make check` 通过。
