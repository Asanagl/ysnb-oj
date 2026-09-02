# 维护手册（运维 Runbook）

> 日常运维操作手册：巡检、服务管理、发布升级、磁盘与缓存、事故处置。
> 面向管理者/接手人；用户侧问题先查 `admin-guide.md` 第八节，判题深水区查
> `judge-sandbox.md` §2.6，部署冷知识查 `deploy.md`。

## 一、服务器速查

| 项 | 值 |
|---|---|
| 主机 | <your-server-ip>（Debian 11，root 登录） |
| 服务 | `oj-api`（HTTP :8080 / gRPC :9090）、`oj-judge`、nginx（:80）、postgresql、redis |
| 程序 | `/opt/oj/oj-api`、`/opt/oj/oj-judge`、维护 CLI `/opt/oj/oj-cli` |
| 配置 | `/opt/oj/oj.env`、`/opt/oj/oj-judge.env`（600） |
| 前端 | `/opt/oj/web`（nginx 根） |
| 数据 | `/opt/oj/data`（testdata/代码）；判题工作区 `/oj-work`；blob 缓存 `/opt/oj/data/judge-cache` |
| 备份 | `/opt/oj/backup/`（db/ + data/snapshot/ + 两个日志） |
| 定时任务 | root crontab：03:00 备份、每月 1 号 04:00 恢复演练 |
| 凭据 | 见 `handover.md` 第二节（此处不放明文） |

## 二、每日/每周巡检（5 分钟）

```bash
systemctl status oj-api oj-judge nginx      # 四个服务全 active
tail -5 /opt/oj/backup/backup.log           # 03:00 备份成功、dump 体积正常
df -h /                                     # 磁盘 < 80%
curl -s http://127.0.0.1/api/v1/languages | head -c 80   # API 活着
```

- 后台「判题机监控」页：judge-1 online、队列归零。
- 每月 1 号后看一眼 `drill.log` 末尾是否 `drill PASSED`。

## 三、服务管理

```bash
systemctl restart oj-api          # 重启 API（改 oj.env 后必须）
systemctl restart oj-judge        # 重启判题机（改 oj-judge.env / 装编译器后）
journalctl -u oj-api -f           # API 实时日志（gin 访问日志在此）
journalctl -u oj-judge -f         # 判题机实时日志（编译/判题/看门狗打点）
```

## 四、发布升级（标准流程）

1. 开发机构建（Windows）：

```bash
cd backend && export PATH="$PATH:/c/Users/Asanagi/tools/go-sdk/go/bin" \
  && export GOPROXY=https://goproxy.cn,direct CGO_ENABLED=0 GOOS=linux
go build -o ../dist/oj-api-linux ./cmd/api
go build -o ../dist/oj-judge-linux ./cmd/judge
cd ../frontend && npm run build && cd ../remote-test
```

2. 推送（remote-test/ 工具链，凭据在本目录 .sshenv/.ojenv）：

```bash
node gen-push.js ../dist/oj-api-linux api push-api.sh
node gen-push.js ../dist/oj-judge-linux judge push-api.sh
node ssh.js runstdin steps/stop-prod.sh     # 先停服！直接覆盖报 Text file busy
node ssh.js runstdin push-api.sh && node ssh.js runstdin push-judge.sh
# 前端：zip dist → push web → 解压（解压脚本见下）
node ssh.js runstdin steps/prod-restart.sh
```

**M5 起的简化通道（不停服，二进制原子替换 + sha256 双端校验）**：
`remote-test/m5-deploy3.mjs`（api/judge 二进制，gzip+base64 经 bash -s
stdin，自动 restart + is-active）与 `m5-frontend-sync.mjs`（前端 dist →
`/opt/oj/web`，nginx 静态根，免 reload）。ssh2 exec 通道直接裸 base64 大
文件会卡死在 channel window——不要用旧 gen-push 通道推 >1MB 的文件。

