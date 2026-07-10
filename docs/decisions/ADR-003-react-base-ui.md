# ADR-003：React、Vite、shadcn/ui 与 Base UI

> 状态：已接受（2026-07-10）

## 决策

Web UI 使用 React、TypeScript、Vite、Tailwind CSS，并以 shadcn/ui 的 Base UI primitives 作为组件基础。

## 理由

- 本地配置工作台需要高质量表单、表格、弹窗、命令菜单与差异视图。
- shadcn/ui 可按需将组件源码纳入仓库，Base UI 提供无样式、可访问的原语。
- Vite 适合构建静态资源，能被 Go 嵌入。

## 后果

- Node 仅作为开发构建依赖；`web/dist` 不提交。
- 每次前端变更后必须执行 `make web-build`，同步 `internal/webui/assets/`。
