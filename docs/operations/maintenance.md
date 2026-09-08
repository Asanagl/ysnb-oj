# 运维 Runbook（维护手册）

> 读者：**值班运维 / 接手人**。本文按「我要做什么」组织，每个任务给出
> 前置条件、步骤、验证方法。首次部署查 [deploy.md](deploy.md)；用户侧运营
> 问题查 [admin-guide.md](../guide/admin-guide.md)；判题沙箱深水区查 [judge-sandbox.md](../development/judge-sandbox.md)。
>
> 两条红线：
>
> 1. **禁止在服务器上跑 npm / vite / go 构建**——3.9GB 内存 + 宿主机
>    balloon 抽内存，已发生两次 OOM 事故。所有构建在开发机完成，只推产物。
> 2. **凭据不入文档**。本文不出现任何真实口令/密钥；取值见 .zcode 本地
>    记录与 `remote-test/.sshenv` / `.ojenv` / `admin.env`（均已 gitignore）。

## 一、服务器速查

| 项 | 值 |
|---|---|
| 主机 | `<your-server-ip>`，Debian 12 (bookworm) 单机，内核 6.1，3.9GB 内存 |
| 服务 | Docker Compose：postgres / redis / api / web 四容器（`restart: unless-stopped`，随宿主机自启）；api 仅回环发布 127.0.0.1:8080/9090，web 宿主网络 :80。宿主机：`oj-judge`（root，gRPC 连 api）+ 可选 `oj-audit` / `oj-audit-watcher`（BPF LSM 审计层，只记不拦，见 `deploy/lsm-audit/`） |
| 防火墙 | ufw：default deny incoming，仅放行 22/80/443 |
| 程序 | `/opt/oj/oj-judge`；api 在 compose 镜像内，维护 CLI 用 `docker compose exec api oj-cli ...` |
| 配置 | `/root/ysnb-oj/.env`（compose 全部密钥）、`/opt/oj/oj-judge.env`（权限 600；改后 restart 对应服务才生效） |
| 前端 | compose web 容器（nginx:1.27-alpine）挂载仓库 `frontend/dist`，发布 = 同步 dist 后 `docker compose restart web` |
| 数据 | docker 卷 pgdata（数据库）/ redisdata / ojdata（testdata 等，容器内 /oj-data）；判题工作区 `/oj-work` |
| 备份 | `/opt/oj/backup/`（db/ + data/snapshot/ + backup.log + drill.log；backup.sh 自动探测 postgres 容器） |
| 定时任务 | root crontab：每天 03:00 备份；每月 1 号 04:00 恢复演练 |
| 单元文件 | `/etc/systemd/system/oj-judge.service` + `oj-judge.service.d/cgroup-cpu.conf`（Debian 12 必装，见下）；可选 `oj-audit*.service`（源在仓库 `deploy/systemd/` 与 `deploy/lsm-audit/`） |

> Debian 12 专用：`oj-judge.service.d/cgroup-cpu.conf` 在 judge 启动前把
> memory/pids/cpu 控制器启用到根组与判题基组（systemd 252 开机惰性启用根组
> 控制器，缺它则早启动的判题机全部提交 SE）。Debian 11 幂等无害。

### SSH 加固基线（2026-09-04 已生效）

- root 仅 ed25519 密钥登录：`PasswordAuthentication no` +
  `PermitRootLogin prohibit-password` + `KbdInteractiveAuthentication no`，
  落点为 drop-in `/etc/ssh/sshd_config.d/00-oj-hardening.conf`
  （drop-in 首匹配优先于主配置）。
- fail2ban 未安装：密钥化后密码爆破面已消除。如需加装：
  `apt install fail2ban` 并启用 sshd jail 即可。
- 变更红线顺序：先装新公钥并**用新终端实测密钥登录成功**，才允许动 sshd
  配置；改完 `sshd -t` → `systemctl reload ssh`（不断开当前会话）→
  新开连接双向验证（密钥通 / 密码拒）。
- 回滚：删除上述 drop-in（或恢复 `/etc/ssh/sshd_config.bak-*` 备份）后
  `systemctl reload ssh`。
- 审计记录见 [security-audit.md](../archive/security-audit.md) §13.4（存档只读）。

## 二、我要发布一次更新

现行发布通道**全部走密钥版 remote-test 脚本，在开发机执行**，服务器上不做
任何构建。本节未列出的 remote-test 旧 push/deploy 脚本均为密码 SSH 时代的
遗留，勿用。脚本默认读取环境变量 `OJ_SSH_HOST` / `OJ_SSH_KEY`（主机与私钥
路径），未设置时用脚本头部的内置默认值。

