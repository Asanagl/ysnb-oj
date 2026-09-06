# 架构总览

> 面向开发者的概览级文档：一页看懂 YSNB OJ 的整体形状、关键决策与数据流。
> 判题机/沙箱深水区（实现细节、参数、故障定位）见 `judge-sandbox.md`；
> 组件版本与 `OJ_*` 配置项速查见 `tech-stack.md`；部署运维见 `../operations/deploy.md`。

## 总览

```
浏览器 ──Vue3 SPA（Vite 产物 dist）──► nginx 边缘（静态托管 + 反代 /api 与 WS Upgrade + 安全头）
                                              │
                                ┌─────────────┴───────────────┐
                                ▼                             ▼
                     /api/v1/*（JWT + RBAC）        /api/v1/public/* 公开 API 面
                     用户面 + AdminLayout 独立后台   （匿名只读；ojk_ Key 独立配额）
                                │
                          Go API（cmd/api）
                          ├─ handler    REST 层：题目/提交/比赛/榜单/团队/题单/jury/admin
                          ├─ judgehub   判题调度：队列消费、租约、断线重排、结果落库
                          ├─ plugin     编译期注册表：题库爬取 / 刷题同步 / webhook 事件钩子
                          └─ logx       slog 单行 JSON → stderr → journald
                                        （后台「日志查看器」经 journalctl 读回）
                                │
              ┌─────────────────┼──────────────────────────┐
              ▼                 ▼                          ▼
        PostgreSQL        Redis（仅判题队列，        gRPC JudgeRelay 双向流
        状态唯一真源       List RPush/BLPop）        proto/oj.proto：Connect(stream⇄stream)
                                                     判题机主动外连：注册/心跳/任务/结果
                                                                ▼
                                                judge-daemon（cmd/judge，完全自研，含沙箱）
                                                ├─ 语言注册表（pkg/judge/languages.yaml，配置驱动）
                                                ├─ 编译沙箱档位（g++/javac）
                                                ├─ 运行沙箱档位（cgroup v2 + ns + seccomp 白名单）
                                                ├─ checker / interactor 编译缓存（sha256）
                                                └─ 测试数据 blob 缓存（sha256，回源 /internal/testdata）
```

## 关键决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 开发语言 | Go（CGO_ENABLED=0） | 单静态二进制，Windows 开发机可交叉编译出 Linux 判题机程序 |
| 沙箱 | 完全自研 `pkg/sandbox` | 用户明确要求完全自研；安全模型见下文，深水区见 `judge-sandbox.md` |
| API↔判题机 | gRPC `JudgeRelay.Connect` 双向流，判题机主动外连 | 判题机零入站防火墙开口；注册/心跳/任务/结果复用一条流，天然支持多判题机水平扩展 |
| 队列 | Redis List（生产）/ 内存（dev） | `internal/queue` 抽象（RPush/BLPop），开发零依赖；Redis 只当队列不当存储 |
| 语言支持 | 数据驱动 `pkg/judge/languages.yaml` | 新增语言只加配置段 |
| 数据库 | PostgreSQL（生产）/ SQLite（dev） | GORM 双方言共用 schema，JSON 类字段存 text；版本口径见「数据存储」 |
| 日志 | slog 单行 JSON → stderr → journald，不写日志文件 | 与系统服务共用一套 `journalctl` 运维面；免轮转免清理；journald `SystemMaxUse` 兜底磁盘上限；级别映射 PRIORITY，`journalctl -p err` 直接过滤 |
| 插件 | 编译期注册表（`init()` + `Register`），不做动态加载 | 部署形态是静态二进制 + 配置，Go `.so` 动态加载跨工具链脆弱；无动态链接面，每个适配器就是一个可审计的新文件 |
| 前后端 | Vue3 SPA + `dist` 由 nginx 静态托管，API 独立 | 前端是纯静态产物，可独立构建替换；nginx 边缘统一入口、安全头与 WS Upgrade，API/gRPC 端口不出内网 |

## 模块边界

