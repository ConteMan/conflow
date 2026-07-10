# Spec 003：Config Pack 契约与注册表

> 状态：已实现（2026-07-10）
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

## 示例

列出可选择的内置 Pack：

```http
GET /api/v1/packs
Host: 127.0.0.1:9010
```

```json
{
  "data": [
    {
      "ref": "mobile-ad-monetization/v1",
      "name": "mobile-ad-monetization",
      "version": "v1",
      "description": "Versioned contract for mobile advertising configuration."
    }
  ],
  "meta": {"request_id": "req_01J...", "revision": 1}
}
```

请求客户端明确支持的 schema 版本：

```http
GET /api/v1/packs/mobile-ad-monetization/versions/v1/schema?schema_version=1
Host: 127.0.0.1:9010
```

如果客户端声明的版本与当前 Pack schema 不兼容，返回 `422 schema_incompatible`；非法版本参数返回 `400 invalid_request`。

## 非范围

- GUI 中创建 Pack、动态脚本或通用低代码 schema 编辑器。
- 第三方 Pack 下载、签名和插件市场。

## 实现说明

- `internal/packs` 将 Pack 分为纯声明式 `Definition`（metadata、schema、字段默认值与 UI metadata）和未来运行期的 compiler / validator / semantic differ / migrator 接口。字段类型限制为 GUI 可稳定消费的 `string`、`boolean`、`integer`、`number`、`object`、`array` 与 `reference`。注册表只接受编译进二进制的声明，不加载脚本、插件或用户代码，也不依赖 Source 或 Provider 包。
- 项目清单继续原样保存 `pack.id`；只有需要查询 Pack 时才由注册表解析 `<name>/<version>`。Pack API 直接返回可写入清单的 `ref`，调用方不自行拼接。未知 Pack、未知版本和客户端声明的 schema 版本不兼容分别使用 `pack_not_found`、`pack_version_not_found` 和 `schema_incompatible`。
- Pack API 的 `meta.revision` 与 ETag 表示进程内 registry revision，独立于项目 manifest revision；Pack 查询令牌不得用于项目写请求的 `If-Match`。
- `mobile-ad-monetization/v1` 作为可解析的内置契约占位。具体广告实体、校验、编译与差异语义仍由 Spec 006–008 定义，避免在本 Spec 固化领域规则。
- `GET /api/v1/packs/{pack_name}/versions/{pack_version}/schema` 的可选 `schema_version` 表示客户端精确支持的 schema 版本；省略时返回当前版本。schema 中的 migration 仅为声明入口，实际迁移执行由后续草稿服务编排。

## 验收

- 一个最小测试 Pack 可通过注册表完成 metadata、schema、默认值和能力查询。
- 未知 Pack、未知版本和 schema 不兼容返回稳定错误码。
- Pack 接口不导入 `internal/source` 或 `internal/provider`。
- OpenAPI、Go DTO 和 TypeScript 类型一致，`make check` 通过。
