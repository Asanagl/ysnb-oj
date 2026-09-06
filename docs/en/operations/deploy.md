# Deployment

> **Intended readers**: developers/operators deploying YSNB OJ to their own
> server. Organized by task, each with a verification step. Day-2 operations
> (patrols, releases, incidents) live in `maintenance.md`; the admin console
> in `admin-guide.md`.

Task index: cold deploy → section 2 (A one-click / B bare metal) ｜ add a
judge → 3 ｜ logs → 4 ｜ backups & restore drills → 5 ｜ HTTPS (open item) →
6 ｜ env var reference → 7 ｜ maintenance CLI → 8 ｜ troubleshooting → 9.

## 1. Before you start: requirements

- **OS**: the backend is Linux-only (the sandbox needs cgroup v2 and
  namespaces); Windows/macOS are unsupported. Ubuntu 22.04+ / Debian 12+
  recommended; Debian 11 runs fine in production (seccomp KILL_PROCESS needs
  kernel ≥ 4.14 — older kernels degrade automatically).
- **cgroup v2** (a hard judge dependency) — verify first:
  `stat -fc %T /sys/fs/cgroup` must print `cgroup2fs`.
- **Frontend bundles are built on a dev machine** and synced with the repo;
  **never run npm/go builds on the server** (OOM risk on small boxes):
  `cd frontend && npm ci && npm run build`.
- **Port red line**: 8080 (API HTTP) and 9090 (API gRPC) must only be
  reachable from 127.0.0.1 (or the compose network); only nginx's 80 (and
  optionally 443 later) faces the internet. The compose route publishes api
  on loopback `127.0.0.1:8080/9090` (consumed by the host-side judge and
  probes); the bare-metal route enforces this with a firewall.
- Path choice: **A. one-click Docker Compose** (most cases,
  `deploy/one-click.sh`) or **B. bare metal + systemd** (no Docker).

> **Topology (identical on both routes)**: postgres/redis/api run in
> containers; the **judge runs on the host** (the sandbox talks to cgroup v2
> directly — wrapping it in a container adds no isolation), and web (nginx)
> uses host networking to share `deploy/nginx.conf` verbatim with the
> bare-metal route. `proxy_pass 127.0.0.1:8080` therefore points at the api's
> loopback-published port on both routes — no config drift.

## 2. Deploy from zero on a fresh server

### Route A: one-click Docker Compose (recommended)

Prerequisites: ① Docker ≥ 24 (`apt install docker.io docker-compose-plugin`
or official docs); ② `frontend/dist` built on a dev machine and synced with
the repo; ③ a root shell with cgroup v2 enabled. Then from the repo root:

```bash
bash deploy/one-click.sh
```

The script (stops on any failure with triage pointers; idempotent):

1. Prechecks (docker / `frontend/dist` / cgroup v2);
2. Generates `.env`: PG password, JWT secret, judge shared secret (DAEMON),
   initial admin password and CLI token — all random via openssl; an
   **existing `.env` is never overwritten**;
3. `docker compose up -d --build` (postgres/redis/api/web), polling
   `/api/v1/languages` until the API is ready;
4. Judge: if installed, runs `--selftest` and enables the systemd unit; if
   not, prints a 3-step install guide (below) — the web UI and problem bank
   work without a judge, submissions just stay PENDING;
5. Installs backup cron (daily 03:00 backup + monthly restore drill,
   section 5);
6. Frontend smoke test, then prints the URL, the initial admin password
   (**note it down now**), where to generate invite codes, and oj-cli usage.

Verify:

```bash
docker compose ps    # postgres / redis / api / web all running (web = host network)
docker compose exec api wget -qO- http://127.0.0.1:8080/api/v1/languages   # languages JSON
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1/api/v1/languages # expect 200 via nginx
```

Open `http://<server-ip>/` and log in with `OJ_ADMIN_USERNAME` /
`OJ_ADMIN_PASSWORD` from `.env` (created on first boot only, when the user
table is empty).

