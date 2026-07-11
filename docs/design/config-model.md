# 配置模型

> 状态：M1 基线。首个 Pack 的字段允许在对应 Spec 中演进；分层语义变更需新增 ADR。

## 名词

| 名词 | 含义 |
|---|---|
| 项目（Project） | 一个 App 或业务产品，例如 PDF Launcher |
| 环境（Environment） | 同一项目的 development、staging、production 等发布范围 |
| 配置包（Pack） | 一个受版本管理的业务模型，例如移动广告配置 |
| 源适配器（Source Adapter） | 配置保存和读取方式，例如 Git JSON |
| 发布适配器（Provider Adapter） | 发布目标交互方式，例如 Firebase Remote Config |

## 分层与定向草稿

草稿不是叠加在所有配置之上的第五层，而是对一个目标层的完整替换。每个项目只有一份共享的项目基线草稿；每个环境可有一份环境覆盖草稿。读取某个环境时，先分别解析两个目标层，再计算 effective value：

```text
resolved baseline = draft.baseline ?? source.baseline
resolved environment override =
  draft.environment_override[environment_id]
  ?? source.environment_override[environment_id]

effective =
  Pack 安全默认值
  < resolved baseline
  < resolved environment override
```

这里的 `??` 表示草稿 replacement 是否存在，不是 JSON 的 null 合并运算。显式安装的空对象 `{}` 仍然是“存在的 replacement”，与没有草稿不同。baseline replacement 在项目内共享，因此从任一环境视角修改 baseline 都会影响同一份项目草稿；`environment_id` 只决定读取视角和 environment override 的目标。

层间合并固定为：对象递归合并，标量替换，数组整体替换；上层字段缺失时回退到低层。空对象不会清空低层对象；显式 `null` 只有在 Pack 字段声明 `nullable=true` 时才是合法值。Firebase 当前值不参与上述优先级；它用于生成发布计划、检测并发变化与发布后验证。

详细的草稿 presence、字段来源和并发合同见 [Spec 004](../specs/004-draft-layering.md) 与 [ADR-005](../decisions/ADR-005-targeted-draft-layer.md)。

## 项目清单

每个项目根目录可使用 `.conflow/project.yaml` 作为可提交的项目入口：

```yaml
version: 1
project:
  id: photo-editor
  name: Photo Editor
  release_confirmation_policy:
    production_low_risk_mode: environment_id
pack:
  id: mobile-ad-monetization/v1
source:
  type: managed-file
environments:
  - id: development
    name: Development
    kind: development
    provider:
      type: firebase-remote-config
      project_id: photo-editor-dev
  - id: production
    name: Production
    kind: production
    provider:
      type: firebase-remote-config
      project_id: photo-editor-prod
    publish:
      requires_confirmation: true
```

环境 `id` 是稳定、不透明的技术标识；`name` 是 PM 可见名称；`kind` 固定为 `development`、`staging`、`production` 或 `custom`。客户端只根据服务端返回的 `kind` 判断 Production 风险状态，不从 `id`、`name`、Provider project ID 或确认开关推断。`id` 与 `kind` 创建后不可变，`name` 可编辑；同一项目允许存在多个同类环境。

Conflow 尚未冻结稳定 manifest 格式，本约束直接纳入 manifest version 1。此前的实验性 manifest 必须显式补齐 `name` 与 `kind`；服务端不得静默猜测环境类别。

项目可选 `git-json` 源适配器，以读取并写回既有仓库中 `config/remote-config/` 一类结构。适配器映射是项目配置，不承担广告业务规则。

## Git JSON Mapping Profile

`git-json` 项目在 manifest 的 `source.profile` 指向仓库根目录内的声明式 YAML profile。profile 只描述既有 JSON/YAML 字段与 Pack 中立配置集合之间的格式映射；它不包含项目名称、发布凭据或业务校验规则。路径使用 RFC 6901 JSON Pointer，文件路径必须是仓库内相对路径。

```yaml
source:
  type: git-json
  profile: config/conflow-ad-profile.yaml
```

