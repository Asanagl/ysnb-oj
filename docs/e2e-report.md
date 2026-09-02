# YSNB OJ — 测试与可用性报告（e2e-report）

> 测试执行环境: Windows 11 + Git Bash, Go 1.27.0, Node 24.16.0
> 测试日期: 2026-08-28
> 测试资产: `backend/pkg/judge/pipeline_test.go`(新增), `scripts/e2e-test.sh`(新增), `scripts/ws-probe.mjs`(新增), `scripts/zipmake.go`(新增, 构造测试用 zip)
> 业务源码零修改（未为让测试通过而改动任何 backend/frontend 源码）

---

## 一、执行摘要（整体可用性结论）

**核心功能链路（认证、RBAC、题目/测试数据管理、提交、比赛、榜单、WS 推送）在 Windows 可测范围内全部可用且行为符合设计：E2E 80/80 断言全绿，判题编排流水线 30 个用例全绿，双平台构建与 vet 干净。**

但存在 **1 个高危安全漏洞**：几乎所有按 id 取实体的端点把 gin 路径参数直接传给 GORM `First/Delete`，gorm 将非纯数字字符串当作**原生 SQL 片段**执行，已实测可通过 UNION 注入以普通用户身份**拖走 admin 的 bcrypt 口令哈希**、并通过 submissions 视图实现**任意文件读取**。上线（尤其暴露到校园网/公网）前必须修复。

其余 5 项为中低严重度的功能/一致性问题。均未修改源码，已在下方给出精确位置与修复建议。

---

## 二、Phase A — 基线

| 项目 | 命令 | 结果 |
|---|---|---|
| 后端 Windows 构建 | `go build ./...` | PASS |
| 后端 Linux 交叉构建（判题机产物） | `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./...` | PASS |
| 静态检查 | `go vet ./...` | PASS（无告警） |
| 后端单元测试 | `go test ./...` | PASS（auth / handler / queue / judge 全部 ok） |
| 前端构建 | `cd frontend && npm run build` | PASS（vite built in 604ms；有 >500kB chunk 体积告警，仅性能提示） |

---

## 三、Phase B — 判题编排流水线测试（mock Runner）

**测试环境限制（已验证，属预期而非 bug）**：`Service.Run` 第一步 `chownWorkspace` 调用 `os.Chown`，Windows 上 Go 返回 `EWINDOWS`（go1.27 SDK `syscall_windows.go:1348`），因此 **`Service.Run` 在 Windows 上必然 SE**，真实沙箱为 Linux-only。为在 Windows 测纯编排逻辑，`pipeline_test.go` 用 `runPipeline` 驱动与 `Run` 完全相同的内部阶段（`Reg.Get → compileSubmission → prepareTool → judgeCases → aggregate`），仅跳过 chown，并用预置 BinCache 的方式让 `prepareTool` 走缓存命中路径（同样无 chown）。附带测试 `TestServiceRunChownLimitation` 把该平台差异固化成断言（Linux 上自动 skip）。

mockRunner 可编程：compile 场景在 `-o` 目标处创建"编译产物"、失败时写 stderr（供 CE 消息）；run 场景按 `stdin` 内容决定输出与资源占用；checker/interactor 按 `argv[0]` 识别。测试数据经 `httptest.NewServer` 供给 `/internal/testdata/<pid>/<idx>/<in|out>` 并断言 Bearer daemon token 已携带。

| # | 场景 | 结果 |
|---|---|---|
| 1 | 编译失败 → CE 且 CompileMessage 含编译器 stderr；编译 SE（沙箱状态 / Go error 两条路径）→ SE | PASS |
| 2 | 全 AC：diff 相等，time/mem 取各 case 最大值（42ms/1000KB），无早停跑满 2 case | PASS |
| 3 | 字节不等但 token 等价（多余空白/换行、CRLF vs LF）→ AC | PASS |
| 4 | WA → 整体 WA 且 message="output mismatch"；**乱序 Cases 列表**（列表序 2,0,1，状态 WA/TLE/WA）下按 case index 排序取第一个失败 → TLE | PASS |
| 5 | 用户程序 TLE/MLE/RE 状态与 message 透传（"time limit exceeded" / "memory limit exceeded" / RE 消息 trim） | PASS |
| 6 | blob sha256 不匹配 → 该 case SE("sha mismatch")；fetch 404 → SE("status 404")；整体 SE | PASS |
| 7 | SPJ：checker exit 0 → 沿用 AC；exit 1 → WA 且 message 取 checker stderr 首行（无 stderr 时回退 "checker exit N"）；checker SE → SE 带前缀；**用户 RE 时 checker 不被调用** | PASS |
| 8 | 交互题 RunPair：interactor exit 0 + 用户 OK → AC（time/mem 取用户程序读数）；exit 1 → WA + interactor stderr；exit 2 → SE("interactor exit 2")；exit 0 但用户 TLE → TLE；interactor SE / RunPair 基础设施错误 → SE | PASS |
| 9 | 语言不存在 → SE 且 Error 含 `unknown language`，0 个 case 被执行 | PASS |
| 10 | （附加）`Service.Run` 在 Windows 的 chown 限制固化测试 | PASS |

执行：`go test ./pkg/judge/ -v -count=1` — **30/30 全绿**（含原有 equalTokens/aggregate/registry/cache 测试）。

**本阶段未发现源码 bug。** 一处设计观察：case 级 runner 错误落在 `res.Cases[i].Message` 而 `res.Error` 仅承载 workspace/registry/compile 级错误——行为一致、已按实际语义断言。

---

## 四、Phase C — API E2E（真实 HTTP，`scripts/e2e-test.sh`）

