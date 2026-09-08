# 判题机与沙箱技术文档

> 面向要改判题核心或排查判题问题的开发者。概览级架构与安全模型见
> `architecture.md`；本文展开实现细节、参数、故障定位方法。
> 代码位置：判题机 `backend/cmd/judge` + `backend/internal/daemon`；
> 判题编排 `backend/pkg/judge`；沙箱 `backend/pkg/sandbox`；调度
> `backend/internal/judgehub`；协议 `backend/proto/oj.proto`。

## 一、判题机进程（oj-judge）

### 1.1 生命周期

```
启动 → Preflight（cgroup v2 可写性等）
     → 加载语言注册表（内嵌 languages.yaml，可用 OJ_LANGUAGES_FILE 覆盖）
     → judge.NewService（沙箱 + 语言表 + 工作区 + blob 缓存 + 回源客户端）
     → daemon.Run：gRPC 连接 API（OJ_API_ENDPOINT，默认判题机主动外连）
```

- `--selftest`：部署/换机后必跑。三项检查：cgroup v2 可写 → `/bin/echo`
  基础沙箱运行 → wall-clock 看门狗（100ms 限额能杀掉死循环）。全过输出
  `== all selftests passed ==`。
- 注册：判题机以 `OJ_DAEMON_NAME`（默认 judge-1）+ `OJ_DAEMON_TOKEN`
  （= API 侧 `OJ_DAEMON_SECRET`）注册；API 落库 `JudgeDaemon` 表供监控页。
- 并发：`OJ_MAX_PARALLEL` 控制同时在判的任务数（生产 = 1——3.9GB 内存
  预算下的实测安全值，论证见 §2.7 第 5 条）。

### 1.2 gRPC 协议（proto/oj.proto）

```proto
service JudgeRelay {
  rpc Connect(stream DaemonMessage) returns (stream ApiMessage);
}
```

单条双向流承载全部语义：注册、心跳（含 load/mem 遥测）、任务下发、
结果回传、testdata 拉取授权。判题机主动外连 → 判题机无需开入站防火墙，
天然支持多判题机水平扩展。

### 1.3 工作区与缓存布局

| 路径 | 内容 |
|---|---|
| `$OJ_WORK_ROOT/judge-<runid>/` | 单次判题工作区：源码、编译产物、checker/interactor、运行 IO、compile_err.txt |
| `$OJ_WORK_ROOT/tool-*` | checker/interactor 编译临时区（用后即删） |
| `$OJ_DATA_DIR/judge-cache/` | 测试数据 blob 缓存（按 sha256），缺失的从 API `/internal/testdata/...` 拉取 |

判题失败（CE/SE）的工作区**故意保留**（日志会打印 `workspace kept at ...`）
用于事后定位；成功运行的即用即删。保留区会慢慢积累，清理方法见
[运维 Runbook](../operations/maintenance.md)。

### 1.4 判题管线（pkg/judge）

1. **写源码**：`main.cpp` / `main.py` / `Main.java`（语言表的 source_file）。
2. **编译**（有 compile 段的语言）：独立沙箱档位，60s 超时 / 1GB 内存 /
   FSIZE 64MB / stack 512MB / pids 256；产物留工作区。
3. **准备 checker / interactor**：源码 sha256 全局缓存（`tool-*` 编译一次，
   之后硬链接进工作区），g++ -O2 -std=c++17。
4. **逐测试点运行**：数据按 sha256 校验拉取并缓存；每点独立沙箱运行
   （可按 CaseWorkers 并行、比赛提交首败即停——详见 §2.8）；比对策略按
   题目 judge_mode：default（diff，行尾/末空白容差）/ spj
   （`./checker <in> <ans> <out>`，exit 0=AC）/ interactive（双向管道）。
5. **聚合**：多测试点取最坏（SKIPPED 不参与定级）；CE/RE 等状态语义见
   [选手手册](../guide/user-guide.md)结果表。