**Judge (host systemd) install** — after compose is up, follow section 3.
For a co-located judge:

```ini
OJ_API_ENDPOINT=127.0.0.1:9090
OJ_FETCH_BASE=http://127.0.0.1:8080
```

Without the one-click script, manual compose: `cp .env.example .env` → fill
every `change-me*` (`openssl rand -base64 32`) → `docker compose up -d
--build`; then add the judge (above) and the backup cron (section 5).

### Route B: bare metal + systemd

All commands as root. Install root `/opt/oj/`: `oj-api` / `oj-judge` /
`oj-cli` binaries, `oj.env` / `oj-judge.env` (config, mode 600), `web/`
(frontend static root), `data/` (testdata/code/uploads — the original
asset), `backup/` (backup artifacts and drill logs).

**1. System packages** (PostgreSQL + Redis + nginx + judge toolchain; for a
separate judge machine the toolchain only goes there, section 3):

```bash
apt update
apt install -y postgresql redis-server nginx \
  build-essential openjdk-17-jdk-headless python3
```

**2. Database & system user**:

```bash
runuser -u postgres -- psql -c "CREATE ROLE oj LOGIN PASSWORD '<DB密码>';"
runuser -u postgres -- createdb -O oj oj
useradd -r -d /opt/oj -s /usr/sbin/nologin oj || true
mkdir -p /opt/oj/data /opt/oj/web /opt/oj/backup
```

Verify: `runuser -u postgres -- psql -d oj -c 'select 1'` returns a row.

**3. Binaries & frontend** (artifacts in `dist/` and `frontend/dist/`,
built on the dev machine — the server never runs go/npm):

```bash
cp dist/oj-api-linux   /opt/oj/oj-api
cp dist/oj-judge-linux /opt/oj/oj-judge
cp dist/oj-cli-linux   /opt/oj/oj-cli
chmod +x /opt/oj/oj-api /opt/oj/oj-judge /opt/oj/oj-cli
cp -r frontend/dist/. /opt/oj/web/
chown -R oj:oj /opt/oj/data
```

**4. Configuration** (`chmod 600` both; full variable table in section 7).

`/opt/oj/oj.env`:

```ini
OJ_MODE=prod
OJ_LISTEN=:8080
OJ_GRPC_ADDR=:9090
OJ_DATA_DIR=/opt/oj/data
OJ_FETCH_BASE=http://127.0.0.1:8080
OJ_DB_DRIVER=postgres
OJ_DB_DSN=host=127.0.0.1 user=oj password=<DB密码> dbname=oj sslmode=disable
OJ_REDIS_ADDR=127.0.0.1:6379
OJ_JWT_SECRET=<openssl rand -base64 32>
OJ_DAEMON_SECRET=<openssl rand -base64 32>
OJ_CLI_TOKEN=<openssl rand -hex 16>
OJ_ADMIN_USERNAME=admin
OJ_ADMIN_PASSWORD=<initial admin password, first boot only>
OJ_LOG_LEVEL=info
```

`/opt/oj/oj-judge.env`:

```ini
OJ_MODE=prod
OJ_DATA_DIR=/opt/oj/data
OJ_API_ENDPOINT=127.0.0.1:9090
OJ_FETCH_BASE=http://127.0.0.1:8080
OJ_DAEMON_NAME=judge-1
OJ_DAEMON_TOKEN=<same value as OJ_DAEMON_SECRET in oj.env>
OJ_MAX_PARALLEL=1
OJ_WORK_ROOT=/oj-work
OJ_LOG_LEVEL=info
```

> `OJ_MAX_PARALLEL`: **small-memory machines must set 1** (each parallel slot
> peaks around 1 GB of compile cgroup; a 3.9 GB box requires 1 in practice).
> Fatter machines can raise it. Note the judge env also needs
> `OJ_JWT_SECRET` set — config loading requires it even on the judge.