脚本自启动服务器（每次全新 `data-e2e` sqlite + 内存队列 + 固定 OJ_DAEMON_SECRET）、每步断言打印 PASS/FAIL、自清理（taskkill + 删临时目录，含 EXIT trap），退出码非 0 表示有失败。**实测两轮均 `PASS=80 FAIL=0`，一键可复跑。**

覆盖矩阵与结果：

| # | 流程 | 结果 |
|---|---|---|
| 1 | GET /languages 200 且含 cpp；未登录 /auth/me 401 | PASS |
| 2 | admin 登录拿 token（bootstrap 凭据）；错误密码 401 | PASS |
| 3 | 邀请码生成 → 注册成功 → 重复用户名 400 → 无效邀请码 400 → 用尽的邀请码(max_uses=1 已用) 400 | PASS |
| 4 | RBAC：普通用户 /admin/users 403；无 daemon token 访问 /internal/testdata 401；错误 daemon token 401 | PASS |
| 5 | CSV 导入（含 1 个坏行）→ created=2、坏行 Err 非空、生成密码返回且可登录 | PASS |
| 6 | 题目 CRUD：建题→列表→详情（samples=2、can_manage）；普通用户可见 members、**hidden 详情 404 且列表不可见**；普通用户 PUT/POST 题目 403；空 title 400；tags 传非数组 400 | PASS |
| 7 | 测试数据：PowerShell Compress-Archive 打 1.in/1.out/2.in/2.out 上传 stored=2；GET testdata 2 条且带 64 位 sha256；非 zip 上传 400；**带 `../../evil.txt` 的 zip 上传后确认 data 目录内外均无落盘**；daemon token 拉取 blob 内容逐字节一致 | PASS |
| 8 | 提交流：正常提交 200 且 PENDING；未知语言 400；空代码 400；>256KB 400；hidden 题 404 | PASS |
| 9 | 比赛：创建 ACM 比赛并挂题 → 窗口内带 contest_id 提交 200 → 未来窗口 400 → 未挂题带 contest_id 400 → standings 有 rows 且 **PENDING 不计解题/罚时（solved=0, penalty_ms=0）** | PASS |
| 10 | WS：合法 token 连接 → onopen + 订阅 admin:daemons + 1s 稳定；无 token → 握手被拒（401 before upgrade） | PASS |
| 11 | rejudge 已有提交 200；不存在的 404；/admin/daemons 200 且 queue_length 数值型 | PASS |
| 12 | /auth/login 发 >1MB JSON → 400（gin MaxBytesReader 1MB；413/400 均在验收口径内，实测 400） | PASS |
| 13 | 限流精确断言：同用户第 9~15 个提交（含各 4xx）无 429，第 16 个起 429（15 次/分钟窗口） | PASS |
| 14 | 并发：5 个并发提交全 200，id 各异且连续递增 | PASS |

---

## 五、Phase D — Bug 猎捕结果

### BUG-001 【高危】路径参数未净化直达 GORM → SQL 注入（实测：admin 口令哈希外泄 + 任意文件读）

- **位置**：所有把字符串路径参数直接作为 gorm 条件的调用点，典型为
  - `backend/internal/handler/problems.go:96`（getProblem: `s.DB.First(prob, c.Param("id"))`）
  - `backend/internal/handler/submissions.go:175`（getSubmission）、`:89`（createSubmission 的 problem 查询不受影响，其用整型 binding）
  - `backend/internal/handler/contests.go:104,59`（getContest/updateContest）、`backend/internal/handler/admin_internal.go:46`（rejudge）、`backend/internal/handler/testdata.go:251`（deleteTestdata）、`backend/internal/handler/auth_users.go:233`（deleteInviteCode）
- **根因**：gorm `BuildCondition`（gorm.io/gorm/statement.go:286-311）对 `First(db, str)`/`Delete(db, str)`：`strconv.Atoi` 失败且无占位符参数时，字符串被当作**原生 SQL WHERE 片段**（`clause.Expr{SQL: s}`）。gin 的 `:id` 参数原样传入。
- **复现**（普通用户即可）：
  1. `GET /api/v1/problems/1=1` → 200 返回**第一行题目**（应为 404）
  2. `GET /api/v1/problems/1.5`、`/submissions/2e0` → 200 返回各自表第一行
  3. UNION 拖库（响应 title 字段即返回 `e2e-admin:$2a$10$...` 完整 bcrypt 哈希）：
     `GET /api/v1/problems/0=1 UNION SELECT 1,(SELECT username||':'||password_hash FROM users WHERE role='admin' LIMIT 1),'x','','','','','','',1,1,'members','default','','',1,'2026-01-01 00:00:00','2026-01-01 00:00:00'`
  4. **任意文件读**（`/submissions/:id` 注入行把 `user_id` 伪造成自己 → `canViewCode` 放行 → 服务端 `os.ReadFile(code_path)` 把文件内容放进 `code` 字段）：
     `GET /api/v1/submissions/0=1 UNION SELECT 999,2,1,NULL,'cpp','README.md',0,'PENDING',0,0,0,'','[]','2026-01-01 00:00:00',NULL,NULL` → `code` 字段 = README.md 全文（实测成功）
- **实际 vs 期望**：实际 = 任意 SELECT 注入/首行泄露/文件读取；期望 = 非法 id 一律 404。
- **缓解性实测**：`Delete` 路径（如 `DELETE /problems/1/testdata/1=1`）被 gorm "missing WHERE" 防护拦下，未发生全表删除（行仍存在），但接口**仍返回 200 {"ok":true}**（错误被吞，见 BUG-002）。
- **修复建议**：入口统一 `strconv.ParseUint(c.Param("id"), 10, 64)`（参照 `testdata.go` serveTestdata 已有的数字校验），或改用参数化条件 `First(&prob, "id = ?", id)`。逐点排查上述 7 处调用。