**交互题**：用户程序与 interactor 各自进沙箱，由父进程搭桥双向管道；
任一方退出立即终止另一方；interactor 看门狗固定 30s，用户侧时限 =
题目时限 × 语言倍率。exit 0=AC / 1=WA（stderr 首行给选手）/ 其余=SE。
测试点允许没有 `.out`（interactor 自判）。

### 1.5 语言档位（pkg/judge/languages.yaml）

内嵌默认 + 文件覆盖，加语言 = 加配置段 + 装工具链，零代码改动：

| 字段 | 含义 |
|---|---|
| `source_file` | 选手代码落盘文件名（Java 必须 Main.java） |
| `compile.argv / timeout_s / mem_mb` | 缺省 = 解释型语言 |
| `run.argv` | `{MEM}` 占位符替换为题目内存限制（Java -Xmx 注入） |
| `time_multiplier` | 题目时限倍率（Python 4×、Java 3×） |
| `mem_overhead_mb` | 内存放宽（Python +128、Java +512） |
| `stack_kb` | RLIMIT_STACK |
| `as_limited` | 是否同时设 RLIMIT_AS（JVM 必须关：保留巨量虚拟地址，靠 cgroup RSS 兜底） |

当前三档：C++17（g++ -O2 -std=c++17 -Wall）、Python 3（python3 -B）、
Java 17（javac + java -Xss256m -Xmx{MEM}m -XX:-UsePerfData）。

## 二、沙箱（pkg/sandbox）

### 2.1 威胁模型与总原则

选手代码 = 完全不可信，checker/interactor 一样进沙箱。原则：**默认拒绝、
fail-closed**——文件系统最小可写、无网络、降权 uid 65534、资源全部封顶、
系统调用白名单外一律 SIGKILL。白名单漏放的表现是「某语言特性 RE」
（可用性问题），而非安全问题。

### 2.2 两段式执行（stage2 re-exec）

`Run(spec)` 由父进程完成：随机 runid 建 cgroup → `json.Marshal(spec)` 放进
环境变量 `OJ_STAGE2_SPEC` → `clone` 出**新 namespaces**（Mount/Pid/Net/Ipc/
Uts）并重执行自身二进制进入 `Stage2Main`。子进程按序：

1. `stage2Setup`：挂载私有视图（见 2.3）→ 降权 uid/gid 65534 → rlimits。
2. `resolveArgv0`：**在 seccomp 之前**按 PATH 解析出工具绝对路径
   （Go ≥1.21 的 PATH 探测走 newfstatat，过滤后会被杀）。
3. `loadSeccomp(profile)`：`prctl(PR_SET_SECCOMP)` 装过滤（不用 seccomp(2)
   裸系统调用——部分云内核返回 EOPNOTSUPP）。
4. `execve` 目标程序。此后任何白名单外系统调用 = SIGSYS。

### 2.3 文件系统视图

| 挂载点 | 处理 |
|---|---|
| `/` | bind 重挂为只读（MS_NOSUID\|NODEV），先 make-rprivate 阻断传播 |
| `/proc` | 新 procfs 实例（只读，仅本 PID ns） |
| `/sys` | 新 sysfs 实例（只读） |
| `/run` | tmpfs 8MB |
| `/tmp` | tmpfs 256MB（mode 1777） |
| `/dev` | tmpfs 16MB + 手工 mknod null/zero/full/random/urandom；`/dev/shm` tmpfs 64MB |
| `/home` `/root` `/var` | 空 tmpfs 盖死（防只读视图泄露宿主文件） |
| `/tmp/ojws`（ns 内） | 绑定宿主 `$OJ_WORK_ROOT/judge-<runid>`，**唯一可写点**（chown 65534, 0700） |

### 2.4 资源控制