### 发布前端

前置（在开发机执行）：`frontend/` 下构建通过。

```bash
cd frontend && npm run build && cd ..
node remote-test/m5-frontend-sync-key.mjs
```

脚本行为：本地 `frontend/dist` 打 tar.gz → 密钥 SSH 通道 base64 分块传输 →
服务器端 sha256 校验（不一致报 `SHA_MISMATCH` 并中止）→ 原子替换
`/opt/oj/web`（旧目录自动挪为 `/opt/oj/web.old`）。nginx 是纯静态根，
无需 reload。

验证：脚本末尾打印 `index HTTP=200`；浏览器 Ctrl+F5 强刷确认新版本
（index.html 带 no-cache，正常刷新即可拿到新入口）。

回滚（在服务器执行）：

```bash
rm -rf /opt/oj/web && mv /opt/oj/web.old /opt/oj/web
```

### 发布后端（api 镜像 / oj-judge 二进制）

api 走 compose 镜像：改完 `backend/` 源码后同步到服务器仓库，然后：

```bash
cd /root/ysnb-oj && docker compose build api && docker compose up -d api
```

（镜像内构建 Go，3.9GB 机器约 5-15 分钟；`restart: unless-stopped` 保证
其间宿主机重启也能自愈。）

oj-judge 是宿主机单二进制，在开发机交叉编译出 Linux 产物：

```bash
cd backend
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ../dist/oj-judge-linux ./cmd/judge
cd ..
node remote-test/oj-push-one-binary.mjs judge
```

脚本行为：gzip+base64 经密钥 SSH 推送 → 服务器端 sha256 校验 → 落位
`/opt/oj/oj-judge.new`。**脚本不替换、不重启**，输出 `PLACED` 只代表传输
与校验完成。判题机升级前先停服务（systemd 持有旧文件，直接覆盖报
Text file busy）：

```bash
systemctl stop oj-judge
mv -f /opt/oj/oj-judge.new /opt/oj/oj-judge
systemctl start oj-judge
```

验证（在服务器执行）：

```bash
systemctl is-active oj-judge                                              # active
docker compose ps                                                         # 四容器 running
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8080/api/v1/languages   # 200
journalctl -u oj-judge -n 20 --no-pager                                   # 判题机已重连
```

最后到管理后台提交一道 A+B，确认判题链路端到端通。也可用探活脚本
`node remote-test/m5-health.mjs`（在开发机执行）自动跑上面的检查。

### 变体（均在开发机执行）

- 推小文件（env、脚本、配置样例等）：
  `node remote-test/push-small.mjs <本地路径> <服务器绝对路径>`。
- 前端发布：`node remote-test/m5-frontend-sync-key.mjs`（同步 dist 后在
  服务器 `docker compose restart web`）。
- nginx 配置：web 容器挂载仓库 `deploy/nginx.conf`，改完同步到服务器仓库
  后 `docker compose restart web`（宿主机无 nginx）。

以上脚本全部走 SSH 密钥通道（默认 `~/.ssh/id_ed25519`，可用 `OJ_SSH_KEY` /
`OJ_SSH_HOST` 覆盖），2026-09-04 起服务器已关闭密码登录。

## 三、我要排查一个判题 SE

前置：拿到出问题的提交（submission id 可从提交记录 URL 或后台查）。

1. **看日志定位**。免 SSH：管理后台 →「日志查看器」（admin 及以上），unit
   选 `oj-judge`，关键字填 submission id 或 `SE`。命令行（在服务器执行）：

   ```bash
   journalctl -u oj-judge -p err -S -1h --no-pager    # 判题机最近 1 小时错误
   docker compose logs api --since 24h 2>&1 | grep "submission judged"
   ```

2. **找保留现场**。CE/SE/RE 的判题工作区会被自动保留（判题侧的 crash
   dump），日志里有 `workspace kept at ...` 一行：

   ```bash
   ls -dt /oj-work/judge-* | head    # 按时间倒序找对应现场
   ```

   工作区内有用户源码（`main.*`）等现场文件，可直接复盘。

3. **需要逐 case 明细时开 debug**：在 `/opt/oj/oj-judge.env` 加
   `OJ_LOG_LEVEL=debug` → `systemctl restart oj-judge` → 重新提交复现。
   排查完改回 `info` 再 restart（debug 输出逐 case 耗时/内存/编译 stderr，
   量大，别长期开）。

