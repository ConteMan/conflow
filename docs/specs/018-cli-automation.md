# Spec 018：CLI 能力对齐与自动化契约

> 状态：待实现  
> 依赖：Spec 007–011、016、017

## 目标

让无 GUI 场景通过稳定 CLI 完成校验、计划、拉取、发布、回滚和 Git 准备，并与 HTTP API 共享领域语义。

## 范围

- 命令族：`project`、`environment`、`source`、`validate`、`plan`、`pull`、`publish`、`release`、`rollback`、`git`。
- `--json` 输出使用稳定 schema 和错误码，不输出面向人的前缀文本。
- 明确退出码：成功、校验失败、冲突、Provider 失败、使用错误。
- 非交互模式、幂等键、确认参数和敏感输入来源。
- CLI / API golden fixtures 确保同一用例结果一致。
- CI 示例：只校验、生成 Plan artifact、人工门禁后发布。

## 非范围

- CLI 调用本地 HTTP 服务。
- 在命令行参数中接受明文 token 或 unit secret。

## 验收

- 同一无效配置的 CLI JSON 与 API 诊断 code/path/severity 一致。
- 非 TTY 发布缺少确认或幂等键时明确失败。
- 所有命令提供 `--help`、示例和稳定退出码测试。
- 文档包含 GitHub Actions 的安全使用示例，但不提供默认自动生产发布。