| 手段 | 内容 |
|---|---|
| cgroup v2 | `memory.max`（swap 关）、`cpu.max`、`pids.max`（spec 未给则默认 512） |
| RLIMIT | CORE=0；FSIZE（输出封顶防刷盘）；STACK；AS（`as_limited` 才设）；CPU = 时限+1 秒兜底 |
| 看门狗 | wall-clock 到点杀 **PID1**（init = 用户程序），全树即灭；触发即判 TLE |
| 判定 | watchdog→TLE；cgroup OOM→MLE；SIGXCPU→TLE；SIGKILL 且 RSS>90% 限→MLE；exit 0→OK；其余→RE（exit code/signal 入 message） |

### 2.5 seccomp 白名单（重要：历史教训在这里）

cBPF 经典过滤器，策略 = 白名单 + SIGKILL：

- **公共段**：文件 IO/内存/信号/futex/clone(线程)/prlimit/statx 等常规集；
  `newfstatat`（glibc stat 与动态加载器走它）与 `clock_getres`（JVM 启动即探测）。
- **clone/fork/vfork/clone3**：两个 profile 都放行（编译需要 cc1/as/ld 子进程；
  运行档位线程用 clone，fork 系放行是为了 python multiprocessing 等合法场景，
  配合 pids.max 封顶）。
- **AF_UNIX socket 组**（socket/connect/bind/listen/accept/setsockopt/
  getsockname 等）：**compile 与 run 两个 profile 都放行**——JDK 启动即建
  socket(AF_UNIX)。沙箱在网络命名空间内（无任何接口），放行这些不构成
  出网能力；AF_INET 连接对象根本不存在。
- **编译档位差异**：seccomp 白名单当前对 compile/run 基本一致；资源与
  可写视图一致。未来若收紧运行档，注意交互题 interactor 也运行在此档。

**分块线性链**：白名单最初是单条比较链，每条用 uint8 算跳转偏移——
条目超过 127 后偏移回绕，合法调用落进过期 KILL、非法调用落进过期 ALLOW
（真实事故：Java 全线空 stderr RE，`glibc stat()` 被 SIGSYS 杀死）。
现实现按 ≤120 条分块，块尾用 `JA +2` 蹦床跳下一块，块尾不匹配跳过本块
ALLOW 入下一块；任意长度安全。

**防回归测试**：`pkg/sandbox/seccomp_sim_test.go` 内置 cBPF 解释器——
逐条验证白名单号到 ALLOW、0..459 全部非法号到 kill 类动作、所有跳转
偏移在 uint8 内。Windows 开发机跑不了（纯 Linux 代码），交叉编译测试
二进制推到判题机执行：

```bash
CGO_ENABLED=0 GOOS=linux go test -c -o sandbox.test ./pkg/sandbox/
# 推到判题机后：./sandbox.test -test.v
```

**为什么是 cBPF 而不是 eBPF（想"升级过滤器"前必读）**：

- seccomp 的内核接口**只接受经典 BPF**：`PR_SET_SECCOMP(SECCOMP_MODE_FILTER)`
  加载的就是 cBPF 指令数组（`sock_fprog`），内核不存在"eBPF seccomp"
  模式——社区多次提案均未合入主线。Docker / isolate / bubblewrap 等
  主流沙箱同样走 cBPF。
- "cBPF 过时、eBPF 更快"在这个场景不成立：内核自 3.18 起把所有加载的
  cBPF（含 seccomp 过滤器）**内部自动翻译成 eBPF 指令并 JIT 成机器码**。
  生产机实测 `net.core.bpf_jit_enable = 1`（Debian 11 / kernel 5.10）——
  执行期跑的本来就是 JIT 后的 eBPF，cBPF 只是加载格式；过滤开销纳秒级，
  "换 eBPF"拿不到任何性能或安全收益。
- 唯一的真 eBPF 替代是 **BPF LSM**（挂 LSM 钩子的 eBPF 程序）：需要
  root + 内核 `CONFIG_BPF_LSM` + `lsm=bpf` 启动参数，策略是**宿主机全局**
  的，会失去"每进程 fail-closed 白名单"语义，复杂度高一个量级——经评估
  （2026-09）**不作为判题强制层**；未来在支持内核上以审计模式（只记录、
  永不拦截）试点的规划见 `bpf-lsm-pilot.md`。过滤器演进在本节约束内进行。