4. 若某语言**集体** SE/RE 且 stderr 为空 → 大概率 seccomp 误杀：
   `journalctl -u oj-judge | grep SIGSYS`，按
   [judge-sandbox.md](../development/judge-sandbox.md) 的 strace 流程定位
   缺的系统调用。

验证：修复后同一代码重新判过，日志出现对应 `submission judged` 且 status
正常；排查完的 `/oj-work/judge-*` 现场手动删除（成功的会自动删）。

## 四、我要巡检（5 分钟清单）

在服务器执行：

```bash
docker compose ps                               # 四容器全部 running
systemctl is-active docker oj-judge             # 全 active（装了审计层再加 oj-audit oj-audit-watcher）
tail -5 /opt/oj/backup/backup.log        # 今天 03:00 备份成功、dump 体积正常
df -h /                                  # 磁盘 < 80%
free -m                                  # 3.9GB 小内存机，看 available
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1/api/v1/languages   # 200
docker compose logs api --since 24h 2>&1 | grep "submission judged" | tail -5
journalctl -u oj-judge -p err -S today --no-pager | tail -20
```

网页侧：管理后台「判题机监控」judge-1 online、队列归零；每月 1 号之后
`tail /opt/oj/backup/drill.log` 末尾应是 `drill PASSED`。装了审计层的
再看一眼 `systemctl is-active oj-audit oj-audit-watcher` 与
`cat /sys/kernel/tracing/trace_pipe | grep -c oj-audit`（有判题活动时应
持续增长）。

通过标准：以上全部正常。任一项异常 → 按 §九 事故表处置。

## 五、服务管理与日志速查

```bash
docker compose restart api    # 改 .env 后必须
systemctl restart oj-judge    # 改 oj-judge.env / 装编译器后
```

oj-judge 经 gRPC（127.0.0.1:9090）连 compose 里的 api：重启 api 会让判题
机短暂断连（WARN `daemon disconnected`）并自动重连，在判任务由 15 分钟
租约回收重排。两个都要动时**先 api 后 oj-judge**（同时 restart 也能自愈，
只是多一轮重连）。

- **全站 502**：web 容器后面的 api 死了 →
  `docker compose logs api --n 100`（高频原因：.env 改坏、JWT secret
  缺失导致启动失败）。
- **`/api/v1/ws` 返回 503**：WebSocket 连接数到上限（1024）→
  `docker compose restart api` 清空连接；频繁出现再查连接泄漏。

### 日志速查

- 两个组件均为 JSON 结构化日志（logx/slog）→ stderr：oj-judge 进 stderr
  进 journald（journalctl 可查），compose 里的 api 进容器日志
  （`docker compose logs api`），级别默认 `info`。
- 免 SSH：管理后台 →「日志查看器」读 oj-judge 的 journald 日志（systemd
  路线下还可读 oj-api），按 unit/关键字在服务端过滤；compose 路线的 api
  日志在服务器上 `docker compose logs api` 查看。
- 命令行（在服务器执行）：

  ```bash
  docker compose logs api -f             # API 实时日志
  journalctl -u oj-judge -f               # 判题机实时日志
  journalctl -u oj-judge -p err -S -1h    # 判题机最近 1 小时错误
  ```

- 临时 debug：api 的 `OJ_LOG_LEVEL=debug` 写进 `.env` 后
  `docker compose up -d api`；judge 的写进 `/opt/oj/oj-judge.env` →
  restart oj-judge → 排查完改回。
- 磁盘：journald 上限 `SystemMaxUse=200M`
  （`/etc/systemd/journald.conf.d/99-oj.conf`）；手动收缩
  `journalctl --vacuum-size=100M`。
- 排错锚点：每个提交判完一条 INFO `submission judged`（含 submission/
  status/time_ms/memory_kb/score）；SE 另有 ERROR `submission judged SE`；
  判题机断连有 WARN `daemon disconnected`。

## 六、我要清理磁盘

前置：`df -h /` 确认紧张；先
`du -xh --max-depth=1 / 2>/dev/null | sort -h | tail` 定位大头再动手。

