# 部署文档

> **读者对象**：要把 YSNB OJ 部署到自己服务器的开发者/运维。本文按任务组织，每个任务都附验证方法；日常运维（巡检/发布/事故处置）见 `maintenance.md`，后台使用见 `admin-guide.md`。

任务索引：从零部署 → 第二节（A 一键 / B 裸机）｜接入判题机 → 第三节｜日志 → 第四节｜备份与恢复演练 → 第五节｜HTTPS（开口项）→ 第六节｜环境变量全表 → 第七节｜维护 CLI → 第八节｜故障排查 → 第九节。

## 一、开始之前：系统要求

- **操作系统**：后端 Linux-only（沙箱依赖 cgroup v2 与 namespaces），Windows / macOS 不受支持。建议 Ubuntu 22.04+ / Debian 12+；生产实测 Debian 11 也在稳定运行（seccomp KILL_PROCESS 需内核 ≥ 4.14，更老的内核会自动降级）。
- **cgroup v2**（判题硬依赖），先验证：`stat -fc %T /sys/fs/cgroup` 必须输出 `cgroup2fs`。
- **前端产物在开发机构建**，随仓库同步到服务器；**服务器上禁止 npm / go 构建**（小内存机会 OOM）：`cd frontend && npm ci && npm run build`。
- **端口红线**：8080（API HTTP）与 9090（API gRPC）只允许 127.0.0.1（或 compose 内网）可达，公网只暴露 nginx 的 80（及将来可选的 443）。Compose 路线用 `expose` 不发布端口；裸机路线用防火墙兜底。
- 路径选择：**A. Docker Compose 一键**（大多数场景，`deploy/one-click.sh`）或 **B. 裸机 + systemd**（不想装 Docker 的校内服务器）。

## 二、任务：我要在一台新服务器上从零部署

### 路径 A：Docker Compose 一键部署（推荐）

前置：① Docker ≥ 24（`apt install docker.io docker-compose-plugin` 或按官方文档）；② 开发机构建的 `frontend/dist` 已随仓库同步；③ root shell 且 cgroup v2 已启用。然后在仓库根目录：

```bash
bash deploy/one-click.sh
```

脚本自动完成（任一步失败即停并打印排查入口；幂等，可重复执行）：

1. 前置检查（docker / `frontend/dist` / cgroup v2）；
2. 生成 `.env`：PG 密码、JWT 密钥、判题机共享密钥（DAEMON）、admin 初始密码、CLI 令牌全部用 openssl 现场随机生成；**已存在的 `.env` 不覆盖**；
3. `docker compose up -d --build`，轮询 `/api/v1/languages` 等待 API 就绪；
4. 判题机自验 `docker compose exec -T judge oj-judge --selftest`；
5. 安装备份 cron（每日 03:00 备份 + 每月 1 号 04:00 恢复演练，见第五节）；
6. 前端冒烟，最后打印访问地址、admin 初始密码（**立即记下**）、邀请码入口与 oj-cli 用法。

验证：

```bash
docker compose ps    # postgres / redis / api / judge / web 全部 running
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1/api/v1/languages   # 期望 200（经 nginx 反代）
```

浏览器打开 `http://<服务器IP>/`，用 `.env` 里的 `OJ_ADMIN_USERNAME` / `OJ_ADMIN_PASSWORD` 登录（仅首次启动、库中无用户时创建）。

> **若 `/api/v1/languages` 返回 502**：compose 网络里 web 容器的 `127.0.0.1` 指向容器自身。把 `deploy/nginx.conf` 两处 `proxy_pass http://127.0.0.1:8080;` 改为 `http://api:8080;`（按服务名寻址），然后 `docker compose restart web`。

不用一键脚本、手动 compose：`cp .env.example .env` → 填掉所有 `change-me*`（`openssl rand -base64 32` 生成随机串）→ `docker compose up -d --build`；再手动补 selftest（命令同上第 4 步）与备份 cron（第五节）。

