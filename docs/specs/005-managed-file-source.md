# Spec 005：Managed File 源适配器

> 状态：已实现  
> 依赖：Spec 004

## 目标

为没有既有仓内格式的新项目提供简单、可审阅的 Conflow 原生文件存储，作为首个完整 Source Adapter。

## 范围

- `.conflow/data/base.yaml` 保存项目基线。
- `.conflow/data/environments/<environment_id>.yaml` 保存环境覆盖。
- 原子写入：同目录临时文件、fsync、rename；失败不破坏旧文件。
- canonical 序列化、稳定排序、source digest 和外部修改检测。
- Source Adapter 接口：load、save、status、capabilities。
- 草稿保存与 Source Adapter 提交分离：显式 save 才写回源。

## API

- `GET /api/v1/source`
- `GET /api/v1/source/status`
- `POST /api/v1/drafts/{environment_id}:save`

## CLI

- `conflow source status --workspace <path>`
- `conflow save --workspace <path> --environment <id>`

## 非范围

- Git branch、commit、push 或 PR。
- 凭据存储和 Provider 快照。

## 验收

- 崩溃/写入失败测试证明旧文件保持可读。
- 相同语义重复保存产生字节稳定输出和相同 digest。
- 外部修改后保存返回 revision 冲突，不静默覆盖。
- 文件不包含凭据或发布 token，`make check` 通过。
