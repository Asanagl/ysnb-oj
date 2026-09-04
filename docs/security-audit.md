# 安全审计报告（V1 · 2026-08-28）

审计方式：Mimosa 深度静态扫描（两轮）+ govulncheck 依赖分析 + 人工 SecAudit
（OWASP Top 10 / 认证授权 / 输入校验 / 敏感数据 / 沙箱逃逸面）。

## 一、扫描产物与封印

| 轮次 | Scan ID | 封印（sha256） | 发现 |
|---|---|---|---|
| 修复前 | `scan-2026-08-28T15-25-00.005Z-077511240f22` | `ecca298e59e859da12dd589f62a9a825e304ffe7b7ceeea1e267ed549ae57669` | 8（6h/2m）· 依赖通告 3 条命中 |
| 审计修复后 | `scan-2026-08-28T15-30-56.946Z-c9ccfdf70bea` | `829d12d07f1a90ae88108ac802606b092b3b23bd18447549c0f8a3c7021bbf20` | 6（4h/2m，全部为下表研判的运维参数误报）· 依赖通告 **0** 命中 |
| 测试修复后 | `scan-2026-08-28T17-33-34.332Z-ae275914b43d` | `725c8b455ed3fa4884a8f6317cc18d31e5bcd18db2b4c481e781569f6b868344` | 6（同上误报）· 依赖通告 **0** 命中 |
| 云测修复后 | `scan-2026-08-29T06-21-03.224Z-e5aa32c4110d` | `5491c23dd8b911ec5f1c1e904b4898ad652283548217857239715425c7e04d97` | 8（同上误报）· 依赖通告 **0** 命中 |
| 裁判/后台功能后 | `scan-2026-08-29T13-29-56.871Z-36476bb3f06d` | `0cdb9a2d17af56ae92ff427aeb3dd34ee46c8c301dc65db0c01c38ed3aa42a3a` | 8（同上误报）· 依赖通告 **0** 命中 |
| 审计修复后（最终） | `scan-2026-08-29T13-38-26.964Z-4ddf00daddb1` | `e0f4537025c95caee303c262532498e08d1ba7760a86da38ae532f1119a5f79c` | 8（同上误报）· 依赖通告 **0** 命中 |
| 可用性测试+审计修复后 | `scan-2026-08-29T16-23-41.037Z-643adb009115` | `c08836c522ff4e9b115d99ff7e476669e2b6f4ca4141d3e9c11753b43ce28ecf` | 8（同上误报）· 依赖通告 **0** 命中 |

修复前后对比：testdata 写入污点（2 high）已消除；依赖漏洞（pgx SQL 注入
GO-2026-5004、golang-jwt DoS GO-2025-3553）经升级（pgx v5.10.0、jwt v5.3.1）
后 govulncheck 报 **0 个代码可达漏洞**。

