# 部署文档

## 〇、一键部署（最快路径）

适合「拿到一台新服务器，想把 OJ 跑起来」的场景。前置三件事：

1. 服务器装好 Docker（用发行版软件包 `apt install docker.io docker-compose-plugin`
   或按 Docker 官方文档安装，≥ 24）；
2. 开发机构建前端产物并同步：`cd frontend && npm ci && npm run build`，把
   整个仓库（**含 `frontend/dist`**）传到服务器；
3. root shell、cgroup v2（`stat -fc %T /sys/fs/cgroup` 输出 `cgroup2fs`）。

然后一条命令：

```bash
bash deploy/one-click.sh
```

脚本自动完成：前置检查 → 生成 `.env`（openssl 现场随机生成全部密钥、
admin 初始密码和 CLI 确认令牌，已存在的 .env 不会覆盖）→
`docker compose up -d --build` → 等待 API 就绪 → 判题机 `--selftest` →
安装备份/演练 cron → 前端冒烟。结束时打印访问地址、管理员密码
（**立即记下**）、邀请码入口和 **维护 CLI 用法**。

幂等：重复执行安全；`.env` 不会被覆盖。

## 〇-2、服务器内维护 CLI（oj-cli）

部署后如需在服务器上创建/找回超级管理员（例如首次部署想自选密码、
或超管密码丢失被锁在门外），用容器内置的 `oj-cli`：

```bash
# 创建（或提升+重置）超级管理员
docker compose exec api oj-cli --token "$(grep '^OJ_CLI_TOKEN=' .env | cut -d= -f2)" \
  create-superadmin --username newadmin --password 'YourStrongPass1'

# 应急：重置任意用户密码 / 改角色
docker compose exec api oj-cli --token "$T" reset-password --username admin --password 'NewPass123'
docker compose exec api oj-cli --token "$T" set-role --username someone --role setter

# 只读：列用户 / 生成邀请码 / 健康诊断（DB/Redis/判题机）
docker compose exec api oj-cli users --q admin
docker compose exec api oj-cli --token "$T" invite --max-uses 30
docker compose exec api oj-cli doctor
```

安全模型：

- **无网络入口**——CLI 只能通过 `docker compose exec`（即持有服务器
  shell 的人）触达，不新增任何攻击面；
- **二次确认**——除 `users`/`doctor` 外的命令都要求 `--token` 等于
  `.env` 里的 `OJ_CLI_TOKEN`（防误触、防脚本误调）；
- 直连数据库执行（API 服务挂了也能用，正是锁死自救的场景）；
- `create-superadmin` 对已存在用户是「提升+重置+解封」语义，可自救。

systemd（裸机）路线同样内置：二进制在 `/opt/oj/oj-cli`，会自动读取
`/opt/oj/oj.env`（与 oj-api 同一份 EnvironmentFile），直接：

```bash
/opt/oj/oj-cli doctor
OJ_CLI_TOKEN=<token> /opt/oj/oj-cli create-superadmin --username u --password p --token "$OJ_CLI_TOKEN"
```

## 一、方式 A：Docker Compose（手动，推荐）

前置：Linux 服务器（Ubuntu 22.04+ / Debian 12+），Docker ≥ 24，cgroup v2
（`stat -fc %T /sys/fs/cgroup` 输出 `cgroup2fs`）。

```bash
# 1. 准备密钥
cp .env.example .env
vim .env            # 填写全部 change-me 值；openssl rand -base64 32 生成随机串

# 2. 前端产物（本机或 CI 执行一次）
cd frontend && npm install && npm run build && cd ..

# 3. 构建并启动
docker compose up -d --build

# 4. 验证
docker compose logs -f api        # 等待 "http listening"
curl -s http://127.0.0.1:8080/api/v1/languages | head -c 120
```

浏览器访问 `http://<服务器IP>/`，用 `.env` 里的 `OJ_ADMIN_USERNAME/PASSWORD`
登录（仅首次启动时创建）。

## 二、方式 B：systemd（校内服务器裸机部署）

```bash
# 交叉编译产物已在 dist/（在开发机执行，或服务器上安装 Go 1.27 自行编译）
mkdir -p /opt/oj/data
cp dist/oj-api-linux /opt/oj/oj-api && chmod +x /opt/oj/oj-api
cp dist/oj-judge-linux /opt/oj/oj-judge && chmod +x /opt/oj/oj-judge
cp dist/oj-api-linux /opt/oj/oj-api
cp deploy/systemd/*.service /etc/systemd/system/
```

`/opt/oj/oj.env`（API 机）：

```ini
OJ_MODE=prod
OJ_LISTEN=:8080
OJ_GRPC_ADDR=:9090
OJ_DATA_DIR=/opt/oj/data
OJ_DB_DRIVER=postgres
OJ_DB_DSN=host=127.0.0.1 user=oj password=<DB密码> dbname=oj sslmode=disable
OJ_REDIS_ADDR=127.0.0.1:6379
OJ_JWT_SECRET=<随机32字节>
OJ_DAEMON_SECRET=<随机32字节>
OJ_ADMIN_USERNAME=<管理员>
OJ_ADMIN_PASSWORD=<初始密码>
OJ_FETCH_BASE=http://<服务器内网IP>:8080
```

