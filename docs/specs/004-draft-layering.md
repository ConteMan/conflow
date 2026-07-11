# Spec 004：草稿与分层配置模型

> 状态：已实现  
> 依赖：Spec 002、003

## 目标

建立可解释、可并发保护的定向草稿模型。PM 可选择修改项目“通用值”（baseline）或当前环境专属值（environment override），API 返回服务端计算的 effective value、字段来源与实际受影响环境，前端不自行模拟分层。

## 核心模型

草稿是目标层的完整 replacement，不是 effective value 之上的最高优先级 overlay：

```text
resolved baseline = draft.baseline ?? source.baseline
resolved environment override =
  draft.environment_override[environment_id]
  ?? source.environment_override[environment_id]

effective = Pack defaults < resolved baseline < resolved environment override
```

- `baseline` replacement 在项目内共享；从任何环境路径修改 baseline 都写入同一项目草稿。
- `environment_override` replacement 只属于路径指定的环境。
- `environment_id` 是 GET 和写入时的视角，不创建环境私有的 baseline 副本。
- 所有 scope 的写入共用项目级、单调递增的 `draft revision`；源适配器的 `source_revision` 是独立并发域。
- `??` 只判断 replacement presence。显式 `{}` 是存在的 replacement；缺失才回退到 source。

## 合并与值语义

- 对象递归合并。
- 标量替换低层值。
- 数组整体替换，不按索引或实体 ID 隐式合并。
- 上层字段缺失时回退到低层。
- 空对象 `{}` 不清空低层对象；reset 为 `{}` 可能 dirty 但 effective value 不变。
- 显式 JSON `null` 是一个值，不等于缺失；只有 Pack `FieldSchema.nullable=true` 时合法。
- Draft layer state 分别暴露 `source`、`draft`、`resolved` 与 `dirty`。`draft` 使用 `{"present":false}` / `{"present":true,"value":{...}}` union 区分 missing 与显式 `{}`。
- Field state 将 `pack_default`、`baseline`、`draft_baseline`、`environment_override`、`draft_environment_override`、`effective` 扁平返回，前端不拆解嵌套 source/draft 状态。每个值都使用 `{"present":false}` / `{"present":true,"value":...}` union；`value` 可以是 schema 允许的 `null`。
- Field state 同时必返 `environment_override_allowed`、`is_environment_overridden`、`source_revision` 和 `nullable` 展示信息。
- 字段 path 使用 RFC 6901 JSON Pointer；`~` 与 `/` 分别编码为 `~0` 与 `~1`。
- 字段来源 `origin` 固定为 `pack_default`、`baseline`、`draft_baseline`、`environment_override` 或 `draft_environment_override`。origin 必须区分 source 与 draft；Pack 当前要求字段提供 default，因此 effective field 不使用 `missing` origin。

## API

- `GET /api/v1/drafts/{environment_id}`
- `PUT /api/v1/drafts/{environment_id}`
- `POST /api/v1/drafts/{environment_id}:reset`
- `POST /api/v1/drafts/{environment_id}:discard`

所有成功响应返回 `DraftView`：

- `environment_id`、`pack_ref`、`source_revision`；
- `dirty`、稳定排序且非 null 的 `dirty_scopes`；二者只汇总当前视角可见的共享 baseline replacement 与当前 `environment_id` 的 override replacement，不汇总其他环境的 override 草稿。
- `baseline`、`environment_override` 的 layer state；
- `effective`、按 RFC 6901 path 稳定排序且非 null 的 `field_states`；
- 服务端计算、按项目环境顺序稳定返回且非 null 的 `affected_environments`。

最小读响应形状如下；实际字段状态由 Pack schema 展开：

```json
{
  "data": {
    "environment_id": "development",
    "pack_ref": "fixture-config/v1",
    "source_revision": "src-1",
    "dirty": false,
    "dirty_scopes": [],
    "baseline": {
      "source": {"present": true, "value": {}},
      "draft": {"present": false},
      "resolved": {"present": true, "value": {}},
      "dirty": false
    },
    "environment_override": {
      "source": {"present": false},
      "draft": {"present": false},
      "resolved": {"present": false},
      "dirty": false
    },
    "effective": {},
    "field_states": [],
    "affected_environments": []
  },
  "meta": {"request_id": "req_01JEXAMPLE", "revision": 1}
}
```

`affected_environments` 只归因于当前 DraftView 展开的两个 replacement：共享 baseline 与当前 `environment_id` 的 override。服务端比较“保留这两个 replacement”与“仅移除这两个 replacement、其他环境草稿状态保持不变”的 effective value；只有实际变化的环境才进入集合。baseline 草稿可能影响多个环境；被更高层环境覆盖遮蔽的 baseline 字段不算该环境受影响。当前视角没有 replacement 时，即使其他环境有 override 草稿，`dirty=false`、`dirty_scopes=[]`、`affected_environments=[]`；但其他环境写入仍会递增项目级 draft revision，使旧 ETag 冲突。

### 写请求

`PUT` 完整替换一个目标 scope，不提供通用 JSON Patch：

```json
{
  "expected_source_revision": "src_01J...",
  "write_scope": "baseline",
  "configuration": {}
}
```

