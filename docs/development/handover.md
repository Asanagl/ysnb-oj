# 接手导读（Handover）

> 读者：**新接手的开发者**。目标：读完本文（约 30 分钟）能定位任何一块
> 代码、知道怎么开发、怎么过门禁、怎么发布。本文只做导读与指路——路由、
> 权限、API、判题细节一律链接到对应文档，不复制全文。
>
> 建议阅读顺序：本文 → [architecture.md](architecture.md)（架构总览）→
> 按需查 [judge-sandbox.md](judge-sandbox.md) / [api.md](../reference/api.md)。

## 一、这个项目是什么（60 秒）

YSNB OJ——沈阳化工大学 ACM 队**完全自研**的在线评测系统（You Submit,
Never Be rejected）。明确排除了复用 go-judge、二开 Hydro / DOMjudge 的
路线：判题沙箱是自己写的，这是项目的核心约束，也是核心资产。

- **后端**：Go 1.27，Gin + GORM（生产 PostgreSQL / 自测 SQLite）+ Redis
  队列；API 与判题机之间走 gRPC JudgeRelay 双向流。
- **前端**：Vue 3 + TypeScript + Vite + Tailwind CSS v4 + shadcn-vue
  （reka-ui），Pinia / ECharts / TipTap / CodeMirror 6。
- **沙箱**：cgroup v2 + namespaces + seccomp cBPF + stage2 re-exec，
  **仅 Linux**（这决定了后端从不在 Windows/macOS 开发机上跑）。
- **生产**：Debian 11 单机（3.9GB 内存），systemd + nginx + PostgreSQL +
  Redis，API 与判题机同机；SSH 仅密钥登录（密码登录已关闭）。

## 二、能力全景（当前真实功能面）

- **赛制**：ACM + IOI（case-scores 按测试点部分分）、组队赛、封榜/滚榜、
  补题模式、首败即停。
- **实时判题状态**：提交后页面原地跟进 排队中 → 评测中 → 终态（完整判定名），
  判题机逐测试点完成即流式上报、前端 Hydro 风格圆点条逐个点亮 + n/m 进度；
  `submission:<id>` WS 主题属主可见（或 admin）。入口组件
  `LiveVerdictCard` + `useLiveSubmission`，挂在题目页 / 比赛题页 / 提交
  详情页。
- **题目**：SPJ / 交互题、题单（训练计划）、小组（组队/公告/共享题单）、
  用户出题 + 审核流、题解区（提交后解锁）、题目包导入（自有 / DOMjudge /
  Hydro 三种格式）、外站题目导入（全员开放：普通用户走审核流并带
  「待补测试数据」标记，setter 及以上免审核）。
- **M5 插件系统**：三类扩展点 ProblemSource（题目爬取）/
  SubmitLogFetcher（刷题同步）/ EventHook（判题事件钩子），内置
  codeforces / luogu / atcoder / nowcoder 适配器 + webhook 钩子；
  支撑外部题导入与共享导出。
- **外部刷题同步**：小时级 ticker 拉取绑定用户的多平台提交记录，已整合
  进个人中心（绑定管理、统计徽章、跨平台最近 AC、热力图多平台合并、
  外站题目导入入口）。
- **角色**：super_admin 全站**唯一**（自动继任：授予即传位），新装首建
  管理员固定 uid 0；admin 及以下可多人。
- **公开 API**：`/public/*` + API Key 鉴权 + cph 桥接（Competitive
  Companion / VSCode cph 直接导入题目样例）。
- **/admin 独立后台**：用户、题目、比赛、审核、判题机、插件、API Key、
  **日志查看器**。
- **结构化日志**：`internal/logx`（slog JSON → stderr → journald），
  `OJ_LOG_LEVEL` 免重编译调级别；INFO 级每条提交输出一行
  `submission judged` 锚点；后台 `GET /admin/logs` + 前端日志查看器页
  可直接检索 journald。

## 三、凭据与环境指路（只指路，零明文）

铁律：**任何真实口令、密钥、主机地址都不进仓库与文档**。取值只看：

