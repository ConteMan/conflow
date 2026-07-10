# Spec 019：跨平台发版、签名与更新

> 状态：待实现  
> 依赖：Spec 015、018

## 目标

在核心工作流稳定后，形成可验证的 macOS、Linux、Windows 发布和受控更新体验。

## 范围

- GoReleaser 构建三平台 amd64/arm64 archive、checksums、SBOM 和 GitHub Release。
- 发布前构建 React 并验证嵌入资产无漂移。
- 版本、commit、构建时间注入与 `conflow version --json`。
- 平台签名、公证和供应链证明方案；未具备签名时明确标注限制。
- Homebrew / Scoop 等分发是否进入 v1，由实现前 ADR 根据维护成本决定。
- 自更新仅在校验签名、checksum、渠道和回滚策略明确后实现；默认需要用户确认。

## 非范围

- 静默后台更新、强制更新和遥测。
- 云端许可证或付费分发。

## 验收

- tag workflow 在三个 OS / 两个架构生成可运行 archive 和 checksums。
- 干净机器验证 `version`、`init`、`validate`、`serve` 与嵌入 GUI。
- 更新失败不会破坏现有二进制；checksum 或签名失败时拒绝替换。
- 发布清单、CHANGELOG、双语 README 和维护文档同步更新。
