# Spec 029：受管 Remote Config 参数全量对账

> 状态：已实现
> 依赖：Spec 008、009、010、020、024
> 关联：GitHub Issue #74、#75

## 目标

构建发布计划时，无条件对账完整本地受管参数集合与 Firebase 远端快照；即使本地配置相对上次发布没有变化，也能发现并安全修复远端缺失或漂移。

## 范围

- `mobile-ad-monetization/v2` 将完整 `Desired` 编译为按 `parameter_key` 唯一的受管参数集合，并与当前远端快照逐项比较。
- 本地受管参数远端缺失时生成 `added`；默认值不同时生成 `updated`；值一致时不生成远端变化。
- 本地已明确删除或重命名的历史受管参数，仅在该 key 存在于发布基线且远端仍存在时生成 `deleted`。
- 远端额外且不属于当前或历史受管集合的参数只提示保留，不自动删除。
- 一个远端参数变化聚合全部相关本地语义原因与受影响实体，不因多个业务字段映射到同一聚合参数而重复。
- 当远端缺失或漂移且没有本地语义变化时，为该参数创建合成语义节点 `managed_remote_drift`，表达“本地目标未变，但远端受管参数不一致”。
- 首次发布没有成功发布记录时，以空配置作为历史基线，并把当前完整配置显示为 additions，不再产生环境覆盖先删除再添加的反向变化。
- Review、风险和 Web UI 区分“本地无未发布修改”与“远端受管参数一致”。

## 对账与安全边界

1. 配置包仍是受管默认值的事实源；Firebase 是发布目标和审计对象。
2. 全量对账只覆盖编译器声明的受管参数，以及发布基线中曾受管且被本地明确删除或重命名的参数。
3. 远端额外参数保留，并以 `unmanaged_remote_parameter_preserved` 低风险提示展示。
4. 受管参数存在未建模条件值时继续以 `unmodeled_remote_condition` 阻断，不允许确认绕过。发布合并只更新默认值，不删除或改写条件值。
5. Plan 保持不可变，继续绑定草稿 revision、source digest、远端 ETag、内容摘要与发布审计。
6. 对账结果按参数 key 稳定排序和去重；Provider 只应用 Plan 明确选择的远端变化。

## Plan / API

- `PlanChangeKind` 追加 `managed_remote_drift`。
- 合成语义节点没有 `direct_entity_ref`，其 `remote_parameter_node_ids` 指向唯一远端变化。
- `RemoteParameterChange.change_kind` 继续使用 `added`、`updated` 或 `deleted`；`caused_by_semantic_change_ids` 至少包含本地语义节点或对应的合成漂移节点。
- 新增风险原因：
  - `managed_remote_drift`：缺失补齐为 medium，远端默认值漂移覆盖为 high，均要求确认。
  - `unmanaged_remote_parameter_preserved`：low，不要求确认，不生成远端变化。
- Review Markdown 至少输出本地语义变化数、远端参数变化数和受管远端漂移数。

## UI

- 发布计划树显示“受管远端漂移”，并在无直接实体引用时以参数 key 和远端变化类型作为主要上下文。
- “已同步”文案改为“本地无未发布修改”或同义表达，避免暗示 Firebase 已完成全量一致性校验。
- 远端一致性以最新 Plan 的对账结果为准；Draft dirty 状态只表达本地实体相对发布基线的变化。

## 非范围

- 自动删除未受管 Firebase 参数。
- 建模、合并或改写 Firebase 条件值。
- 将 Firebase 远端内容反向写入配置包。
- 改变 v1 配置包的既有按语义变化投影行为。

## 验收

- 已有成功发布基线且本地目标未变、远端为空：每个受管 key 生成一个 `managed_remote_drift` addition。
- 远端部分缺失：只补齐缺失或漂移的受管 key；完全一致时远端变化为零。
- 远端未受管额外参数被保留并提示，不进入发布删除集合。
- Firebase 存在条件值时 Plan 阻断，发布合并测试证明条件值不被静默覆盖。
- 多个实体映射同一频控、广告位或策略聚合参数时，远端变化按 key 去重并聚合全部原因。
- 自定义参数、Feature Switch、频控、Placement、广告策略、网络模式与网络设置均参与完整受管集合对账。
- 已有成功发布基线但远端缺参时，零本地变化的 Plan 仍能恢复缺失参数。
- 首次发布 source 已含完整环境配置且远端为空时，产生由本地 additions 驱动的远端 additions，不生成 `managed_remote_drift`，也不出现同字段删除与添加成对变化。
- PDF Home Pro Development 等价 fixture：远端已有 1 个参数、完整目标 37 个参数时，Plan 产生 36 个 `managed_remote_drift` additions；合并后参数总数为 37，未修改本地配置值。
- OpenAPI、生成类型、Go 测试、Web 测试与文档一致，`make check` 通过。