### BUG-002 【中】测试数据跨题删除 + 删除失败仍报成功 + 磁盘孤儿文件

- **位置**：`backend/internal/handler/testdata.go:240-253`（deleteTestdata）
- **复现**：题目 P1 的 case id=1，对题目 P2 调 `DELETE /api/v1/problems/2/testdata/1` → 200 {"ok":true}，P1 的 case 行被删（实测确认）。同一函数不校验 `tc.ProblemID == prob.ID`；`s.DB.Delete` 的错误被忽略（即使删除失败也 200）；也不删除 `testdata/<pid>/<n>.in/.out` 磁盘文件（产生孤儿 blob，且 daemon 仍可按旧 manifest 下载）。
- **期望**：校验 case 归属；错误返回 5xx；删除行时同步删除磁盘文件。

### BUG-003 【中】setter 角色无法通过 API 授予，出题流程不可达

- **位置**：`backend/internal/handler/auth_users.go:186-189`（importUsers 固定 `Role: model.RoleUser`）；`router.go` 全部 admin 路由无任何角色更新端点。
- **复现**：admin 通过 CSV 导入创建的用户角色恒为 `user`；API 中不存在 PUT /admin/users/:id/role 之类端点 → 除首位 admin 外无人能成为 setter/admin，出题工作流只能改库。
- **期望**：提供 admin 的角色管理端点（或在导入 CSV 中支持角色列）。

### BUG-004 【中/低】隐藏比赛榜单可被任意登录用户查看

- **位置**：`backend/internal/handler/contests.go:180-192`（getStandings 无 visibility 检查；对照 getContest:109-113 有）
- **复现**：创建 `visibility:"hidden"` 比赛 → 普通用户 `GET /contests/:id` 404（正确），但 `GET /contests/:id/standings` **200** 返回 rows（实测确认）。
- **期望**：与 getContest 一致的可见性判定。

### BUG-005 【低】WS 订阅无主题级授权

- **位置**：`backend/internal/wsq/hub.go:83-106`（readLoop 接受任意 subscribe），`router.go:93` 仅校验 token 合法性。
- **复现**：普通用户连接后发送 `{"subscribe":"admin:daemons"}` → 连接保持、订阅生效（实测确认）。`judgehub` 会向 `admin:daemons` 发布 daemon 心跳/注册信息（hub.go:212），登录用户可被动接收基础设施信息。
- **期望**：`admin:*` 主题要求 admin 角色（在 wsAuth 后按 claims 过滤订阅）。

### BUG-006 【低】CSV 导入响应字段名大写，与全 API 小写蛇形风格不一致

- **位置**：`backend/internal/handler/auth_users.go:142-148`（importUserRow 无 json tag → 序列化为 `Username/StudentNo/Nickname/Password/Err`）
- **复现**：`POST /admin/users/import` 响应 `{"created":2,"rows":[{"Username":...,"Err":"empty username"}]}`（实测）。前端/调用方需匹配大小写，易错。
- **期望**：补 json tag（`username/student_no/nickname/password/err`）。

---

## 六、"验证为预期行为"清单（勿当 bug 修复）

1. **Windows 上提交后停留 PENDING、admin/daemons 的 queue_length 增长**：判题沙箱 Linux-only，本机无 daemon 消费队列。
2. **`Service.Run` 在 Windows 上 SE "workspace chown"**：`os.Chown` 在 Windows 返回 EWINDOWS，属平台限制；判题编排已用 `runPipeline`（相同内部阶段）覆盖测试。
3. **`GET /submissions` 不带 `mine=1` 时返回全部用户的提交**（身份可见、代码隐藏）：V1 设计如此（登录制社区，`submissions.go:143-171`）；他人代码在比赛未结束/非本人时确实不下发（已断言）。
4. **zip 内 `../../evil.txt` 条目被忽略**：`caseFileRe` 只匹配 basename 的 `^\d+\.(in|out|ans)$`，无路径穿越（已实测确认无落盘）。
5. **>1MB 登录体 → 400**（gin `capJSONBody(1<<20)` + MaxBytesReader），符合"4xx 即通过"口径。
6. **限流阈值**：login 10/min/IP、register 5/min/IP、submit 15/min/user——第 16 次提交才 429（精确断言通过）；限流器先于参数校验执行，非法提交也计数（设计使然）。
7. **PENDING 不入榜**：`computeStandings` 只把 AC 计解题、WA/TLE/MLE/RE 计罚时，PENDING 既不计也不罚（已断言 rows 全 0 解题）。
8. **/dev 模式 JWT secret 与 daemon secret 自动生成**：仅 dev 便捷路径，prod 缺配置会直接启动失败。
9. **前端构建的 chunk 体积告警**（>500kB）：vite 性能提示，构建成功。
10. **并发提交 id 连续递增**：sqlite 单写者串行分配主键，行为正常。

---

## 七、可复现测试资产与复跑指引

| 资产 | 说明 |
|---|---|
| `backend/pkg/judge/pipeline_test.go` | 判题编排 30 用例（含负面），`go test ./pkg/judge/ -v`，gofmt 干净 |
| `scripts/e2e-test.sh` | 一键 E2E：自启/自清/幂等/每步 PASS-FAIL/退出码非 0 表失败。运行 `bash scripts/e2e-test.sh`，两轮实测 80/80 |
| `scripts/ws-probe.mjs` | WS 探针（valid/anon 两模式，exit 0=PASS），由 e2e-test.sh 调用 |
| `scripts/zipmake.go` | 构造测试 zip（含路径穿越样本），`go run scripts/zipmake.go -mode testdata\|traversal -out x.zip` |

