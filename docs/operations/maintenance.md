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
| 主机 | `<your-server-ip>`，Debian 11 单机，3.9GB 内存 |
| 服务 | `oj-api`（oj 用户；HTTP :8080 / gRPC :9090，仅绑 127.0.0.1）、`oj-judge`（root）、nginx（:80/:443）、postgresql、redis-server |
| 防火墙 | ufw：default deny incoming，仅放行 22/80/443 |
| 程序 | `/opt/oj/oj-api`、`/opt/oj/oj-judge`、维护 CLI `/opt/oj/oj-cli` |
| 配置 | `/opt/oj/oj.env`、`/opt/oj/oj-judge.env`（权限 600；改后 restart 对应服务才生效） |
| 前端 | `/opt/oj/web`（nginx 静态根；每次发布自动保留上一份为 `/opt/oj/web.old`） |
| 数据 | `/opt/oj/data`（testdata 等原始资产）；判题工作区 `/oj-work`；blob 缓存 `/opt/oj/data/judge-cache` |
| 备份 | `/opt/oj/backup/`（db/ + data/snapshot/ + backup.log + drill.log） |
| 定时任务 | root crontab：每天 03:00 备份；每月 1 号 04:00 恢复演练 |
| 单元文件 | `/etc/systemd/system/oj-api.service`、`oj-judge.service`（源在仓库 `deploy/systemd/`） |

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

### 发布后端（oj-api / oj-judge 二进制）

前置（在开发机执行）：Go 工具链可用，交叉编译出 Linux 产物：

```bash
cd backend
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../dist/oj-api-linux ./cmd/api
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../dist/oj-judge-linux ./cmd/judge
cd ..
```

推送（在开发机执行）：

```bash
node remote-test/oj-deploy-binaries.mjs
```

脚本行为：gzip+base64 经密钥 SSH 推送 → 服务器端 sha256 校验 → 落位
`/opt/oj/oj-api.new` 与 `/opt/oj/oj-judge.new`。**脚本不替换、不重启**，
输出 `PUSH_ALL_OK` 只代表传输与校验完成。

替换并重启（在服务器执行）：

```bash
mv -f /opt/oj/oj-api.new /opt/oj/oj-api
mv -f /opt/oj/oj-judge.new /opt/oj/oj-judge
systemctl restart oj-api oj-judge
```

`mv` 改名替换对运行中进程安全（旧 inode 随旧进程退出释放），无需先停服。

验证（在服务器执行）：

```bash
systemctl is-active oj-api oj-judge                                       # 两个 active
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8080/api/v1/languages   # 200
journalctl -u oj-judge -n 20 --no-pager                                   # 判题机已重连
```

最后到管理后台提交一道 A+B，确认判题链路端到端通。也可用探活脚本
`node remote-test/m5-health.mjs`（在开发机执行）自动跑上面的检查。

### 变体（均在开发机执行）

- 只发一个二进制：`node remote-test/oj-push-one-binary.mjs <api|judge>`
  （同样只落到 `.new`，替换+重启仍在服务器手动做）。
- 推小文件（env、脚本、配置样例等）：
  `node remote-test/push-small.mjs <本地路径> <服务器绝对路径>`。
- nginx 配置：改仓库 `deploy/nginx.conf` 后
  `node remote-test/m5-nginx-sync.mjs`（先备份为 `oj.bak-sec`，`nginx -t`
  门禁通过才替换并 reload）。

> 注意：`m5-health.mjs` / `m5-nginx-sync.mjs` 读的是 `.sshenv` 密码通道，
> SSH 密钥化后若连接失败，直接 SSH 上机执行等价命令（健康检查三条见上；
> nginx 同步 = 备份现有配置 → `nginx -t` → 替换 → `systemctl reload nginx`）。

## 三、我要排查一个判题 SE

前置：拿到出问题的提交（submission id 可从提交记录 URL 或后台查）。

