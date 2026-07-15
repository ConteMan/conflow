# Spec 022：v2 Pack 广告位编译格式修复

> 状态：待实现  
> 依赖：Spec 020  
> 关联：Spec 023（绑定矩阵改版引入 network 维度后，unit_bindings→units 过滤逻辑需同步对齐）

## 目标

`compile_v2.go` 的 `v2Placements` 输出与 `remote-config.md §2.2` 客户端合约存在 8 处字段名 / 结构差异，导致 Firebase 推送的配置对不上客户端解析逻辑。修复后，Conflow 生成的 `ad_placements_config` JSON 需与合约精确匹配，无需客户端做任何兼容处理。

## 范围

### 字段映射变更（编译层）

| 现有字段 | 合约字段 | 变更类型 |
|---|---|---|
| `client_id` | `id` | 改名 |
| `key` | `placement` | 改名 |
| `ad_type` | `type` | 改名 |
| `fallback_behavior` | `fallback` | 改名 |
| `cache_ttl`（duration 对象） | `cache_ttl_seconds`（Number / null） | 改名 + 类型转换 |
| `cache_policy` | —（合约无此字段） | 删除 |
| —（来自 `enabled_switch_id` 解引用） | `enabled_config_key`（String） | 新增 |
| —（来自新增 placement 字段） | `network_mode`（String） | 新增 |
| `unit_bindings: [{environment_id, platform, network, unit_id_ref}]` | `units: {<network>: {unit_id}}` | 结构重写 |

### duration → seconds 转换规则

`cache_ttl` 存储为 `{"unit": "seconds"|"minutes"|"hours"|"days", "value": N}` 或 `null`。编译时转换为秒数整数：

- `null` → `null`
- `seconds` → `value`
- `minutes` → `value * 60`
- `hours` → `value * 3600`
- `days` → `value * 86400`

### `enabled_config_key` 解引用

`enabled_switch_id` 是 feature_switch 实体的 ID 引用。编译时从 `desired["feature_switches"]` 中找到对应实体，读取其 `fields.key` 字段，输出为 `enabled_config_key`。找不到时输出空字符串 `""`。

### `units` map 构建

当前 `unit_bindings[]` 数组包含所有环境和平台的绑定，发布到特定 Firebase 项目时只有当前环境的值有意义。

编译时按 `environmentID` 过滤 unit_binding，以 `network` 为 key 聚合为 map：

```json
"units": {
  "max":   {"unit_id": "<unit_id_ref>"},
  "admob": {"unit_id": "<unit_id_ref>"}
}
```

- 只保留 `environment_id == 当前编译目标环境` 的绑定
- 每个 network 取第一条（按 `v2BindingSortKey` 排序后最小的）
- unit_binding 无 `platform` 过滤——合约的 `units` map 不区分 platform

### 函数签名变更

`compileV2Parameters`、`desiredParameterValues`、`MergeFirebaseTemplate` 均需增加 `environmentID string` 参数：

```go
func compileV2Parameters(desired map[string]any, environmentID string) map[string]any
func desiredParameterValues(desired map[string]any, packRef, environmentID string) map[string]any
func MergeFirebaseTemplate(remoteTemplate, desiredJSON []byte, changes []RemoteParameterChange, packRef, environmentID string) ([]byte, error)
```

`plan.go` 调用 `compileV2Parameters` 时传入 `in.EnvironmentID`。  
`release.go` 调用 `MergeFirebaseTemplate` 时传入发布目标的 `environmentID`。

### Schema 变更

`internal/packs/mobile_ad_v2.go` placement entity 新增字段：

```go
field("network_mode", FieldTypeString, false, true, `"admob"`, "广告链路", "覆盖全局 ad_network_mode；空表示继承全局配置。", "select", "投放", 5, enum("admob", "max"))
```

> 维护者修订（2026-07-15）：`network_mode` 调整为可空，空值表示继承全局 `ad_network_mode`。

### 测试变更

`internal/plan/compile_v2_test.go` 中 `TestCompileV2ParametersPlacements` 断言结构从 `client_id / unit_bindings[]` 改为 `id / units map`，并验证 `enabled_config_key`。  
`MergeFirebaseTemplate` 测试调用统一补充 `environmentID` 参数。

## 非范围

- 不修改 `ad_frequency_policies_config` 的输出格式（合约 §2.1 已匹配）
- 不修改 `version` 字段（输出仍为 `2`）
- 不修改 placement entity 的其他已有字段
- 不处理 `app-pdf-launcher-doc` 仓库的脚本（bootstrap-import-bundle.mjs 在后续导入时自行补充 `network_mode`，已有实体可通过 `import apply` 更新）

## 验收

- `go test ./internal/plan/...` 全部通过，含改后的 `TestCompileV2ParametersPlacements`
- 以 development 环境为目标，在 app-pdf-launcher-config workspace 执行 `conflow plan`（或触发 UI plan），生成的 `ad_placements_config` JSON 反序列化后：
  - 每个 placement 有字段 `id, placement, type, enabled_config_key, network_mode, units, frequency_policy_type, frequency_policy_id, load_timeout_ms, cache_ttl_seconds, fallback`
  - `units` 为 `{"max": {"unit_id": "..."}}` map（development 环境已有 android/max 绑定）
  - 无 `client_id`、`key`、`ad_type`、`cache_policy`、`unit_bindings` 字段