最终回归（收尾时复跑）：backend `go build ./...` + Linux 交叉构建 + `go vet` + `go test ./... -count=1` 全 PASS、gofmt 无 diff；frontend `npm run build` PASS；`scripts/e2e-test.sh` 复跑 PASS=80 FAIL=0。测试进程与临时数据目录均已清理（无遗留 oj-api.exe、无 data-e2e 残留）。

---

## 八、修复状态追记（2026-08-29，主 agent）

BUG-001~006 已全部修复并回归：

| Bug | 修复 | 验证 |
|---|---|---|
| BUG-001 SQL 注入（高危） | 新增 `paramID`/`problemByID`/`contestByID`/`submissionByID`（`ids.go`）严格数值解析，替换全部 14 处 `First/Delete(db, c.Param(...))` | 报告中的 3 个注入复现（`1=1`、`2e0`、UNION 拖库）实测均 404，无泄露 |
| BUG-002 testdata 跨题删除 | `deleteTestdata` 校验 case 归属、DB 错误返回 5xx、同步删除磁盘 blob | 代码路径 + 全量回归 |
| BUG-003 无角色管理端点 | 新增 `PUT /admin/users/:id/role`（admin 专用；禁止改自己的角色防锁死） | 接口注册 + 回归 |
| BUG-004 隐藏比赛榜单泄露 | 抽取 `canViewContest`，`getStandings` 与 `getContest` 同一口径 | 代码路径 + 全量回归 |
| BUG-005 WS 主题未授权 | `Hub.Handler(authorize)` 按连接角色过滤订阅；`admin:*` 仅限 admin（`router.go wsTopicAuth`） | 代码路径 + 全量回归 |
| BUG-006 CSV 响应字段大写 | `importUserRow` 补小写蛇形 json tag | E2E 脚本断言同步更新为修复后契约，复跑 **PASS=80 FAIL=0** |

最终回归：`go build ./...`（Windows）+ `GOOS=linux` 交叉构建 + `go vet` + `go test ./... -count=1`（4 包全绿，含流水线 30 用例）+ `bash scripts/e2e-test.sh`（80/80）+ Mimosa 第三轮扫描封印 `725c8b45…`（发现数不变、依赖通告 0）。`dist/` 下三个二进制均为修复后重建。

---

## 九、用户出题/审核流 E2E（2026-08-30，云端复测）

新增 `remote-test/user-problem-e2e.mjs`（直连生产 <your-server-ip>，覆盖本轮新增的
用户出题 + 审核流 + 题解区全链路），15 项断言：

| # | 断言 | 结果 |
|---|------|------|
| 1 | 普通用户注册成功 | PASS |
| 2 | 用户出题创建成功 | PASS |
| 3 | 自动隐藏（visibility=hidden） | PASS |
| 4 | 自动进入待审（review_status=pending） | PASS |
| 5 | 不出现在公开题库 | PASS |
| 6 | 未审核题不可提交（`题目尚未通过审核，暂不能提交`） | PASS（复测） |
| 7 | 出现在管理员待审队列 | PASS |
| 8 | 管理员审核通过后作者可见 | PASS |
| 9 | 状态变为 approved | PASS |
| 10 | 普通成员（其他登录用户）可见 | PASS |
| 11 | 题解创建成功 | PASS |
| 12 | 官方题解置顶生效 | PASS |
| 13 | 分楼层回复创建成功 | PASS |
| 14 | 回复列表正确（2 层） | PASS |
| 15 | 删除题解生效 | PASS |

**复测原因（用户指令：变动后不得沿用旧轮结果）**：首轮 14/15，第 6 项暴露真实 bug ——
审核流引入"作者可见自己 pending 题"后，作者仍可向未审核题提交判题（提交成功 id=26）。
修复：`canSeeProblem` 对作者自己的 pending/rejected/draft 题仅授予可见性；
`createSubmission`（`submissions.go`）独立拦截非 approved 题（作者 draft 除外），
返回 400 `题目尚未通过审核，暂不能提交`。重建（Windows + Linux 交叉）、部署生产、
复跑 E2E：**15/15 PASS**。E2E 断言同步更新为修复后契约。


---

## 十、题解区解锁门禁 E2E（2026-08-30，云端）

新增 `remote-test/solution-gate-e2e.mjs`（12 项断言，规则经用户确认）：

**规则**：题解区默认隐藏；用户对某题有**任意一次提交**（含 CE/补题）即解锁该题
题解区的读与发帖；**比赛进行中**该比赛所有题的题解区对参赛者全锁（读+发帖），
赛后自动开放；豁免名单 = 题目作者 + 管理员 + 该比赛裁判。

| 断言 | 结果 |
|---|---|
| 管理员登录 / 生成邀请码 / 注册新用户 / 登录 | PASS ×4 |
| 未提交时 GET 题解区 → 403 | PASS |
| 未提交时 POST 题解 → 403 + 明确文案「题解区未解锁：请先提交本题（比赛结束后自动开放）」 | PASS |
| 管理员豁免 → 200 | PASS |
| 提交一次后 GET → 200、POST → 200（发帖后清理） | PASS ×2 |

**回归**：`user-problem-e2e.mjs` 复跑 15/15（题解区 handler 改动后原审核流用例
不受影响——审核流中题目批准后作者即视为可管理，走豁免路径）。

**实现**：后端 `solution_gate.go`（`solutionUnlocked` + `solutionGuard`，
list/create 共用同一门禁）；前端 `ProblemDetailView.vue` 收到 403 时渲染
锁定态面板（🔒 + 提示文案），不弹错误。比赛进行中的全锁由
`problemContest` → `StartTime < now < EndTime` 判定，无需额外配置。

---

## 十一、多管理员/封禁 + 全页出题编辑器 E2E（2026-08-30，云端）