1. **看日志定位**。免 SSH：管理后台 →「日志查看器」（admin 及以上），unit
   选 `oj-judge`，关键字填 submission id 或 `SE`。命令行（在服务器执行）：

   ```bash
   journalctl -u oj-judge -p err -S -1h --no-pager    # 判题机最近 1 小时错误
   journalctl -u oj-api -S today --no-pager | grep "submission judged"
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
systemctl is-active oj-api oj-judge nginx postgresql redis-server   # 全 active
tail -5 /opt/oj/backup/backup.log        # 今天 03:00 备份成功、dump 体积正常
df -h /                                  # 磁盘 < 80%
free -m                                  # 3.9GB 小内存机，看 available
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1/api/v1/languages   # 200
journalctl -u oj-api -u oj-judge -p err -S today --no-pager | tail -20
```

网页侧：管理后台「判题机监控」judge-1 online、队列归零；每月 1 号之后
`tail /opt/oj/backup/drill.log` 末尾应是 `drill PASSED`。

通过标准：以上全部正常。任一项异常 → 按 §九 事故表处置。

## 五、服务管理与日志速查

```bash
systemctl restart oj-api      # 改 oj.env 后必须
systemctl restart oj-judge    # 改 oj-judge.env / 装编译器后
```

oj-judge 经 gRPC（127.0.0.1:9090）连 oj-api：重启 oj-api 会让判题机短暂
断连（WARN `daemon disconnected`）并自动重连，在判任务由 15 分钟租约回收
重排。两个都要重启时**先 oj-api 后 oj-judge**（同时 restart 也能自愈，只是
多一轮重连）。

- **全站 502**：nginx 后面的 oj-api 死了 →
  `journalctl -u oj-api -n 100 --no-pager`（高频原因：oj.env 改坏、JWT
  secret 缺失导致启动失败）。
- **`/api/v1/ws` 返回 503**：WebSocket 连接数到上限（1024）→
  `systemctl restart oj-api` 清空连接；频繁出现再查连接泄漏。

### 日志速查

- 两个进程均为 JSON 结构化日志（logx/slog）→ stderr → journald 收集，
  级别默认 `info`。
- 免 SSH：管理后台 →「日志查看器」，读 oj-api/oj-judge 两 unit 最近日志，
  可按 unit/关键字在服务端过滤（前提 oj 用户在 `systemd-journal` 组，
  已配置；查看器报 500 先查这个）。
- 命令行（在服务器执行）：

  ```bash
  journalctl -u oj-api -f                 # API 实时日志
  journalctl -u oj-judge -f               # 判题机实时日志
  journalctl -u oj-judge -p err -S -1h    # 判题机最近 1 小时错误
  ```

- 临时 debug：`OJ_LOG_LEVEL=debug` 写进 `/opt/oj/oj.env` 或
  `/opt/oj/oj-judge.env` → restart 对应服务 → 排查完改回。
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

