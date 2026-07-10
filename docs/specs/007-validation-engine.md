# Spec 007：校验、引用完整性与发布就绪度

> 状态：待实现  
> 依赖：Spec 006

## 目标

将字段、引用、业务和环境校验统一为稳定诊断模型，供 CLI、API 和 GUI 共用。

## 校验层级

1. 字段：必填、类型、枚举、命名、范围。
2. 引用：开关、频控、广告位和环境绑定存在且类型匹配。
3. 业务：权限前禁弹、失败不阻断、类型与频控兼容、危险阈值。
4. 环境：Production unit、Provider、凭据引用和禁止覆盖字段。
5. 发布就绪度：未解决错误、未建模条件值、远端基线和计划状态。

## 诊断模型

每条诊断包含稳定 `code`、JSON Pointer `path`、`severity`、中文 message、实体引用、修复建议和可选文档链接。severity 固定为 `info`、`warning`、`error`、`blocking`。

## API

- `POST /api/v1/drafts/{environment_id}:validate`
- `GET /api/v1/drafts/{environment_id}/diagnostics`

## CLI

- `conflow validate --environment <id> [--json]`
- 有 `error` / `blocking` 时退出码非零；warning 不改变退出码。

## 非范围

- 自动修改配置。
- 执行任意用户脚本或 CEL/Rego 规则。

## 验收

- 同一 fixture 经 CLI 和 API 得到相同诊断 code/path/severity。
- 自定义频控对象复用完整频控校验，不只检查“是对象”。
- Production 缺少 unit binding、删除被引用策略、非法广告类型均有稳定诊断。
- 前端不解析 message 即可定位字段和风险等级。
