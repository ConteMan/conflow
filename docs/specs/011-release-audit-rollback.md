# Spec 011：发布审计、默认值下载与回滚

> 状态：待实现  
> 依赖：Spec 010

## 目标

为每次发布保存可追踪证据，并支持基于 Firebase 版本的受控回滚和 Android 等客户端默认值导出。

## 范围

- Release 记录：项目、环境、Pack、操作者、时间、source digest、plan digest、远端前后版本和 ETag。
- 发布前/后模板摘要与语义 diff；敏感数据脱敏。
- Firebase 版本列表与回滚；回滚本身创建新的 Release 记录。
- 下载 XML / JSON / plist 默认值，并带来源版本 metadata。
- 审计保留策略、导出和损坏检测。

## API

- `GET /api/v1/environments/{environment_id}/releases`
- `GET /api/v1/environments/{environment_id}/releases/{release_id}`
- `POST /api/v1/environments/{environment_id}/releases/{release_id}:rollback`
- `GET /api/v1/environments/{environment_id}/defaults?format=xml|json|plist`

## CLI

- `conflow release list/show`
- `conflow rollback --environment <id> --release <release_id> --confirm`
- `conflow defaults download --environment <id> --format xml`

## 非范围

- 长期云端审计仓库和组织级合规报表。
- 自动把默认值提交到客户端仓库。

## 验收

- 发布、失败发布和回滚的审计语义不同且可检索。
- 回滚仍受 ETag、幂等和 Production 确认保护。
- 默认值文件与指定远端版本一致，并可通过 digest 验证。
- 删除本地敏感凭据不影响历史审计可读性。
