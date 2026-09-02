# 接手文档（Handover）

> 给下一任维护者。读完这篇 + `architecture.md`，你应该能在 30 分钟内
> 定位任何一块代码、发布一次更新、处置一次故障。

## 一、这个项目是什么

沈阳化工大学 ACM 队**完全自研**的在线评测系统（明确排除了复用 go-judge /
二开 Hydro/DOMjudge 的路线，沙箱自己写——这是项目的核心约束）。能力：
题库（含 SPJ/交互题）、训练题单、训练小组、ACM 赛制比赛（封榜/滚榜/组队赛）、
用户出题审核流、题解区（提交后解锁）、题目包导入（自有/DOMjudge/Hydro）。
生产部署于云服务器 **<your-server-ip>**，单机（API + 判题机同机）。

当前状态：V1 已收尾验收（2026-08-30，`e2e-report.md` 第十六节），数据已
复位为示例内容，等待第一批真实用户导入。下一个规划模块：**讨论区**。

## 二、账号与凭据（只指路，不写明文）

| 项 | 位置 |
|---|---|
| root SSH 密码 | 团队负责人掌握（未轮换历史值的遗留项，建议尽快改） |
| admin 应用账号 | `remote-test/.ojenv`（本地）；服务器 `/opt/oj/oj.env` 的 `OJ_ADMIN_PASSWORD` |
| PostgreSQL oj 密码 | 同上文件 `OJ_DB_DSN` 内；服务器侧另有 `/root/credentials-new.txt`（0600） |
| JWT / daemon 密钥 | 同上两个文件；判题机侧在 `/opt/oj/oj-judge.env` 的 `OJ_DAEMON_TOKEN` |
| SSH 自动化凭据 | `remote-test/.sshenv`（本地，供 ssh.js） |
| 上轮轮换记录 | `remote-test/.credentials-2026-08-30.txt`（本地，用后删） |

> 接手后第一件事：确认这些文件都在、都改了密码；人员变动必须全量轮换
> （改 oj.env → systemctl restart，见 `maintenance.md` 第七节）。

## 三、代码库导读

```
backend/            Go 1.27 后端（cmd/api、cmd/judge 两个入口）
  proto/oj.proto      判题 gRPC 协议（JudgeRelay.Connect 双向流）
  internal/handler/   全部 REST API（按 auth/problem/contest/admin 分文件）
  internal/judgehub/  判题调度：队列消费、租约、断线重排
  internal/daemon/    判题机客户端
  pkg/sandbox/        自研沙箱（仅 Linux；seccomp_sim_test.go 是护栏测试）
  pkg/judge/          判题编排 + languages.yaml（加语言只改这里）
frontend/           Vue3 + TS + Vite + Element Plus + TipTap
  src/views/          页面（ProblemEditor 全页出题器等）
  src/api/client.ts   全部后端接口的 TS 封装
deploy/             backup.sh / restore-drill.sh / systemd 单元 / nginx.conf
docs/               文档全集（见 docs/README.md 地图）
remote-test/        部署与 E2E 工具链（见下）+ 本地凭据 env（勿入库）
```

## 四、日常开发流程

- **后端不在开发机本地运行**（Linux-only：沙箱依赖 cgroup v2 + namespaces，
  Windows/macOS 不支持）。前端本地只跑工具链：`frontend` 里
  `OJ_DEV_API_TARGET=http://<生产地址> npm run dev` 把 /api（含 WS）代理到
  云服务器生产 API（地址见 `remote-test/.ojenv` 的 BASE，勿写死入库）；
  后端改动在 Linux 侧构建验证（或交叉编译后部署）。
- 构建/测试门禁：`go build ./... && go vet ./...`（Linux 上）；前端
  `npx vue-tsc --noEmit` + `npm run build`（本地）。
- E2E：`remote-test/*.mjs` 直打生产 API（凭据 .ojenv），每模块一套，
  结果记入 `docs/e2e-report.md`。注意登录限流 10/min/IP，多套脚本要间隔 70s。
