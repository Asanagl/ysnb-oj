<div align="center">

# YSNB OJ

**You Submit, Never Be rejected.**

一套“完全自研”的在线评测系统：Go 1.27 后端 + 自研 Linux 判题沙箱 + Vue 3 前端 + PostgreSQL / Redis。从校内 ACM 集训队的日常训练与比赛中长出来，开箱即用。

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/Asanagl/ysnb-oj)](https://github.com/Asanagl/ysnb-oj/releases)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Vue](https://img.shields.io/badge/Vue-3-4FC08D?logo=vuedotjs&logoColor=white)](https://vuejs.org)
[![Docs](https://img.shields.io/badge/Docs-VitePress-646CFF?logo=vitepress&logoColor=white)](https://asanagl.github.io/ysnb-oj/)

**[简体中文](README.md)** | [English](README.en.md)

📖 [在线文档站](https://asanagl.github.io/ysnb-oj/) · 🐛 [Issues](https://github.com/Asanagl/ysnb-oj/issues)

<img src="docs/screenshots/home-light.png" alt="首页（亮色主题）" width="49%"/><img src="docs/screenshots/contest-ioi-dark.png" alt="IOI 榜单（暗色主题）" width="49%"/>

</div>

---

## 为什么是 YSNB OJ

市面上的开源 OJ 不少，但判题沙箱大多是复用 go-judge 或二开 Hydro / DOMjudge。
YSNB OJ 走了一条更难的路线：**判题沙箱完全自研**（cgroup v2 + namespaces +
seccomp cBPF 白名单 + stage2 重执行），并把校内 ACM 集训队日常训练与比赛真正
需要的能力做齐——ICPC / IOI 赛制、组队赛、封榜滚榜、SPJ / 交互题、多平台刷题
数据聚合、公开 API 与 cph 桥接，一个不缺。

## 功能一览

| | 特性 |
|---|---|
| 🧑‍⚖️ **判题核心** | 自研沙箱（只读根 + uid 65534 降权 + seccomp 白名单 + 看门狗）、编译缓存、测试点并行判题、流式比对；语言配置驱动（C++17 / Python 3 / Java 17 内置，加语言只改 yaml） |
| 🏆 **赛制** | ACM（实时榜单 / 罚时 / 封榜 / 滚榜揭晓）、IOI（测试点分值表 + 部分分）、组队赛（ICPC 三人一队）、补题模式、首败即停 |
| 📝 **题目** | Markdown + KaTeX 题面、SPJ / 交互题（testlib 风格约定）、题目包导入导出（自有 / DOMjudge / Hydro）、外站题面爬取、用户出题 + 审核流、题解区（提交解锁） |
| 📈 **训练** | 题单（进度实时推导）、小组（邀请码 / 公告 / 题单共享 / 队内排行）、个人主页（365 天热力图）、多平台刷题聚合（Codeforces / 洛谷 / AtCoder / 牛客） |
| ⚡ **实时** | WebSocket 主题订阅：判题状态原地推进、测试点圆点逐个点亮、榜单增量刷新；提交详情页自动跟进 |
| 🔌 **平台** | 多判题机（gRPC 双向流 + 租约调度）、公开 API + cph 桥接（IDE 一键建题）、webhook 回调、维护 CLI（oj-cli）、结构化日志 + 后台日志查看器 |
| 🔒 **安全** | 沙箱逃逸测试集常态化运行、登录 / 注册 / 提交限流、安全响应头双层、API 端口不出内网；**完整测试数据永不外泄（公平性红线）** |

<details>
<summary><b>实测数字</b>（2C4G 单机云端实测，非实验室理想值）</summary>

| 项 | 结果 |
|---|---|
| 提交洪峰 | 8 并发用户 × 8 题 = 64 提交，40s 注入完毕，64/64 AC |
| 洪峰期间延迟 | API 1.9–3.9 ms；榜单计算 2.1–6.3 ms |
| 判题稳定性 | 50-case 题 12 连跑 ×2，12/12 AC，零幻影 RE/SE |
| 端到端回归 | 六套 E2E 脚本 82 项断言全绿 |
| 代码规模 | 后端 13.2k 行 Go，前端 6.2k 行 Vue/TS |

</details>

## 🚀 快速开始

前置：一台 Linux 服务器（Ubuntu 22.04+ / Debian 12+），Docker ≥ 24，cgroup v2
已启用（`stat -fc %T /sys/fs/cgroup` 输出 `cgroup2fs`）。

```bash
# 1) 开发机构建前端，并同步整个仓库（含 frontend/dist/）到服务器
cd frontend && npm ci && npm run build

# 2) 服务器上，root 执行一条命令
bash deploy/one-click.sh
```

脚本自动完成：前置检查 → 现场用 openssl 随机生成 `.env`（全部密钥 + admin 初始
密码，已有 `.env` 不覆盖）→ `docker compose up -d --build`（postgres/redis/api/web
四件容器化）→ 等待 API 就绪 → 判题机自检（已装则跑 `--selftest`，未装打印三步
安装指引——判题机跑宿主机，不进容器）→ 安装备份/演练 cron → 前端冒烟。脚本
幂等，重复执行安全。

不想用 Docker？[systemd 裸机部署](docs/operations/deploy.md)（方式 B）同样完整。

### 本地开发

后端是 Linux-only（沙箱依赖 cgroup v2 + namespaces），**不在开发机本地运行**；
Windows/macOS 只跑前端工具链，`/api` 代理到任意 Linux 部署：

```bash
cd frontend
npm install
OJ_DEV_API_TARGET=http://<你的部署地址> npm run dev   # http://localhost:5173
```

构建与测试在 Linux 上进行（非 Linux 开发机先交叉编译）：

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o oj-judge ./cmd/judge
sudo ./oj-judge --selftest     # 验证 cgroup v2 / namespace / seccomp 环境
cd backend && go test ./...    # 沙箱外核心逻辑单测
bash scripts/e2e-test.sh       # 一键 API E2E（自启自清）
```

## 🏗️ 架构

```
浏览器 ──Vue3 SPA──► Nginx ──► Go API Server ──► PostgreSQL（生产）/ SQLite（自测）
                                  │         └──► Redis（判题队列 / 限流 / 会话）
                                  │  WebSocket 推送（submission / contest / admin:daemons）
                                  │  插件注册表（编译期）：webhook + 外部平台适配器
                                  │ gRPC JudgeRelay（bidi stream，判题机主动外连）
                                  ▼
                           judge-daemon × N（完全自研，含沙箱）
                           ├─ 语言注册表（languages.yaml，配置驱动）
                           ├─ 编译沙箱档位（g++/javac）+ 编译缓存（sha256）
                           ├─ 运行沙箱（cgroup v2 + namespaces + seccomp 白名单）
                           ├─ checker / interactor（SPJ / 交互题）
                           └─ 测试数据 blob 缓存（sha256）
```

## 📚 文档

在线文档站：<https://asanagl.github.io/ysnb-oj/>（**中文** ·
[English](https://asanagl.github.io/ysnb-oj/en/)）

| 文档 | 内容 |
|---|---|
| [选手手册](docs/guide/user-guide.md) | 注册刷题、实时判定、结果含义、题单、比赛、小组 |
| [管理·出题人手册](docs/guide/admin-guide.md) | 角色矩阵、出题审核、比赛编排、判题机监控、日志查看器 |
| [SPJ / 交互题约定](docs/guide/interactive.md) | checker / interactor 调用约定 |
| [部署](docs/operations/deploy.md) | 一键 / Docker Compose / systemd、备份恢复、故障排查 |
| [运维 Runbook](docs/operations/maintenance.md) | 巡检、发布升级、磁盘缓存、事故处置 |
| [架构总览](docs/development/architecture.md) | 关键决策、判题数据流、沙箱安全模型 |
| [判题机与沙箱](docs/development/judge-sandbox.md) | 判题管线、seccomp 白名单、深水区 |
| [公开 API](docs/reference/api.md) | /public/*、cph 端点、API Key |
| [接手导读](docs/development/handover.md) | 30 分钟定位任何一块代码 |

完整地图见 [docs/README.md](docs/README.md)。

## 🔒 安全

判题安全是本项目的核心设计约束。沙箱完全自研（明确排除复用 go-judge /
二开 Hydro / DOMjudge 沙箱的路线），纵深防线：namespaces 隔离、只读根、
敏感路径 tmpfs 遮盖、uid 65534 降权、cgroup v2 三重资源限制、rlimit、
wall-clock 看门狗、seccomp cBPF 白名单（默认 SIGKILL），网络命名空间内无网络。

沙箱逃逸测试集（读诱饵文件、/proc 探测、路径穿越、写宿主、联网、fork
炸弹等）作为判题机常规测试资产运行；cBPF 程序另有逐条解释器穷举测试。
**完整判题测试数据永不通过公开 API 暴露**——公平性红线。

审计过程与残余风险见 [docs/archive/security-audit.md](docs/archive/security-audit.md)。

## 🤝 贡献

欢迎 Issue 与 PR，流程与规范见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 📄 License

[MIT](LICENSE) © Asanagl
