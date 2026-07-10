# Spec 008：构建计划、语义 Diff 与风险分析

> 状态：待实现  
> 依赖：Spec 005、007

## 目标

把当前草稿编译成可审阅、不可变的 Plan，回答“会改什么、影响谁、风险多大”，并作为发布的唯一输入。

## 范围

- Pack 编译为 provider-neutral desired configuration。
- 项目基线、环境覆盖、当前远端快照三方差异。
- 业务语义 diff、原始参数 diff、引用影响范围和风险等级。
- 不可变 `plan_id`、草稿 revision、source digest、remote ETag、生成时间和过期时间。
- review JSON / Markdown 与 Provider 输入 artifact。
- 计划过期、草稿改变或远端 ETag 改变时自动失效。

## API

- `POST /api/v1/drafts/{environment_id}:plan`
- `GET /api/v1/plans/{plan_id}`
- `GET /api/v1/plans/{plan_id}/artifacts/{artifact_name}`

## CLI

- `conflow plan --environment <id> [--format text|json]`
- `conflow plan ... --output <dir>`

## 风险示例

- 低：说明文字或无行为影响的 metadata。
- 中：单广告位关闭、unit binding 调整。
- 高：共享频控降低、全局开关、生产网络切换。
- 阻断：删除被引用策略、覆盖未建模条件值、远端基线缺失。

## 验收

- 180 秒改为 300 秒时，语义 diff 展示受影响广告位而非只展示 JSON 数字。
- 相同输入生成相同内容 digest；时间和本机路径不进入 digest。
- 修改草稿或拉取新远端模板后旧 plan 无法发布。
- artifact 不包含凭据或真实 token。
