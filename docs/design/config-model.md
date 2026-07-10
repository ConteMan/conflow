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

## 优先级

```text
Pack 安全默认值
  < 项目基线配置
  < 环境覆盖
  < 当前草稿
```

Firebase 当前值不参与上述优先级；它用于生成发布计划、检测并发变化与发布后验证。

## 项目清单

每个项目根目录可使用 `.conflow/project.yaml` 作为可提交的项目入口：

```yaml
version: 1
project:
  id: photo-editor
  name: Photo Editor
pack:
  id: mobile-ad-monetization/v1
source:
  type: managed-file
environments:
  - id: development
    provider:
      type: firebase-remote-config
      project_id: photo-editor-dev
  - id: production
    provider:
      type: firebase-remote-config
      project_id: photo-editor-prod
    publish:
      requires_confirmation: true
```

项目可选 `git-json` 源适配器，以读取并写回既有仓库中 `config/remote-config/` 一类结构。适配器映射是项目配置，不承担广告业务规则。

## 环境覆盖

项目基线承载广告位、频控策略与失败兜底；环境覆盖承载开关、unit ID、网络模式及明确允许的阈值差异。Pack 可将字段标记为“不可按环境覆盖”。

这样生产环境调整频控或绑定真实 unit ID，不会复制或污染开发环境的完整配置。
