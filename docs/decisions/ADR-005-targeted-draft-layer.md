# ADR-005：草稿定向替换配置层

> 状态：已接受（2026-07-11）

## 背景

项目配置由 Pack 默认值、项目基线和环境覆盖组成。早期设计把“当前草稿”画成最高优先级层，但没有说明草稿写的是项目基线还是环境覆盖。若把一份无作用域草稿直接盖在 effective value 上，保存时无法可靠回写源层，也无法回答修改影响哪些环境。

同时，空对象、缺失 replacement 和显式 `null` 有不同含义。仅靠可空 JSON 字段无法区分这些状态，多标签页和源文件外部变更也需要分别受本地 draft revision 与 source revision 保护。

## 决策

- 草稿是对 `baseline` 或 `environment_override` 目标层的完整 replacement，不是 effective value 之上的通用 overlay。
- 项目只有一份共享 baseline replacement；environment override replacement 按环境保存。API 路径中的 `environment_id` 是读取和写入视角，不把 baseline 草稿复制为环境私有数据。
- 先用存在的 draft replacement 替代对应 source 层，再按 `Pack defaults < resolved baseline < resolved environment override` 计算 effective value。
- 对象跨层递归合并，标量替换，数组整体替换，缺失字段回退；空对象不会清空低层对象。JSON `null` 仅在字段 schema 声明 `nullable=true` 时合法。
- API 使用显式 presence union 区分缺失、空对象与 `null`。`reset` 安装显式空对象 replacement，`discard` 移除 replacement。
- 所有 baseline 与 environment override 写入共用项目级、单调递增的 draft revision；Source Adapter 提供独立、不可混用的 source revision。
- 每个通过前置条件与校验的写入都递增 draft revision，即使 replacement 内容相同或 discard 目标已经缺失。
- DraftView 只展开共享 baseline 与路径环境的 override；dirty 状态和影响环境都归因于这两个 replacement，不汇总其他环境草稿。其他环境写入仍通过共享 revision 触发并发冲突。
- 服务端返回 effective value、逐字段来源与实际受影响环境；GUI 不复刻合并或影响分析逻辑。

## 理由

- 定向 replacement 能无歧义地写回 Managed File 或 Git JSON，并保持源适配器只负责持久化格式。
- 项目级 draft revision 让跨环境和多标签页修改进入同一并发域，避免 baseline 修改与其他环境保存静默覆盖。
- 显式 presence 与服务端 field state 能让 PM UI 准确解释“沿用源值”“已重置为空对象”和“显式无值”。
- 根据 effective value 实际变化计算影响环境，避免把 dirty 状态误当作发布影响。

## 后果

- 草稿存储必须保留 replacement presence，不能用零值对象代替缺失。
- 保存、reset 和 discard 都必须同时校验 draft revision 与 source revision；冲突返回同一原子快照。
- 对某环境读取 DraftView 时，需要组合项目共享 baseline、该环境 override、Pack schema 与源快照。
- 后续实体 CRUD 必须落到明确 write scope；Spec 004 不提前定义 Spec 006 的实体引用和业务规则。