- `.zcode/` 本地记录（gitignored）；
- `remote-test/.sshenv`（SSH 密钥通道）、`remote-test/.ojenv`（生产
  API 地址与 admin 账号）、`remote-test/admin.env`——全部 gitignored，
  勿入库、勿外发。

服务器侧运行配置在 `/opt/oj/oj.env` 与 `/opt/oj/oj-judge.env`
（权限 600，改后 restart 对应服务才生效）。**人员变动 = 全量轮换**，
轮换流程见 [maintenance.md](../operations/maintenance.md) 第八节。

## 四、代码库导读

### 顶层布局

```
backend/              Go 后端（全部服务代码）
  cmd/api/            API 服务入口（HTTP :8080 / gRPC :9090）
  cmd/judge/          判题机 daemon 入口
  cmd/cli/            容器内维护 CLI（oj-cli，锁死自救用，见 deploy.md）
  proto/oj.proto      JudgeRelay 双向流协议定义；pb/ 是生成物，勿手改
  internal/           全部业务实现（下详）
  pkg/sandbox/        自研沙箱（Linux-only，项目核心资产）
  pkg/judge/          判题编排 + languages.yaml（加语言只改这个 yaml）
frontend/             Vue 3 前端（下详）
deploy/               one-click.sh、systemd 单元、nginx.conf、备份/恢复脚本
dist/                 本地交叉编译产物（oj-api-linux 等），推服务器的来源
docs/                 文档全集（VitePress 站，README.md 是地图）
remote-test/          部署与 E2E 工具链 + 本地凭据 env（gitignored）
scripts/              e2e-test.sh、ws-probe.mjs、zipmake.go 等杂项
```

### 后端 internal/ 逐包指路

```
internal/config/      OJ_* 环境变量装配（config.go，加配置先改这里）
internal/auth/        JWT 签发/校验、密码哈希、角色
internal/model/       全部 GORM 表结构（model.go 单文件，找字段先翻它）
internal/store/       数据库连接与自动迁移
internal/queue/       Redis 提交队列
internal/handler/     全部 REST 路由与处理函数（router.go 是路由总图）
internal/judgehub/    判题调度：hub.go 判题机连接管理；
                      tasks.go 队列消费、租约、断线重排、结果落库
internal/daemon/      判题机侧 gRPC 客户端
internal/plugin/      M5 插件系统：plugin.go 是注册表（init() Register，
                      与 languages.yaml 同一哲学），codeforces/luogu/
                      atcoder/nowcoder 四个适配器 + hooks.go webhook
internal/logx/        结构化日志层：slog JSON → stderr → journald，
                      OJ_LOG_LEVEL 控级别（logx.go 头部注释是约定全文）
internal/external/    外部刷题同步的持久化模型（为防 store→handler
                      import cycle 独立成包）
internal/public/      API Key 模型（同理）
internal/wsq/         主题制 WebSocket 集线器（提交状态/榜单实时推送）
internal/sysload/     负载采样（Linux-only，build tag 隔离）
```

### 后端关键文件（按"我要改什么"索引）

| 我要… | 看这里 |
|---|---|
| 加/改 REST 路由 | `handler/router.go`（/admin、/public 分组与鉴权中间件全在这） |
| 改日志查看器 | `handler/admin_logs.go`——头部注释是安全模型（子进程 argv 与请求完全隔离），`admin_logs_test.go` 钉住该性质，扩展前先读 |
| 改外部刷题同步 | `handler/external_sync.go`——time.Hour ticker，单平台失败只记日志不拖垮其他 |
| 改公开 API / cph | `handler/public_api.go`、`handler/public_cph.go`（cph 桥接只出样例，绝不出站完整测试数据）；细则见 [api.md](../reference/api.md) |
| 改插件 | `internal/plugin/plugin.go` + 对应适配器文件；管理端接口在 `handler/plugins_admin.go` |
| 改判题调度/日志锚点 | `internal/judgehub/tasks.go`——INFO `submission judged` 锚点行在这里 |
| 改实时判题进度上报 | `pkg/judge` 的 `Task.OnCase`（串行/停败/并行三路径触发）→ `internal/daemon` 的 `CaseProgress` 上报 → `internal/judgehub/hub.go` 的 `handleCaseProgress`（校验 inflight 归属后转 WS）；best-effort，不得阻塞判题 |
| 改 `submission:<id>` WS 权限 | `handler/router.go` 的 `wsTopicAuthorizer`——属主/admin 才可订阅，属主判定查库 |
| 改沙箱/判题 | 先读 [judge-sandbox.md](judge-sandbox.md) 再动手；`pkg/sandbox/seccomp_sim_test.go` 是 seccomp 解释器护栏测试 |
| 加评测语言 | 只改 `pkg/judge/languages.yaml` |