> 追记（测试 agent 发现，2026-08-29）：E2E 测试 agent 实测发现 GORM
> `First(db, c.Param("id"))` 内联条件导致的 SQL 注入（可 UNION 拖库 + 任意
> 文件读）及 5 项中低危问题（详见 docs/e2e-report.md BUG-001~006）。全部修复
> 并回归通过：所有 id 参数改为严格数值解析（paramID/*ByID），testdata 跨题
> 删除、隐藏比赛榜单泄露、WS admin 主题未授权、CSV 响应字段、角色管理端点
> 一并处理。最终验证：单元测试 4 包全绿、E2E 80/80、注入复现 404。

govulncheck（修复依赖后）：**0 个代码可达漏洞**；剩余 1 条模块级通告
GO-2026-5932（x/crypto/openpgp 不再维护）——本项目仅用 bcrypt，未导入
openpgp，不适用。

## 二、人工审查发现（均已修复）

| # | 严重度 | 位置 | 问题 | 修复 |
|---|---|---|---|---|
| H1 | 高 | handler/router.go | WebSocket 路由挂 `RequireAuth()`，浏览器 WS 无法带 Authorization 头 → 实时推送永远 401；若为修通而去掉鉴权则任何人可未授权订阅提交事件 | 新增 `wsAuth()`：校验 `?token=` 查询参数中的 JWT 后才允许升级连接；前端 `connectWS` 同步携带 token |
| H2 | 高 | handler/submissions.go | 提交限流器定义了但未接线，判题资源可被单用户打满 | `Server.Submits` 全局实例接入 `createSubmission`（15 次/分钟/用户，超限 429） |
| M1 | 中 | handler/middleware.go（新增） | 未鉴权端点（登录/注册）与提交接口无请求体上限，可被大包内存 DoS | 全局 `bodyCap(160MB)`（兼容 128MB zip 上传）+ 登录/注册/提交收紧 1MB；登录 10 次/分/IP、注册 5 次/分/IP 滑动窗口限流 |
| M2 | 中 | handler/testdata.go | `serveTestdata` 直接用路由参数拼路径；gin 参数解码行为存在变化风险 | 参数强制数值校验（范围限制）后再拼接；越界/非法一律 400 |
| L1 | 低 | handler/testdata.go | zip 测试点文件名 `Atoi` 错误被忽略，超长数字名会静默落到 case 0 | 显式错误处理 + `1..100000` 范围校验，非法条目跳过（同时闭合静态扫描的污点证据缺口） |

## 三、Mimosa 静态发现研判（8 条）

静态污点类 advisory 均需人工确认可利用性，逐条结论：

| 发现 | 严重度 | 结论 | 依据 |
|---|---|---|---|
| cmd/api `--config` 命令行参数 → config.Load 读文件（×4，2h/2m） | high/med | **不适用（误报）** | 数据源是运维启动进程时的 argv，攻击者无法控制服务进程参数；属标准服务器模式。风险归运行环境权限模型 |
| cmd/judge `OJ_LANGUAGES_FILE` 环境变量 → NewRegistry 读文件（×2，1h/1m） | high/med | **不适用（误报）** | 环境变量由 systemd/运维注入；能在判题机上改它的人已经拥有该机 root。配置覆盖属设计意图 |
| handler testdata `storeCase` HTTP 输入 → 文件写入（×2，high） | high | **已缓解 + 加固** | zip 条目名经 `^(\d+)\.(in|out|ans)$` 只取 basename，写入路径为 `DataDir/testdata/<pid>/<纯数字>.in`，无遍历空间；本轮补上 Atoi 错误处理与范围校验（L1）后证据链闭合 |

## 四、沙箱逃逸面（专项）

判题机进程以 root 运行，沙箱是唯一边界，因此按最高标准设计并已实现：
seccomp cBPF 默认拒绝白名单（禁 socket/ptrace/mount/unshare/setuid/chown/
io_uring/bpf/kexec 等）、`/` 只读 bind + 敏感路径 tmpfs 遮盖、全新 procfs/sysfs
（只读）、工作区 chown 65534 + 0700、cgroup memory(cpu,pids) + rlimits + 墙钟
看门狗 + cgroup.kill 全树终止、setuid 降权执行。已知残留限制（fail-closed，
非逃逸向量）：PID1 由用户程序承担，孤儿进程随 namespace 退出回收，受
pids.max 封顶；白名单漏放表现为合法语言特性 RE 而非提权。

上线前动作：校内 Linux 服务器执行 `oj-judge --selftest`；建议后续增加
逃逸用例集（seccomp 计划绕过、cgroup 限额逃逸、/proc/sys 写、mount 逃逸）
作为 CI 常规回归。

## 五、接受的残余风险（部署要求）

| 风险 | 缓解/条件 |
|---|---|
| gRPC JudgeRelay 明文（insecure credentials） | 仅限校内网段；判题机共享密钥鉴权。公网化前必须加 TLS 或 WireGuard 隧道 |
| JWT 存 localStorage（XSS 情形下可窃取） | SPA 标准取舍；题面渲染 markdown-it `html:false` + KaTeX 默认不信任模式，链接协议校验内置；无用户可注入 HTML 的入口 |
| 导入用户接口在响应中返回生成的初始密码 | 仅 admin 角色；设计用于一次性分发，文档要求导入后立即下发并建议用户改密 |
| WS CheckOrigin 放行 | socket 仅推送已鉴权订阅数据；建议 Nginx 层限制 Origin（deploy/nginx.conf 后续可加） |
| 密码策略下限 6 位 | 邀请制校内场景；CSV 导入密码为 10 位随机码 |

## 六、门禁结论

按 AGENTS.md 质量门禁：修复后 **无高危漏洞残留**——静态剩余条目全部为
"进程启动参数/环境变量 → 配置文件读取"类误报（数据源为运维输入，攻击者
不可控，已在第三节逐条研判），依赖 0 可达漏洞，人工发现的高危项（WS 鉴权、
限流接线）与测试 agent 发现的高危项（GORM 内联 SQL 注入）全部修复并回归
通过。真实 Linux 云服务器全链路实测通过（见 docs/cloud-test-report.md）。
**安全门禁：通过**。

## 七、增补审计（2026-08-29 晚 · 裁判功能 + 后台管理系统轮次）

覆盖新增代码：裁判操作（取消成绩/作弊打星/重测/公告/封榜/调时）、比赛
独立副本、公开用户主页、后台管理系统（/admin 框架 + stats 四接口）、
心跳负载上报。

**工具**：Mimosa deep ×2（功能后 + 审计修复后，见上表第 5/6 行封印）+
govulncheck（0 可达漏洞）+ 新增接口人工权限审查。

**人工审查结论（12 个新变更接口逐一核对）**：

| 接口组 | 守卫 | 结论 |
|---|---|---|
| /contests/:id 裁判操作 ×10（cancel/flags/rejudge/freeze/time/notices 写） | `judgeGuard`（creator/admin）+ `paramID` 严格数值解析 + `submissionInContest` 归属校验 | ✓ 无越权路径 |
| GET /users/:id/profile | RequireAuth；返回字段白名单（id/username/nickname/role/created_at + 统计聚合），**不含** student_no/口令哈希 | ✓ 按公开主页设计 |
| GET /admin/stats/* ×4 | admin 组 `RequireRole(RoleAdmin)` | ✓ |
| GET /contests/:id/standings.csv | RequireAuth + `canViewContest` | ✓ |
| POST /contests/:id/problems（独立副本复制） | `canManageContest` + 源题归属校验（拒绝跨赛复制）+ 纯数字 ID 路径 | ✓ |

**发现并已修复（2 项）**：

| # | 严重度 | 问题 | 修复 |
|---|---|---|---|
| A1 | 中 | 隐藏比赛公告泄露：`GET /contests/:id/notices` 缺 `canViewContest` 检查（BUG-004 同类，裁判功能轮遗漏） | listNotices 补可见性门禁，404 语义与 getContest 一致 |
| A2 | 低（功能） | 并发峰值统计排除比赛提交（`contest_id IS NULL` 过滤），赛时负载被低估 | 移除过滤，统计全量提交 |

**门禁结论（增补后维持）：通过**。全部扫描封印可复核
（`~/.mimosa/security-scans/project-682dc23f858064c09ee15e7e/`）。

---

## 八、增补审计（2026-08-30 · 用户出题审核流轮次）

覆盖新增代码：用户出题（`user_problems.go`：createUserProblem/myProblems/
updateMyProblem/reviewProblem/pendingProblems）、审核流可见性改动
（`problems.go` canSeeProblem）、提交拦截（`submissions.go`
createSubmission）、题解区（`solutions.go`，含 Markdown 富文本内容 64KB 上限、
父级同题校验、官方置顶 pin 仅作者/管理员）、注册信息编辑（`user_reg_edit.go`）。

**权限矩阵人工核对**：

| 接口 | 守卫 | 结论 |
|---|---|---|
| POST/GET/PUT /my/problems* | RequireAuth；PUT 校验作者本人 + 状态机（rejected→pending 重审，approved 不可再改内容绕审核） | ✓ |
| POST /admin/problems/:id/review | admin 组 | ✓ |
| GET /admin/problems/pending | admin 组 | ✓ |
| 提交未审核题 | createSubmission 独立拦截（author draft 除外）——E2E 第 6 项首轮回归暴露后修复并复测 | ✓ |
| 题解 CRUD | RequireAuth + 题目可见性 + 编辑/删除限本人，pin/delete 限作者或管理员 | ✓ |
| PUT /admin/users/:id（学号/昵称） | admin 组；学号无唯一约束（允许重复，按用户确认设计） | ✓ |
| 报名编辑/取消 | 赛前限定 + 本人或裁判 | ✓ |

**XSS 面**：题解/回复内容为 Markdown 富文本（TipTap 输出 HTML），前端渲染层
已按白名单样式隔离；后端存原文 + 64KB 上限；用户输入题目元数据走 JSON 绑定，
无 `html/template` 注入路径。审核通过前内容仅作者可见，泄露面收敛。

**门禁结论：通过**。E2E 15/15（`docs/e2e-report.md` 第九节），
构建 gofmt/vet 干净，Linux 交叉构建 + 生产部署复测通过。

---

## 九、增补审计（2026-08-30 · 题解区解锁门禁）

**变更**：`solution_gate.go` 新增；`solutions.go` list/create 改走
`solutionGuard`（可见性 + 解锁双检）。规则：任意提交解锁；比赛进行中全锁；
作者/管理员/裁判豁免（`canJudgeContest` 复用，无新增权限面）。

**核对**：
- 门禁覆盖 GET 与 POST 两条路径，无旁路（update/delete 仍由
  `canModerateSolution` 把守，其本身要求先能看到内容，未登录/未解锁拿不到 sid）；
- 403 文案不泄露题目内容，仅提示解锁条件；
- 解锁判定只查 `submissions.user_id+problem_id` 存在性（Limit 1），无时序绕过；
- 比赛锁基于服务器时间，前端仅是展示层，绕过 UI 直接打 API 同样被 403
  （E2E locked-post-403 实测）。

**门禁结论：通过**。E2E 12/12 + 审核流回归 15/15
（`docs/e2e-report.md` 第十节）。

---

## 十、增补审计（2026-08-30 · 多管理员/封禁轮次）

**变更**：新增 `super_admin` 角色（`roles.go` 收口 isAdminRole/isSetterRole，
全仓 12 处角色比较统一走 helper，杜绝遗漏）；`super_admin.go` 提供角色提升
（/role/super）与封禁（/ban、/ban/super）；`banGate` 中间件对每个已认证请求
做 banned 主键检查（活会话即时生效）；路由拆分 staff（setter+）/admin/
super 三层。模型新增 `User.Banned`，登录处显式 403。

**核对结论**：
- 权限提升唯一入口在 super 层：`PUT /admin/users/:id/role/super` 由
  `requireSuperAdmin` 把守；旧 `PUT /admin/users/:id/role` 收窄为
  user↔setter，且对 admin/super_admin 目标显式 403（admin 无法自我或互越
  提权，E2E admin-cannot-grant-admin=403 实测）；
- 锁死防护：不可改自己角色（双接口一致）；降级最后一名 super_admin 前做
  存量检查（「系统必须保留至少一名超级管理员」）；种子 admin 已一次性
  升级为首个 super（UPDATE ... WHERE role='admin' 幂等，UPDATE 1）；
- 封禁边界：admin 只能封 user/setter，super 不能封 super/自己；banGate
  按 UID 实时校验，封禁在 JWT 剩余生命周期内同样生效（E2E
  banned-live-api-403）；封禁不影响既有比赛/提交记录，仅阻断登录与请求；
- banGate 的代价是每个已认证请求 +1 次主键查询（users 表 ≤500 行，可忽略；
  未缓存的原因：封禁必须即时，缓存 TTL 会留下封禁生效窗口）；
- E2E 15/15 + 回归（题解区 12/12、审核流 15/15）。

**门禁结论：通过**。

---

## 十一、增补审计（2026-08-30 · 题单/团队/组队赛/用户管理轮次）

**覆盖新增代码**：题单（problem_lists.go）、训练小组（teams.go，含邀请码/
拉人/踢人/转让队长/公告/共享题单/队内排行）、组队赛（Contest.TeamMode，
以队报名 + 榜单按队聚合）、super_admin 角色与封禁（super_admin.go、
banGate 中间件）、全页出题编辑器（ProblemEditorView/MyProblemEditorView）
及配套前端。

**Mimosa deep 扫描**：封印 `sha256:bf4af7f5…`（scan-2026-08-30T10-28-35，
286 个依赖包，0 条匹配通告）。报告 8 项发现，逐项核实结论：
- 4 项指向 `remote-test/ssh.js`（本地测试驱动脚本，非生产代码路径，
  凭据走 .sshenv/gitignored）与 `main.go` 命令行参数 → config.Load——
  这是"启动参数指定配置文件路径"的常规模式，路径来自部署者而非攻击者，
  且 API 进程以 systemd 低权限运行；不成立。
- 2 项指向 judge 启动时从环境变量加载 languages.yaml 路径——配置面而非
  用户输入面，判题机不直接接触用户 HTTP 输入；不成立。
- 报告自注 `inconclusive`（调用图不完整），8 项均为静态 advisory 待人工
  确认类，无业务逻辑候选（0 business-logic candidate）。

**人工审查（逐接口权限矩阵核对）**：

| 接口组 | 守卫 | 结论 |
|---|---|---|
| /lists 写操作（create/update/delete） | RequireRole(setter+)；update/delete 另验 canEditList（创建者或 setter+） | ✓ |
| /lists 读 | RequireAuth（登录制社区全站公开） | ✓ 按产品设计 |
| /teams 全部 | 建队 RequireAuth；管理动作经 teamGuard（队长专属）；join 验邀请码 + 容量；踢人禁止自踢、队长转让需目标在队 | ✓ |
| /teams/:id/board | RequireAuth；只读聚合 | ✓ |
| 组队赛报名 | contest.TeamMode 时仅队长（team.CaptainID == claims）；成员数 ≤ TeamCapacity；成员已单独报名 → 整队报名被拒（唯一索引兜底） | ✓ |
| 榜单 teamStandings | 复用 canViewContest；纯展示层聚合，computeStandings 本体未动 | ✓ |
| /admin users/:id/role/super、/ban/super | requireSuperAdmin 中间件 | ✓ |
| PUT /users/:id/role（旧） | 收窄为 user↔setter；对 admin/super_admin 目标显式 403 | ✓ |
| banGate | 每请求按 UID 主键查 banned；封禁活会话即时生效 | ✓ |

**发现并修复（1 项）**：

| # | 严重度 | 问题 | 修复 |
|---|---|---|---|
| A3 | 中 | 前端队长拉人调用了 `GET /users?q=`（该路径为 admin 专属 listUsers）——普通队长调用必 403，功能不可用；且若后端曾把它开放给全员会泄露全量用户名册。实际风险：仅功能性缺陷，无数据泄露（路由本就 admin 门禁） | 新增 `GET /users/search`（RequireAuth，仅返回 id/username/nickname，Limit 20，复用 searchContestUsers 的白名单字段口径），前端改走该端点 |

**SQL 注入面复核**：新模块全部经 `paramID`/GORM 参数化
（`Where("invite_code = ?", c.Param("code"))` 为参数化查询，非拼接），
无 `First(db, c.Param(...))` 类模式回潮。

**回归**：teams 17/17、lists 19/19、user-problem 15/15（修复部署后全量复跑）；
`go build/vet/test` 全绿；前端 `npm run build` 通过。

**视觉测试**：桌面 1440×900 与手机 375×812 两档视口实测——
全页出题编辑器左右双栏 + 实时预览正常渲染（Markdown 标题/LaTeX 公式
即时呈现），手机端单栏堆叠且预览置顶；导航栏「题单」「小组」入口齐备；
登录态保持正常；题单/小组空态提示与创建入口正常。

**门禁结论：通过**。

## 十二、增补审计（2026-08-30 · M4 轮次：题目包导入/SPJ 验收/备份）

### 审计范围

题目包导入（自有/DOMjudge/Hydro 三格式解析 + 落盘）、checker/interactor
文件上传、导出扩展、沙箱 seccomp 白名单重构、备份/恢复脚本。

### 安全审查发现与处置

1. **【修复】沙箱 seccomp cBPF uint8 跳转回绕（高危·系统性）**
   原线性链白名单超过 127 条后跳转偏移回绕，filter 对合法/非法系统调用
   给出错误裁决。已重构为分块线性链（每块 ≤120 条 + JA 蹦床），并用
   解释器测试（seccomp_sim_test.go）穷举验证所有裁决路径。此 bug 同时是
   Java 判题 CE/RE 的根因（详见 e2e-report 第十五节）。
2. **【修复】JDK 判题所需的 AF_UNIX socket 系统调用**：javac/java 启动
   即建 socket(AF_UNIX)。白名单仅对 compile/run profile 放开 socket 组
   系统调用；判题沙箱网络命名空间内无任何网络接口，AF_INET 连接不可达，
   不构成出网能力。spj checker / interactor / 选手程序继续默认拒绝
   （judge.go 为各档位显式指定 Profile）。
3. **【通过】题目包导入面**：zip 流式落临时文件（上限 128MB），条目
   读取走既有 readZipFile 限额；路径拼接只取正则/后缀校验后的相对名，
   重编号为 1..N，zip 内路径不落盘；无 .in 的包整包拒绝；导入题目固定
   hidden + 由导入者（setter+，路由层 RequireRole）所有，需要人工审核
   后才公开。
4. **【通过】checker/interactor 上传**：setter+ 权限 + 判题模式前置校验
   + 扩展名白名单（.cpp/.cc/.cxx）+ 512KB 上限，内容按文本入库，与贴源
   码同一条数据路径。
5. **【通过】备份脚本**：backup.sh 仅 root cron 执行，全部输入为操作员
   级环境变量（无外部输入流）；pg_dump 走 runuser peer 认证不落明文密码；
   restore-drill 只在一次性容器/临时库操作，崩溃自动 dropdb。Mimosa 对
   脚本注释文本的「命令注入」提示经核实为误报（无用户输入汇点）。

### Mimosa deep 扫描

本轮随代码提交触发 hook 实时检测（无新增 HIGH/HOLD）；上轮封印
（sha256:bf4af7f5…）之后新增代码面以人工审查 + E2E 覆盖为主，
完整复扫将在收尾验收轮执行。

**门禁结论：通过**（1 项高危已在沙箱层修复并测试覆盖）。

## 十三、开源前加固轮（2026-09-02 · YSNB OJ 更名 + 开源准备）

### 13.1 扫描产物

| 轮次 | Scan ID | 封印（sha256） | 发现 |
|---|---|---|---|
| 开源前最终轮 | `scan-2026-09-02T13-27-31.297Z-40fa86f5cd77` | `a4b3100bece8c3f38de24bb49f4059b970d4d2422d21dd4ce9680a4738ac82d0` | 10（6h/4m，全部为既有研判的运维参数误报，与 2026-08-28 基线同源）· 依赖风险 **0** |

误报研判（与历史轮次一致）：`OJ_CONFIG`/`OJ_*` 环境变量与命令行参数进入
`config.Load()` / `NewRegistry()` 的文件路径读取——判题机/服务器环境变量
由 root 管控（systemd EnvironmentFile），不是不可信输入；remote-test/ssh.js
被跨文件关联属工具链代码，不参与部署产物。0 业务逻辑候选。

### 13.2 应用层加固（对照安全复核报告）

**P0（已修复）**
1. **XFF 伪造绕过限流**：gin 增加 `SetTrustedProxies(["127.0.0.1","::1"])`；
   nginx `proxy_set_header X-Forwarded-For $remote_addr`（覆盖而非追加），
   客户端伪造 XFF 不再影响 `c.ClientIP()`。
2. **API/gRPC 端口直连暴露**：docker-compose 的 api 服务 `ports: 8080/9090`
   改 `expose`（仅内网）；systemd 生产部署本就绑定 127.0.0.1（`ss -tlnp`
   复核确认），外网仅经 nginx。

**P1（已实施）**
3. **安全响应头**：后端全局中间件（`internal/handler/security.go`）+
   nginx 双层设置：`X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY`、
   `Referrer-Policy: strict-origin-when-cross-origin`、CSP（default-src 'self'，
   style 允许 unsafe-inline 供 Element Plus 运行时注入）、
   `X-Permitted-Cross-Domain-Policies: none`。外网 curl 验证四头齐落。
   HSTS 留待 TLS 上线后启用（nginx.conf 内注释位）。
4. **写端点限流补全**：新增 per-user 滑窗限流器，挂载：
   题目管理 20/min、题解写 10/min、用户建题 6/min、团队/题单 20/min、
   比赛 10/min、外部爬取触发 6/min、绑定/同步 4/min（防滥用为代理爬虫）。
   limiter map 带机会式清理（>4096 桶时剪枝），防伪造 key 撑内存。
5. **WS 访问日志**：`/api/v1/ws` 在 nginx 关闭 access_log（?token= 不落日志）。

**P2（记录为后续项）**
- 题目/比赛 payload 长度上限（title max=200、statement max=64K）——
  当前受 160MB bodyCap 与 128MB zip 上限保护，DB 单条膨胀风险低但存在；
- zip 解压总量预算（现 per-file 256MB，无累计上限）；
- LIKE 通配符转义（全表扫描 DoS 面，轻微）；
- `requireDaemonToken` 常量时间比较；
- WS CheckOrigin 同源校验；
- 注册用户名字符白名单。

### 13.3 基础设施层（生产已生效）

| 措施 | 状态 |
|---|---|
| ufw 防火墙 | **已启用**：default deny incoming，仅放行 22/80/443（sec-ufw.mjs 幂等脚本） |
| 判题机 OJ_MAX_PARALLEL | 2 → **1**（3.9GB 单机编译 OOM 修复，压测验证） |
| nginx 加固配置 | 已推送（备份 oj.bak-sec），nginx -t 门禁 + reload 验证 |
| 8080/9090 对外发布 | 已移除（compose expose + systemd 127.0.0.1 双确认） |

### 13.4 SSH 加固（2026-09-04 已闭环）

root ed25519 公钥已安装并经用户在场实测密钥登录成功后才执行配置变更：
`PasswordAuthentication no` + `PermitRootLogin prohibit-password`
（+ `KbdInteractiveAuthentication no`），drop-in 落点
`/etc/ssh/sshd_config.d/00-oj-hardening.conf`，`sshd -T` 实测生效，
新开密钥连接正常、密码连接实测被拒。fail2ban 经评估未安装（密钥化后
密码爆破面已消除）。操作顺序红线和回滚路径见 `docs/maintenance.md` 第七节。

**门禁结论：通过**（P0/P1 全部落地并有外网验证；P2 记录在案）。