- **改判题核心（尤其 pkg/sandbox）必须**：跑 `seccomp_sim_test.go`（交叉编译
  推判题机执行，命令见 `judge-sandbox.md` §2.5），再走生产 E2E（三套 m4-*）。

## 五、发布一次更新（实操）

标准流程已在 `maintenance.md` 第四节写成命令级清单。要点复述：
交叉编译两个 Linux 二进制 → `gen-push.js` 生成推送脚本 → **先停服再推**
（Text file busy）→ 前端 zip 用 `fix-web3.sh` 解压（PowerShell 反斜杠坑）→
`prod-restart.sh` 自带三重验证 → 后台提交一道题确认判题链路。

**全新服务器从零部署**：`deploy/one-click.sh`（Docker Compose 路线）——
前置条件、执行方式、自动生成的密钥说明见 `docs/deploy.md` §〇。

**锁死自救**：超管密码丢失/账号锁死时，用容器内置维护 CLI
（`docs/deploy.md` §〇-2）：`docker compose exec api oj-cli --token <OJ_CLI_TOKEN>
create-superadmin …`。生产服务器的 CLI 令牌在 `/opt/oj/oj.env` 的
`OJ_CLI_TOKEN`（2026-09-02 生成，见凭据清单）。

## 六、已知坑与历史教训（前人踩过，别再踩）

1. **seccomp cBPF uint8 跳转回绕**：白名单 >127 条时偏移回绕，Java 全线
   空 stderr RE。已改分块线性链 + 解释器测试。加系统调用时必须带测试。
2. **JDK 需要的 socket 系统调用**：javac/java 启动即建 AF_UNIX socket；
   白名单已放行（网络 ns 内无网卡，不构成出网）。别"顺手"删掉。
3. **判题机缺 JDK**：Java 全 CE 时先查 `which javac`。
4. **比赛题是独立副本**：赛题改动不影响原题；提交打原题 id 必 404。
5. **建赛默认 require_registration=true**：不报名不能交。
6. **PowerShell 压 zip 的反斜杠**：前端发布解压必须用 fix-web3.sh。
7. **登录限流**：E2E 连跑会 429，间隔 70 秒；限流本身是特性别"修"掉。
8. **覆盖二进制前先停服**：systemd 持有文件句柄。
9. **DOMjudge validator 参数约定**与本 OJ checker 不同，导入后必须人工核对。
10. **文档与实现同步**：改行为必须改对应文档（本目录），否则接手人会被误导。
11. **ssh2 通道推大文件**：exec 通道裸写 >1MB 会卡死在 channel window，
    必须 gzip+base64 经 `bash -s` stdin（现成脚本 `m5-deploy3.mjs` /
    `m5-frontend-sync.mjs`，带 sha256 双端校验，免停服原子替换）。
12. **proto 重生成**：`protoc --go_out=. --go_opt=module=github.com/ysnb/oj`
    才会正确落 `pb/`；漏 module 会生成到 `proto/` 下产生新旧两份并存。

## 七、文档地图

见 `docs/README.md`——每份文档管什么、什么时候查它。

## 八、待办与路线

- [ ] **讨论区**（下一模块，V1 预留了 handler 分组模式照抄即可）
- [x] 插件系统 / 题库爬取与共享导出 / IOI 赛制 / 刷题统计报表（2026-09-02 M5 落地，见 e2e-report §19、api.md「站内集成接口」）
- [ ] 训练计划（编排型）（V1 明确不做、已留扩展点）
- [ ] 多判题机负载均衡（gRPC 多 daemon 已天然支持，加机器即用）
- [ ] root SSH 密码轮换（上一任遗留，见第二节）
- [ ] 真实题库导入：用「导入题目包」吃 ICPC 工具链产出的 DOMjudge 题包
- [ ] 上线前压测复跑：`remote-test/steps/stress-full.sh`（基线见 e2e-report §16.1）
