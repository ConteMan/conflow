# Spec 003：Config Pack 契约与注册表

> 状态：待实现  
> 依赖：Spec 001

## 目标

定义稳定、版本化的 Config Pack 接口，使新 App 可以选择已有 Pack，而新业务域通过新增 Pack 扩展，不修改项目、草稿和发布工作流。

## 范围

- Pack 引用解析：保持清单中的 `pack.id: <name>/<version>` 为不透明 ref。
- Pack metadata：名称、版本、说明、能力、实体类型、支持的环境覆盖字段。
- Entity definition：字段 schema、UI 元数据、ID 规则、默认值、敏感级别和删除策略。
- Pack compiler / validator / semantic differ 接口边界；具体广告逻辑留给 Spec 006–008。
- 内置 Pack 注册表和未知/不兼容版本错误。
- Pack schema 自身的版本与迁移入口，不支持运行用户代码。

## API

- `GET /api/v1/packs`
- `GET /api/v1/packs/{pack_name}/versions/{pack_version}`
- `GET /api/v1/packs/{pack_name}/versions/{pack_version}/schema`

API 返回用于能力发现和表单构建的声明数据，不暴露 Go 类型名。

## 非范围

- GUI 中创建 Pack、动态脚本或通用低代码 schema 编辑器。
- 第三方 Pack 下载、签名和插件市场。

## 验收

- 一个最小测试 Pack 可通过注册表完成 metadata、schema、默认值和能力查询。
- 未知 Pack、未知版本和 schema 不兼容返回稳定错误码。
- Pack 接口不导入 `internal/source` 或 `internal/provider`。
- OpenAPI、Go DTO 和 TypeScript 类型一致，`make check` 通过。