profile 的版本当前固定为 `1`。每个 `files` 条目声明可读写文件及其 `json` 或 `yaml` 格式；每个 `mappings` 条目将一个记录数组映射到一个集合。`scope: baseline` 写入项目基线，`scope: environment_override` 通过 `environment_id_path` 将记录分配到对应环境覆盖。`id_path` 和 `fields` 都相对于数组中的单条记录。

```yaml
version: 1
files:
  - path: config/ads.json
    format: json
mappings:
  - name: parameters
    collection: feature_switches
    scope: baseline
    file: config/ads.json
    records_path: /parameters
    id_path: /key
    fields:
      key: {path: /key}
      default_value: {path: /enabled, transform: string_to_boolean}
      risk_level: {path: /risk}
      rollback_method: {path: /rollback}
  - name: frequency
    collection: frequency_policies
    scope: baseline
    file: config/ads.json
    records_path: /frequency
    id_path: /id
    fields:
      cooldown_ms: {path: /cooldown_seconds, transform: seconds_to_milliseconds}
      interval_ms: {path: /interval_ms}
      max_count: {path: /max_count}
      shift_count: {path: /shift_count}
      positions: {path: /positions}
  - name: placements
    collection: placements
    scope: baseline
    file: config/ads.json
    records_path: /placements
    id_path: /id
    fields:
      key: {path: /key}
      ad_type: {path: /type}
      enabled: {path: /enabled}
      network_mode: {path: /network}
      frequency_policy_id: {path: /frequency_id}
      load_timeout_ms: {path: /timeout_ms}
      cache_policy: {path: /cache}
      fallback_behavior: {path: /fallback}
  - name: environment-bindings
    collection: unit_bindings
    scope: environment_override
    file: config/ads.json
    records_path: /unit_bindings
    id_path: /id
    environment_id_path: /environment
    fields:
      placement_id: {path: /placement}
      environment_id: {path: /environment}
      platform: {path: /platform}
      unit_id_ref: {path: /unit_id}
      status: {path: /status}
```

支持的转换是 `identity`（或省略）、`seconds_to_milliseconds` 与 `string_to_boolean`，写回时使用其逆转换。未参与 mapping 的文件字段及记录字段保持原样。任何已映射字段缺失、转换失败、未知转换或出现 `conditionalValues` 条件值都会产生 round-trip 阻断诊断；`source:import` 和保存均不得绕过该阻断。保存前 `source:preview-save` 返回逐文件 diff、受影响文件和 source digest；保存只在 Git 工作区干净且 source revision 未变化时执行同目录原子替换。

`project.release_confirmation_policy.production_low_risk_mode` 是项目级、版本化的发布确认策略，取值为 `environment_id`（默认）或 `acknowledgement`。为兼容已有 manifest，省略字段等价于默认值；新建或迁移后的 manifest 应显式写入默认值。它只放宽低风险 Production Plan 是否必须输入环境 ID；高风险逐项确认、blocking 风险和最终确认要求由服务端按 Plan 计算，不能由 UI 或环境名称推断。`environment.publish.requires_confirmation` 保留为一般确认适用性，不能用来表达确认强度。

## 环境覆盖

项目基线承载广告位、频控策略与失败兜底；环境覆盖承载开关、unit ID、网络模式及明确允许的阈值差异。Pack 可将字段标记为“不可按环境覆盖”。

这样生产环境调整频控或绑定真实 unit ID，不会复制或污染开发环境的完整配置。

## Pack 契约

项目清单中的 `pack.id` 是不透明引用，当前格式为 `<name>/<version>`，例如 `mobile-ad-monetization/v1`。项目层只保存和展示该值；只有 Pack 注册表在能力发现、表单构建或后续草稿处理时解析它，因此新增 Pack 不要求修改项目、源适配器或发布适配器。

每个 Pack 版本声明独立的 schema version、实体 ID 规则、字段类型与默认值、敏感级别、删除策略、允许环境覆盖的字段和表单 UI metadata。schema migration 是显式声明的入口；不执行项目文件、网络下载内容或用户提供的脚本。编译、校验和语义差异计算由 Pack 边界定义，具体领域行为由对应 Pack Spec 实现。