### 前端 src/ 布局

```
src/api/client.ts     全部后端接口的 TS 封装（唯一 HTTP 出口，加接口先加这里）
src/router/index.ts   路由 + 角色守卫（/admin 子路由含 logs 日志查看器页；
                      /external 已删除并重定向 /profile——刷题数据整合进个人中心）
src/stores/auth.ts    Pinia 会话状态
src/views/            页面：Admin* 九个后台页（含 AdminLogsView 日志查看器）、
                      ProblemEditor / MyProblemEditor 全页出题器；
                      外部平台绑定 / 刷题统计 / 外站题目导入已整合进
                      ProfileView（原 ExternalPracticeView 已删除）
src/layouts/          MainLayout（选手侧）/ AdminLayout（后台）
src/components/       CodeEditor（CodeMirror 6）、MarkdownEditor（TipTap）、
                      ScoreBoard、LiveVerdictCard（实时判定卡片：状态推进 +
                      测试点圆点条 + 完整判定名）、ui/（shadcn-vue 组件）
src/composables/      useChart（ECharts）、useTheme、useResponsive、
                      useLiveSubmission（提交状态 WS 跟进 + 断线轮询兜底）
```

## 五、日常开发流程

- **后端从不在开发机本地运行**：沙箱依赖 cgroup v2 + namespaces，仅
  Linux。后端改动在 Linux 侧构建验证，或交叉编译后部署验证。
- **前端本地开发**：`frontend/` 下
  `OJ_DEV_API_TARGET=<部署地址> npm run dev`，把 `/api`（含 WebSocket）
  代理到已部署的 Linux 环境；地址见 `remote-test/.ojenv`，勿写死入库。
  （`vite.config.ts` 里 `ws: true` 是必须的，判题结果推送走 WS。）
- **提交门禁**（全绿才准合入/发布）：
  1. 前端 `npx vue-tsc --noEmit` 零错误 + `npm run build` 通过；
  2. 后端（Linux 上）`go build ./... && go vet ./... && go test ./...`；
  3. 六套 remote-test E2E 直打生产 API，基线全过：

     | 脚本 | 基线 |
     |---|---|
     | `m4-import-export-e2e.mjs` | 22 PASS |
     | `m4-languages-e2e.mjs` | 13 PASS |
     | `m4-spj-interactive-e2e.mjs` | 8 PASS |
     | `m5-plugins-ioi-external-e2e.mjs` | 42 PASS |
     | `public-api-e2e.mjs` | 22 PASS |
     | `stop-on-fail-e2e.mjs` | 7 PASS |

     登录限流 10 次/分/IP，**多套脚本之间间隔 ≥70 秒**，否则 429。
- **改判题核心（尤其 pkg/sandbox、pkg/judge）必须**：
  1. 给逻辑补纯函数单测；
  2. 跑 seccomp 解释器测试 `seccomp_sim_test.go`（交叉编译推判题机
     执行，命令见 [judge-sandbox.md](judge-sandbox.md)）；
  3. 再走上面六套 E2E。

## 六、发布流程（现行密钥通道）

命令级清单在 [maintenance.md](../operations/maintenance.md) 第二节，
这里只给骨架。**红线：禁止在服务器上跑任何构建**（3.9GB 内存，已发生
OOM 事故），所有构建在开发机完成，只推产物。

- **前端**：`cd frontend && npm run build`，然后
  `node remote-test/m5-frontend-sync-key.mjs`（tar.gz → 密钥通道 →
  sha256 校验 → 原子替换 `/opt/oj/web`，旧版自动留 `web.old` 可回滚）。
