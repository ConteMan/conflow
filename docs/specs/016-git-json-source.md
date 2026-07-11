# Spec 016：Git JSON 源适配器与迁移

> 状态：已实现  
> 依赖：Spec 004、005、008

## 目标

支持已有项目将仓内 JSON/YAML 继续作为事实源，并以显式 mapping profile 导入、编辑和写回，而不把具体文件格式变成全局业务规则。

## 范围

- Git workspace 探测、干净/脏状态和当前 branch 信息。
- mapping profile：参数、频控、广告位、环境 binding 的路径和转换规则。
- 导入既有源为 Pack 业务实体，输出 round-trip 诊断。
- 写回前生成文件 diff、source digest 和受影响文件清单。
- 原子写回且保留不受管理内容；无法 round-trip 时阻止写回。
- 内置 PDF Launcher 等价 profile 作为 fixture，不把项目名写入通用领域代码。

## API

- `POST /api/v1/source:inspect`
- `POST /api/v1/source:import`
- `POST /api/v1/source:preview-save`
- `POST /api/v1/drafts/{environment_id}:save`

## CLI

- `conflow source inspect/import/preview-save`

## 非范围

- 自动提交、推送或创建 PR；留给 Spec 017。
- Markdown 文档同步和任意模板语言。

## 验收

- 当前 PDF Launcher fixture 可导入后生成等价业务实体和 Firebase desired config。
- 未受管理 JSON 字段保持不变；条件值和无法映射字段明确阻断。
- 脏工作树不被静默覆盖；写回前展示逐文件 diff。
- round-trip golden tests 和 Windows/macOS/Linux 路径测试通过。