验证：`df -h /` 回落到 80% 以下；`systemctl is-active oj-api oj-judge nginx`
仍全部 active。

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
systemctl stop oj-api oj-judge
# 2. 恢复数据库
systemctl start postgresql
runuser -u postgres -- dropdb --if-exists oj
runuser -u postgres -- createdb oj
runuser -u postgres -- pg_restore -d oj /opt/oj/backup/db/oj-<日期>.dump
# 3. 恢复数据目录（testdata 等）
rsync -a /opt/oj/backup/data/snapshot/ /opt/oj/data/
# 4. 起服务
systemctl start oj-api oj-judge
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
T=$(grep '^OJ_CLI_TOKEN=' /opt/oj/oj.env | cut -d= -f2-)
/opt/oj/oj-cli create-superadmin --username <admin账号> --password '<新密码>' --token "$T"
```

该命令对已有用户是「提升+重置+解封」语义，超管锁死自救也用它，详见 [deploy.md](deploy.md) 第八节。

### 数据库 oj 角色密码（在服务器执行）

```bash
runuser -u postgres -- psql -c "ALTER ROLE oj PASSWORD '<新密码>'"
# 同步改 /opt/oj/oj.env 的 OJ_DB_DSN 中 password=
systemctl restart oj-api
```

### JWT / 判题机密钥（在服务器执行）

`/opt/oj/oj.env` 的 `OJ_JWT_SECRET` 与 `OJ_DAEMON_SECRET`；
`/opt/oj/oj-judge.env` 的 `OJ_DAEMON_TOKEN` 必须与 `OJ_DAEMON_SECRET`
**同值**。两处一起改后 `systemctl restart oj-api oj-judge`。注意：换 JWT 后
所有用户登录态立即失效；daemon 两侧不同值 → 判题机直接 offline。

验证：`/api/v1/languages` 200、后台判题机监控 online、admin 用新密码
重新登录成功。

## 九、常见事故处置

| 现象 | 处置 |
|---|---|
| 全部提交 PENDING | 后台「判题机监控」看 judge-1 是否 offline → `journalctl -u oj-judge -n 50`；`OJ_DAEMON_TOKEN` 与 `OJ_DAEMON_SECRET` 不同值是高频原因 |
| 某语言全 CE | 判题机缺编译器：`apt install openjdk-17-jdk-headless` 等，装完 `systemctl restart oj-judge` |
| 某语言全 SE/RE 且空 stderr | 沙箱 seccomp 误杀：`journalctl -u oj-judge \| grep SIGSYS`，按 [judge-sandbox.md](../development/judge-sandbox.md) 用 strace 定位 |
| 提交一直 JUDGING | 15 分钟租约到期会被自动回收重排；或 `systemctl restart oj-judge` 触发重连回收 |
| 全站 502 | oj-api 死了：`journalctl -u oj-api -n 100`（常见：oj.env 改坏、JWT secret 缺失启动失败） |
| WS 503 | 连接数到上限 1024：`systemctl restart oj-api`（见 §五） |
| 数据库连不上 | PG 挂或 oj 角色密码与 DSN 不一致：`runuser -u postgres -- psql -d oj -c 'select 1'` |
| 超管密码丢失/锁死 | oj-cli 自救，见 §八 |
| 磁盘爆 | 按 §六 顺序清；**不要**删 `/opt/oj/data/testdata` |
| 服务器 OOM / 莫名变卡 | 3.9GB 单机：确认没人在服务器上跑 npm/go 构建（两次事故根因）；`OJ_MAX_PARALLEL` 保持 1 |
| 疑似被入侵 | `last -f /var/log/wtmp`、`ss -tnp`、查 nginx 访问日志异常 IP；按 §八 全量轮换 |

## 十、定期维护任务

| 周期 | 任务 |
|---|---|
| 每天（自动） | 03:00 备份（cron）；巡检时扫一眼 `backup.log` |
| 每周 | §四 巡检 + `apt update && apt upgrade`（如需重启机器，先停 oj-api/oj-judge） |
| 每月 | 确认 `drill.log` 末尾 `drill PASSED`；抽一份备份走一次恢复演练全流程 |
| 每季度 | 评估磁盘增长（testdata/备份）；评估是否加第二台判题机（见 judge-sandbox.md） |
| 人员变动 | 按 §八 全量轮换 SSH 公钥、admin、DB、JWT/daemon 密钥，并更新交接文档 |

## 十一、配置变更索引

| 想改什么 | 改哪里 | 生效方式 |
|---|---|---|
| 并行判题数 | `/opt/oj/oj-judge.env` 的 `OJ_MAX_PARALLEL`（生产=1，3.9GB 内存慎调大） | restart oj-judge |
| 日志级别 | `/opt/oj/oj.env` / `oj-judge.env` 的 `OJ_LOG_LEVEL` | restart 对应服务 |
| 会话时长 | `/opt/oj/oj.env` 的 `OJ_JWT_EXPIRE_HOURS` | restart oj-api |
| journald 磁盘上限 | `/etc/systemd/journald.conf.d/99-oj.conf` 的 `SystemMaxUse` | restart systemd-journald |
| 备份保留期 | `/opt/oj/backup.sh` 头部 `KEEP_DAYS/KEEP_MONTHS` | 下次备份生效 |
| nginx | 源文件 `deploy/nginx.conf`（走 §二 同步），或服务器直改 `/etc/nginx/sites-available/oj` | `nginx -t && systemctl reload nginx` |
| SSH 策略 | `/etc/ssh/sshd_config.d/00-oj-hardening.conf` | `sshd -t && systemctl reload ssh` |
| 时限倍率/内存放宽/加语言 | `backend/pkg/judge/languages.yaml` + 服务器装对应工具链 | 重发 oj-judge 二进制（§二） |
| 限流阈值 | `backend/internal/handler/middleware.go`（login/register）、`submissions.go`（submit） | 重发 oj-api 二进制（§二） |