```
backend/
├── cmd/api              HTTP+gRPC 主程序
├── cmd/judge            判题机主程序（含 --selftest）
├── cmd/cli              维护 CLI
├── internal/
│   ├── handler/         REST 层（auth/problem/submission/contest/teams/lists/jury/admin）
│   │   ├── public_api.go + public_cph.go   公开 API 面（/public/*）与 /admin/api-keys 密钥管理
│   │   ├── admin_logs.go                   日志查看器（固定 argv 调 journalctl，无注入面）
│   │   ├── plugins_admin.go                插件清单、IOI 分值表（case-scores）、webhook 钩子管理
│   │   └── external_sync.go                刷题统计同步 worker（小时级 ticker，串行抓取）
│   ├── judgehub/        判题调度：队列消费、租约、断线重排、结果落库
│   ├── daemon/          判题机客户端（重连、心跳、并发闸）
│   ├── plugin/          扩展注册表（ProblemSource / SubmitLogFetcher / EventHook 三类扩展点）
│   ├── external/        外部平台提交记录模型（刷题统计报表数据源）
│   ├── public/          API Key 凭据模型（库里只存 sha256）
│   ├── logx/            slog JSON → stderr → journald；OJ_LOG_LEVEL 免重编译调级别
│   ├── queue/           队列抽象（Redis List / 内存双实现）
│   ├── wsq/             WebSocket 主题广播（submission:N 属主可见 / contest:N / admin:daemons）
│   ├── auth/            JWT + bcrypt + RBAC 中间件
│   ├── model/ store/    GORM 模型与迁移
│   └── config/ sysload/ 配置加载 / 宿主机负载采样（后台判题机监控）
└── pkg/
    ├── sandbox/         沙箱库（仅 Linux 有实现，其他平台为 stub）
    └── judge/           判题编排（与传输层解耦，纯逻辑可测）

frontend/src/
├── layouts/             MainLayout（用户面）与 AdminLayout（独立后台外壳，按角色分级显菜单）
├── views/               页面（Admin*View 系列挂在 AdminLayout 下）
├── composables/         useChart / useResponsive / useTheme / useLiveSubmission（提交状态实时跟进）
├── lib/                 toast / confirm / utils（UI 原语）
├── api/client.ts        axios 集中封装
└── stores/              Pinia（auth）
```

扩展方式：新功能按既有模式各加 `internal/handler` 分组 + `internal/model` 实体；新外部平台
按 `internal/plugin` 的扩展点各加一个带 `init() Register` 的文件。团队（teams）与刷题统计
（external sync）已按此模式落地；仍预留的后续功能只有讨论区与训练计划。

## 判题数据流

1. `POST /submissions` → 校验（登录/题目可见性/语言/比赛窗口/报名/提交限流）→ 代码落盘
   → 状态 `PENDING` 入队（Redis List）。
2. 判题机经 gRPC `JudgeRelay.Connect` 注册（共享密钥）→ judgehub 按容量拉队列 → 组装任务
   （代码 + 题目限制 + judge_mode 源码 + 测试点 sha256 清单；IOI 题每点附带 `Score`）→
   提交置 `JUDGING` + 15 分钟租约 → WS 广播 `submission:<id>`。
3. 判题机按 sha256 经 `/internal/testdata` 回源下载缺失测试数据并缓存 → 编译（带缓存，
   checker/interactor 同理）→ 逐测试点沙箱运行 → 比对/特判/交互 → 回传 `TaskResult`
   （含逐点 verdict 与得分）。每个测试点亮出结果后，判题机还会经同一条 gRPC 流
   即时上报 `CaseProgress`（best-effort：发送失败只丢进度不影响判题与最终结果），
   judgehub 校验该提交确在此判题机名下后转 WS 推送。
4. `finalize` 落库（status/time_ms/memory_kb/score/cases，租约清除）→ 打 INFO 日志
   `submission judged`（每条提交一行，是日志查看器按提交 id / verdict 检索的主锚点）→
   WS 广播 `submission:<id>`；比赛提交额外广播 `contest:<id>`（standings-dirty，榜单增量
   刷新）；判题机上下线广播 `admin:daemons`（仅 admin 可订阅）；
   `plugin.EmitJudgeEvent` 在独立 goroutine 异步扇出 webhook 钩子，绝不阻塞判题。
   `submission:<id>` 的消费者 = 题目页 / 比赛题页 / 提交详情页（前端
   `useLiveSubmission` 订阅，驱动实时判定卡片与测试点圆点条，断线自动降级轮询）；
   该主题**属主可见**（或 admin）——判题实况不对其他选手暴露，属主判定在
   订阅时查库（`handler/router.go` 的 `wsTopicAuthorizer`）。
5. 容错：判题机断线 → 其租约内任务立即重回 `PENDING`；API 启动时 `RequeuePending` 全量
   重建队列，后台 `StartRequeueScanner` 兜底处理 lease 过期的孤儿任务——数据库是唯一
   真源，队列随时可重建。

## 数据存储

- **PostgreSQL**：两条部署路线口径一致——compose 模板用 `postgres:16-alpine`
  （`docker-compose.yml`，postgres/redis/api/web 四件容器化，判题机跑宿主机）；
  裸机 Debian 11 用系统源 PostgreSQL 13（tech-stack.md 记 13.23）。
  GORM 双方言（postgres/sqlite）共用 schema，dev 默认 SQLite 零依赖。
