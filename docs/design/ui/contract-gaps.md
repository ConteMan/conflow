# UI 假设与后端合同缺口

> 状态：活动清单（首次审计：2026-07-10）。本清单记录 UI flows / 原型已经依赖、但尚未被后端 Spec、HTTP 语义或 OpenAPI 完整表达的行为。它不是 API 事实源；缺口只有在对应 Spec、`docs/design/http-api.md`、`api/openapi.yaml` 和契约测试合并后才算关闭。

## 使用方式

- UI Spec 开工前，必须关闭它所依赖的阻塞项，或在 Spec 中明确降级范围。
- 每个缺口通过独立的 contract-only PR 收口：只修改 Spec、跨域 HTTP 语义、OpenAPI、示例、生成类型和 contract fixture，不混入 Handler 或 React 实现。
- flows / 状态矩阵新增后端行为假设时，必须引用已合并合同或本清单 ID；不能只把行为画进原型。
- `fixtures.md` 保留给人阅读。可执行事实源在 Spec 006 建立为仓库级结构化 contract fixture，Go golden tests、API tests 和 UI E2E 读取同一份数据，不解析 Markdown。

## Spec 013 开工门槛

| ID | UI 假设与证据 | 当前缺口 | 最小合同建议 | 归属 |
|---|---|---|---|---|
| `UI-API-001` | 顶栏必须持续识别 Production，并显示环境显示名（`flows/01-project-environment-switch.md:13,17,27`；`docs/specs/013-ui-project-shell.md:12-14`）。 | API 要求 ID 不透明，但 `Environment` 只有 `id`，没有显示名或环境类别；前端不能用 `id == "production"` 推断。 | `Environment` 增加稳定的环境类别与显示名；Production 风险语义由服务端字段决定。先更新 Spec 002 与 OpenAPI。 | 002 / 013 |
| `UI-API-002` | 项目或环境保存遇到 `412` 时，UI 要展示最新状态并允许重载（`flows/01-project-environment-switch.md:16`；Spec 013 验收包含 revision 冲突）。 | 通用错误只保证 `current_revision`，未规定如何取得与该 revision 对应的当前资源；直接再次 GET 的一致性协议也未定义。 | 为 `revision_mismatch` 规定 typed conflict payload，或规定带新 ETag 的权威 reload URL / GET 流程；必须能稳定取得服务端当前资源，且不把 diff 交给错误 message。 | 002 / 013 |
| `UI-API-003` | 顶栏有全局“未发布修改”位置，切换环境后仍要恢复认知（`DESIGN.md:31`；`pm-main-path.md:42,49`）。 | Bootstrap 和环境响应没有 draft summary；013 又不依赖 004。 | 013 只建立状态槽，不伪造 dirty；004 定义按环境的 draft summary 后再接入。若 013 验收要求真实状态，则先将 004 加为合同依赖。 | 004 / 013 |
| `UI-API-004` | 确认强度是项目级设置，项目可放宽低风险 Production Plan 的确认（评审记录 `2026-07-10-structure-directions.md:9,57`）。 | `Project` 没有发布策略，`Environment.publish` 只有 `requires_confirmation: boolean`，无法表达已收口的策略。 | 由 Spec 010 定义策略语义与默认值，再回写 Spec 002/OpenAPI 的项目模型；013 只在合同合并后展示设置。 | 002 / 010 / 013 / 015 |

`UI-API-001`、`UI-API-002` 的合同阻塞已关闭。`UI-API-003` 通过“只实现状态槽”降级；`UI-API-004` 延后到 015，013 不得先创造临时字段。

处理状态（2026-07-10）：

- `UI-API-001`：合同已定义。`Environment.name/kind` 进入 OpenAPI；只有 `kind=production` 驱动 Production UI。
- `UI-API-002`：合同已定义。manifest 写冲突返回原子 `current_state`、`current_revision` 与匹配 ETag。
- `UI-API-003`：已降级，不阻塞 013。013 只实现状态槽，真实 dirty 由 004 接入。
- `UI-API-004`：已延后，不阻塞 013。项目级确认策略由 010/015 收口，013 不创建临时字段。