### 路径 B：裸机 + systemd

以下以 root 执行。安装根 `/opt/oj/`：`oj-api` / `oj-judge` / `oj-cli`（三个二进制）、`oj.env` / `oj-judge.env`（配置，权限 600）、`web/`（前端产物，nginx 静态根）、`data/`（测试数据/代码/上传物，原始资产）、`backup/`（备份产物与演练日志）。

**1. 装系统依赖**（PostgreSQL + Redis + nginx + 判题工具链；判题机分离部署时工具链只需装在判题机上，见第三节）：

```bash
apt update
apt install -y postgresql redis-server nginx \
  build-essential openjdk-17-jdk-headless python3
```

**2. 建库与系统用户**：

```bash
runuser -u postgres -- psql -c "CREATE ROLE oj LOGIN PASSWORD '<DB密码>';"
runuser -u postgres -- createdb -O oj oj
useradd -r -d /opt/oj -s /usr/sbin/nologin oj || true
mkdir -p /opt/oj/data /opt/oj/web /opt/oj/backup
```

验证：`runuser -u postgres -- psql -d oj -c 'select 1'` 返回一行。

**3. 部署二进制与前端**（产物在仓库 `dist/` 与 `frontend/dist/`，开发机构建，服务器上不跑 go/npm）：

```bash
cp dist/oj-api-linux   /opt/oj/oj-api
cp dist/oj-judge-linux /opt/oj/oj-judge
cp dist/oj-cli-linux   /opt/oj/oj-cli
chmod +x /opt/oj/oj-api /opt/oj/oj-judge /opt/oj/oj-cli
cp -r frontend/dist/. /opt/oj/web/
chown -R oj:oj /opt/oj/data
```

**4. 写配置**（两个文件都 `chmod 600`；完整变量表见第七节）。

`/opt/oj/oj.env`：

```ini
OJ_MODE=prod
OJ_LISTEN=:8080
OJ_GRPC_ADDR=:9090
OJ_DATA_DIR=/opt/oj/data
OJ_FETCH_BASE=http://127.0.0.1:8080
OJ_DB_DRIVER=postgres
OJ_DB_DSN=host=127.0.0.1 user=oj password=<DB密码> dbname=oj sslmode=disable
OJ_REDIS_ADDR=127.0.0.1:6379
OJ_JWT_SECRET=<openssl rand -base64 32 生成>
OJ_DAEMON_SECRET=<openssl rand -base64 32 生成>
OJ_CLI_TOKEN=<openssl rand -hex 16 生成>
OJ_ADMIN_USERNAME=admin
OJ_ADMIN_PASSWORD=<仅首启建管理员用的初始密码>
OJ_LOG_LEVEL=info
```

`/opt/oj/oj-judge.env`：

```ini
OJ_MODE=prod
OJ_DATA_DIR=/opt/oj/data
OJ_API_ENDPOINT=127.0.0.1:9090
OJ_FETCH_BASE=http://127.0.0.1:8080
OJ_DAEMON_NAME=judge-1
OJ_DAEMON_TOKEN=<与 oj.env 的 OJ_DAEMON_SECRET 同值>
OJ_MAX_PARALLEL=1
OJ_WORK_ROOT=/oj-work
OJ_LOG_LEVEL=info
```

> `OJ_MAX_PARALLEL`：**内存小的机器必须设 1**（每个并行槽位的编译 cgroup 峰值约 1GB，3.9GB 生产机实测必须 =1），大内存机器可放宽。

**5. 判题环境自验**（必做，必须 root）：

```bash
/opt/oj/oj-judge --selftest
# 期望输出三行 PASS（cgroup v2 writable / basic jailed run (/bin/echo) /
# wall-clock watchdog (100ms limit)），最后 == all selftests passed ==
```

任何一行 FAIL 先解决再继续（高频原因：cgroup v2 未启用、非 root）。

**6. 装 systemd 单元并启动**：