| 对象 | 位置 | 处置 |
|---|---|---|
| 判题保留工作区 | `/oj-work/judge-*` | CE/SE/RE 现场；确认无在排查问题后清超 7 天的：`find /oj-work -maxdepth 1 -name 'judge-*' -mtime +7 -exec rm -rf {} +` |
| blob 缓存 | `/opt/oj/data/judge-cache` | 可安全清空（按需重拉）；**别删** `/opt/oj/data/testdata` 本体 |
| 备份 | `/opt/oj/backup` | `/opt/oj/backup.sh` 自动保留 14 天日备 + 6 个月月备；仍膨胀先查脚本头部 `KEEP_DAYS/KEEP_MONTHS` 是否被改大，再手工删超期 dump |
| journald | `/var/log/journal` | 200M 上限自动生效；手动 `journalctl --vacuum-size=100M` |

验证：`df -h /` 回落到 80% 以下；`docker compose ps` 四容器与
`systemctl is-active oj-judge` 仍全部 running/active。

## 七、我要恢复一次备份

### 只验证备份可用（无风险，随时可做，在服务器执行）

```bash
/opt/oj/restore-drill.sh "$(ls -t /opt/oj/backup/db/*.dump | head -1)"
```

起一个一次性环境把 dump 恢复进去并打印各表行数，不触碰生产（每月 1 号
04:00 cron 已自动跑，结果在 `drill.log`）。

### 灾难恢复（真实回滚生产，在服务器执行）

前置：确认要回到的备份日期；通知用户有停服窗口。

```bash
# 1. 停服务，避免恢复期间产生新写入
docker compose stop api
systemctl stop oj-judge
# 2. 恢复数据库（compose 里的 postgres 容器）
docker compose exec postgres psql -U oj -c 'drop database if exists oj'
docker compose exec postgres psql -U oj -c 'create database oj'
cat /opt/oj/backup/db/oj-<日期>.dump | docker compose exec -T postgres pg_restore -U oj -d oj
# 3. 恢复数据目录（testdata 等）
rsync -a /opt/oj/backup/data/snapshot/ /opt/oj/data/
# 4. 起服务
docker compose start api
systemctl start oj-judge
```

验证：`curl -s http://127.0.0.1:8080/api/v1/languages` 返回 200；admin
登录后台抽查题目与提交记录；提交一道 A+B 确认判题链路。判题机的
judge-data 是缓存，无需恢复，会自动重拉。

## 八、我要轮换密钥 / 凭据

凭据取值一律见 .zcode 本地记录与 `remote-test/.sshenv|.ojenv|admin.env`
（已 gitignore）。随机值生成：`openssl rand -base64 32`。

### SSH 公钥轮换（在服务器执行，遵守 §一 红线顺序）

1. 新公钥**追加**到 `/root/.ssh/authorized_keys`（旧钥先别删）；
2. **新开终端**实测新密钥能登录（当前会话保持不断开）；
3. 确认后删旧公钥行即可，无需动 sshd 配置；
4. 若确实改了 sshd 配置：`sshd -t` → `systemctl reload ssh` → 新连接双向
   验证。回滚：删 `/etc/ssh/sshd_config.d/00-oj-hardening.conf` → reload。

### admin 应用密码（在服务器执行）

```bash
cd /root/ysnb-oj
T=$(grep '^OJ_CLI_TOKEN=' .env | head -1 | cut -d= -f2-)
docker compose exec api oj-cli create-superadmin --username <admin账号> --password '<新密码>' --token "$T"
```

该命令对已有用户是「提升+重置+解封」语义，超管锁死自救也用它，详见 [deploy.md](deploy.md) 第八节。

### 数据库 oj 角色密码（在服务器执行）

```bash
docker compose exec postgres psql -U oj -c "ALTER ROLE oj PASSWORD '<新密码>'"
# 同步改 .env 的 OJ_PG_PASSWORD，然后
cd /root/ysnb-oj && docker compose up -d api   # 重建 api 容器读新密码
```

### JWT / 判题机密钥（在服务器执行）

`/root/ysnb-oj/.env` 的 `OJ_JWT_SECRET` 与 `OJ_DAEMON_SECRET`；
`/opt/oj/oj-judge.env` 的 `OJ_DAEMON_TOKEN` 必须与 `OJ_DAEMON_SECRET`
**同值**。改 .env 后 `docker compose up -d api` 重建，两处一起改后
`systemctl restart oj-judge`。注意：换 JWT 后所有用户登录态立即失效；
daemon 两侧不同值 → 判题机直接 offline。

验证：`/api/v1/languages` 200、后台判题机监控 online、admin 用新密码
重新登录成功。

## 九、常见事故处置

