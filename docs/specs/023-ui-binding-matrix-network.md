# Spec 023：UI 环境绑定矩阵改版（按广告网络维度）

> 状态：待实现  
> 依赖：Spec 014、Spec 020  
> 关联：Spec 022（编译层依赖 entity ID 格式，两 Spec 需保持一致）

## 目标

当前"环境绑定"矩阵以 `iOS / Android` 为列，不反映实际业务维度：项目目前是纯 Android，iOS 列没有意义；更重要的是，同一个 placement 在同一环境下需要为 MAX 和 AdMob 分别填写 unit ID（即 `units` map 里的两个 key），现有矩阵无法表达这个 network 维度。

改版后矩阵列 = 广告网络（`MAX / AdMob`），行 = 环境，用户在每个格子填写该网络的 unit ID。

## 范围

### entity ID 格式变更

现有格式：`ub_${environmentID}_${platform}_${placementID}`  
新格式：`ub_${environmentID}_android_${network}_${placementID}`

- `platform` 固定为 `android`（当前项目只支持 Android，iOS 后续通过新 Spec 扩展）
- `network` 加入 ID，区分同一 placement 下不同广告网络的绑定
- `android` 作为固定字面量占位，保持格式层次一致，便于后续扩展为可变 platform

示例：`ub_development_android_max_scan_entry_interstitial`

### entity fields 变更

UI 创建 unit_binding 时，fields 需包含 `network` 字段（现有实现中缺失）：

```json
{
  "placement_id": "scan_entry_interstitial",
  "environment_id": "development",
  "platform": "android",
  "network": "max",
  "unit_id_ref": "a4135a8af9583f71",
  "status": "configured"
}
```

### 广告位详情页（PlacementDetail）binding-matrix

修改 `ConfigurationEditor.tsx` 中的 `PlacementBindingSection` 组件：

- 列定义从 `["ios", "android"]` 改为 `["max", "admob"]` 两个 network 列
- 列头标签：`MAX` / `AdMob`
- `rowID(environmentID, network)` = `` `ub_${environmentID}_android_${network}_${placementID}` ``
- `bindingFor(environmentID, network)` 按 entity_id 匹配（与现有逻辑一致）
- 创建/更新 entity 时写入 `platform: "android"` 和 `network`（从列定义取）
- editing state key 格式：`` `${environmentID}:${network}` ``
- 说明文字：`"按环境维护 MAX 和 AdMob 的广告单元 ID。"`

### 环境绑定总览（BindingOverview）

修改 `ConfigurationEditor.tsx` 中的 `BindingOverview` 组件：

- 列从 `environments × [iOS, Android]` 改为 `environments × [MAX, AdMob]`
- 表头：`{environment.name} MAX` / `{environment.name} AdMob`
- 按 entity_id 精确匹配（使用新格式 rowID）
- 缺失判断：`value` 为空时显示"未绑定"

### 引导文案更新

第 184 行 empty guide 文案：`"保存广告位后，按环境填写 iOS 和 Android 单元 ID。"` → `"保存广告位后，按环境填写 MAX 和 AdMob 单元 ID。"`

### 已有实体迁移

现有通过 `seed-unit-bindings.mjs` 创建的 28 条实体 ID 格式为 `ub_development_android_${placementID}`（不含 network），需在 app-pdf-launcher-doc 仓库：

1. 删除旧实体（已有脚本可复用）
2. 重新执行更新后的种子脚本（新格式含 network）

此迁移工作在 Spec 023 UI 实现后、正式 Firebase 发布前完成，不是本 Spec 的验收阻塞条件。

## 非范围

- 不支持 iOS 绑定（platform 暂时固定 android；iOS 通过后续 Spec 扩展）
- 不新增 UI 编辑 `network_mode` 字段（placement 的 `network_mode` 在基础信息 tab 通过现有字段编辑器处理）
- 不修改 unit_binding entity schema（字段已有 `network`，只修改 UI 行为）
- 不修改 `v2BindingSortKey` 逻辑（Go 侧排序无需变化）

## 验收

- 在广告位详情的"环境绑定"区域看到两列：`MAX` 和 `AdMob`（不出现 iOS / Android 列标签）
- 填写 MAX unit ID 后保存，entity id 格式为 `ub_development_android_max_<placementID>`，fields 包含 `network: "max"`
- 环境绑定总览页列头为 `{环境名} MAX` / `{环境名} AdMob`
- `go test ./web/...`（如有前端单测）或 `pnpm build` 无报错