3. 前端解压坑：PowerShell `Compress-Archive` 的 zip 条目是 `\` 分隔，
   Linux unzip 会变成扁平的 `assets\X.js` 文件。用现成的
   `steps/fix-web3.sh`（python zipfile + 反斜杠归位）解压，或直接用
   `m5-frontend-sync.mjs`（tar.gz 通道无此问题）。
4. 验证：`steps/prod-restart.sh` 自带检查（服务 active、判题机 connected、
   /languages 200）＋后台提交一道 A+B 确认判题链路。

## 五、磁盘与缓存清理

| 对象 | 位置 | 处置 |
|---|---|---|
| 判题保留工作区（CE/SE 现场） | `/oj-work/judge-*` | 故障排查完即可删：`find /oj-work -maxdepth 1 -name 'judge-*' -mtime +7 -exec rm -rf {} +` |
| blob 缓存 | `/opt/oj/data/judge-cache` | 可安全清空（按需重拉），别删 testdata 本体 |
| 备份 | `/opt/oj/backup` | 脚本自带保留策略（日备 14 天 + 月备 6 个月），一般不用手清 |
| 数据库日志膨胀 | — | PG 默认不大动；磁盘紧张先看 backup 与 /oj-work |

## 六、常见事故处置

| 现象 | 处置 |
|---|---|
| 全部提交 PENDING | 判题机监控看 judge-1 是否 offline；`journalctl -u oj-judge -n 50`；token 不匹配是高频原因（两处必须同值） |
| 某语言全 CE | 判题机缺编译器：`apt install openjdk-17-jdk-headless` 等，装完 `systemctl restart oj-judge` |
| 某语言全 SE/RE 且空 stderr | 沙箱 seccomp 误杀：`journalctl -u oj-judge | grep SIGSYS`；按 `judge-sandbox.md` §2.6 用 strace 定位缺的系统调用 |
| 提交一直 JUDGING 不出结果 | 15 分钟租约到期会被扫描器回收重排；等或 `systemctl restart oj-judge` 触发重连回收 |
| API 502 | nginx 后面 oj-api 死了；`journalctl -u oj-api -n 100`（常见：oj.env 改坏、JWT secret 缺失启动失败） |
| 数据库连不上 | PG 挂或 oj role 密码与 DSN 不一致；`runuser -u postgres -- psql -d oj -c 'select 1'` |
| 超管密码丢失/锁死 | `/opt/oj/oj-cli create-superadmin --username admin --password 新密码 --token "$OJ_CLI_TOKEN"`（令牌在 oj.env；Docker 路线用 `docker compose exec api oj-cli …`），详见 deploy.md §〇-2 |
| 磁盘爆 | 先删 /oj-work 老工作区与 judge-cache，再查 backup；**不要**删 /opt/oj/data/testdata |
| 疑似被入侵 | `last -f /var/log/wtmp`、`ss -tnp`、查 nginx 访问日志异常 IP；轮换全部凭据（handover.md 第二节清单） |

## 七、定期维护任务

| 周期 | 任务 |
|---|---|
| 每周 | 巡检（第二节）+ `apt update && apt upgrade`（重启前先停服务） |
| 每月 | 确认恢复演练 PASS；抽查一次备份恢复到临时库走全流程 |
| 每季度 | 评估磁盘增长（testdata/备份）；考虑第二台判题机（见 judge-sandbox.md §四末） |
| 人员变动 | 轮换 root SSH、admin、DB、JWT/daemon 密钥（改 oj.env → restart），更新交接文档 |

## 八、配置变更索引

| 想改什么 | 改哪里 |
|---|---|
| 时限倍率/内存放宽/加语言 | `backend/pkg/judge/languages.yaml`（重发判题机二进制）+ 装工具链 |
| 限流阈值 | `backend/internal/handler/middleware.go`（login/register）与 `submissions.go`（submit） |
| 并行判题数 | `/opt/oj/oj-judge.env` 的 `OJ_MAX_PARALLEL`（restart 生效） |
| 会话时长 | `OJ_JWT_EXPIRE_HOURS` |
| 备份保留期 | `/opt/oj/backup.sh` 头部 `KEEP_DAYS/KEEP_MONTHS` |
| nginx | `/etc/nginx/...`（改完 `nginx -t && systemctl reload nginx`） |