新增 `remote-test/multi-admin-e2e.mjs`（15 项断言）。前提：种子账号
`admin` 已由一次性脚本（`steps/promote-super.sh`，幂等）升级为首个
`super_admin`。

**角色体系（本轮变更）**：`super_admin > admin > setter > user`。
super_admin 独占「授予/收回 admin、super_admin」与「封禁任意非 super 账号」；
admin 保持用户资料/重置密码/邀请码/重测等能力，并可在 user↔setter 间移动用户、
封禁普通用户；setter 进入后台受限页（审核队列/题库/比赛管理）。

| 断言 | 结果 |
|---|---|
| super 登录 | PASS |
| super 授予 admin ×2（/role/super） | PASS |
| 新 admin 重登后持新角色 | PASS |
| admin 试图授予 admin → 403（角色提升不可越级） | PASS |
| admin 移动 user↔setter → 200 | PASS |
| admin 封禁普通用户 → 200；封禁其他 admin → 403 | PASS |
| super 封禁/解封 → 200 | PASS |
| 被封禁用户：活会话 API 403 + 登录 403；解封后登录恢复 | PASS ×3 |
| super 不可改自己角色（防锁死）→ 400 | PASS |
| 降级清理恢复 → 200 | PASS |

**首次运行插曲**：两处非产品问题——(1) E2E 用旧 JWT 调新角色权限（角色在
token 里），重登后通过；(2) 断言口径把「admin 封禁普通用户」误记为应拒绝，
按确认的权限矩阵修正为允许。修正后 15/15。中途撞到登录限流 429（10/min/IP）
属预期防护。

**回归**：`solution-gate-e2e.mjs` 12/12、`user-problem-e2e.mjs` 15/15。

**全页出题编辑器**（`ProblemEditorView.vue` / `MyProblemEditorView.vue`，
路由 `/problems/new|:id/edit` 与 `/my/problems/new|:id/edit`）：
左右分栏（左编辑/右实时预览），≥1280px 双栏、以下单栏且预览置顶；
样例结构化编辑（不再是 JSON 字符串）；题库管理页与「我的题目」页的
新建/编辑均改为跳转全页编辑器；驳回题目支持一键重投（resubmit-only 接口）。
前端 `npm run build` 通过；视觉验收随前端包部署生产。

---

## 十二、手动删除邀请码 E2E（2026-08-30，云端）

新增 `remote-test/invite-delete-e2e.mjs`（6 项断言）。说明：后端
`DELETE /admin/invite-codes/:id` 与 handler 此前已存在（admin 层路由），
本轮补齐前端入口——用户管理页邀请码表格新增删除按钮（气泡二次确认，
提示"已注册用户不受影响"）与 API 封装，随前端包部署生产。

| 断言 | 结果 |
|---|---|
| super 登录、生成邀请码 | PASS ×2 |
| 删除邀请码 → 200 | PASS |
| 删除后不再出现在列表 | PASS |
| 用已删除的邀请码注册 → 400（invalid invite code） | PASS |
| 未登录调用删除 → 401 | PASS |

业务口径：所有邀请码均可删（未用完/已用完/已过期），删除只影响后续注册，
已注册账号与既有数据不受影响；删除即时生效（注册接口按码即时查询）。

---

## 十三、训练题单（Problem Lists）E2E（2026-08-30，云端）

新增 `remote-test/lists-e2e.mjs`（19 项断言）。训练计划第一阶段落地：
简单列表型题单，全站公开（登录即可浏览），创建/编辑限 setter 及以上
（含题单创建者本人）。

**接口**：`GET /lists`、`GET /lists/:id`（带调用者进度）、
`POST/PUT/DELETE /lists`。进度（todo/tried/ac）读时从提交表推导
（取消的提交不计），无独立状态表——没有失效问题。

| 断言 | 结果 |
|---|---|
| super 登录 / 普通用户注册登录 | PASS ×2 |
| 题库取两道 approved 题 | PASS |
| 创建题单（含顺序+备注） | PASS |
| 普通用户列表可见（全站公开） | PASS |
| 详情：2 题、顺序保持、备注、editable=false | PASS ×4 |
| 初始进度 todo | PASS |
| 提交一次后进度变为 tried/ac（轮询等待判题） | PASS |
| 更新：换序、改描述生效 | PASS ×2 |
| 匿名访问 401（登录制社区） | PASS |
| 普通用户建单 403 | PASS |
| 删除后详情 404 | PASS ×2 |

**前端**：`/lists` 列表页（卡片式）、`/lists/:id` 详情页（进度标签，
点击行跳题面）、`/lists/new|:id/edit` 编辑器（搜索添加、上下移动、备注），
顶部导航新增「题单」入口。构建 + 部署通过。

**下一阶段（已确认顺序）**：团队/小组 → 讨论区 → 收尾验收。

---

## 十四、训练小组 + 组队赛（ICPC 三人一队）E2E（2026-08-30，云端）

新增 `remote-test/teams-e2e.mjs`（17 项断言）。按用户确认的口径实现：

- **小组**：人人可建队（建队者即队长），人数上限建队时可配（1-5，默认 3
  对齐 ICPC）；加入方式 = 邀请码 + 队长按用户名拉人；队长+队员两级角色，
  队长可拉人/踢人/转让队长/重置邀请码/发公告/共享题单，最后一名成员
  （即队长）退出时小组自动解散。
- **队内功能**：公告（队长发/删）、题单共享（复用题单模块）、队内排行
  （按共享题单范围内的去重 AC 数/尝试题数排序）。
- **组队赛**（Contest.TeamMode）：比赛创建/编辑可开启，指定队伍人数上限；
  报名以队为单位——**仅队长可报名**（403 守卫），全员生成共享 TeamID 的
  报名行；有队员已单独报名时整队报名被拒（唯一索引兜底，提示先取消）；
  重复报名幂等；榜单聚合为**一行一队**（任一队员过题即队伍过题，取最早
  AC 时间，错误尝试合并计罚时；★/作弊标记跟随队长），`team_mode` 标志
  随榜单下发。个人赛完全不受影响。