- **后端**：交叉编译 `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build`
  出 `dist/oj-api-linux` / `oj-judge-linux` →
  `node remote-test/oj-deploy-binaries.mjs`（只推到服务器 `.new` 文件，
  **不替换不重启**）→ SSH 上机 `mv -f` 改名替换 +
  `systemctl restart oj-api oj-judge`（rename 对运行中进程安全，无需
  先停服）→ 后台提交一道 A+B 确认判题链路。
- **全新服务器冷部署**：`deploy/one-click.sh`（Docker Compose 路线），
  见 [deploy.md](../operations/deploy.md)。
- **锁死自救**：超管密码丢失用维护 CLI `oj-cli create-superadmin`，
  见 deploy.md 维护 CLI 一节。

## 七、已知坑（前人踩过，别再踩）

1. **seccomp cBPF 曾有的 uint8 跳转回绕**：白名单 >127 条时偏移回绕，
   Java 全线 RE。已修复为分块线性链，但教训永存——**加系统调用必须
   同时加 `seccomp_sim_test.go` 测试**。
2. **JDK 需要 socket 系统调用**：javac/java 启动即建 AF_UNIX socket，
   seccomp 白名单已放行（网络 namespace 内无网卡，不构成出网）。
   别"顺手"删掉。
3. **Java 全 CE 时先查判题机 `which javac`**——多半是镜像缺 JDK。
4. **比赛题是独立副本**：赛题改动不影响原题；提交打原题 id 必 404。
5. **建赛默认 require_registration=true**：不报名不能交，验收时别
   误判为 bug。
6. **登录限流是特性**：E2E 连跑触发 429 属正常，间隔 ≥70 秒，别把
   限流"修"掉。
7. **DOMjudge validator 参数约定与本 OJ checker 不同**，题目包导入后
   必须人工核对调用约定（见 [interactive.md](../guide/interactive.md)）。
8. **proto 重生成落点**：`protoc --go_out=. --go_opt=module=github.com/ysnb/oj`
   才会正确落 `pb/`；漏 module 会生成到 `proto/` 下，新旧两份并存、
   编译报字段缺失。
9. **大文件传输勿手写 ssh2 exec 裸写**（>1MB 卡死在 channel window）：
   一律用现成密钥脚本（gzip+base64+sha256 双端校验）。
   `remote-test/gen-push.js` 等密码 SSH 时代脚本均为遗留，**勿用**。
10. **Vue `<script setup>` 的 TDZ 白屏**：`watch(..., { immediate: true })`
    若写在它引用的 `const` 声明**之前**，注册时回调立即执行、读到
    TDZ 中的变量 → ReferenceError → 整页白屏且无构建期报错。教训：
    setup 里带 immediate 的 watch 必须放在所引用声明**之后**。
11. **文档与实现同步**：改行为必须同步改对应文档（docs/ 下），否则
    下一个接手人会被误导。

## 八、文档导航

[docs/README.md](../README.md) 是文档站首页兼文档地图——每份文档管什么、
什么时候查它都在那里。最常用的几份：

- 路由/权限/公开 API 细节 → [reference/api.md](../reference/api.md)
- 判题机与沙箱深水区（改判题前必读）→ [development/judge-sandbox.md](judge-sandbox.md)
- 发布/巡检/事故处置/密钥轮换 → [operations/maintenance.md](../operations/maintenance.md)
- 冷部署与环境变量全表 → [operations/deploy.md](../operations/deploy.md)
- 选手/管理员手册 → [guide/](../guide/user-guide.md)
- 历轮 E2E 与审计记录（只读存档）→ [archive/](../archive/e2e-report.md)

## 九、待办与路线

近期明确项：

- [ ] **TLS/HTTPS 开口**（前置与步骤见 deploy.md HTTPS 一节）
- [ ] user-guide / admin-guide **截图待补**（docs/screenshots/）
- [ ] **v1.0.0 待发布**（打 tag + release 收尾）
- [ ] **生产 E2E 测试数据待清理**（历轮直打生产留下的测试账号/题目/比赛）

中长期候选：

- [ ] 讨论区（handler 分组模式照抄现有模块即可）
- [ ] 训练计划（编排型）（已留扩展点）
- [ ] 多判题机负载均衡（gRPC 多 daemon 天然支持，加机器即用）