“合同已定义”表示前后端可以基于已合并 OpenAPI 并行实现；对应运行时代码和 E2E 仍须在实现 PR 中完成，不能把 mock 视为验收通过。

## Spec 014 开工门槛

| ID | UI 假设与证据 | 当前缺口 | 最小合同建议 | 归属 |
|---|---|---|---|---|
| `UI-API-005` | PM 可以选择修改“通用值”或“只改当前环境”，并在保存前看到影响环境（`pm-main-path.md:41`；`flows/02-placement-editing.md:23`）。 | 004 只列出按环境的整份 draft PUT，没有定义基线与环境覆盖分别写到哪里，也没有影响环境 read model。 | Draft DTO 明确 `baseline`、`environment_override`、`effective` 和写入 scope；服务端返回影响环境集合，前端不自行模拟四层合并。 | 004 |
| `UI-API-006` | 每个字段显示通用值、当前环境专属值、effective value、是否可覆盖和来源 caption（`DESIGN.md:34,87`；Spec 014:14,21）。 | 004 只描述“每个字段携带 origin”，没有可序列化结构、JSON Pointer 约定或 null/缺失表达。 | 定义 field-state read model：稳定 path、各层值、effective value、origin、overrideability、source revision；固定 object/array/null 语义。 | 004 |
| `UI-API-007` | C3 保存冲突要对照“我的输入”和“服务端当前值”（`flows/02-placement-editing.md:17`；Spec 014:24）。 | `412` 只有 revision；整份 draft 可能过大，且没有实体级 reload / comparison 合同。 | 004 contract PR 必须选择 typed current snapshot 或原子 reload 协议；冲突响应至少携带当前 draft revision 和发生冲突的 scope。UI 只做展示，不做自动合并。 | 004 / 014 |
| `UI-API-008` | UI 按实体 CRUD、查询引用者，并在删除被引用策略时展示引用清单（`flows/02-placement-editing.md:18,24`）。 | 004 仅有整份 draft API；006 说复用通用 API，但没有实体路径、引用查询或受限删除响应。 | 定义 Pack-neutral entity resources、稳定实体引用和 `referenced_by[]`；删除冲突返回 typed references，不靠解析 message。 | 004 / 006 |
| `UI-API-009` | 字段 caption 还要表达“线上当前值”（评审记录 `2026-07-10-structure-directions.md:47`；Spec 014:14）。 | 009 保护完整 Firebase 快照，不返回普通 API；尚无 provider-neutral 的受管理字段远端投影。 | 明确是 014 提供只读 remote projection，还是把线上对照严格延后到 Plan。若保留字段 caption，009/008 必须返回脱敏、可映射到字段 path 的远端值摘要。 | 008 / 009 / 014 |
| `UI-API-010` | 保存时字段错误行内展示；完整校验返回可跳转诊断（`flows/02-placement-editing.md:15`；`flows/03-validation.md:14,24`）。 | 004 写入校验与 007 完整校验的边界不清；OpenAPI 尚无诊断 DTO。 | 固定写入时 structural errors 与显式 validate diagnostics 的分工；二者共享稳定 `code/path/entity_ref`，但 readiness 只由 007 产生。 | 004 / 007 |
| `UI-API-011` | 校验页显示上次时间、基于哪个草稿版本、是否过期和当前环境 readiness（`flows/03-validation.md:11-19`）。 | 007 定义单条诊断字段，但没有 ValidationResult、freshness 或 readiness 合同。 | 定义 `validated_draft_revision`、`validated_at`、`status/stale`、`readiness` 与 diagnostics；草稿变化后旧结果保留但明确过期。 | 007 |
| `UI-API-012` | UI 使用“阻断 / 警告 / 建议”，并认为只有阻断影响 readiness（`flows/03-validation.md:23`）。 | 007 使用 `info/warning/error/blocking`，CLI 又规定 `error/blocking` 非零；“error 是否阻止 Plan”存在语义分歧。 | 在 007 中固定 severity、CLI exit 和 publish readiness 的映射；UI 文案只做稳定枚举的本地化。 | 007 / 014 / 015 |
| `UI-API-013` | UI 列表、诊断、大 diff 与后端 golden tests 必须使用同一组实体 key 和场景。 | 当前 `fixtures.md` 是人工文档，不能直接作为 Go / E2E 测试输入；原型与主路径已出现命名、计数漂移。 | Spec 006 建立版本化结构化 fixture；场景 overlay 分别覆盖 validation、Plan、412 和 ETag 冲突；Go 与 E2E 读取同一文件并断言稳定业务结果。 | 006 / 007 / 008 / 014 |

