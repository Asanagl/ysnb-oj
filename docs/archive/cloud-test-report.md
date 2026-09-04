# 云服务器实测报告（2026-08-29）

> **存档说明**：本文档是 2026-09 上线阶段的历史快照，只读、不再更新。其中描述的工具链与配置（如密码 SSH、旧部署脚本）可能已被取代；现行口径以文档站首页（docs/index.md）导航的各分册为准。

环境：Debian 11 · x86_64 · kernel 5.10.0-10 · cgroup v2 · 2C4G（用户提供 root）。
方式：`remote-test/`（Node ssh2，stdin 流驱动，固定字面量 exec）+ `steps/*.sh` 阶段脚本。
部署：`oj-api-linux`（dev/sqlite，:8080/:9090）+ `oj-judge-linux`（cloud-judge-1，/oj-work）。

## 结果总览

| 验证项 | 结果 |
|---|---|
| 沙箱自检 `oj-judge --selftest` | PASS ×3（cgroup v2 / 越狱运行 / 看门狗） |
| C++ 闭环 AC/WA/TLE/RE/CE | PASS ×5（AC 8ms/13.4MB，双 case） |
| Python3 AC | PASS |
| SPJ（checker exit 0/1 语义） | PASS ×2 |
| 交互题（双向管道，interactor 判定） | PASS ×2 |
| 重判 rejudge 往返 | PASS |
| **API 重启后 PENDING 恢复** | PASS（re-enqueued→AC） |
| 判题机监控在线状态 + 心跳 | PASS（心跳稳定 30min+，0 错误） |
| 本地单元测试 / 双平台构建 / 前端构建 | PASS / PASS / PASS |

## 云端发现并修复的真 bug（共 6 个，本机/单测不可复现）

| # | 严重度 | 问题 | 根因与修复 |
|---|---|---|---|
| C1 | 高 | stage2 无法挂载 workspace（所有判题 SE） | stage2 先用 tmpfs 盖 /tmp 再 bind 位于 /tmp 的 workspace，源路径被遮盖。修复：workspace 改挂到命名空间内固定点 `/tmp/ojws`，敏感路径遮盖挪到 bind 之后（顺带隔离了同 WorkRoot 的兄弟运行目录） |
| C2 | 高 | seccomp 安装失败 EOPNOTSUPP（95） | 该云内核对裸 `seccomp(2)` 系统调用返回 EOPNOTSUPP（C 同参数成功）；内核两者走同一挂载路径。修复：改用 `prctl(PR_SET_SECCOMP)`（3.17+ 全支持）。探针过程见 steps/secc-probe |
| C3 | 高 | 编译全 CE：stage2 被 SIGSYS 击杀（syscall=262） | Go 1.21+ 的 `os.Stat` 走 `newfstatat(262)`，而 `resolveArgv0` 排在 seccomp 安装之后——安装过滤器后立刻 stat 自杀。修复：argv0 解析挪到 seccomp 之前（execve 之后不得再有任何 Go 侧系统调用） |
| C4 | 中 | blob 拉取 404 | 判题机拼 URL 缺 `/api/v1` 路由前缀。修复 fetch.go |
| C5 | 中 | blob 拉取 401 | judge 侧用 `cfg.JWT.DaemonSecret`（judge 进程不设置）而非 `cfg.Judge.DaemonToken`。修复 cmd/judge 传参 |
| C6 | 中 | 解释型语言源码未写入工作区（python exit 2） | 源码写文件只在 compile 分支内。修复：Run 中无条件先写源码 |

## 云端发现并修复的并发/可靠性 bug（3 个）

| # | 严重度 | 问题 | 修复 |
|---|---|---|---|
| R1 | 中 | RunPair 一方退出即互杀：用户正常退出导致 interactor 被杀（误报 SE） | 管道 EOF 本身就是收尾信号，删除互杀逻辑；父进程持有管道端副本的泄漏改为“启动后即释放”，真死锁由双方看门狗兜底 |
| R2 | 中 | 判题机心跳静默死亡 → 监控永远 offline | gRPC 流并发 Send（心跳 vs 结果）违反 grpc 语义。修复：`sendMu` 串行化 + 心跳失败日志化并继续重试 |
| R3 | 低 | API 优雅关停的 dropConn(offline) 与新 API 的 registerConn(online) 竞态 → 状态卡 offline | 心跳同时自愈 status=online（一个心跳周期内恢复） |

## 顺带补齐的功能与可观测性

- 题目 payload 增加 `checker_source`/`interactor_source` 字段（SPJ/交互题此前无法经 API 配置）。
- SE 详情落库（compile_message 追加 `[judge SE]`）+ CE/SE 保留现场工作区 + 判题机结果日志。
- API 启动时 `RequeuePending`：重启不再遗弃 PENDING 提交。

## 最终安全扫描（本代码态）

Mimosa deep：`scan-2026-08-29T06-21-03.224Z-e5aa32c4110d`，封印
`sha256:5491c23dd8b911ec5f1c1e904b4898ad652283548217857239715425c7e04d97`——
8 条全部为既有研判的"运维参数→配置文件"误报，依赖通告 0。

## 服务器现状

服务保留运行（/root/ojtest，dev/sqlite 模式），便于人工复核：
- 管理员：`admin`（密码在 /root/ojtest/oj.env 的 OJ_ADMIN_PASSWORD）
- 停止：`pkill -f oj-api-linux; pkill -f oj-judge-linux`
- **安全提醒：root 密码已在聊天中出现，验收后请立即修改**；oj.env 含等价密钥（chmod 600）。
