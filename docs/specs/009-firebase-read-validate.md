# Spec 009：Firebase Provider 认证、拉取与预校验

> 状态：待实现  
> 依赖：Spec 002、005、008

## 目标

实现 Firebase Remote Config Provider 的只读与安全预校验能力，在任何发布前取得当前模板、ETag、版本和默认值事实。

## 范围

- Provider 接口：connect/status、pull、validate、capabilities。
- 本地认证资料只保存引用；支持的认证方式和凭据存储需在实现 PR 的安全说明中明确。
- 拉取当前模板、ETag 和版本 metadata；完整模板作为受保护快照保存。
- 调用 Firebase `validate_only`，映射上游错误为稳定领域错误。
- 检测受管理参数中的条件值、未知参数和模板结构差异。
- Operation 资源和 SSE 进度基础。

## Operation 编排边界

Provider 读取、预校验和 Plan 构建共享 Operation 合同，但不共享职责：Spec 009 只负责获得/验证受保护的 Firebase 远端事实，Spec 008 只负责冻结基于该事实的 Plan，Spec 010 才能写远端。

- `POST /remote:pull` 创建 `operation_type=remote_pull`，阶段为 `queued -> reading_remote -> snapshotting -> completed`；成功 result 指向 `remote_snapshot` 摘要，而非完整模板。
- `POST /remote:validate` 创建 `operation_type=remote_validate`，必须引用 `ready` Plan；阶段为 `queued -> validating_remote -> completed`。它不改变 Plan 的快照令牌，发布仍需再次验证。
- `POST :plan` 创建 `operation_type=plan`。它可复用未过期的受保护快照，但当需要读取时，Plan Operation 自己进入 `reading_remote`，不会要求 UI 先调用 pull。读取成功后进入 `compiling -> analyzing -> completed`。
- 远端读取失败不会自动写入、覆盖或删除本地缓存；Plan 可按 Spec 008 产生 `preview_only`，显式标记 non-publishable。显式 pull/validate Operation 则以结构化失败结束。

读取 Operation 的 `remote_state` 恒为 `unchanged`，因为它不写 Firebase；无法判断读请求是否已到达上游不改变这个结论。发布/回滚才可能为 `changed` 或 `unknown`。

## 脱敏远端值投影（UI-API-009）

新增 `GET /api/v1/environments/{environment_id}/remote/projection`，返回最新受保护快照的 provider-neutral `RemoteValueProjection`。每项固定为 `projection_id`、`entity_ref`、RFC 6901 `field_path`、`parameter_key`、`value_summary`、`snapshot_etag`、`observed_at`、`availability` 与 `redacted`；未映射、敏感或不可读的值不返回原值，只用稳定 availability/reason 表达。它不返回完整模板、条件值正文、unit ID 原值、认证信息或 Firebase token。

推荐独立只读 projection 端点，并允许 Plan 的 `remote_parameter_changes` 链接同一 `projection_id`。字段 caption 因而可以展示“线上当前值”而不要求先构建 Plan；Plan 页仍能把该值放入证据链。备选方案是只把投影嵌入 Plan：数据一致性最强，但编辑器需要先生成 Plan 才能显示线上值，且 Plan 过期后 caption 不可用。若独立端点的映射错误，代价是 UI 展示了错误对照但不影响发布，因为发布权威仍是 Plan/ETag；因此该端点明确为只读展示，不能作为发布前置条件。

## API

- `GET /api/v1/environments/{environment_id}/provider`
- `POST /api/v1/environments/{environment_id}/provider:connect`
- `POST /api/v1/environments/{environment_id}/remote:pull`
- `POST /api/v1/environments/{environment_id}/remote:validate`
- `GET /api/v1/operations/{operation_id}`
- `GET /api/v1/events?operation_id=<id>`
- `GET /api/v1/environments/{environment_id}/remote/projection`

## CLI

- `conflow provider status --environment <id>`
- `conflow pull --environment <id>`
- `conflow remote validate --environment <id> --plan <plan_id>`

## 非范围

- 发布、回滚和写入 Git。
- Firebase A/B Testing、Personalization 和 Analytics。

## 共享合同 fixture

`testdata/contracts/mobile-ad-monetization/v1/plan-risk-operation-rollback.json` 同时规定远端脱敏 projection、Plan 读取 Operation 与 `remote_state=unchanged` 的最小稳定场景。实现测试可以替换 fake Provider 的返回值，但不能向 fixture 加入完整 Firebase 模板或凭据。

## 验收

- fake Provider 覆盖认证成功/失败、ETag、超时、上游 4xx/5xx 和取消。
- 未建模条件值生成 blocking 诊断，不会被合并器静默删除。
- 日志、API 和快照 metadata 不泄露 token；完整快照不作为普通 API 响应返回。
- SSE 断线后可通过 Operation 轮询获得最终状态。
- 远端不可达时，explicit pull/validate 返回结构化失败；Plan 只可返回 `preview_only`，从不把旧 ETag 伪装为当前事实。
- projection 能以 `entity_ref + field_path` 供编辑器 caption 定位，且 API、日志和 fixture 都没有真实远端敏感值或完整模板。