**5. Judge environment self-test** (mandatory, as root):

```bash
/opt/oj/oj-judge --selftest
# expect three PASS lines (cgroup v2 writable / basic jailed run (/bin/echo) /
# wall-clock watchdog (100ms limit)), then == all selftests passed ==
```

Resolve any FAIL before continuing (usual causes: cgroup v2 disabled, not
root).

**6. systemd units**:

```bash
cp deploy/systemd/oj-api.service deploy/systemd/oj-judge.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now oj-api oj-judge
```

The units' permission model is **intentional — do not "harden" it**:
`oj-api` runs as user `oj` with `NoNewPrivileges` +
`ProtectSystem=strict` + `ReadWritePaths=/opt/oj/data` (a plain web
service, least privilege); `oj-judge` must run as **root** and the unit
**deliberately omits** systemd sandboxing directives — sandbox isolation
(namespaces/cgroup/privilege drop) is created by the judging process itself
for every task, and systemd-level restrictions would break it.

Verify:

```bash
systemctl is-active oj-api oj-judge    # active / active
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/api/v1/languages   # expect 200
```

**7. nginx** (template `deploy/nginx.conf`: static `/opt/oj/web` + `/api`
reverse proxy + `/api/v1/ws` WebSocket upgrade + the four security headers):

```bash
cp deploy/nginx.conf /etc/nginx/conf.d/oj.conf
nginx -t && systemctl reload nginx
```

Verify: `curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1/` → 200;
log in as admin in the browser and confirm judge-1 shows online under the
judge monitor.

**8. Wrap up**: configure logs (section 4) and backups (section 5).

## 3. Add a judge machine

A co-located judge is covered by section 2; this section is for a
**standalone judge** (or a second one for capacity). As root:

1. Install the toolchain and confirm cgroup v2:

   ```bash
   apt update && apt install -y build-essential openjdk-17-jdk-headless python3
   stat -fc %T /sys/fs/cgroup   # must print cgroup2fs
   ```

2. Copy the judge binary to `/opt/oj/oj-judge` and write
   `/opt/oj/oj-judge.env` (chmod 600):

   ```ini
   OJ_MODE=prod
   OJ_API_ENDPOINT=<API host intranet IP>:9090
   OJ_FETCH_BASE=http://<API host intranet IP>:8080
   OJ_DAEMON_NAME=judge-2          # unique per judge; shown on the monitor
   OJ_DAEMON_TOKEN=<same as the API's OJ_DAEMON_SECRET>
   OJ_MAX_PARALLEL=1
   OJ_WORK_ROOT=/oj-work
   OJ_LOG_LEVEL=info
   OJ_JWT_SECRET=<any value; config loading requires it>
   ```

3. Firewall on the API machine: gRPC 9090 and the testdata download 8080 must
   be reachable from the judge's **intranet IP** (firewall layer, never the
   public internet). Bare metal: oj-api listens on all interfaces by default —
   constrain with ufw/iptables. Compose: api is loopback-published only —
   change the publish address to the intranet IP for cross-machine judges
   (`ports: ["<IP>:9090:9090", ...]`) and firewall likewise.
4. **Mandatory self-test**: `/opt/oj/oj-judge --selftest` (expected output in
   route B step 5).
5. Judge unit only:
   `cp deploy/systemd/oj-judge.service /etc/systemd/system/` →
   `systemctl daemon-reload && systemctl enable --now oj-judge`.

Verify: 「判题机监控」in the admin console shows judge-2 online; submit an
A+B and it returns AC/WA instead of staying PENDING. A permanently offline
judge is almost always `OJ_DAEMON_TOKEN` ≠ the API's `OJ_DAEMON_SECRET`.

## 4. Configure logging

