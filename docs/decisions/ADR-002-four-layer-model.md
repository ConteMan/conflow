# ADR-002：配置包、项目、源适配器与发布适配器分层

> 状态：已接受（2026-07-10）

## 决策

Conflow 将业务规则放在 Config Pack；项目和环境保存实例化配置；Source Adapter 负责读写格式；Provider Adapter 负责目标平台交互。

## 理由

不同 App 常有不同广告位、环境和文件格式，但不应迫使系统重写发布审计、ETag 保护和 CLI。反过来，Git JSON 或 Firebase 模板也不能定义业务安全规则。

## 后果

- 新 App 若仍使用已有 Pack，只需创建项目和环境。
- 新存储格式只需新增 Source Adapter。
- 新业务域才需要开发新 Pack。
