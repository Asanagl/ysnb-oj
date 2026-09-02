# YSNB OJ

> **You Submit, Never Be rejected.**

**YSNB OJ** 是一套**完全自研**的在线评测系统——Go 1.27 后端 + 自研 Linux 判题沙箱
+ Vue 3 前端 + PostgreSQL / Redis。从校内 ACM 集训队的日常训练与比赛中长出来，
开箱即用：ICPC / IOI 赛制、组队赛、封榜滚榜、SPJ / 交互题、多平台刷题数据聚合、
公开 API 与 cph 桥接，一个不缺。

<!-- 徽章占位：开源发布时把链接换成你的仓库地址，CI 徽章等 workflow 就绪后再启用 -->
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Vue](https://img.shields.io/badge/Vue-3-4FC08D?logo=vuedotjs&logoColor=white)](https://vuejs.org)
[![CI](https://img.shields.io/badge/CI-pending-lightgrey)](#)

---

## 特性

### 判题核心（自研沙箱）

- **自研沙箱 `pkg/sandbox`**：每次运行新开一组 namespaces（PID/Mount/Net/IPC/UTS），
  根文件系统只读重挂（NOSUID/NODEV），唯一可写点为任务工作区；root 完成挂载后
  降权 uid 65534 运行选手程序
- **资源防线**：cgroup v2 `memory.max` / `cpu.max` / `pids.max` + RLIMIT
  （输出封顶/栈/地址空间）+ wall-clock 看门狗（杀 PID1 即灭全树）
- **seccomp cBPF 白名单**：默认 SIGKILL，禁 socket 系 / ptrace / mount / unshare /
  setuid 系；白名单采用分块线性链，任意长度安全，并配 cBPF 解释器穷举测试防回归
- **判题管线 `pkg/judge`**：编译缓存、测试点级并行判题（worker 池）、
  零分配 token 级流式比对、全判定状态机（AC/WA/TLE/MLE/RE/CE/SE）、重判
- **比赛首败即停**：比赛内提交一旦某测试点不通过即跳过余下测试点（补题/训练跑满全量），
  错解判题成本从 O(全部 case) 降到 O(首个失败 case)
- **语言配置驱动**：C++17 / Python 3 / Java 17 内置档位（时限倍率 + 内存放宽），
  新语言只需给 `pkg/judge/languages.yaml` 加一段配置再装工具链

### 赛制

- **ACM 赛制**：实时榜单、20 分钟罚时、封榜冻结、赛后滚榜揭晓（逐步公开被冻结名次）
- **IOI 赛制**：测试点分值表 + 总分校验，部分分解提交（`score=30 + verdict=WA`），
  榜单按总分展示
- **组队赛**：ICPC 三人一队，以队报名（仅队长可报）、任一队员提交计入队伍成绩，
  榜单一行一队（取最早 AC、合并罚时）
- **裁判工具**：打星（成绩不计排名）、作弊标记（保留行但剔除计分）、
  手动封榜/解封、调整比赛时间
- **补题**：比赛结束转入补题模式，提交不再影响榜单，题解区自动开放

### 题目

- **题面**：Markdown + KaTeX 公式渲染，所见即所得编辑（TipTap），样例/标签/时限/内存
- **SPJ / 交互题**：checker 调用约定 testlib 风格（exit 0=AC / 1=WA），
  交互题用户程序与 interactor 各自沙箱、经双向管道通信
- **测试数据**：zip 上传（路径白名单 + 统一重编号，无路径穿越）、逐点 sha256 校验、
  测试点分值（IOI）
- **题目包导入导出**：自有格式 + **DOMjudge** + **Hydro** 包全支持，
  导出包含 checker / interactor
- **外部题面爬取**：`source + external_id` 一键导入（如 Codeforces 1900A），
  自动标注来源、默认隐藏待补测试数据
- **用户出题 + 审核流**：用户建题自动隐藏进入待审队列，审核通过后入库；
  题解区按「提交过才解锁」门禁，比赛进行中全锁

### 训练

- **题单**：公开浏览，setter 及以上创建/编辑，进度（todo/tried/ac）从提交实时推导
- **团队/小组**：邀请码加入、队长管理、公告、题单共享、队内排行
- **个人主页**：365 天做题热力图、30 天趋势、状态分布、按标签统计
- **多平台刷题聚合**：绑定 Codeforces / 洛谷 / AtCoder / 牛客 账号自动同步，
  本站与外部平台活动合并成一张热力图 + 平台报表

### 平台

- **多判题机**：gRPC 双向流、判题机主动外连（无需开入站防火墙）、租约调度、
  断线自动重排、心跳监控
- **实时推送**：WebSocket 主题订阅（`submission:<id>` / `contest:<id>` /
  `admin:daemons`），判题结果与榜单增量即时刷新，主题级角色授权
- **公开 API + cph 桥接**：`/api/v1/public/*` 匿名可读 + API Key，
  `/problems/:id/cph` 直接输出 cph（VSCode Competitive Programming Helper）
  可导入的题目 JSON，配合 Competitive Companion 在 IDE 里一键建题
- **插件系统**：编译期注册表——webhook 判题回调（QQ 机器人挂一个 URL 即可）+
  Codeforces/洛谷/AtCoder/牛客 适配器 + 题面爬虫，webhook 目标带 SSRF 校验
- **维护 CLI（oj-cli）**：容器内/裸机直连数据库的应急自救工具——找回超管、
  重置密码、改角色、生成邀请码、健康诊断，`--token` 二次确认、无网络入口
- **一键部署 + 备份演练**：`deploy/one-click.sh` 一条命令起全套；
  每日全量备份（日备 14 天 + 月备 6 个月）+ 每月自动恢复演练

### 实测数字

> 来自 `docs/e2e-report.md` 的云端实测，非实验室理想值（2C4G 单机、API+判题同机）。

| 项 | 结果 |
|---|---|
| 提交洪峰 | 8 并发用户 × 8 题 = 64 提交，40s 注入完毕，64/64 AC |
| 洪峰期间延迟 | API 1.9–3.9 ms；榜单计算 2.1–6.3 ms |
| 判题稳定性 | 50-case 题 12 连跑 ×2，12/12 AC，零幻影 RE/SE |
| 端到端回归 | 9 套 E2E 脚本 127 项断言全绿 |
| 代码规模 | 后端 13.2k 行 Go，前端 6.1k 行 Vue/TS |

## 快速开始

### 一键部署（Docker，推荐）

前置：一台 Linux 服务器（Ubuntu 22.04+ / Debian 12+），装好 Docker ≥ 24，
cgroup v2 已启用（`stat -fc %T /sys/fs/cgroup` 输出 `cgroup2fs`）。

```bash
# 1) 开发机构建前端，并同步整个仓库（含 frontend/dist/）到服务器
cd frontend && npm ci && npm run build

# 2) 服务器上，root 执行一条命令
bash deploy/one-click.sh
```

脚本自动完成：前置检查 → 现场用 openssl 随机生成 `.env`（全部密钥 + admin 初始
密码，已有 `.env` 不覆盖）→ `docker compose up -d --build` → 等待 API 就绪 →
判题机沙箱自检 `--selftest` → 安装备份/演练 cron → 前端冒烟。结束时打印访问
地址与管理员密码（**立即记下**）。脚本幂等，重复执行安全。

systemd 裸机部署路线（适合校内已有服务器、不想用 Docker）：
见 **[docs/deploy.md](docs/deploy.md)** 方式 B（systemd 单元 + 环境变量 + nginx 配置齐备）。

### 开发（后端仅 Linux；Windows/macOS 只跑前端工具链）

**后端不在开发机本地运行**——判题沙箱依赖 cgroup v2 + namespaces，是 Linux-only 的，
Windows/macOS 上既不支持运行，也不建议跑一个行为不一致的残缺后端。所有后端交互都
发生在 Linux 部署上（上面任一方式部署出的实例即可）：

```bash
# 前端（本地只跑 vite 工具链，/api 代理到你的 Linux 部署）
cd frontend
npm install
OJ_DEV_API_TARGET=http://<你的部署地址> npm run dev   # http://localhost:5173

# 后端与判题机：在 Linux 上构建、测试、运行（非 Linux 开发机先交叉编译）
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o oj-judge ./cmd/judge
# 拷到 Linux 服务器后：
sudo ./oj-judge --selftest     # 验证 cgroup v2 / namespace / seccomp 环境
sudo ./oj-judge                # 连接 API 开始接单
```

跑测试（在 Linux 上，对着真实部署）：

```bash
cd backend && go test ./...    # 沙箱外核心逻辑：比对、聚合判定、榜单、JWT、队列、CSV
bash scripts/e2e-test.sh       # 一键 API E2E（自启自清，退出码非 0 即失败）
```

## 架构

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

数据流：提交 → 校验（可见性/语言/比赛窗口/限流）→ 入队 → 判题机经 gRPC 拉取
任务（代码 + 限制 + 测试点 sha256 清单）→ 按 sha256 增量拉数据并缓存 → 编译
（带缓存）→ 逐测试点沙箱运行 → 比对/特判/交互 → 回传落库 + WebSocket 广播。
判题机断线时其在租约内的任务立即重回队列，后台扫描器兜底处理孤儿任务。
更多细节见 [docs/architecture.md](docs/architecture.md)。

## 文档地图

| 文档 | 内容 |
|---|---|
| [docs/architecture.md](docs/architecture.md) | 架构总览、关键决策、判题数据流、沙箱安全模型 |
| [docs/tech-stack.md](docs/tech-stack.md) | 技术选型与版本、OJ_* 环境变量速查、限流阈值 |
| [docs/deploy.md](docs/deploy.md) | 部署（一键 / Docker Compose / systemd）、备份与恢复演练、故障排查 |
| [docs/judge-sandbox.md](docs/judge-sandbox.md) | 判题机与沙箱深水区：判题管线、seccomp 白名单、已踩过的坑 |
| [docs/interactive.md](docs/interactive.md) | SPJ / 交互题 checker / interactor 约定 |
| [docs/api.md](docs/api.md) | 公开 API（/public/*）、cph 端点、插件与爬取接口 |
| [docs/user-guide.md](docs/user-guide.md) | 选手手册：注册刷题、结果含义、题单、比赛、小组 |
| [docs/admin-guide.md](docs/admin-guide.md) | 管理·出题人手册：角色矩阵、出题审核、比赛编排、判题机监控 |
| [docs/maintenance.md](docs/maintenance.md) | 运维 runbook：巡检、发布升级、磁盘缓存、事故处置 |
| [docs/security-audit.md](docs/security-audit.md) | 安全审计报告：沙箱逃逸面、每轮加固记录、残余风险 |
| [docs/e2e-report.md](docs/e2e-report.md) | 测试与可用性报告：E2E 矩阵、压测、判题基准、bug 修复史 |
| [docs/cloud-test-report.md](docs/cloud-test-report.md) | 云服务器实测报告：真机判题闭环与云上修复 |
| [docs/handover.md](docs/handover.md) | 接手文档：30 分钟定位任何一块代码 |

仓库结构：`backend/`（cmd/api、cmd/judge、cmd/cli、pkg/sandbox、pkg/judge、internal/*）
· `frontend/`（Vue3 SPA）· `deploy/`（nginx、systemd、one-click.sh、备份脚本）
· `configs/`（配置示例，无真实凭据）· `docs/` · `scripts/`（E2E 与压测脚本）。

## 安全

判题安全是本项目的核心设计约束，沙箱完全自研（明确排除了复用 go-judge /
二开 Hydro / DOMjudge 沙箱的路线）：

- **纵深防线**：namespaces 隔离 + 只读根 + 敏感路径 tmpfs 遮盖 + uid 65534 降权
  + cgroup v2 三重资源限制 + rlimit + wall-clock 看门狗 + seccomp cBPF 白名单
  （默认 SIGKILL），网络命名空间内无网络
- **逃逸测试集**：读诱饵文件、/proc 探测、路径穿越、写宿主、联网、fork 炸弹、
  内存炸弹、seccomp 违规——全部作为判题机上的常规测试资产运行；
  cBPF 程序另有逐条解释器穷举测试（0..459 全部非法系统调用号验证）
- **限流体系**：登录 10 次/分/IP、注册 5 次/分/IP、提交 15 次/分/用户，
  写端点 per-user 滑窗限流，公开 API 独立限流桶；TrustedProxies 固定，
  客户端伪造 XFF 不影响限流口径
- **安全响应头**：nosniff / X-Frame-Options / CSP / Referrer-Policy，后端与 nginx 双层
- **攻击面收敛**：API/gRPC 端口不对网外发布（compose `expose` / systemd 绑 127.0.0.1）、
  防火墙默认拒绝入站（仅 22/80/443）、WS token 不落访问日志、zip 上传白名单重编号
- **公平性红线**：完整判题测试数据（.in/.out 全集）永不通过公开 API 暴露

完整审计过程、沙箱逃逸面专项与接受的残余风险见
**[docs/security-audit.md](docs/security-audit.md)**。

## API

面向机器人、数据看板、CLI/IDE 工具的只读公开 API：
**[docs/api.md](docs/api.md)**。

亮点：`GET /api/v1/public/problems/:id/cph` 直接返回 cph（VSCode Competitive
Programming Helper）可导入的题目 JSON——配合本地 Competitive Companion 回推，
可以做到「在 cph 里粘个题号就建好题」；判题完成还有 webhook 回调
（插件系统），QQ 机器人等第三方挂一个 URL 即可接入。

## 贡献

欢迎 Issue 与 PR，贡献流程与规范见 **[CONTRIBUTING.md](CONTRIBUTING.md)**
（即将补充）。

## License

本项目以 **MIT** 协议开源。`LICENSE` 文件随首个正式发布一起提交；
在此之前，仓库内代码默认按 MIT 授权使用。