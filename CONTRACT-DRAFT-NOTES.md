# CONTRACT-DRAFT-NOTES

> 临时起草说明，不属于正式文档体系；供主控整理维护者决策包使用。

## 决策、备选与代价

| 议题 | 推荐 | 备选 | 若推荐选错的代价 |
|---|---|---|---|
| Plan 创建 | 始终 `202 + Operation`，成功后 GET Plan | 同步本地编译；或 UI 先 pull 后同步 | 对极快本地路径多一次轮询；但避免双响应形状和刷新后丢失状态 |
| 远端不可达 | 生成 `preview_only` 非发布 Plan | 让 Plan 直接失败 | 若 PM 误读状态会造成困惑，因此 API 与 UI 必须强制标记不可发布；换来离线审阅能力 |
| 线上当前值 | 独立脱敏 `/remote/projection`，Plan 只链接同一投影 | 仅嵌入 Plan | 独立 projection 可能与刚生成的 Plan 不同快照；它只供展示，不能授权发布 |
| 确认策略 | `Project.release_confirmation_policy` | 每环境字段；独立配置资源 | 将来若需每环境差异需迁移；当前与已收口的项目级交互一致 |
| 风险 | 服务端提供 risk items、reason code、requirements | UI 按 diff 推导 | 服务端规则初期实现成本高；但 UI 不会在版本升级后产生安全漂移 |
| ETag 冲突 | `412 remote_etag_mismatch` 携带最新脱敏摘要，强制重建 | 自动拉取/重试 | 多一次用户动作；避免静默覆盖远端变更 |
| 回滚 | 异步 preview + 不可变 preview ID，再以同级确认提交 | 直接从 Release 一键回滚 | 多一步网络读取；避免历史版本与当前远端之间的并发盲区 |
| 失败审计 | Release 保存失败 outcome 与 Operation failure，成功/失败明确区分 | 只保留成功记录 | 历史列表更复杂；换来可检索的失败证据且不伪造成功 |

## 合同不变量

- Plan 的 `ready` 是唯一可发布状态；`preview_only`、`invalidated`、`expired` 都不可发布。
- 草稿 revision、source digest、remote ETag 是独立令牌，错误 payload 不得混用其字段。
- 高风险逐项确认和 blocking 风险不可由项目策略、前端或 CLI 绕过。
- GET Operation 是恢复权威；SSE 只是增强。`remote_state=unknown` 只能引导核验，不能声称远端未改变。
- 回滚是新 Release，不改写旧审计记录；完整 Firebase 模板与敏感值不进入 API、fixture、日志或 artifact metadata。

## 维护者终审问题

1. 15 分钟 Plan/rollback preview 默认 TTL 是否符合真实发布审批节奏，或应改为更短的 5 分钟？
2. 是否确认 `Environment.publish.requires_confirmation` 作为兼容字段继续保留，还是允许 API 0.8.0 移除并要求 manifest 迁移？
3. `preview_only` 是否只允许“远端不可达”，还是也允许没有任何远端快照的首次离线项目？当前草案允许两者，但不允许发布。
4. 失败 Release 是否需要默认出现在常规“发布历史”列表，还是 API 默认仅列成功项并通过 query 显式包含失败？当前草案默认包含两者以保证审计可检索。