`reset` 与 `discard` 使用相同前置条件：

```json
{
  "expected_source_revision": "src_01J...",
  "write_scope": "environment_override"
}
```

- 三个写动作都必须携带 `If-Match`，其 ETag 是项目级 draft revision。
- `PUT` 安装请求中的完整 replacement。
- `reset` 安装显式 `{}` replacement；它不同于清空低层值。
- `discard` 移除目标 replacement，使该层重新解析为当前 source 值。
- v1 对每个通过前置条件和校验的写请求都递增一次项目级 draft revision，即使 PUT 内容相同、重复 reset，或 discard 的 replacement 本来就 missing；客户端可以据此把一次已接受写入视为消耗了旧 ETag。
- 写 baseline 时，响应仍以路径中的 `environment_id` 为当前视角，同时 `affected_environments` 返回所有实际受影响环境。

### 固定校验顺序

每次写入按以下顺序执行；前一步失败时不继续后续步骤：

1. 校验 Origin 与 Content-Type。
2. 在解码请求体之前校验 `If-Match` 存在且格式有效。
3. 校验 JSON 语法、未知请求字段和必填字段。
4. 校验项目、环境与 Pack 存在，`write_scope` 可用于当前请求。
5. 在同一锁或事务中读取 draft revision、source revision 与当前 DraftView 快照。
6. 比较 `If-Match`；不匹配返回 `412 revision_mismatch`。
7. 比较 `expected_source_revision`；不匹配返回 `412 source_revision_mismatch`。
8. 对 `PUT` 按 Pack schema 校验 replacement 的结构、字段可空性及 environment override 权限；reset/discard 没有 configuration，跳过 structural validation，也不声明 `422`。
9. 按 `(scope, path, code)` 稳定排序 `PUT` 的全部 structural errors；存在错误时返回 `422 validation_failed`。
10. 原子提交 replacement，递增项目级 draft revision，并从同一提交后快照构造响应与 ETag。

`revision_mismatch` 与 `source_revision_mismatch` 都返回 `current_revision`、`current_source_revision`、`conflict_scope` 和权威 `current_state`；`conflict_scope` 表示本次失败请求试图写入的 scope，不声称能定位哪一次并发写入改变了哪个 scope。响应 ETag、revision 和快照必须来自同一次原子读取。前端用自己的输入与该快照做对照，不自动合并或覆盖。

结构错误使用既有顶层错误码 `validation_failed`；detail 固定包含 `code`、RFC 6901 `path`、`scope` 与人类可读 `message`。detail code 稳定为 `invalid_config_shape`、`field_type_mismatch`、`required_field_missing`、`value_not_allowed`、`explicit_null_forbidden` 或 `environment_override_forbidden`。Spec 004 只负责结构、nullable 与 environment override 权限，不执行引用完整性、领域业务规则或发布 readiness；后者由 Spec 006/007 定义。

## 范围

- baseline、environment override、targeted replacement 与 effective view。
- layer state、field state、origin、overrideability 和 source revision。
- 项目级 draft revision、dirty、reset、discard 与源变更冲突。
- Pack 控制哪些字段允许环境覆盖。
- 空值、缺失、显式 `null`、对象和集合合并语义。
- 可供 Go/API/UI 共用的版本化 contract fixture：[`testdata/contracts/drafts/v1/layering.json`](../../testdata/contracts/drafts/v1/layering.json)。

## 非范围

- 广告等业务实体字段、引用查询和受限删除；由 Spec 006 定义。
- 引用完整性、业务规则、完整校验结果和发布就绪度；由 Spec 007 定义。
- Managed File 的落盘和 source revision 生成方式；由 Spec 005 定义。
- 多人实时协同、字段级自动冲突合并和通用 JSON Patch。

## 实现门槛

- 本 contract-only PR 只冻结合同，不改运行时代码。实现 Spec 004 时必须同步 Go `FieldSchema`、内置 Pack schema 与 Handler tests，使 required `nullable` 字段与 OpenAPI `0.5.0` 对齐；未完成前不得把 Spec 004 标记为“已实现”。
- Draft Store、HTTP Handler 与 CLI 用例必须共用项目级 revision 和同一套固定校验顺序，不能各自解释 presence 或 no-op。

## 验收

- 共享 fixture 与表驱动测试覆盖 targeted replacement、对象递归、标量/数组替换、缺失回退、missing/null、nullable、禁止环境覆盖、reset/discard、两类 revision 冲突和实际影响环境。
- API 返回 effective value、layer state 与 field state，前端不自行合并。
- 多环境或多标签页使用旧项目级 draft revision 保存时返回 typed `412`，不覆盖新值。
- source revision 改变时返回独立的 typed `412`，不得与 draft ETag 混用。
- 其他环境 override 写入会使旧项目级 ETag 冲突，但不会污染当前视角的 dirty、dirty scopes 或 affected environments。
- 相同 replacement、重复 reset 与 discard-missing 均作为已接受写入递增 revision 一次。
- discard 后恢复到当前 source 状态；reset 后保留显式、合法的 `{}` replacement。
- OpenAPI、生成 TypeScript 类型、设计文档、ADR 与 contract fixture 同步，`make check` 通过。