```bash
cp deploy/systemd/oj-api.service deploy/systemd/oj-judge.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now oj-api oj-judge
```

两个单元的权限模型是**刻意的，不要"加固"**：`oj-api` 以 `oj` 用户运行，`NoNewPrivileges` + `ProtectSystem=strict` + `ReadWritePaths=/opt/oj/data`（纯 web 服务，最小权限）；`oj-judge` 必须以 **root** 运行，且单元里**故意不加** systemd 沙箱指令——沙箱隔离（namespace/cgroup/降权）由判题进程自身为每个判题任务创建，systemd 层限制会直接弄坏这套隔离。

验证：

```bash
systemctl is-active oj-api oj-judge    # active / active
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/api/v1/languages   # 期望 200
```

**7. 接 nginx**（模板 `deploy/nginx.conf`：静态 `/opt/oj/web` + `/api` 反代 + `/api/v1/ws` WebSocket 升级 + 安全头四件套）：

```bash
cp deploy/nginx.conf /etc/nginx/conf.d/oj.conf
nginx -t && systemctl reload nginx
```

验证：`curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1/` → 200；浏览器开 `http://<服务器IP>/` 用 admin 登录，后台「判题机监控」确认 judge-1 online。

**8. 收尾**：接着配日志（第四节）与备份（第五节）。

## 三、任务：我要接入一台判题机

判题机与 API 同机时第二节已覆盖；本节针对**独立一台判题机**（或加第二台扩容）。步骤（root）：

1. 装工具链并确认 cgroup v2：

   ```bash
   apt update && apt install -y build-essential openjdk-17-jdk-headless python3
   stat -fc %T /sys/fs/cgroup   # 必须输出 cgroup2fs
   ```

2. 拷判题机二进制到 `/opt/oj/oj-judge`，写 `/opt/oj/oj-judge.env`（chmod 600）：

   ```ini
   OJ_MODE=prod
   OJ_API_ENDPOINT=<API机内网IP>:9090
   OJ_FETCH_BASE=http://<API机内网IP>:8080
   OJ_DAEMON_NAME=judge-2          # 每台判题机唯一，显示在监控页
   OJ_DAEMON_TOKEN=<与 API 机 OJ_DAEMON_SECRET 同值>
   OJ_MAX_PARALLEL=1
   OJ_WORK_ROOT=/oj-work
   OJ_LOG_LEVEL=info
   ```

3. API 机放行：gRPC 9090 与测试数据下载 8080 需对该判题机的**内网 IP** 开白名单（防火墙层面，勿对公网开放）。裸机路线 oj-api 默认监听全接口，用 ufw/iptables 收敛；compose 路线默认不发布端口，外接判题机需自行映射并同样收敛。
4. **必跑自验**：`/opt/oj/oj-judge --selftest`（期望输出见第二节路径 B）。
5. 只装判题单元：`cp deploy/systemd/oj-judge.service /etc/systemd/system/` → `systemctl daemon-reload && systemctl enable --now oj-judge`。

验证：后台「判题机监控」出现 judge-2 且 online；提交一道 A+B 能出 AC/WA 而不是一直 PENDING。判题机一直 offline 的高频原因是 `OJ_DAEMON_TOKEN` 与 API 端 `OJ_DAEMON_SECRET` 不一致。

## 四、任务：我要配好日志

两个进程（oj-api / oj-judge）都通过 logx（log/slog）以**单行 JSON** 输出到 stderr，由 systemd 收进 journald；日志级别直接映射 journald 的 PRIORITY，`journalctl -p err` 无需 grep。

**日志级别**：`OJ_LOG_LEVEL`（`debug|info|warn|error`，默认 `info`，写错值静默回落 info）。排查时临时开 debug——逐 case 的耗时/内存/编译 stderr 明细都会出来：在 `oj.env` 或 `oj-judge.env` 加 `OJ_LOG_LEVEL=debug` 后 `systemctl restart oj-api oj-judge`，排查完改回 `info` 并重启。

