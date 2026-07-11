# Spec 017：Git 评审工作流与审阅产物

> 状态：已实现  
> 依赖：Spec 008、016

## 目标

在 Git Source 模式下，把配置修改转成可评审的分支、文件 diff 和审阅报告，同时保持所有外部写操作显式可控。

## 范围

- 使用系统 Git 获取 status、diff、branch 和 commit metadata。
- 生成建议分支名、英文 Conventional Commit 信息和 review Markdown。
- 可选创建分支、暂存受管理文件和提交；每一步单独确认。
- 默认不 push、不创建 PR；调用外部平台前必须由用户显式执行对应动作。
- 审阅报告包含语义 diff、文件 diff 摘要、校验结果、plan digest 和风险。
- 检测并保护用户既有未提交修改；不使用 reset/checkout 覆盖文件。

## API

- `GET /api/v1/git/status`
- `POST /api/v1/git:prepare`
- `POST /api/v1/git:create-branch`
- `POST /api/v1/git:commit`

Git 写操作要求幂等键和明确文件清单。

## CLI

- `conflow git status/prepare/create-branch/commit`

## 非范围

- 内置 GitHub/GitLab 审批系统、自动 merge 和自动 push。

## 验收

- 有无关用户修改时，只暂存 Conflow 声明的受管理文件。
- 分支或提交失败后工作树可继续人工处理，不留下隐藏状态。
- review Markdown 能独立说明改动、影响、风险和验证结果。
- 所有 Git 命令通过可测试执行器调用，不拼接未经转义的 shell 字符串。