Both processes (oj-api / oj-judge) emit **single-line JSON** via logx
(log/slog) to stderr, collected by systemd into journald; levels map onto
journald PRIORITY, so `journalctl -p err` filters without grep.

**Level**: `OJ_LOG_LEVEL` (`debug|info|warn|error`, default `info`; unknown
values silently fall back to info). For triage switch to debug — per-case
time/memory and the compile stderr tail appear: add `OJ_LOG_LEVEL=debug` to
`oj.env` or `oj-judge.env`, `systemctl restart oj-api oj-judge`, and revert
to `info` afterwards.

**journald disk cap** (mandatory in production so logs cannot crowd out
data):

```bash
mkdir -p /etc/systemd/journald.conf.d
printf '[Journal]\nSystemMaxUse=200M\nCompress=yes\n' \
  > /etc/systemd/journald.conf.d/99-oj.conf
systemctl restart systemd-journald
```

Verify: `journalctl --disk-usage` stays under 200M; `journalctl -u oj-api -n 5`
shows JSON lines.

**Web log viewer** (admin console → 日志查看器, admin+): the backend calls
`journalctl` with a fixed argument set to read recent logs of
`oj-api.service` / `oj-judge.service`; filtering happens server-side (no
shell, no injection surface). Prerequisite: add the API's runtime user to
the journald read group, or the viewer 500s:

```bash
usermod -aG systemd-journal oj && systemctl restart oj-api
```

Verify: the viewer lists oj-api logs; filtering by `submission judged` finds
hits. The viewer depends on journald — **systemd route only**.

**Triage anchor**: every finished submission logs one INFO line, searchable
by submission id / verdict:

```
submission judged  submission=<id> status=<verdict> time_ms=… memory_kb=… score=… problem=…
```

SE gets its own ERROR line; a judge disconnect logs WARN `daemon
disconnected`. CLI fallback (anyone with SSH):

```bash
journalctl -u oj-api -f                    # API live
journalctl -u oj-judge -p err -S -1h       # judge errors, last hour
journalctl -u oj-api -o json | grep submission   # structured search
```

On the compose route the api runs in a container (`docker compose logs -f
api`, same single-line JSON) while **the judge stays on the host** with
journald — judge-side viewer/journalctl usage is identical on both routes;
only the api swaps to `docker compose logs`.

## 5. Backups & restore drills

Two scripts (in `deploy/`):

- `backup.sh` — full backup: PostgreSQL (`pg_dump -Fc` custom format,
  `pg_restore`-able) + an `/opt/oj/data` snapshot. Keeps 14 daily + the 1st
  of each of the last 6 months (`KEEP_DAYS` / `KEEP_MONTHS` in the script
  header).
- `restore-drill.sh <dump>` — restore drill: with docker, spins up a
  throwaway PostgreSQL container, restores for real and prints per-table row
  counts; without docker, builds a temporary `oj_drill_tmp` database locally
  (dropped afterwards). **Neither path ever touches production** — a backup
  "existing" is not "restorable"; the drill is the only proof.

Install (API machine; one-click already installs these — skip to verify):

```bash
install -m 755 deploy/backup.sh deploy/restore-drill.sh /opt/oj/
mkdir -p /opt/oj/backup
( crontab -l 2>/dev/null | grep -v 'backup.sh\|restore-drill.sh' || true;
  echo '0 3 * * * /opt/oj/backup.sh >> /opt/oj/backup/backup.log 2>&1';
  echo '0 4 1 * * /opt/oj/restore-drill.sh $(ls -t /opt/oj/backup/db/*.dump 2>/dev/null | head -1) >> /opt/oj/backup/drill.log 2>&1' ) | crontab -
```