- **Redis**：仅作判题队列（List，RPush/BLPop），不当缓存也不当数据库；队列是
  best-effort，丢了我从 PostgreSQL 重建（见判题数据流第 5 步）。
- **文件**：测试数据 blob、提交代码、上传物落在 `OJ_DATA_DIR`，按 sha256 寻址。

## 沙箱安全模型（pkg/sandbox）

每次运行在**新的一组 namespaces**（PID/Mount/Net/IPC/UTS）中重执行自身：

- **文件系统**：私有可能挂载传播后把 `/` bind 重挂为只读（NOSUID/NODEV）；
  `/proc`、`/sys` 用全新实例（只读）覆盖；`/tmp`、`/dev`、`/dev/shm` 为有上限的
  tmpfs；`/home`、`/root`、`/var`、`data_dir`、`work_root` 上层再盖空 tmpfs 防泄露；
  唯一可写点 = 本任务 workspace（chown 给 uid 65534, mode 0700）。
- **身份**：root 完成挂载后 `setresuid(65534)` 降权运行用户程序/checker/interactor。
- **资源**：cgroup v2 `memory.max`（swap 禁用）、`cpu.max`、`pids.max`；
  RLIMIT: FSIZE（输出封顶）、STACK、AS（按语言可关，JVM 场景靠 cgroup）、
  CPU（秒级兜底）+ wall-clock 看门狗（杀 PID1 = 灭全树）。
- **系统调用**：seccomp cBPF 白名单，默认 SIGKILL。禁止 socket 系、ptrace、
  mount、unshare、setuid 系等；编译档位额外放行 fork/vfork。
- **交互题**：用户程序与 interactor 各自沙箱，经父进程搭桥的双向管道通信；
  任一方退出即终止对方；interactor 约定 exit 0=AC / 1=WA / 其余=SE。

残留风险（已知、可控）：PID1 由用户程序承担，其 fork 出的孤儿进程在退出时随
namespace 一并回收（长跑多 fork 的程序可能有短暂僵尸，受 pids.max 封顶）；
seccomp 白名单按需演进，漏放会导致特定语言特性 RE 而非安全问题（fail-closed）。
实现细节、参数与故障定位见 `judge-sandbox.md`。

## 赛制

- **ACM（mode=acm）**：解题数降序 → 罚时升序；罚时 = AC 时刻 + 每次错误提交 20 分钟
  （CE 不计）。配套能力：封榜（`freeze_time` 自动 / jury 手动）、打星 ★（报名时
  `team_type=starred` 或 jury 标记，行显示但不占名次）、作弊标记（行保留但排除出计分）、
  滚榜揭示（`reveal_count`：从真实榜底部向上逐行揭示，★/作弊行不参与滚榜、保持掩码）。
- **IOI（mode=ioi）**：部分分赛制。题目侧维护逐测试点分值表（`TestCase.Score`，
  `PUT/GET /problems/:id/case-scores`，setter+）；判题按点给分（该点 AC 才得分），
  `submissions.score` 存总分；verdict 仍是 verdict-based——部分分提交的判定可以仍是 WA。
  榜单按总分排名，前端 `ScoreBoard` 按 `mode=ioi` 分叉（表头「满分题/总分」，格子显示
  得分）。封榜与滚榜语义同 ACM。
- **组队赛**：`Contest.TeamMode` 开启后由队长一次报名（带 `TeamID` 与队名，容量默认 3），
  成员快照为每人一行报名记录，榜单按队聚合（`teamStandings`）。
- **补题模式**：比赛结束后的提交标记 `IsPractice=true`（无需报名、题目保持开放），正常
  判题但不计入任何榜单、不改变封榜状态。
- **封榜掩码（公平性红线）**：封榜点之后的提交在榜单上只显示为 pending 计数（ACM 的
  `?n` / IOI 的 `cell.Pending=1`，贡献按 0 计），任何接口都不得提前揭示其 verdict 或
  分值。

## 公开 API 面

`/public/*` 是与 JWT 体系隔离的匿名只读面（`/public/users/:id`、`/public/problems`）；
机读资产端点（`/public/problems/:id/samples`、`/public/problems/:id/cph`）要求
`ojk_` 前缀的 API Key（`X-API-Key` 头，库里只存 sha256，创建时仅展示一次，由
`/admin/api-keys` 管理，吊销不删除以留审计）。限流上匿名调用者共用一个 60 次/分桶、
每个 Key 独占一桶。**判题测试数据（.in/.out）永不暴露——这是公平性红线。**
