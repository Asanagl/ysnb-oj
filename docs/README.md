# 文档地图

> 所有文档的入口。按"你在做什么"索引。

## 我是谁，该看哪份

| 你是 | 看这份 |
|---|---|
| 队员/选手 | [`user-guide.md`](user-guide.md) — 注册、刷题、比赛、小组、FAQ |
| 管理员/出题人 | [`admin-guide.md`](admin-guide.md) — 用户管理、出题、比赛编排、监控 |
| 运维/值班 | [`maintenance.md`](maintenance.md) — 巡检、发布、磁盘、事故处置 |
| 新接手的开发 | 本页 → [`handover.md`](handover.md) → [`architecture.md`](architecture.md) |
| 改判题/沙箱的开发 | [`judge-sandbox.md`](judge-sandbox.md) — 协议、管线、seccomp 深水区 |
| 面试/对外展示 | [`showcase.md`](showcase.md) — 脱敏版个人项目技术总结 |

## 全部文档

| 文档 | 内容 | 更新时机 |
|---|---|---|
| [`handover.md`](handover.md) | 接手文档：项目全景、凭据指路、代码导读、发布流程、已知坑、待办路线 | 每次人员/凭据/架构变动 |
| [`architecture.md`](architecture.md) | 架构总览：技术决策、模块边界、判题数据流、沙箱安全模型（概览级）、赛制 | 架构变动 |
| [`tech-stack.md`](tech-stack.md) | 技术栈清单：全部依赖的实际版本、选型理由、OJ_* 配置速查、限流参数 | 依赖升级 |
| [`api.md`](api.md) | 外部公开 API（/public/*）：鉴权、端点、cph 集成、示例代码 | API 变动 |
| [`judge-sandbox.md`](judge-sandbox.md) | 判题机与沙箱深水区：gRPC 协议、管线细节、languages.yaml 字段、stage2/挂载/cgroup/rlimit/seccomp 实现、故障定位方法、改代码前必读的行为边界 | 判题核心变动 |
| [`maintenance.md`](maintenance.md) | 运维 Runbook：服务器速查、巡检、发布升级实操、磁盘清理、事故处置表、定期任务、配置变更索引 | 运维流程变动 |
| [`deploy.md`](deploy.md) | 冷部署：**一键部署（deploy/one-click.sh，§〇）**、Docker Compose / systemd 两种方式、判题机 selftest、备份与恢复 | 部署方式变动 |
| [`admin-guide.md`](admin-guide.md) | 管理·出题人手册 | 后台功能变动 |
| [`user-guide.md`](user-guide.md) | 选手手册 | 用户可见功能变动 |
| [`interactive.md`](interactive.md) | SPJ checker / 交互 interactor 的调用约定与出题工具链对接（题目包格式细节） | 判题约定变动 |
| [`showcase.md`](showcase.md) | 个人项目技术总结（面试/简历用，**完全脱敏可外发**）：技术栈权衡、沙箱/调度/前后端难点叙事、量化成果 | 简历项目经历变化时 |
| [`e2e-report.md`](e2e-report.md) | 历轮 E2E 与验收记录（压测基线在 §16.1） | 每轮测试后追加 |
| [`security-audit.md`](security-audit.md) | 历轮安全审计与处置记录 | 每轮审计后追加 |
| [`cloud-test-report.md`](cloud-test-report.md) | 早期云端联调报告（历史存档） | 不再更新 |

## 关键速记

- 生产：`<your-server-ip>`，服务 `oj-api`（:8080/:9090）+ `oj-judge` + nginx
  + PostgreSQL + Redis，程序在 `/opt/oj/`，备份 cron 03:00。
- 发布：`remote-test/` 工具链，先停服再推二进制，前端解压用 `fix-web3.sh`。
- 改沙箱/判题：先读 `judge-sandbox.md` §四，改完必须跑 seccomp 解释器测试
  + M4 三套 E2E。
- 报警第一站：`journalctl -u oj-judge`、后台判题机监控页、`maintenance.md`
  第六节事故表。