- **提交归属**：组队赛中任一队员从比赛页提交即计入队伍成绩（提交记录
  保留提交者），与 ICPC 现场三人共机提交一致。

| 断言 | 结果 |
|---|---|
| 3 名测试用户创建/登录 | PASS |
| 建队（3 人上限）| PASS |
| 邀请码加入 ×2、第 4 人被拒（capacity） | PASS ×3 |
| 队员拉人 403 | PASS |
| 公告创建 | PASS |
| 题单共享 | PASS |
| 队内排行 3 人 | PASS |
| 组队赛创建（team_mode） | PASS |
| 队员报名 403 / 队长报名 200（3 成员） | PASS ×2 |
| 重复报名幂等 | PASS |
| 已入队成员单独报名被拒 | PASS |
| 榜单 team_mode 标志 | PASS |

前端：`/teams` 列表（卡片+邀请码加入+创建对话框）、`/teams/:id` 详情
（成员/公告/共享题单/排行四区，队长管理面板），比赛页在组队赛下自动
切换为「以队报名」卡片（选择自己担任队长的小组）。构建+部署通过。

**下一阶段**：讨论区 → 收尾验收。

## 十五、M4：题目包导入导出 + SPJ/交互验收 + 语言档位 E2E（2026-08-30，云端）

**范围**（与用户逐项确认后确定）：题目包导入（自有格式 + DOMjudge + Hydro
全支持）、导出包含 checker/interactor、checker/interactor 文件上传、
SPJ/交互生产 E2E 验收、Python/Java 档位放宽一档并验证、备份脚本 + 恢复演练。

### 15.1 题目包导入导出（m4-import-export-e2e.mjs，22 项）

| 断言 | 结果 |
|---|---|
| 自有格式：建题 → 传 2 个测试点 → checker 上传 → 导出 zip 含 checker/meta | PASS ×5 |
| 自有格式导出再导入：format=own、2 测试点、judge_mode=spj、标题保留、导入后 hidden | PASS ×5 |
| DOMjudge 包（problem.yaml flow-style limits + statements/ + data/{sample,secret}）：标题/时限 1500ms/内存 512MB/3 测试点（嵌套目录自然排序重编号）/模式 default | PASS ×6 |
| Hydro 包（problem.yaml + problem_zh.md + testdata/ + checker.cpp）：标题/时限 2s/模式 spj/2 测试点 | PASS ×4 |
| 负面：无 .in 的 zip 被拒（明确报错） | PASS |
| checker 文件上传（POST /problems/:id/checker）持久化 | PASS ×2 |

### 15.2 SPJ/交互题生产验收（m4-spj-interactive-e2e.mjs，8 项）

真实 checker/interactor 编译执行，正确解与错误解各提交一轮：

| 断言 | 结果 |
|---|---|
| SPJ：checker `./checker in ans out`（答案文件故意错误，由 checker 判定回显）正确解 AC | PASS |
| SPJ：+1 错误解 WA（stderr 反馈展示） | PASS |
| 交互：interactor 发数/收回显（exit 0=AC / 1=WA），正确解 AC | PASS |
| 交互：直接输出 0 的错误解 WA | PASS |
| 交互题测试点允许无 .out | PASS |

### 15.3 语言档位（m4-languages-e2e.mjs，13 项）

| 断言 | 结果 |
|---|---|
| /languages 下发 cpp/python3/java | PASS ×3 |
| 档位参数：Python 4x/128MB、Java 3x/512MB（本轮放宽后生效） | PASS ×4 |
| 同题三语言正确解全部 AC | PASS ×3 |
| Python sleep 3.2s < 4x 档位余量 → AC（1x 下必 TLE，倍率真实生效） | PASS |
| 测试数据 zip 上传 | PASS |

**过程中发现并修复的系统性 bug（重要）**：Java 全部 CE/RE 的根因不在 JDK
（服务器缺 openjdk-17，已安装），而是**自研沙箱 seccomp 白名单的 cBPF 跳转
偏移用 uint8 计算，白名单超过 127 条后静默回绕**——合法系统调用（glibc 的
newfstatat、JVM 启动的 socket(AF_UNIX)/clock_getres）落到过期 KILL 指令被
SIGSYS 杀死，表现为空 stderr 的 RE。修复：白名单改为**分块线性链**（每块
≤120 条，块间 JA 蹦床，任意长度安全），新增 newfstatat/clock_getres 及 JDK
启动所需的 AF_UNIX socket 调用（网络命名空间内无网络，不构成连通性）；并补
**cBPF 程序解释器测试**（pkg/sandbox/seccomp_sim_test.go，在 Linux 判题机
上跑通）：逐条验证白名单号到 ALLOW、0..459 全部非法号到 kill 类动作，防回归。

### 15.4 备份与恢复演练（生产落地）

- `deploy/backup.sh` → `/opt/oj/backup.sh`：pg_dump -Fc（runuser peer 认证，
  自动探测 docker 容器形态）+ data 目录快照（rsync 缺失时降级 cp）+
  保留策略（日备 14 天 + 每月 1 号月备留 6 个月）。
- `deploy/restore-drill.sh` → `/opt/oj/restore-drill.sh`：docker 一次性容器
  或本机临时库 `oj_drill_tmp` 真实恢复并打印行数（不触碰生产库）。
- **实跑结果**：备份 61K dump + 1.1M 数据快照；演练恢复行数与生产一致
  （users 48 / problems 60 / submissions 91 / contests 9 / test_cases 44）。
