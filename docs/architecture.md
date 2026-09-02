# 架构文档（V1）

## 总览

```
浏览器 ──Vue3 SPA──► Nginx ──► Go API Server ──► PostgreSQL（生产）/ SQLite（自测）
                                   │        └──► Redis Stream/List（判题队列）
                                   │ gRPC JudgeRelay（bidi stream）
                                   ▼
                            judge-daemon（完全自研，含沙箱）
                            ├─ 语言注册表（languages.yaml，配置驱动）
                            ├─ 编译沙箱档位（g++/javac）
                            ├─ 运行沙箱档位（cgroup v2 + ns + seccomp 白名单）
                            ├─ checker / interactor 编译缓存（sha256）
                            └─ 测试数据 blob 缓存（sha256）
```

## 关键决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 开发语言 | Go（CGO_ENABLED=0） | 团队确认熟悉；单静态二进制可在 Windows 上交叉编译出 Linux 判题机程序 |
| 沙箱 | 完全自研 `pkg/sandbox` | 用户明确要求完全自研；见下文安全模型 |
| API↔判题机 | gRPC bidi，判题机主动外连 | 无需在判题机开防火墙入站；天然支持多判题机水平扩展 |
| 队列 | Redis（生产）/ 内存（开发） | 接口抽象 `internal/queue`，开发零依赖 |
| 语言支持 | 数据驱动 `pkg/judge/languages.yaml` | 新增语言只加配置段 |
| 数据库 | PostgreSQL 16 / SQLite(dev) | JSON 类字段存 text，两个方言共用 schema |

## 模块边界（为后续功能预留）

```
backend/
├── cmd/api            HTTP+gRPC 主程序
├── cmd/judge          判题机主程序（含 --selftest）
├── internal/
│   ├── handler/       REST 层（auth/problem/submission/contest/admin）
│   ├── judgehub/      判题调度：队列消费、租约、断线重排、结果落库
│   ├── daemon/        判题机客户端（重连、心跳、并发闸）
│   ├── queue/         队列抽象（redis/memory 双实现）
│   ├── wsq/           WebSocket 主题广播（submission:N / contest:N / admin:daemons）
│   ├── auth/          JWT + bcrypt + RBAC 中间件
│   ├── model/ store/  GORM 模型与迁移
│   └── config/
└── pkg/
    ├── sandbox/       沙箱库（仅 Linux 有实现，其他平台为 stub）
    └── judge/         判题编排（与传输层解耦，纯逻辑可测）
```

后续功能（讨论区、训练计划、团队、刷题统计）按既有模式各加 `internal/handler` 分组 +
`internal/model` 实体即可；提交已带 `contest_id/context` 维度，不锁死在训练场景。

## 判题数据流

1. 提交 → 校验（可见性/语言/比赛窗口/限流）→ 代码落盘 → `PENDING` 入队。
2. 判题机 gRPC 注册（共享密钥）→ API 调度器按容量拉队列 → 构造任务
   （代码 + 题目限制 + 测试点 sha256 清单）→ 提交置 `JUDGING` + 15 分钟租约。
3. 判题机按 sha256 下载缺失测试数据并缓存 → 编译（带缓存，checker/interactor 同理）
   → 逐测试点沙箱运行 → 比对/特判/交互 → 回传 `TaskResult`。
4. API 落库 + WebSocket 广播 `submission:<id>`；比赛提交额外广播
   `contest:<id>`（榜单增量刷新）。
5. 容错：判题机断线 → 其在租约内任务立即重回 `PENDING`；后台扫描器兜底
   处理 lease 过期的孤儿任务。

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

## 赛制

ACM：解题数降序 → 罚时升序；罚时 = AC 时刻 + 每次错误提交 20 分钟
（CE 不计）。封榜：`freeze_time` 后的提交不改变榜单，未解出的题显示 `?n`；
比赛结束后自动揭示。OI/IOI 赛制仅预留字段。
