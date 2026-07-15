# Spec 024：实体未发布修改以上次发布为基线

> 状态：待实现  
> 依赖：Spec 004（草稿分层）、Spec 011（发布审计）  
> 关联：Issue #52；#46（发布计划已切换为上次发布基线，本 Spec 将实体级标记对齐同一口径）

## 目标

配置列表与顶部状态条的"未发布修改"反映实体与上次成功发布状态的真实差异：发布后未改动的实体不再标记，实际改动、新增或待删除的实体才标记。

## 背景

当前 dirty 判定基于草稿层来源（`origin` 为 `draft_baseline` / `draft_environment_override` 即视为有修改）。经导入建立的项目全部实体永远命中该条件——即使刚发布完、远端与草稿语义一致，界面仍满屏"未发布修改"，信号失效。#46 已把发布计划基线改为上次发布状态，但实体级标记仍是旧口径，两处判断不一致。

## 范围

### 已发布基线存储

发布或回滚成功后，服务端持久化该环境的实体基线到 `.conflow/released-baseline/<environment_id>.json`：

```json
{
  "environment_id": "development",
  "release_id": "rel_xxx",
  "released_at": "2026-07-15T03:20:00Z",
  "source_revision": "…",
  "entities": {
    "entity:mobile-ad-monetization/v2:placement:app_open": "sha256:…"
  }
}
```

- 哈希对实体 `fields` 的规范化 JSON（键排序、无多余空白）计算，与草稿 revision、时间戳无关。
- 基线覆盖发布时参与编译的全部实体类型；写入与 release 审计记录在同一成功路径上完成，发布失败不更新。
- 文件属于运行时状态，随 `.conflow` 其他状态文件一起管理（不强制入库，由项目自行决定）。

### dirty 判定规则

服务端在草稿实体读模型中输出 `change_status` 字段，取值：

| 值 | 条件 |
|---|---|
| `unchanged` | 基线存在该实体且哈希相等 |
| `modified` | 基线存在该实体且哈希不等 |
| `created` | 基线不存在该实体 |
| `deleted` | 基线存在而草稿已删除（仅在删除清单类读模型中出现） |

- 环境基线文件不存在（从未通过 Conflow 发布）时，全部实体为 `created`——语义如实：确实从未发布。
- 环境 overlay 实体（如 `unit_binding`）与基线中对应环境条目比较。

### API 变更

- `GET /environments/{id}/draft/entities/*` 与实体列表响应的每个实体增加 `change_status`。
- Draft 汇总（供顶部状态条）增加 `changed_entity_count`；"有未发布修改"= `changed_entity_count > 0`。

### UI 变更

- 前端删除 `isEntityDirty` 的本地推断，改用服务端 `change_status`；`unchanged` 不显示标记，`modified` / `created` 分别显示"已修改" / "新增"。
- "仅看未发布修改"筛选与顶部状态条同步切换到新口径。

### 兼容与迁移

- 升级后基线文件缺失属正常状态，自下一次成功发布起建立基线并生效；不做从历史 release 反推基线的自动迁移。
- 旧字段 `origin` 保留（仍表达草稿分层来源），仅不再用于 dirty 展示。

## 非范围

- 字段级差异展示（发布计划的语义 diff 已覆盖）。
- 跨环境的基线对比。
- 基线文件的历史版本管理（每环境仅保留当前基线，历史可从 releases 审计追溯）。

## 验收

- 发布成功后立即刷新列表：全部实体 `unchanged`，顶部状态条不再显示"有未发布修改"。
- 修改任一实体字段后：仅该实体标记 `modified`，计数为 1。
- 新建实体标记 `created`；删除实体后汇总计数包含该删除。
- 回滚成功后基线更新为回滚后状态，列表标记与之一致。
- 无基线文件的项目行为与现状一致（全部标记），下次发布后自动收敛。
- `make check` 通过；新增 Go 单测覆盖基线写入、哈希稳定性（键序无关）与四种 `change_status`。