Verify (run both manually right after installing — don't wait for cron):

```bash
/opt/oj/backup.sh
ls -lh /opt/oj/backup/db/            # a non-empty oj-<date>.dump
/opt/oj/restore-drill.sh "$(ls -t /opt/oj/backup/db/*.dump | head -1)"
# expect the last line: == drill PASSED (backup is restorable) ==
```

Both scripts adapt to either deployment: an `oj-postgres` container triggers
`docker exec … pg_dump` (compose), otherwise `runuser -u postgres` peer auth
(bare metal); hosts without rsync fall back to a cp full copy.

Manual disaster recovery (bare-metal example):

```bash
systemctl stop oj-api oj-judge                            # 1. stop writes
runuser -u postgres -- dropdb --if-exists oj              # 2. restore DB
runuser -u postgres -- createdb oj
runuser -u postgres -- pg_restore -d oj /opt/oj/backup/db/oj-<date>.dump
rsync -a /opt/oj/backup/data/snapshot/ /opt/oj/data/      # 3. restore data dir
systemctl start oj-api oj-judge                           # 4. start services
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/api/v1/languages   # 200
```

> The judge's work dirs (`/oj-work`, judge-data) are caches re-fetchable from
> the API machine — **no backup needed**; `/opt/oj/data` (API machine) is the
> original asset.

## 6. HTTPS (optional open item)

**Port 443 being closed is a deliberate open item, not a fault** — don't
"fix" it as a bug and don't force it without a certificate. HTTP is fine for
campus/intranet use; add TLS when exposing publicly. Two common options
(either one):

- **Caddy**: easiest — automatic certificate issuance/renewal, reverse
  proxying to `127.0.0.1:80`;
- **certbot + existing nginx**: `apt install -y certbot python3-certbot-nginx`
  → `certbot --nginx -d <domain>`.

After enabling TLS:

1. Uncomment the HSTS line reserved in `deploy/nginx.conf`
   (`# enable after TLS: add_header Strict-Transport-Security …`). Note
   nginx's `add_header` does not inherit into locations that declare their
   own `add_header` (explained in the file header) — HSTS must land on the
   443 server block.
2. WebSocket needs nothing extra: the CSP `connect-src` already includes
   `wss:`.

Verify (and check the padlock in a browser, with contest WS still live):

```bash
curl -sI https://<domain>/api/v1/languages | head -1    # 200
curl -sI https://<domain>/ | grep -i strict-transport   # HSTS present
```

## 7. Environment variable reference

Authoritative source: `backend/internal/config/config.go`; every variable
overrides the YAML config (`--config` / `OJ_CONFIG` select a YAML file —
pure env is the recommended ops posture).

Shared (read by both api and judge):

| Variable | Default | Notes |
|---|---|---|
| `OJ_MODE` | `dev` | `dev` = sqlite + in-memory queue, zero deps; production must be `prod` |
| `OJ_LISTEN` | `:8080` | API HTTP listen address |
| `OJ_GRPC_ADDR` | `:9090` | API gRPC listen address (judges connect here) |
| `OJ_DATA_DIR` | `./data` | Root for testdata / code / uploads |
| `OJ_FETCH_BASE` | `http://127.0.0.1:8080` | Base URL judges use to download testdata; use the API host's intranet address for remote judges |
| `OJ_LOG_LEVEL` | `info` | `debug\|info\|warn\|error`; debug adds per-case detail (section 4) |
| `OJ_DB_DRIVER` / `OJ_DB_DSN` | sqlite | Production: `postgres` + PG DSN |
| `OJ_REDIS_ADDR` | — | Submission queue / cache (required in prod) |
| `OJ_JWT_SECRET` | — | **required in prod**; startup refused without it |
| `OJ_JWT_EXPIRE_HOURS` | `72` | Login session lifetime (hours) |
| `OJ_DAEMON_SECRET` | — | Judge shared secret (API side) |
| `OJ_ADMIN_USERNAME` / `OJ_ADMIN_PASSWORD` | — | Creates the admin on **first boot only** (empty user table); the initial admin is uid 0, super_admin |
| `OJ_CLI_TOKEN` | — | Confirmation token for mutating oj-cli commands (section 8) |
| `OJ_LANGUAGES_FILE` | — | Judge override for the built-in languages.yaml (.yaml/.yml only) |

Judge-only:

| Variable | Default | Notes |
|---|---|---|
| `OJ_API_ENDPOINT` | — | API gRPC `host:port` |
| `OJ_DAEMON_NAME` | — | Display name on the monitor, unique per judge |
| `OJ_DAEMON_TOKEN` | — | Must equal the API's `OJ_DAEMON_SECRET` |
| `OJ_MAX_PARALLEL` | `2` | Parallel judge slots; **1 on small-memory machines** (a 3.9 GB box requires 1), raise on fatter ones |
| `OJ_WORK_ROOT` | `/oj-work` | Judge scratch dir; never under /home or data_dir |

The compose route also uses `OJ_PG_USER` / `OJ_PG_PASSWORD` (database
bootstrap) — see `.env.example`.

## 8. Maintenance CLI (oj-cli)

In-server maintenance tool: recover/create the super admin, reset
passwords, change roles, invite codes, health diagnostics. **Talks straight
to the database and works even when the API is down** — exactly the
locked-out scenario. Security model: no network surface (only someone with
a server shell can run it); apart from read-only `users` / `doctor`, every
command requires `--token` to equal the configured `OJ_CLI_TOKEN` (a
second confirmation against typos).

Compose route (binary baked into the image):

```bash
docker compose exec api oj-cli doctor        # DB / Redis / judge health
T=$(grep '^OJ_CLI_TOKEN=' .env | cut -d= -f2)
docker compose exec api oj-cli --token "$T" create-superadmin --username newadmin --password '<strong password>'
docker compose exec api oj-cli --token "$T" reset-password --username admin --password '<new password>'
docker compose exec api oj-cli --token "$T" set-role --username someone --role setter
```

Other subcommands: `users --q <keyword>` (read-only listing), `invite
--max-uses 30` (generate an invite code).

systemd route: the binary at `/opt/oj/oj-cli` auto-reads
`/opt/oj/oj.env` at startup (real env wins); run from a root shell:

```bash
/opt/oj/oj-cli doctor
/opt/oj/oj-cli create-superadmin --username admin --password '<new password>' \
  --token "$(grep '^OJ_CLI_TOKEN=' /opt/oj/oj.env | cut -d= -f2)"
```

`create-superadmin` on an existing user means promote + reset password +
unban — the self-recovery path for a lost/locked super-admin password.

## 9. Troubleshooting

| Symptom | Triage |
|---|---|
| selftest says cgroup unavailable | cgroup v2 disabled or not root; `stat -fc %T /sys/fs/cgroup` should print `cgroup2fs` |
| Judge permanently offline | `OJ_DAEMON_TOKEN` ≠ API's `OJ_DAEMON_SECRET`; gRPC 9090 unreachable (firewall/security group) |
| Submissions stuck PENDING | Queue & judges in the monitor; `journalctl -u oj-judge -n 50` |
| Everything SE | Judge disk full / `/oj-work` unwritable / kernel too old (seccomp KILL_PROCESS needs ≥ 4.14; older kernels degrade automatically) |
| Judge OOM / stuck | `OJ_MAX_PARALLEL` beyond memory capacity; 3.9 GB boxes must use 1 |
| Java all CE/MLE/RE | JDK 17 missing on the judge (`which javac`); or languages.yaml `mem_overhead_mb` too small for the JVM |
| 502 / WebSocket drops | Is oj-api alive behind nginx; is the `/api/v1/ws` Upgrade header stripped by a middlebox |
| Web log viewer 500 | oj user missing from `systemd-journal` (section 4); no journald on compose for the api |
| Logs eating disk | journald `SystemMaxUse` (section 4), default 200M here |
| External curl to 8080/9090 fails | By design: both ports are loopback/intranet-only; the public internet only sees nginx 80 |
