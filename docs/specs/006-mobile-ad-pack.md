# Spec 006：移动广告配置包领域模型

> 状态：待实现  
> 依赖：Spec 003、004

## 目标

实现首个内置 Pack `mobile-ad-monetization/v1`，让 PM 以业务实体管理广告位、频控、开关和环境绑定。

## 实体

- `placement`：稳定 ID、placement、类型、启用开关、network mode、频控引用、超时、缓存、fallback。
- `frequency_policy`：cooldown、interval、max count、shift count、positions。
- `feature_switch`：稳定配置 ID、key、默认值、风险和回滚方式。
- `unit_binding`：环境、平台、unit ID 引用和配置状态。

支持的首版广告类型：`app_open`、`interstitial`、`native`。新增类型必须升级 Pack schema。

## 范围

- 字段 schema、中文 UI metadata、默认值、ID / key 命名约束。
- 基线与环境字段边界：unit binding 必须按环境；稳定 ID 与 placement 不允许环境覆盖。
- 删除策略和引用查询入口。
- Provider-neutral 编译中间模型；Firebase 参数编译留给 Spec 008/009。
- 建立版本化、机器可读的 PDF Launcher 等价 contract fixture；Go golden tests、API tests 与 UI E2E 直接复用，`docs/design/ui/prototypes/fixtures.md` 作为人类可读说明。

## API

通过 Spec 003/004 的 Pack 与 draft 通用 API 暴露实体；不增加广告专用顶层路径。

## 非范围

- Banner、实验、竞价策略、收益分析和广告 SDK 管理。
- Firebase 发布和 Git JSON 兼容。

## 验收

- 共享 contract fixture 可表达 20+ 广告位、共享频控和多环境 unit binding，并由 Pack golden test 与后续 UI E2E 读取同一份数据。
- UI schema 不要求 PM 直接填写长 JSON。
- 删除被引用频控策略前能返回引用者清单。
- Pack schema、默认值和序列化有 golden tests。
