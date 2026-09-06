---
layout: home

hero:
  name: YSNB OJ
  text: You Submit, Never Be rejected.
  tagline: 完全自研的在线评测系统——从部署、运维到开发的全部文档
  actions:
    - theme: brand
      text: 我要部署一套
      link: /operations/deploy
    - theme: alt
      text: 文档导航
      link: '#文档导航'

features:
  - title: 选手 / 队员
    details: 注册刷题、参加比赛（ACM/IOI）、小组与题单、外部平台刷题聚合
    link: /guide/user-guide
  - title: 管理员 / 出题人
    details: 出题与审核、办赛（含封榜滚榜）、用户管理、日志查看器、API 密钥
    link: /guide/admin-guide
  - title: 部署 / 运维
    details: 冷部署、判题机接入、发布更新、备份恢复、SE 排查、巡检
    link: /operations/deploy
  - title: 开发者
    details: 接手导读、架构总览、判题机与沙箱深水区、技术栈与配置
    link: /development/handover
---

## 文档导航

> 按「我要做什么」找入口；每份文档开头都注明了读者对象。

### 使用指南

| 文档 | 内容 |
|---|---|
| [选手手册](./guide/user-guide) | 注册、刷题与实时判定、比赛、小组、个人主页（外站刷题数据与外站题目导入）、FAQ |
| [管理与出题手册](./guide/admin-guide) | 角色权限、出题（标准/SPJ/交互/IOI 分值表）、办赛、审核、判题机监控、日志查看器、API 密钥、插件 |
| [SPJ 与交互题约定](./guide/interactive) | checker / interactor 调用约定、题目包格式对接 |

### 部署与运维

| 文档 | 内容 |
|---|---|
| [部署](./operations/deploy) | 一键脚本与 systemd 两条冷部署路径、判题机接入、日志、备份演练、环境变量全表 |
| [运维 Runbook](./operations/maintenance) | 发布更新、巡检、SE 排查、磁盘清理、事故处置、SSH 加固基线、配置变更索引 |

### 开发

| 文档 | 内容 |
|---|---|
| [接手导读](./development/handover) | 30 分钟定位任何代码：能力全景、代码导读、开发门禁、发布流程、已知坑 |
| [架构总览](./development/architecture) | 技术决策、模块边界、判题数据流、沙箱安全模型、赛制 |
| [技术栈与配置](./development/tech-stack) | 依赖版本、包结构、OJ_* 配置速查、限流参数 |
| [判题机与沙箱](./development/judge-sandbox) | gRPC 协议、判题管线、languages.yaml、seccomp/cgroup 深水区 |
| [项目展示](./development/showcase) | 脱敏版技术总结（面试/对外） |

### API 参考

| 文档 | 内容 |
|---|---|
| [公开 API](./reference/api) | `/public/*` 端点、API Key、cph 集成、M5 集成端点 |

### 历史存档（只读）

[安全审计](./archive/security-audit) · [上线 Readiness 快照](./archive/launch-readiness) · [E2E 测试编年史](./archive/e2e-report) · [早期云端联调报告](./archive/cloud-test-report)
