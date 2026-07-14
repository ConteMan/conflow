# Spec 021：配置导入

> 状态：待实现  
> 依赖：Spec 004、007、008  
> 关联：GitHub Issue #36

## 目标

新项目可以从另一个同 Pack workspace 或外部引导导出产物一次性导入业务配置，经 preview / apply 两阶段受控流程写入草稿，不复制凭据、Provider 配置或发布历史。

## 场景

**场景 A：同 Pack 项目复用**  
两个项目使用相同 Pack（如 `mobile-ad-monetization/v2`），目标项目希望以来源项目的频控策略和广告位作为起点继续修改。

**场景 B：外部引导导出**  
遗留项目的配置以非 Conflow 格式存在（如 Firebase Remote Config JSON），外部脚本将其转换为标准 ImportBundle，由 Conflow 受控导入到新 workspace。

## 核心数据类型

### ImportBundle（导入包，版本化中间格式）

```json
{
  "format_version": 1,
  "pack_ref": "mobile-ad-monetization/v2",
  "schema_version": 2,
  "created_at": "2026-07-14T10:00:00Z",
  "source_id": "可选，来源 workspace 标识",
  "source_revision": "可选，来源 Git revision",
  "source_digest": "sha256:...",
  "entities": {
    "feature_switch": [
      { "id": "enable_splash", "fields": { "key": "enable_splash", "default_value": true, "risk_level": "low", "rollback_method": "关闭开关" } }
    ],
    "frequency_policy": [
      { "id": "global_cap", "fields": { "cooldown": { "unit": "minutes", "value": 30 }, "interval": null, "max_count": null, "shift_count": null, "positions": null } }
    ]
  },
  "decisions_required": [
    { "key": "network_settings.default.active_network", "reason": "无法从来源推导当前广告网络", "hint": "admob / max / ironsource" }
  ]
}
```

- `entities` 只包含业务实体；不包含 `unit_binding`（含环境信息，由应用层决策）。
- `decisions_required[]` 列出无法无损推导的字段；apply 时必须为每项提供 `value`。
- `source_digest` 与 `source_revision` 任一变化后旧产物失效；相同输入必须产生语义等价产物。
- 产物不得包含 Firebase 凭据、访问令牌、私钥或其他认证材料。

### ImportDecision

```json
{ "key": "network_settings.default.active_network", "value": "admob" }
```

`key` 格式：`<entity_type>.<entity_id>.<field_name>`。

### PreviewResult

```json
{
  "preview_token": "...",
  "expires_at": "2026-07-14T10:15:00Z",
  "pack_ref": "mobile-ad-monetization/v2",
  "conflict_mode": "replace",
  "entity_plan": {
    "to_add":     [{ "entity_type": "feature_switch", "id": "enable_splash" }],
    "to_replace": [{ "entity_type": "frequency_policy", "id": "global_cap", "diff": { ... } }],
    "to_skip":    [],
    "to_keep":    []
  },
  "decisions_required": [...],
  "validation_warnings": [...],
  "risks": ["将替换 2 条已有频控策略"]
}
```

`preview_token` 有效期 15 分钟，与 Plan 一致。apply 时 `If-Match` 携带此 token，token 过期或草稿 revision 变化后失效。

## API / CLI

### CLI

```
# 从另一个 workspace 生成 ImportBundle（来源导出）
conflow import export --workspace /path/to/source [--output bundle.json]

# 预览导入效果（不修改草稿）
conflow import preview --workspace . --bundle bundle.json [--conflict-mode replace|merge|skip]
conflow import preview --workspace . --from /path/to/source  [--conflict-mode replace|merge|skip]

# 应用导入（修改草稿，原子）
conflow import apply --workspace . --bundle bundle.json [--conflict-mode replace] [--decisions decisions.json]
conflow import apply --workspace . --from /path/to/source  [--conflict-mode replace] [--decisions decisions.json]
```

`--conflict-mode`：
- `replace`（默认）：来源实体覆盖目标同 ID 实体。
- `merge`：只写入目标中不存在的实体；已有 ID 保留。
- `skip`：跳过所有冲突，只写入目标中完全不存在的实体类型。

所有命令支持 `--json` 输出结构化结果（符合 Spec 018 自动化合同）。

### HTTP API

```
POST /api/v1/environments/{env_id}/draft/import/preview
  Body: ImportBundle JSON
  Query: conflict_mode=replace|merge|skip
  Response: PreviewResult

POST /api/v1/environments/{env_id}/draft/import/apply
  If-Match: <preview_token>
  Body: { "decisions": [ImportDecision] }
  Response: { "applied_count": N, "revision": "..." }
```

两个端点均支持来源为 bundle JSON body 或 `from_workspace` path（仅 CLI，HTTP 端点只接受 bundle body）。

### 来源导出（conflow import export）

等价于读取来源 workspace 的草稿层可编辑实体并序列化为 ImportBundle。不包含：

- Firebase 凭据或 Provider 配置
- `unit_binding` 实体（含环境 ID，属于来源专属）
- 发布历史或远端快照

`source_digest` 为规范化实体 JSON 的 SHA-256；`source_revision` 取来源 workspace managed-file 的 snapshot token 或 Git commit hash（如有）。

## 范围

- CLI `import export / preview / apply` 三个子命令。
- HTTP `POST .../import/preview` 和 `POST .../import/apply` 两个端点。
- Web UI Import 入口（配置页「导入」按钮 → 上传 bundle → Preview 对话框 → Apply 确认）。
- 冲突模式 `replace / merge / skip`。
- `decisions_required` 填写与校验。
- apply 原子性：失败时恢复 apply 前草稿快照。
- preview_token 15 分钟有效期；draft revision 变化后 token 失效（`412 Precondition Failed`）。

## 非范围

- 跨 Pack 或跨 schema major version 的迁移（`pack_ref` 不一致时拒绝，不做字段猜测）。
- `unit_binding` 的自动导入（需要人工绑定环境 / 平台 / 网络）。
- Firebase Remote Config JSON 到 ImportBundle 的转换（由外部引导导出脚本实现，Conflow 只接受标准 ImportBundle）。
- 增量/追加式多次导入的冲突追踪（每次 import 是独立操作）。
- 多并发 import 队列。

## 验收

1. `conflow import export --workspace A --output bundle.json` 生成合法 ImportBundle，`format_version=1`，不含凭据或 `unit_binding`。
2. `conflow import preview --workspace B --bundle bundle.json` 返回 PreviewResult，包含 `preview_token`、`entity_plan` 和 `decisions_required`；B 草稿不变。
3. `decisions_required` 不为空时，`conflow import apply` 不携带 `--decisions` 则返回错误，携带完整 decisions 则成功。
4. apply 成功后 `conflow validate --workspace B` 通过（无必填字段错误）。
5. apply 失败（如写入错误）后 B 草稿状态与 apply 前完全一致。
6. `preview_token` 过期或 B 草稿在 preview 后被修改，apply 返回 412；重新 preview 后可继续。
7. `--conflict-mode merge` 时已有 ID 的实体保留原值，仅新 ID 实体写入。
8. `--conflict-mode skip` 时没有任何已有实体被覆盖。
9. HTTP `POST .../import/preview` 和 `POST .../import/apply` 与 CLI 等价，支持 `--json`。
10. Web UI 可通过上传 bundle 文件触发 preview，展示 `entity_plan`，填写 decisions 后 apply。