`/opt/oj/oj-judge.env`（判题机）：

```ini
OJ_MODE=prod
OJ_DATA_DIR=/opt/oj/judge-data
OJ_API_ENDPOINT=<API机内网IP>:9090
OJ_FETCH_BASE=http://<API机内网IP>:8080
OJ_DAEMON_NAME=judge-1
OJ_DAEMON_TOKEN=<与API机 OJ_DAEMON_SECRET 相同>
OJ_MAX_PARALLEL=2
OJ_WORK_ROOT=/oj-work
```

```bash
# 先验证判题环境（必须 root）
sudo /opt/oj/oj-judge --selftest
# 期望输出: PASS cgroup v2 writable / PASS basic jailed run / PASS wall-clock watchdog
sudo systemctl enable --now oj-api oj-judge
```

判题机工具链：`apt install build-essential openjdk-17-jdk-headless python3`。

前端（API 机）：`frontend/dist` 交给 Nginx，参考 `deploy/nginx.conf`
（把 `proxy_pass http://api:8080` 改为 `http://127.0.0.1:8080`）。

## 三、备份与恢复（必做）

提供两个脚本（`deploy/` 目录）：

- `backup.sh` — 每日全量：PostgreSQL（pg_dump 自定义格式）+ `data_dir`（testdata/上传物）rsync 快照；
  保留最近 14 天日备 + 最近 6 个月每月 1 号的月备。
- `restore-drill.sh` — 恢复演练：起一个一次性 PostgreSQL 容器把指定备份恢复进去并打印行数，
  验证备份真实可用，不触碰生产。

安装（API 机，裸机 systemd 部署为例；两脚本已部署到生产 <your-server-ip> 并演练通过）：

```bash
cp deploy/backup.sh deploy/restore-drill.sh /opt/oj/
chmod +x /opt/oj/backup.sh /opt/oj/restore-drill.sh
mkdir -p /opt/oj/backup
# crontab：每日 3:00 备份，日志追加到 backup.log
(crontab -l 2>/dev/null; echo '0 3 * * * /opt/oj/backup.sh >> /opt/oj/backup/backup.log 2>&1') | crontab -
# 每月 1 号 4:00 恢复演练（自动取最新 dump）
(crontab -l 2>/dev/null; echo '0 4 1 * * /opt/oj/restore-drill.sh $(ls -t /opt/oj/backup/db/*.dump | head -1) >> /opt/oj/backup/drill.log 2>&1') | crontab -
```

Docker Compose 部署时无需改脚本：`backup.sh` 自动探测 `oj-postgres` 容器名；
裸机则通过 `runuser -u postgres`（peer 认证）导出。`restore-drill.sh` 在有
docker 的机器上用一次性容器演练，否则在本机建临时库 `oj_drill_tmp` 恢复后
删除——两者都不触碰生产库。

手工恢复步骤（灾难场景，bare-metal 为例）：

```bash
# 1. 停服务，避免恢复过程中产生新写入
sudo systemctl stop oj-api oj-judge
# 2. 恢复数据库（root 下用 runuser；有 sudo 的机器用 sudo -u postgres）
systemctl start postgresql
runuser -u postgres -- dropdb --if-exists oj
runuser -u postgres -- createdb oj
runuser -u postgres -- pg_restore -d oj /opt/oj/backup/db/oj-<日期>.dump
# 3. 恢复数据目录（testdata 等）
rsync -a /opt/oj/backup/data/snapshot/ /opt/oj/data/
# 4. 起服务并抽查
systemctl start oj-api oj-judge
curl -s http://127.0.0.1:8080/api/v1/languages | head -c 120
```

> 判题机 `judge-data` 是缓存性质（可由 API 机重新拉取），无需纳入备份；
> `/opt/oj/data`（API 机）才是原始资产。
> 服务器若无 rsync，`backup.sh` 自动降级为 cp 全量拷贝。

## 四、故障排查

| 现象 | 排查 |
|---|---|
| selftest 报 cgroup 不可用 | 内核未启用 cgroup v2 或非 root；`stat -fc %T /sys/fs/cgroup` |
| 判题机一直 offline | `OJ_DAEMON_TOKEN` 与 API 的 `OJ_DAEMON_SECRET` 不一致；gRPC 9090 不通 |
| 提交一直 PENDING | `管理→判题机监控` 看队列长度与判题机状态；判题机日志 grep `session ended` |
| 全部提交 SE | 判题机磁盘满 / `/oj-work` 不可写 / 内核过老（seccomp KILL_PROCESS 需 ≥4.14，老内核自动降级） |
| Java 全部 MLE/RE | 检查判题机是否装了 JDK 17；languages.yaml 的 mem_overhead_mb 是否够 JVM |
| 502/跨域 | Nginx `client_max_body_size`、`/api/v1/ws` 的 Upgrade 头 |