以上条目全部关闭后再启动 014。特别是 `UI-API-005` 到 `UI-API-008`，它们决定编辑器的数据边界，不能由 React 先发明 DTO。

## Spec 015 开工门槛

| ID | UI 假设与证据 | 当前缺口 | 最小合同建议 | 归属 |
|---|---|---|---|---|
| `UI-API-014` | Plan 进入页自动构建，并区分本地编译、远端读取和远端不可达（`flows/04-plan-diff.md:11-17`）。 | 008 未规定同步响应还是 Operation；009 才定义远端 pull，二者编排边界不清。 | `POST :plan` 固定同步/异步语义；若返回 Operation，定义稳定 stage、失败码、结果 Plan ID。远端不可达时是否允许 non-publishable preview 也必须由合同决定。 | 008 / 009 |
| `UI-API-015` | Plan 用“业务变更 → 受影响实体 → 远端参数”的树展示（评审记录 `2026-07-10-structure-directions.md:42`）。 | 008 只有概念，没有可直接渲染的 DTO；UI 可能被迫从 raw Firebase diff 推导业务影响。 | Plan 返回稳定节点 ID、semantic changes、affected entities、remote parameter changes 和 artifact metadata；业务影响由服务端/Pack 判定。 | 008 |
| `UI-API-016` | 风险等级、原因、高风险逐项确认和阻断项由权威结果驱动（`flows/04-plan-diff.md:13,18`；`flows/05-production-release.md:12,23`）。 | 008 只有风险示例，没有枚举、规则来源、risk item ID 或 confirmation requirement。 | 定义风险枚举、稳定 reason code、`risk_items[]`、`blocking_reasons[]` 和服务端计算的 `confirmation_requirements`。前端不得重算风险。 | 008 / 010 |
| `UI-API-017` | Plan 绑定 draft revision、source digest、remote ETag 和 TTL，任一变化即失效（`flows/04-plan-diff.md:25`）。 | 008 未定义生命周期状态、失效时机和错误码；HTTP 只要求“未过期 Plan”。 | Plan 返回 `status`、快照令牌、`expires_at` 和稳定 invalidation reason；010 对过期与失效分别返回固定错误码。 | 008 / 010 |
| `UI-API-018` | ETag 冲突后强制重建 Plan，并高亮线上侧变化（`flows/04-plan-diff.md:17`；`flows/05-production-release.md:17`）。 | 010 只要求返回 `412 remote_etag_mismatch`，没有最新远端摘要或恢复协议。 | 规定冲突后如何取得新 remote snapshot/version 摘要与对比；本地 revision 和 remote ETag 始终使用不同字段与错误域。 | 008 / 009 / 010 |
| `UI-API-019` | Production 确认提交环境 ID 和逐项风险确认（Spec 015:14）；高风险项任何项目设置都不能跳过。 | HTTP 仍写“确认短语或二次确认状态”，没有结构化 confirmation schema。 | Release request 定义 `confirmation.environment_id`、`acknowledged_risk_item_ids`；服务端按 Plan requirements 和项目策略逐项校验。 | 010 |
| `UI-API-020` | 发布展示预校验、提交、确认生效；失败必须回答线上是否变化；关闭页面后可恢复（`flows/05-production-release.md:13,16`；Spec 015:25,40）。 | Operation 只有通用状态概念，缺 stage、result、远端结果确定性和恢复发现方式。 | Operation 定义稳定 stage、结构化 failure、`remote_state: unchanged|changed|unknown`、result resource；SSE 仅增强，GET 轮询是权威恢复路径。 | 009 / 010 |
| `UI-API-021` | 回滚前展示目标版本与差异，使用与发布同级的 ETag、幂等和确认（`flows/05-production-release.md:19,25`）。 | 011 有回滚动作，但没有 preview、confirmation 或 Operation DTO。 | 定义 rollback preview/read model、回滚请求、confirmation、Idempotency-Key 和 Operation result；回滚仍生成新的 Release。 | 011 |

