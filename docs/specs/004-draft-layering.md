# Spec 004：草稿与分层配置模型

> 状态：待实现  
> 依赖：Spec 002、003

## 目标

建立 `Pack 默认值 < 项目基线 < 环境覆盖 < 当前草稿` 的可解释合并模型，让 GUI 能展示每个值的来源，并安全保存并发修改。

## 范围

- 项目基线、环境覆盖、当前草稿和解析后 effective view。
- 每个字段携带 `origin`、是否覆盖、是否可覆盖和源 revision。
- 草稿 revision、dirty 状态、reset、discard 和外部源变更冲突。
- Pack 控制哪些字段允许环境覆盖；非法覆盖作为校验错误。
- 空值、缺失值、显式 `null` 和集合替换语义固定化。

## API

- `GET /api/v1/drafts/{environment_id}`
- `PUT /api/v1/drafts/{environment_id}`
- `POST /api/v1/drafts/{environment_id}:reset`
- `POST /api/v1/drafts/{environment_id}:discard`

`PUT` 完整替换草稿，必须带 `If-Match`。v1 不提供通用 JSON Patch。

## 非范围

- 业务实体字段与广告规则。
- 多人实时协同编辑和自动冲突合并。

## 验收

- 表驱动测试覆盖四层优先级、集合替换、显式 null、禁止覆盖和源变更冲突。
- API 返回 effective value 与来源，不要求前端自行合并。
- 多标签页旧 revision 保存返回 `412`，不覆盖新值。
- discard 后恢复到当前源适配器状态，reset 后保留合法的空草稿。