- crontab：每日 03:00 备份、每月 1 号 04:00 演练，已安装生效。

**下一阶段**：讨论区 → 收尾验收（清理测试数据、凭据轮换提醒、压测复测）。

## 十六、收尾验收（2026-08-30，云端）

按用户确认的四项口径执行：压测全链路复测、清理保留少量示例、只轮应用层
凭据、双册文档。

### 16.1 全链路压测（steps/stress-full.sh）

规模按限流上限设计（注册 5/min/IP、登录 10/min/IP、提交 15/min/用户）：

| 指标 | 结果 |
|---|---|
| 提交洪峰：8 并发用户 × 8 题（比赛独立副本）= 64 提交 | 40s 注入完毕，**64/64 AC** |
| 洪峰期间 API 延迟（/problems 六次采样） | 1.9–3.9 ms |
| 榜单延迟（64 提交落库后 5 次采样） | 2.1–6.3 ms |
| 判题吞吐 | 2 并行判题机 + 编译缓存下，洪峰结束时已全部判完 |
| 登录限流器 | 连续错误登录按设计触发 429（前段 401） |

### 16.2 清理前全模块回归

9 套 E2E（solution-gate 12 / multi-admin 15 / invite-delete 6 / lists 19 /
teams 17 / user-problem 15 / m4-import-export 22 / m4-spj-interactive 8 /
m4-languages 13）= **127 项断言全部通过**。

### 16.3 应用层凭据轮换

- 服务器端现场生成全部新值（脚本零凭据字面量）：admin 密码、PostgreSQL
  `oj` 角色密码、OJ_JWT_SECRET、OJ_DAEMON_SECRET。
- 旧 env 自动备份（oj.env.bak-*，0600）；oj.env / oj-judge.env 重写后重启。
- 自验证：新密码登录 OK、/languages 200、判题机以新 daemon 密钥重连。
- 凭据清单交付本地 `remote-test/.credentials-2026-08-30.txt`（勿入库）；
  服务器侧副本 `/root/credentials-new.txt`（0600，确认后建议删除）。
- **root SSH 密码未轮换**（用户选择只轮应用层，需自行 passwd root）。

### 16.4 测试数据清理 + 示例重建

- 清空：全部测试/压测用户（仅留 admin）、题目、提交、比赛、题单、小组、
  题解、邀请码、报名/标记/公告；testdata 目录孤儿清理；序号重置。
- 重建示例：**A+B Problem**（public，2 个测试点）、「入门题单」（含 A+B）、
  「新生演示赛」（1 小时后开场、2 小时时长、需报名）、新邀请码
  （30 次可用，见后台邀请码页）。清理前备份留存
  `/opt/oj/backup/db/oj-2026-08-30.dump`，可整体回滚。
- **清理后 smoke 全绿**：登录 / 题库含 A+B / 题单 1 item / 演示赛 /
  languages 200 / 前端 200 / 真实提交判题 **AC**（轮换后全链路验证）。

### 16.5 双册文档

- `docs/user-guide.md` — 选手手册：注册/刷题/结果含义/题解区/题单/比赛
  （含组队赛）/小组/常见问题。
- `docs/admin-guide.md` — 管理·出题人手册：角色矩阵、用户管理、出题与
  题目包导入、比赛编排与赛中裁判、判题机监控、备份恢复、周检清单、
  故障排查速查。

### 结论

**收尾验收通过。** 系统处于交付状态：生产服务运行中、数据已复位、凭据
已轮换（root 除外）、备份/演练 cron 生效、文档齐备。遗留动作（用户侧）：
轮换 root SSH 密码；确认凭据清单后删除服务器 /root/credentials-new.txt。

## 十七、判题效率优化轮（2026-09-01，云端）

目标（与用户逐项确认）：单提交延迟优先、比赛首败即停、允许动沙箱核心
（配逃逸测试集护栏）、基准+全量回归+逃逸测试作验收。

### 17.1 交付

| 项 | 内容 |
|---|---|
| 测试点级并行 | 每 case 独立子工作区 + worker 池 + case 序聚合 + SE 取消（pkg/judge/judge_parallel.go） |
| 比赛首败即停 | proto `stop_on_fail` → judgehub 按比赛/补题上下文设置 → 调度跳过未启动 case（SKIPPED） |
| 零分配流式比对 | equalTokens 双指针流式扫描替换 strings.Fields 全量分配；3000 组随机语料与旧语义交叉验证等价 |
| 沙箱护栏测试 | 逃逸测试集（读诱饵/proc/路径穿越/写宿主/网络/fork 炸弹/内存炸弹/seccomp 违规）+ cBPF 解释器测试 + 单次运行耗时探针，全部在判题机实跑 |

### 17.2 过程中定位并修复的沙箱缺陷（本轮最大价值）

并行压测暴露了一个间歇性幻影 RE（选手程序正常却报 RE，exit=2、
stderr 为 Go runtime fatal、wall≈20ms）。逐层定位出**五个叠加缺陷**：

1. **applyLimits 是死代码**：runProcess 从未调用，memory.max/cpu.max/
   pids.max 从未生效——所有资源防线形同虚设。
2. **subtree_control 未启用**：接上后写 memory.max EACCES——cgroup v2
   规则，子组 controller 文件需父组 subtree_control 启用。Preflight 现在
   幂等启用 memory/pids/cpu。
3. **rlimits 误伤 stage2**：setrlimit 作用于调用进程，RLIMIT_AS 掐死
   Go runtime 自身的 arena 预留 → 随机 fatal。移到 execve 前一刻。
4. **测试二进制 stage2 递归**：go test 二进制的 main 不认 OJ_STAGE2，
   沙箱子进程把整个测试套件重跑一遍。TestMain 钩子修复。