**journald 磁盘上限**（生产必配，防日志挤占业务磁盘）：

```bash
mkdir -p /etc/systemd/journald.conf.d
printf '[Journal]\nSystemMaxUse=200M\nCompress=yes\n' \
  > /etc/systemd/journald.conf.d/99-oj.conf
systemctl restart systemd-journald
```

验证：`journalctl --disk-usage` 稳定在 200M 以内；`journalctl -u oj-api -n 5` 能看到 JSON 行。

**网页日志查看器**（管理后台 → 日志查看器，admin 及以上）：后端以固定参数调用 `journalctl` 读 `oj-api.service` / `oj-judge.service` 最近日志，搜索在服务端完成（无 shell、无注入面）。前置：把 API 运行用户加进 journald 读取组，否则查看器报 500：

```bash
usermod -aG systemd-journal oj && systemctl restart oj-api
```

验证：后台日志查看器能列出 oj-api 日志；按关键字 `submission judged` 过滤有结果。注意查看器依赖 journald，**仅 systemd 路线可用**。

**排错锚点**：每次判题完成产生一行 INFO，可按提交号/结果检索：

```
submission judged  submission=<id> status=<verdict> time_ms=… memory_kb=… score=… problem=…
```

判题 SE 另有 ERROR 行；判题机断连有 WARN `daemon disconnected`。命令行兜底（任何有 SSH 权限的运维可查）：

```bash
journalctl -u oj-api -f                    # API 实时日志
journalctl -u oj-judge -p err -S -1h       # 判题机最近 1 小时错误
journalctl -u oj-api -o json | grep submission   # 结构化检索
```

Docker Compose 路线没有 journald，用 `docker compose logs -f api` / `docker compose logs -f judge`，日志本体同为单行 JSON。

## 五、任务：我要配好备份与恢复演练

两个脚本（`deploy/` 目录）：

- `backup.sh`——全量备份：PostgreSQL（`pg_dump -Fc` 自定义格式，可 `pg_restore`）+ `/opt/oj/data` 快照。保留最近 14 天日备 + 最近 6 个月每月 1 号的月备（脚本头 `KEEP_DAYS` / `KEEP_MONTHS` 可调）。
- `restore-drill.sh <dump>`——恢复演练：有 docker 就起一个一次性 PostgreSQL 容器真实恢复并打印各表行数；没有 docker 就在本机建临时库 `oj_drill_tmp`（恢复完即删）。**两条路都不碰生产库**——备份"存在"不等于"能恢复"，演练是唯一证明。

安装（API 机；路径 A 的一键脚本已自动装好，直接跳到验证）：

```bash
install -m 755 deploy/backup.sh deploy/restore-drill.sh /opt/oj/
mkdir -p /opt/oj/backup
( crontab -l 2>/dev/null | grep -v 'backup.sh\|restore-drill.sh' || true;
  echo '0 3 * * * /opt/oj/backup.sh >> /opt/oj/backup/backup.log 2>&1';
  echo '0 4 1 * * /opt/oj/restore-drill.sh $(ls -t /opt/oj/backup/db/*.dump 2>/dev/null | head -1) >> /opt/oj/backup/drill.log 2>&1' ) | crontab -
```

验证（装完立刻手动各跑一次，别等 cron）：

```bash
/opt/oj/backup.sh
ls -lh /opt/oj/backup/db/            # 应有非空的 oj-<日期>.dump
/opt/oj/restore-drill.sh "$(ls -t /opt/oj/backup/db/*.dump | head -1)"
# 期望末尾输出：== drill PASSED (backup is restorable) ==
```

脚本自动适配两种部署：检测到 `oj-postgres` 容器就走 `docker exec … pg_dump`（compose），否则 `runuser -u postgres` peer 认证（裸机）；没有 rsync 的机器自动降级为 cp 全量拷贝。

手工灾难恢复（裸机示例）：