| 现象 | 处置 |
|---|---|
| 全部提交 PENDING | 后台「判题机监控」看 judge-1 是否 offline → `journalctl -u oj-judge -n 50`；`OJ_DAEMON_TOKEN` 与 `OJ_DAEMON_SECRET` 不同值是高频原因 |
| 某语言全 CE | 判题机缺编译器：`apt install openjdk-17-jdk-headless` 等，装完 `systemctl restart oj-judge` |
| 某语言全 SE/RE 且空 stderr | 沙箱 seccomp 误杀：`journalctl -u oj-judge \| grep SIGSYS`，按 [judge-sandbox.md](../development/judge-sandbox.md) 用 strace 定位 |
| 提交一直 JUDGING | 15 分钟租约到期会被自动回收重排；或 `systemctl restart oj-judge` 触发重连回收 |
| 全站 502 | compose 里的 api 死了：`docker compose logs api --n 100`（常见：.env 改坏、JWT secret 缺失启动失败） |
| WS 503 | 连接数到上限 1024：`docker compose restart api`（见 §五） |
| 数据库连不上 | postgres 容器挂或密码与 DSN 不一致：`docker compose exec postgres psql -U oj -d oj -c 'select 1'` |
| 重启后四容器没起来 | 确认 `docker compose ps` 与 `restart: unless-stopped`（PR #12 后已内置）；`docker compose up -d` 手动拉起 |
| 重启后判题全 SE | Debian 12 根组控制器惰性启用被 jump 过：确认 `oj-judge.service.d/cgroup-cpu.conf` 在位（见 §一），`systemctl restart oj-judge` |
| 超管密码丢失/锁死 | oj-cli 自救，见 §八 |
| 磁盘爆 | 按 §六 顺序清；**不要**删 testdata 本体（ojdata 卷内） |
| 服务器 OOM / 莫名变卡 | 3.9GB 单机：确认没人在服务器上跑 npm/go 构建（两次事故根因）；`OJ_MAX_PARALLEL` 保持 1 |
| 疑似被入侵 | `last -f /var/log/wtmp`、`ss -tnp`、查 web 容器访问日志异常 IP；按 §八 全量轮换 |

## 十、定期维护任务

| 周期 | 任务 |
|---|---|
| 每天（自动） | 03:00 备份（cron）；巡检时扫一眼 `backup.log` |
| 每周 | §四 巡检 + `apt update && apt upgrade`；重启机器后按 §四 巡检一遍（compose 与 oj-judge 均已配自启） |
| 每月 | 确认 `drill.log` 末尾 `drill PASSED`；抽一份备份走一次恢复演练全流程 |
| 每季度 | 评估磁盘增长（testdata/备份）；评估是否加第二台判题机（见 judge-sandbox.md） |
| 人员变动 | 按 §八 全量轮换 SSH 公钥、admin、DB、JWT/daemon 密钥，并更新交接文档 |

## 十一、配置变更索引

| 想改什么 | 改哪里 | 生效方式 |
|---|---|---|
| 并行判题数 | `/opt/oj/oj-judge.env` 的 `OJ_MAX_PARALLEL`（生产=1，3.9GB 内存慎调大） | restart oj-judge |
| 日志级别 | api：`.env` 加 `OJ_LOG_LEVEL` 后 `docker compose up -d api`；judge：`oj-judge.env` 的 `OJ_LOG_LEVEL` | 重建 api 容器 / restart oj-judge |
| 会话时长 | `.env` 的 `OJ_JWT_EXPIRE_HOURS` | `docker compose up -d api` |
| journald 磁盘上限 | `/etc/systemd/journald.conf.d/99-oj.conf` 的 `SystemMaxUse` | restart systemd-journald |
| 备份保留期 | `/opt/oj/backup.sh` 头部 `KEEP_DAYS/KEEP_MONTHS` | 下次备份生效 |
| nginx | 源文件 `deploy/nginx.conf`（web 容器挂载），同步到服务器仓库后 | `docker compose restart web` |
| SSH 策略 | `/etc/ssh/sshd_config.d/00-oj-hardening.conf` | `sshd -t && systemctl reload ssh` |
| 时限倍率/内存放宽/加语言 | `backend/pkg/judge/languages.yaml` + 服务器装对应工具链 | 重发 oj-judge 二进制（§二） |
| 限流阈值 | `backend/internal/handler/middleware.go`（login/register）、`submissions.go`（submit） | api 镜像重建（§二） |
| 审计钩子集/白名单逻辑 | `deploy/lsm-audit/oj_audit.bpf.c` | 目标机重编译 loader（见该目录 README） |
