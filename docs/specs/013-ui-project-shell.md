# Spec 013：Web 应用壳、项目与环境管理 UI

> 状态：待实现  
> 依赖：Spec 002、003、012

## 目标

交付稳定的 React 应用壳、API client 和项目/环境管理界面，为后续领域编辑器提供统一状态与错误处理。

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

## 验收

- Playwright 或等价 E2E 覆盖新建环境、修改项目、revision 冲突和 API 失败恢复。
- Production 环境在所有主页面持续可见，不只依赖颜色。
- 断开 Go API 时有可行动错误页，并显示 request ID（如有）。
- 前端构建产物同步进 Go 二进制，`make check` 通过。
