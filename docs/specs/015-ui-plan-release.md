# Spec 015：校验、Plan、发布与回滚 UI

> 状态：待实现  
> 依赖：Spec 008、009、010、011、012、014

## 目标

完成从配置草稿到安全发布的 PM 主流程，并清晰展示诊断、影响、风险、进度、历史和回滚证据。

## 范围

- 校验中心：按实体、严重级别和字段路径聚合，可跳转修复。
- Plan：业务语义 diff、Firebase 参数 diff、影响对象、风险分级和 artifact 下载。
- Provider 拉取/验证进度；SSE 断开自动回退 Operation 轮询。
- Production 发布确认：环境、版本、风险、ETag 和确认要求同时展示。
- 发布成功/失败、版本历史、默认值下载和回滚确认。
- plan 过期、draft revision 冲突、remote ETag 冲突的恢复路径。

## API

消费 Spec 007–011 的 validation、plan、operation、release 和 defaults 端点。

## 非范围

- 多人审批、定时发布、通知中心和组织权限。

## 验收

- E2E 覆盖正常发布、校验失败、计划过期、ETag 冲突、网络失败和回滚。
- 发布按钮只在服务端判定就绪后可进入确认；前端禁用不替代服务端校验。
- 原始 Firebase diff 可查看，但默认首先展示业务语义 diff。
- 用户关闭页面后重新打开，仍可恢复正在运行的 Operation 与最终结果。