### 2.6 沙箱故障定位方法

先用 `journalctl -u oj-api` 找 `submission judged` 锚点行定位到具体提交
（每次判题完成打一条 INFO；SE 另有 ERROR 行）；免 SSH 时可用管理后台
「日志查看器」页直接检索。再按现场深入：

1. **看保留工作区**：CE/SE/**RE** 的 `/oj-work/judge-<id>` 里有源码、compile_err.txt、
   复现脚本。判题机日志 grep `workspace kept`。RE 保留现场是定位幻影
   判题的关键（见 2.7）。
2. **SIGSYS 定位缺哪个系统调用**：`strace -f -o /tmp/trace.txt -p $(pidof oj-judge)`
   后触发一次提交，找 `killed by SIGSYS` 前最后一条未完成系统调用——
   修法是把它加进白名单并补测试。
3. **验证宿主行为差异**：`runuser -u nobody -- <cmd>` 对比沙箱内外表现，
   区分「缺系统调用」「缺文件」「权限/属主」三类问题。
4. **judge 机日志**：`journalctl -u oj-judge`（编译状态、每任务耗时、blob 缓存
   miss、看门狗触发都会打点；非 OK 的 run 会额外打 exit/signal/msg）。

### 2.7 已踩过的坑（改判题核心前必读，血泪换来的）

这四条都是在做判题效率优化时真实踩到、用上述方法论定位的。它们的共同
教训：**沙箱栈里的任何一层（测试二进制、Go runtime、cgroup、rlimit）都
可能以「用户程序的幻影 RE」的形式爆雷。**

1. **测试二进制的 stage2 递归**。沙箱的 stage2 是重执行 `/proc/self/exe`；
   生产判题机的 main() 会检查 `OJ_STAGE2` 环境变量并转入 Stage2Main，
   但 **go test 编译出的测试二进制的 main 是 testing.Main**——不认识这个
   环境变量，于是沙箱化的子进程把整个测试套件又跑了一遍（还嵌套自己的
   stage2），父进程 Wait 看到的是一个在递归测试的进程，所有断言都在
   看门狗边缘。修复：`pkg/sandbox/main_hook_test.go` 的 TestMain 先查
   `OJ_STAGE2` 再进测试。**任何要跑沙箱测试的二进制都必须有这个钩子。**
   症状：每个"逃逸测试"都精确耗时 = WallLimit（看门狗杀的），断言却
   全过——因为子进程真把事干了，只是慢。
2. **Pdeathsig 与 PID namespace**。曾怀疑 Go forkExec 的孤儿检查
   （getppid()==0）在 CLONE_NEWPID 子进程里误触发 kill(pid-1)（pidns
   init 对无 handler 信号免疫）导致 waitid 挂死。已移除 Pdeathsig：
   守护进程崩溃的清理由租约扫描器 + 看门狗兜底。
3. **rlimits 误伤 stage2 自身**。setrlimit 作用于调用进程——在 stage2
   setup 阶段设 RLIMIT_AS 会把 Go runtime 自己预留的数百 MB PROT_NONE
   arena 掐死，随机出现「fatal error: runtime: cannot allocate memory」
   幻影 RE。修复：rlimits 移到 Stage2Main 的 **execve 前一刻**——限制
   绑定 exec 后的镜像，不影响已映射的 Go 运行时。
4. **cgroup 资源链三连**：(a) `applyLimits` 曾是**死代码**——runProcess
   根本没调它，memory.max/cpu.max/pids.max 从未生效，所有 MLE 防线都在
   裸奔；(b) 接上后写 memory.max 又 EACCES——cgroup v2 规则：子组的
   controller 文件只在**父组 subtree_control 启用了对应 controller** 时
   才可写，Preflight 现在会幂等地启用 memory/pids/cpu（本机还发现
   nsdelegate 挂载选项下，给代理子组启 controller 会触发内核
   mem_cgroup_css_alloc 挂死——测试直接用生产 base）。Debian 12
   （systemd 252）还有一层：根组控制器是开机过程**惰性启用**的，开机
   早期启动的 judge 写 base 的 subtree_control 会 EACCES 且静默失败
   （bullseye→bookworm 原地升级后真实踩到：判题全 SE），故 Preflight
   的启用写入带重试与生效校验；(c) stage2
   （Go supervisor）和选手程序同 cgroup，memory.max 要加 64MB headroom，
   否则并发下内核先杀 supervisor。
5. **并发预算 = 主机内存，不是凭感觉**。每次沙箱运行 = Go supervisor +
   tmpfs 挂载 + 页表 ≈ 数百 MB 真实内存。判题总并发 = MaxParallel(任务)
   × CaseWorkers(每任务的并行 case 数)，**这个乘积不得超过主机内存预算**。
   3.9GB 内存机器上 2×2 就足以让内核 OOM-killer 制造幻影 RE/SE。默认
   CaseWorkers=1；加内存后调 `OJ_MAX_PARALLEL` 与 CaseWorkers 前先按
   `峰值内存 ≈ 并发数 × (cgroup 上限 + 64MB headroom)` 估算。

### 2.8 判题效率设计（算法层）

- **测试点级并行**：每个 case 独立子工作区（`ws/cases/<idx>/`，编译产物
  复制进去），worker 池按 `CaseWorkers` 并发，结果按 case 序号聚合；任一
  case SE 时取消同任务其余 worker（整个提交注定 SE，早停省资源）。
- **首败即停（比赛语义）**：`Task.StopOnFail`（proto `stop_on_fail`，
  judgehub 按提交是否在比赛中且非补题设置）——首个非 AC case 后其余
  全部标 SKIPPED，不启动。E2E 验证：30 case 题 WA 在第 1 点，实判 1 个
  跳过 29 个。
- **零分配流式比对**：默认判题的 token 比对是双指针流式扫描（无
  strings.Fields 的切片分配），等价性有 3000 组随机语料交叉验证。
- **比对顺序**：先整字节比较（完全相同即 AC，零成本），再退化到流式
  token 比较。

## 三、API 侧调度（internal/judgehub）

1. 提交校验（可见性/审核/比赛窗口/限流 15/min/用户）→ 代码落盘 → PENDING 入队
   （Redis Stream；开发用内存实现）。
2. 判题机注册后按容量拉任务 → 构造 Task（代码 + 限制 + 测试点 sha 清单）→
   置 JUDGING + **15 分钟租约**。
3. 回传落库 + WS 广播 `submission:<id>`（比赛题再广播 `contest:<id>`）。
4. 容错：判题机断线 → 其在租约任务立刻重回 PENDING；后台扫描器兜底回收
   lease 过期孤儿；API 重启时 `RequeuePending`。
5. testdata 下发走 `/internal/testdata/:pid/:case/:kind`（daemon token 鉴权，
   路径参数全数字校验防穿越）。

## 四、已知行为边界（改代码前必读）

- 比赛题是**独立副本**：setContestProblems 复制题目与测试数据，改原题不回灌；
  提交必须打到副本 id（对原题 id 提交会 404）。
- `require_registration` 建赛默认 true：不报名就提交会被拒。
- 判题机升级二进制前必须先停服务（systemd 持有旧文件，直接覆盖报
  Text file busy）。
- seccomp 白名单加系统调用前先想威胁模型，并**同步补 seccomp_sim_test
  用例**；任何跳转布局改动必须跑全量解释器测试。
- 新判题机上架流程：装工具链 → `/opt/oj/oj-judge --selftest` 全过 →
  配 oj-judge.env（token = API 的 OJ_DAEMON_SECRET）→ systemctl enable --now。