```bash
systemctl stop oj-api oj-judge                            # 1. 停服务，避免恢复期间写入
runuser -u postgres -- dropdb --if-exists oj              # 2. 恢复数据库
runuser -u postgres -- createdb oj
runuser -u postgres -- pg_restore -d oj /opt/oj/backup/db/oj-<日期>.dump
rsync -a /opt/oj/backup/data/snapshot/ /opt/oj/data/      # 3. 恢复数据目录
systemctl start oj-api oj-judge                           # 4. 起服务
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/api/v1/languages   # 200
```

> 判题机工作目录（`/oj-work`、judge-data）是缓存性质，可由 API 机重新拉取，**无需备份**；`/opt/oj/data`（API 机）才是原始资产。

## 六、任务：我要上 HTTPS（可选开口项）

**当前 443 未启用是有意留下的开口项，不是故障**——请勿当 bug 去"修"，也不要在没有证书的情况下硬开。校内/内网使用 HTTP 即可；对外开放时再按本节上 TLS。两种常见做法（二选一）：

- **Caddy**：最省事，自动签发与续期证书，反代到 `127.0.0.1:80`；
- **certbot + 现有 nginx**：`apt install -y certbot python3-certbot-nginx` → `certbot --nginx -d <域名>`。

启用 TLS 后收尾两件事：

1. 打开 `deploy/nginx.conf` 里预留的 HSTS 注释行（`# enable after TLS: add_header Strict-Transport-Security …`）。注意 nginx 的 `add_header` 不向声明了自己 `add_header` 的 location 继承（文件头注释有说明），HSTS 要落在 443 server 块上。
2. WebSocket 无需额外配置：CSP 的 `connect-src` 已含 `wss:`。

验证（同时浏览器确认锁标、比赛页 WS 实时推送正常）：

```bash
curl -sI https://<域名>/api/v1/languages | head -1    # 200
curl -sI https://<域名>/ | grep -i strict-transport   # 有 HSTS 头
```

## 七、环境变量全表

以 `backend/internal/config/config.go` 为准；所有变量都是对 YAML 配置的覆盖（也可 `--config` / `OJ_CONFIG` 指定 YAML 文件，运维上推荐纯 env）。

通用（API 与判题机都读）：

| 变量 | 默认 | 说明 |
|---|---|---|
| `OJ_MODE` | `dev` | `dev` = sqlite + 内存队列零依赖起服务；生产必须 `prod` |
| `OJ_LISTEN` | `:8080` | API HTTP 监听地址 |
| `OJ_GRPC_ADDR` | `:9090` | API gRPC 监听地址（判题机接入） |
| `OJ_DATA_DIR` | `./data` | 测试数据 / 代码 / 上传物根目录 |
| `OJ_FETCH_BASE` | `http://127.0.0.1:8080` | 判题机下载测试数据的 base URL；判题机在外机时填 API 机内网地址 |
| `OJ_LOG_LEVEL` | `info` | `debug\|info\|warn\|error`；debug 输出逐 case 明细（第四节） |
| `OJ_DB_DRIVER` / `OJ_DB_DSN` | sqlite | 生产：`postgres` + PG DSN |
| `OJ_REDIS_ADDR` | — | 提交队列 / 缓存（prod 必填） |
| `OJ_JWT_SECRET` | — | **prod 必填**，缺失拒绝启动 |
| `OJ_JWT_EXPIRE_HOURS` | `72` | 登录会话时长（小时） |
| `OJ_DAEMON_SECRET` | — | 判题机共享密钥（API 端） |
| `OJ_ADMIN_USERNAME` / `OJ_ADMIN_PASSWORD` | — | 仅**首次启动**（库中无用户时）创建管理员 |
| `OJ_CLI_TOKEN` | — | oj-cli 变更类命令的确认令牌（第八节） |
| `OJ_LANGUAGES_FILE` | — | 判题机覆盖内置 languages.yaml（仅接受 .yaml/.yml） |

判题机专用：