## 已发现的术语与数据漂移

| 问题 | 处理 |
|---|---|
| 主路径使用 `app_open_cooldown`，fixture 的 key 是 `app_open_session_gate`。 | 以 fixture key 为准；主路径改为 `app_open_session_gate`。原型首轮截图保留为历史证据，不作为实现命名事实源。 |
| 主路径与发布 flow 写“输入环境名”，评审已收口为“输入环境 ID”。 | flows 统一改为“环境 ID”；UI 可同时展示人类可读名称，但确认值使用 API 返回的 ID。 |
| “14 处业务变更”混合了直接编辑和派生影响，表中实际是 5 项直接修改与 10 个受影响广告位。 | fixture 改为分别统计“5 项直接修改 / 10 个受影响实体”；Plan DTO 后续也分别返回 direct changes 与 affected entities。 |
| Spec 008 示例使用 `180 → 300`，共享大 diff 使用 `30 → 120`。 | 允许作为两个具名 scenario，但 contract fixture 必须分别定义输入和期望结果，禁止把数值当成同一场景。 |
| UI 使用“通用值 / 此环境专属值 / 未发布修改”，后端使用 `baseline / environment_override / draft`。 | 技术字段保留英文稳定名；PM 文案映射集中维护在 `flows/README.md`，组件不得各自翻译。 |

## Contract-only PR 拆分

1. **013 前置合同：项目、环境与冲突 read model（已完成）**
   `UI-API-001`、`UI-API-002` 已关闭；`UI-API-003`、`UI-API-004` 已明确降级或延后。API `0.4.0`、HTTP 语义、Spec 002/013、生成类型和响应示例已经对齐。
2. **004 草稿合同**  
   关闭 `UI-API-005` 到 `UI-API-007`、`UI-API-010` 的草稿部分。先合并合同，再实现 004。
3. **006/007 领域与校验合同**  
   关闭 `UI-API-008`、`UI-API-010` 到 `UI-API-013`。同时提交可执行 fixture 第一版。
4. **008–010 Plan / 风险 / 发布合同**  
   关闭 `UI-API-014` 到 `UI-API-020`，统一 Plan 生命周期、ETag 恢复、风险权威和 Operation。
5. **011 审计与回滚合同**  
   关闭 `UI-API-021`，再进入 Spec 015 的回滚 UI。

## 审计结论

- Spec 013 的合同前置已满足，可以开始应用壳、Production 状态和 revision 冲突交互；最终 E2E 前仍须完成 API `0.4.0` 的 Go 运行时对齐。
- Spec 014 必须等待 004、006、007 的 contract-only PR 全部合并。
- Spec 015 必须消费服务端权威的 readiness、Plan lifecycle、risk items、confirmation requirements 和 Operation state，不能在前端复制规则。