5. **并发预算超卖**：2 任务槽 × 2 case worker ≈ 1.3GB 并发沙箱内存，
   3.9GB 主机被 OOM-killer 制造幻影 RE/SE（连带 PostgreSQL/nginx 被杀）。
   CaseWorkers 默认 1，文档写明「任务×case 并发 ≤ 主机内存预算」的上调
   条件。

修复后：沙箱测试套件（17 项，含逃逸/解释器/耗时）全绿；50-case 提交
12 连跑 ×2 全 AC 零幻影；全量 9 套 E2E 127 项全绿。

### 17.3 首败即停 E2E（stop-on-fail-e2e.mjs，7 项）

30-case 题在比赛内提交 WA 解：verdict WA、首败点=1、实判 1 个、
SKIPPED 29 个、耗时段内（<20s）。补题/训练提交不受影响（跑满全量）。

### 17.4 基准数字（诚实呈现）

基准负载：50 测试点、单 case ≈350ms 实跑（1s 时限）、cpp/python3、
AC/WA/TLE 三形态，中位数取 2–3 跑：

| 场景 | 修复前基线 | 修复后 | 说明 |
|---|---|---|---|
| cpp-AC | 1132ms | 1255–1949ms | 并行关闭后与基线同量级；后台干扰下偶有抖动 |
| py-AC | 7857ms | 7916–8325ms | 同上 |
| cpp-TLE | 65459ms | 65434–66264ms | TLE 语义不变 |

**结论**：本轮主收益不是单机延迟倍数，而是 (a) 找回并修复了从未生效的
cgroup 资源防线（安全核心）；(b) 首败即停让比赛中错解的判题成本从
O(全部 case) 降到 O(首个失败 case)；(c) 零分配比对消除大输出场景的
分配器压力；(d) 并行基础设施就绪——RAM 扩容后把 CaseWorkers 调到
内存预算允许的值即可获得近线性加速，无需再改代码。

**门禁结论：通过**。

## 十八、安全审计 + 实际判题复测（2026-09-01，云端）

判题效率优化轮（第十七节）全部改动落地后的独立复核。

### 18.1 Mimosa deep 扫描

- scanId：`scan-2026-09-01T12-47-59.565Z-7d9bdfb5fa1b`
- 封印：`sha256:7eed7d21558b43d01ea0380d9cba0edfe97ed0c92c44e89c1ef012e91c348f58`
- 产物：`~/.mimosa/security-scans/project-682dc23f858064c09ee15e7e/scan-…-7d9bdfb5fa1b`
- 依赖风险：286 包，离线 advisory 匹配 **0**
- Findings：8 项，全部为既有研判的「运维参数 → 配置文件路径」静态污点
  误报（`OJ_*` 环境变量/命令行参数进入配置文件读取——判题机环境变量由
  root 管控，非不可信输入），**0 业务逻辑候选**；与此前基线同源，无新增
  代码面引入的问题。

### 18.2 实际判题全链路（生产 API → 队列 → 判题机 → 落库 → WS）

| 验证 | 结果 |
|---|---|
| 语言档位（cpp/python3/java 三语言 AC + 档位参数 + Python 4x 余量） | 13/13 |
| SPJ（checker AC/WA）+ 交互（interactor AC/WA） | 8/8 |
| 比赛首败即停（30 case 题 WA@1 → 实判 1 跳过 29） | 7/7 |
| 50-case 题 12 连跑稳定性 | 12/12 AC，零幻影 RE/SE |
| 服务健康（四服务 active、api 200） | PASS |

**结论：安全审计与实际判题双双通过。**

## 十九、M5：插件系统 + IOI 赛制 + 外部刷题同步 E2E（2026-09-02，云端）

四项新功能全链路（脚本 `remote-test/m5-plugins-ioi-external-e2e.mjs`，
**42/42 全绿**）。新 api/judge 二进制 + 前端 dist 均已部署生产
（`m5-deploy3.mjs` / `m5-frontend-sync.mjs`，base64+heredoc+sha256 校验通道）。

| 验证组 | 结果 |
|---|---|
| 插件清单（webhook 钩子 + cf/luogu/atcoder/nowcoder 适配器 + cf 爬虫） | 7/7 |
| 外部题面导入（CF 1A 预导入、来源标注、未知插件拒绝） | 4/4 |
| IOI 分值表（不存在测试点拒绝 / 设置 / 总分校验 / 读回） | 4/4 |
| IOI 全链路（建 ioi 赛 → 挂题 → 部分分解提交 → score=30 + verdict=WA → 榜单总分/格子分） | 8/8 |
| 共享导出（statement 在包内、checker.cpp 不出站） | 2/2 |
| 外部刷题（CF 绑定同步零错误、报表平台统计 + 365 天活动桶、profile platforms 桶、解绑） | 7/7 |
| webhook 管理（回环 SSRF 拒绝、非 http(s) scheme 拒绝） | 2/2 |

### 19.1 过程中修复的缺陷

1. **per-case Score 未随 CaseRef 传递**：`judgeOneCase`/`judgeOne` 构造
   `CaseResult` 时丢掉了 `cr.Score`，提交得分恒 0——单测没覆盖到
   "CaseRef→CaseResult" 的字段搬运。修复：两处构造补 `Score: cr.Score`。
2. **`GET /problems/:id/case-scores` 路由漏注册**（只有 PUT）。
3. **proto 重生成落点**：`--go_out=.` 需配 `module=github.com/ysnb/oj`
   才会落 `pb/`；此前误落 `proto/` 导致新旧两份并存、编译报 Score 字段缺失。

**结论：M5 四项功能（插件系统 / 题库爬取与共享导出 / IOI 部分分赛制 /
外部刷题统计报表）全部通过生产 E2E，正式并入交付面。**