| 变量 | 默认 | 说明 |
|---|---|---|
| `OJ_API_ENDPOINT` | — | API 的 gRPC `host:port` |
| `OJ_DAEMON_NAME` | — | 监控页显示名，每台判题机唯一 |
| `OJ_DAEMON_TOKEN` | — | 必须等于 API 端 `OJ_DAEMON_SECRET` |
| `OJ_MAX_PARALLEL` | `2` | 并行判题槽位；**小内存机器设 1**（3.9GB 生产机实测必须 =1），大内存机器可放宽 |
| `OJ_WORK_ROOT` | `/oj-work` | 判题临时目录，不能放在 /home 或 data_dir 下 |

compose 路线另有 `OJ_PG_USER` / `OJ_PG_PASSWORD`（建库用），见 `.env.example`。

## 八、维护 CLI（oj-cli）

服务器内维护工具：创建/找回超管、重置密码、改角色、邀请码、健康诊断。**直连数据库执行，API 挂了也能用**——正是锁死自救的场景。安全模型：无网络入口（只有持服务器 shell 的人能执行）；除只读的 `users` / `doctor` 外都要 `--token` 等于配置里的 `OJ_CLI_TOKEN`（二次确认，防误触）。

compose 路线（容器内置）：

```bash
docker compose exec api oj-cli doctor        # DB / Redis / 判题机健康诊断
T=$(grep '^OJ_CLI_TOKEN=' .env | cut -d= -f2)
docker compose exec api oj-cli --token "$T" create-superadmin --username newadmin --password '<强密码>'
docker compose exec api oj-cli --token "$T" reset-password --username admin --password '<新密码>'
docker compose exec api oj-cli --token "$T" set-role --username someone --role setter
```

其余子命令：`users --q <关键字>`（只读列用户）、`invite --max-uses 30`（生成邀请码）。

systemd 路线：二进制 `/opt/oj/oj-cli` 启动时自动读取 `/opt/oj/oj.env`（真实环境变量优先），裸 root shell 直接跑：

```bash
/opt/oj/oj-cli doctor
/opt/oj/oj-cli create-superadmin --username admin --password '<新密码>' \
  --token "$(grep '^OJ_CLI_TOKEN=' /opt/oj/oj.env | cut -d= -f2)"
```

`create-superadmin` 对已存在用户是「提升为超管 + 重置密码 + 解封」语义，超管密码丢失/被锁时用它自救。

## 九、故障排查

| 现象 | 排查 |
|---|---|
| selftest 报 cgroup 不可用 | 内核未启用 cgroup v2 或非 root；`stat -fc %T /sys/fs/cgroup` 应输出 `cgroup2fs` |
| 判题机一直 offline | `OJ_DAEMON_TOKEN` 与 API 的 `OJ_DAEMON_SECRET` 不一致；gRPC 9090 不通（防火墙/安全组） |
| 提交一直 PENDING | 后台「判题机监控」看队列与判题机状态；`journalctl -u oj-judge -n 50` |
| 全部提交 SE | 判题机磁盘满 / `/oj-work` 不可写 / 内核过老（seccomp KILL_PROCESS 需 ≥ 4.14，老内核自动降级） |
| 判题机 OOM / 卡死 | `OJ_MAX_PARALLEL` 超出内存承载；3.9GB 机器必须 =1 |
| Java 全部 CE/MLE/RE | 判题机缺 JDK 17（`which javac`）；或 languages.yaml 的 `mem_overhead_mb` 不够 JVM |
| 502 / WebSocket 断 | nginx 后面的 oj-api 是否存活；`/api/v1/ws` 的 Upgrade 头是否被中间设备剥掉 |
| 网页日志查看器 500 | oj 用户不在 `systemd-journal` 组（第四节）；compose 路线无 journald，查看器不可用 |
| 日志占磁盘 | journald `SystemMaxUse`（第四节），默认 200M |
| 外网 curl 8080/9090 不通 | 设计如此：两端口仅 127.0.0.1 / 内网可达，公网只走 nginx 80 |
