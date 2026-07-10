# Spec 001：Go CLI、本地服务与嵌入式 React 基础

> 状态：已实现（2026-07-10，`05dde72`）

## 范围

- `conflow version`、`init`、`serve`、`validate` 四个 CLI 命令。
- `.conflow/project.yaml` 读写与基础校验。
- `GET /api/v1/health`。
- React Vite 首页与嵌入式静态资源服务。
- Makefile、CI、GoReleaser 和质量门禁。

## 非范围

- Git JSON 兼容。
- Firebase 网络调用、OAuth、ETag、发布或回滚。
- 广告实体编辑和语义化 diff。

## 验收

```sh
make web-build
make check
go run ./cmd/conflow init --dir /tmp/conflow-example
go run ./cmd/conflow validate --workspace /tmp/conflow-example
go run ./cmd/conflow serve --workspace /tmp/conflow-example
```

服务返回健康检查，浏览器根路径显示项目与配置包占位工作台。
