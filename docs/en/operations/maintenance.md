# Ops runbook (maintenance)

> For the **on-call operator / successor**. Organized by task, each with
> prerequisites, steps and verification. First deployment:
> [deploy.md](deploy.md); user-facing operations:
> [admin-guide.md](../guide/admin-guide.md); sandbox deep dive:
> [judge-sandbox.md](../development/judge-sandbox.md).
>
> Two red lines:
>
> 1. **Never run npm / vite / go builds on the server** — 3.9 GB of RAM plus
>    a balloon device grabbing memory back has caused two OOM incidents.
>    Build on the dev machine, ship artifacts only.
> 2. **No credentials in documents**. No real passwords/keys appear here;
>    values live in local `.zcode` records and `remote-test/.sshenv` /
>    `.ojenv` / `admin.env` (all gitignored).

## 1. Server quick reference

| Item | Value |
|---|---|
| Host | `<your-server-ip>`, Debian 12 (bookworm) single box, kernel 6.1, 3.9 GB RAM |
| Services | Docker Compose: postgres / redis / api / web (`restart: unless-stopped`, start with the host); api published on loopback 127.0.0.1:8080/9090 only, web on host network :80. Host side: `oj-judge` (root, gRPC to api) + optional `oj-audit` / `oj-audit-watcher` (BPF LSM audit layer, log-only, see `deploy/lsm-audit/`) |
| Firewall | ufw: default deny incoming, only 22/80/443 allowed |
| Binaries | `/opt/oj/oj-judge`; api lives inside the compose image — maintenance CLI via `docker compose exec api oj-cli ...` |
| Config | `/root/ysnb-oj/.env` (all compose secrets), `/opt/oj/oj-judge.env` (mode 600; restart the service to apply) |
| Frontend | compose web container (nginx:1.27-alpine) mounting the repo `frontend/dist`; release = sync dist then `docker compose restart web` |
| Data | docker volumes pgdata (database) / redisdata / ojdata (testdata etc., `/oj-data` in the container); judge workspace `/oj-work` |
| Backups | `/opt/oj/backup/` (db/ + data/snapshot/ + backup.log + drill.log) |
| Cron | root crontab: 03:00 daily backup; 04:00 on the 1st, restore drill |
| Units | `/etc/systemd/system/oj-judge.service` + `oj-judge.service.d/cgroup-cpu.conf` (required on Debian 12, see below); optional `oj-audit*.service` (sources in `deploy/systemd/` and `deploy/lsm-audit/`) |

> Debian 12 note: `oj-judge.service.d/cgroup-cpu.conf` enables the
> memory/pids/cpu controllers on the root cgroup and the judge base
> before the judge starts (systemd 252 enables root controllers lazily
> during boot; without it an early-boot judge turns every submission
> into SE). Harmless and idempotent on Debian 11.

### SSH hardening baseline (effective 2026-09-04)

- root login with ed25519 keys only: `PasswordAuthentication no` +
  `PermitRootLogin prohibit-password` + `KbdInteractiveAuthentication no`,
  in drop-in `/etc/ssh/sshd_config.d/00-oj-hardening.conf`
  (first match in drop-ins wins over the main config).
- fail2ban is not installed: key-only login removed the brute-force surface.
  If wanted: `apt install fail2ban` and enable the sshd jail.
- Change order red line: install the new public key and **verify key login
  from a fresh terminal** before touching sshd config; then `sshd -t` →
  `systemctl reload ssh` (does not drop current sessions) → verify from a
  new connection both ways (key works / password rejected).
- Rollback: remove the drop-in (or restore the
  `/etc/ssh/sshd_config.bak-*` backup), then `systemctl reload ssh`.
- Audit trail: [security-audit](/archive/security-audit) §13.4
  (read-only archive).

## 2. Release an update

All releases go through the **key-based remote-test scripts, run on the dev
machine**; no builds happen on the server. Old remote-test push/deploy
scripts not listed here are password-SSH era leftovers — do not use. The
scripts read `OJ_SSH_HOST` / `OJ_SSH_KEY` env vars (host and private key
path), falling back to the built-in defaults in their headers.

### Frontend release

Prerequisite (dev machine): the build passes in `frontend/`.

```bash
cd frontend && npm run build && cd ..
node remote-test/m5-frontend-sync-key.mjs
```

What it does: tars local `frontend/dist` → base64 over the key SSH channel →
server-side sha256 verification (`SHA_MISMATCH` aborts) → atomic swap of
`/opt/oj/web` (old dir kept as `/opt/oj/web.old`). nginx serves pure static
files, no reload needed.

Verify: the script prints `index HTTP=200` at the end; hard-refresh
(Ctrl+F5) in the browser to confirm (index.html is no-cache, a normal
refresh picks up the new entry).

Rollback (on the server):

```bash
rm -rf /opt/oj/web && mv /opt/oj/web.old /opt/oj/web
```

### Backend release (api image / oj-judge binary)

The api ships as a compose image: after changing `backend/` sources, sync
them to the server repo and run:

```bash
cd /root/ysnb-oj && docker compose build api && docker compose up -d api
```

(Go builds inside the image; expect 5-15 minutes on the 3.9 GB box.
`restart: unless-stopped` keeps it self-healing across host reboots.)

The judge is a host-side single binary — cross-compile on the dev machine:

```bash
cd backend
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ../dist/oj-judge-linux ./cmd/judge
cd ..
node remote-test/oj-push-one-binary.mjs judge
```

What it does: gzip+base64 over the key SSH channel → server-side sha256
verification → lands as `/opt/oj/oj-judge.new`. **The script does not swap
or restart**; `PLACED` only means transfer and verification succeeded.
Stop the judge before upgrading (systemd holds the old file; overwriting
fails with Text file busy):

```bash
systemctl stop oj-judge
mv -f /opt/oj/oj-judge.new /opt/oj/oj-judge
systemctl start oj-judge
```

Verify (on the server):

```bash
systemctl is-active oj-judge                                              # active
docker compose ps                                                         # four containers running
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8080/api/v1/languages   # 200
journalctl -u oj-judge -n 20 --no-pager                                   # judge reconnected
```

Finish by submitting an A+B in the admin console to confirm the judging
path end to end. `node remote-test/m5-health.mjs` (dev machine) automates
the checks above.

### Variants (dev machine)

- Push small files (env, scripts, config samples):
  `node remote-test/push-small.mjs <local> <server absolute path>`.
- Frontend release: `node remote-test/m5-frontend-sync-key.mjs` (sync dist,
  then `docker compose restart web` on the server).
- nginx config: the web container mounts the repo `deploy/nginx.conf`; edit
  it, sync the repo on the server, then `docker compose restart web` (no
  host nginx).

All scripts use the SSH key channel (default `~/.ssh/id_ed25519`; override
with `OJ_SSH_KEY` / `OJ_SSH_HOST`). Password login on the server has been
disabled since 2026-09-04.

## 3. Triage a judging SE

Prerequisite: the failing submission id (from the submissions page URL or
the admin console).

1. **Locate via logs**. Without SSH: admin console → 「日志查看器」log viewer
   (admin+), process 「判题机」, keyword = submission id or `SE`. CLI (on the
   server):

   ```bash
   journalctl -u oj-judge -p err -S -1h --no-pager    # judge errors, last hour
   docker compose logs api --since 24h 2>&1 | grep "submission judged"
   ```

2. **Find the preserved workspace**. CE/SE/RE workspaces are kept
   automatically (the judge-side crash dump); the log carries a
   `workspace kept at ...` line:

   ```bash
   ls -dt /oj-work/judge-* | head    # newest first, find the one you need
   ```

   Inside: the user's source (`main.*`) and other scene files for a full
   post-mortem.

3. **Per-case detail when needed**: add `OJ_LOG_LEVEL=debug` to
   `/opt/oj/oj-judge.env` → `systemctl restart oj-judge` → reproduce with a
   fresh submission. Revert to `info` after triage (debug prints per-case
   time/memory and the compile stderr — verbose, don't leave it on).

4. If a **whole language** goes SE/RE with empty stderr → likely a seccomp
   kill: `journalctl -u oj-judge | grep SIGSYS`, then follow the strace flow
   in [judge-sandbox.md](../development/judge-sandbox.md) to find the
   missing syscall.

Verify: after the fix the same code passes and a matching `submission judged`
line appears with a normal status; delete triaged `/oj-work/judge-*`
workspaces manually (successful ones are auto-deleted).

## 4. Patrol (5-minute checklist)

On the server:

```bash
docker compose ps                               # four containers running
systemctl is-active docker oj-judge             # all active (plus oj-audit oj-audit-watcher with the audit layer)
tail -5 /opt/oj/backup/backup.log        # today's 03:00 backup succeeded, dump size sane
df -h /                                  # disk < 80%
free -m                                  # 3.9GB box — watch available
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1/api/v1/languages   # 200
docker compose logs api --since 24h 2>&1 | grep "submission judged" | tail -5
journalctl -u oj-judge -p err -S today --no-pager | tail -20
```

In the browser: judge monitor shows judge-1 online with an empty queue;
after the 1st of the month, `tail /opt/oj/backup/drill.log` ends with
`drill PASSED`. With the audit layer installed also check
`systemctl is-active oj-audit oj-audit-watcher` and
`cat /sys/kernel/tracing/trace_pipe | grep -c oj-audit` (grows while
judging happens).

Pass criterion: everything above normal. Any failure → incident table in
§9.

## 5. Service management & log quick reference

```bash
docker compose restart api    # required after changing .env
systemctl restart oj-judge    # after changing oj-judge.env / installing compilers
```

oj-judge connects to the compose api over gRPC (127.0.0.1:9090): restarting
the api briefly disconnects the judge (WARN `daemon disconnected`) and it
reconnects automatically; in-flight tasks are reclaimed by the 15-minute
lease and requeued. Touching both: **api first, then oj-judge** (restarting
both at once also self-heals, just with an extra reconnect cycle).

- **Site-wide 502**: the api behind the web container died →
  `docker compose logs api --n 100` (usual causes: broken .env edit,
  missing JWT secret failing startup).
- **`/api/v1/ws` returns 503**: WebSocket connection cap (1024) reached →
  `docker compose restart api` clears connections; if frequent, look for a
  connection leak.

### Log quick reference

- Both components log structured JSON (logx/slog) → stderr: the judge goes
  to stderr → journald (readable via journalctl); the compose api goes to
  container logs (`docker compose logs api`), default level `info`.
- Without SSH: admin console → 「日志查看器」log viewer reads the judge's
  journald logs (plus oj-api on the systemd route) with server-side
  unit/keyword filtering; on the compose route the api logs are viewed
  with `docker compose logs api` on the server.
- CLI (on the server):

  ```bash
  docker compose logs api -f             # API live
  journalctl -u oj-judge -f               # judge live
  journalctl -u oj-judge -p err -S -1h    # judge errors, last hour
  ```

- Temporary debug: api `OJ_LOG_LEVEL=debug` in `.env` then
  `docker compose up -d api`; judge: in `/opt/oj/oj-judge.env` →
  restart oj-judge → revert after triage.
- Disk: journald cap `SystemMaxUse=200M`
  (`/etc/systemd/journald.conf.d/99-oj.conf`); manual shrink with
  `journalctl --vacuum-size=100M`.
- Triage anchors: one INFO `submission judged` per finished submission
  (submission/status/time_ms/memory_kb/score); SE logs an additional ERROR
  `submission judged SE`; judge disconnects log WARN `daemon disconnected`.

## 6. Free up disk

Prerequisite: `df -h /` confirms pressure; locate the big consumer with
`du -xh --max-depth=1 / 2>/dev/null | sort -h | tail` first.

| Target | Location | Action |
|---|---|---|
| Preserved judge workspaces | `/oj-work/judge-*` | CE/SE/RE scenes; once nothing is being triaged, clear those older than 7 days: `find /oj-work -maxdepth 1 -name 'judge-*' -mtime +7 -exec rm -rf {} +` |
| Blob cache | `/opt/oj/data/judge-cache` | Safe to empty (refetched on demand); **do not delete** `/opt/oj/data/testdata` itself |
| Backups | `/opt/oj/backup` | `/opt/oj/backup.sh` keeps 14 daily + 6 monthly; if still growing, check `KEEP_DAYS/KEEP_MONTHS` in the script header, then delete expired dumps |
| journald | `/var/log/journal` | The 200M cap self-enforces; manual `journalctl --vacuum-size=100M` |

Verify: `df -h /` back under 80%; `docker compose ps` four containers and
`systemctl is-active oj-judge` all still running/active.

## 7. Restore a backup

### Verify a backup is usable (risk-free, anytime, on the server)

```bash
/opt/oj/restore-drill.sh "$(ls -t /opt/oj/backup/db/*.dump | head -1)"
```

Spins up a throwaway environment, restores the dump and prints per-table
row counts — production untouched (cron already runs it at 04:00 on the
1st; results in `drill.log`).

### Disaster recovery (real production rollback, on the server)

Prerequisites: confirm the backup date to restore to; announce a downtime
window to users.

```bash
# 1. Stop services to avoid new writes during the restore
docker compose stop api
systemctl stop oj-judge
# 2. Restore the database (postgres container in compose)
docker compose exec postgres psql -U oj -c 'drop database if exists oj'
docker compose exec postgres psql -U oj -c 'create database oj'
cat /opt/oj/backup/db/oj-<date>.dump | docker compose exec -T postgres pg_restore -U oj -d oj
# 3. Restore the data dir (testdata etc.)
rsync -a /opt/oj/backup/data/snapshot/ /opt/oj/data/
# 4. Start services
docker compose start api
systemctl start oj-judge
```

Verify: `curl -s http://127.0.0.1:8080/api/v1/languages` returns 200; admin
logs in and spot-checks problems and submissions; submit an A+B to confirm
the judging path. The judge's judge-data is a cache — no restore needed, it
refetches automatically.

## 8. Rotate keys / credentials

Values live in local `.zcode` records and
`remote-test/.sshenv|.ojenv|admin.env` (gitignored). Random values:
`openssl rand -base64 32`.

### SSH key rotation (on the server, following the §1 red-line order)

1. **Append** the new public key to `/root/.ssh/authorized_keys` (keep the
   old one for now);
2. **From a new terminal** verify the new key logs in (keep the current
   session open);
3. Once confirmed, delete the old key line — no sshd config change needed;
4. If you really changed sshd config: `sshd -t` → `systemctl reload ssh` →
   verify from a new connection both ways. Rollback: delete
   `/etc/ssh/sshd_config.d/00-oj-hardening.conf` → reload.

### Admin app password (on the server)

```bash
cd /root/ysnb-oj
T=$(grep '^OJ_CLI_TOKEN=' .env | head -1 | cut -d= -f2-)
docker compose exec api oj-cli create-superadmin --username <admin account> --password '<new password>' --token "$T"
```

On an existing user this means promote + reset + unban — also the
self-recovery path for a locked-out super admin, see
[deploy.md](deploy.md) section 8.

### Database oj role password (on the server)

```bash
docker compose exec postgres psql -U oj -c "ALTER ROLE oj PASSWORD '<new password>'"
# update OJ_PG_PASSWORD in .env, then
cd /root/ysnb-oj && docker compose up -d api   # recreate the api container with the new password
```

### JWT / judge secrets (on the server)

`OJ_JWT_SECRET` and `OJ_DAEMON_SECRET` in `/root/ysnb-oj/.env`;
`OJ_DAEMON_TOKEN` in `/opt/oj/oj-judge.env` must equal `OJ_DAEMON_SECRET`.
After editing .env run `docker compose up -d api`; change both sides
together, then `systemctl restart oj-judge`. Note:
rotating JWT invalidates every login session immediately; mismatched daemon
secrets take the judge offline instantly.

Verify: `/api/v1/languages` 200, judge monitor online, admin logs in with
the new password.

## 9. Incident quick reference

| Symptom | Action |
|---|---|
| All submissions PENDING | Judge monitor: judge-1 offline? → `journalctl -u oj-judge -n 50`; `OJ_DAEMON_TOKEN` ≠ `OJ_DAEMON_SECRET` is the usual cause |
| One language all CE | Missing compiler on the judge: `apt install openjdk-17-jdk-headless` etc., then `systemctl restart oj-judge` |
| One language all SE/RE, empty stderr | Sandbox seccomp kill: `journalctl -u oj-judge \| grep SIGSYS`, locate via strace per [judge-sandbox.md](../development/judge-sandbox.md) |
| Submission stuck JUDGING | The 15-minute lease reclaims and requeues it; or `systemctl restart oj-judge` triggers reconnect + reclaim |
| Site-wide 502 | compose api dead: `docker compose logs api --n 100` (usual: broken .env, missing JWT secret failing startup) |
| WS 503 | Connection cap 1024 hit: `docker compose restart api` (§5) |
| Database unreachable | postgres container down or password ≠ DSN: `docker compose exec postgres psql -U oj -d oj -c 'select 1'` |
| Super admin password lost/locked | oj-cli self-recovery, §8 |
| Containers missing after a reboot | Check `docker compose ps` and `restart: unless-stopped` (built in since PR #12); `docker compose up -d` to start manually |
| Judge all-SE after a reboot | Debian 12 lazy root controller enabling was skipped: confirm `oj-judge.service.d/cgroup-cpu.conf` is in place (§1), `systemctl restart oj-judge` |
| Disk full | Clean per §6; **never** delete the testdata itself (inside the ojdata volume) |
| Server OOM / mysteriously slow | 3.9 GB box: confirm nobody is running npm/go builds on it (two incidents); keep `OJ_MAX_PARALLEL` at 1 |
| Suspected intrusion | `last -f /var/log/wtmp`, `ss -tnp`, check the web container access logs for odd IPs; full rotation per §8 |

## 10. Periodic maintenance

| Cadence | Task |
|---|---|
| Daily (auto) | 03:00 backup (cron); glance at `backup.log` during patrols |
| Weekly | §4 patrol + `apt update && apt upgrade`; after a reboot walk the §4 patrol once (compose and oj-judge both auto-start) |
| Monthly | `drill.log` ends with `drill PASSED`; walk one backup through a full restore drill |
| Quarterly | Assess disk growth (testdata/backups); consider a second judge (judge-sandbox.md) |
| Staff changes | Full rotation per §8: SSH keys, admin, DB, JWT/daemon secrets; update the handover doc |

## 11. Config change index

| What | Where | Applied by |
|---|---|---|
| Parallel judge slots | `OJ_MAX_PARALLEL` in `/opt/oj/oj-judge.env` (production = 1, careful raising on 3.9 GB) | restart oj-judge |
| Log level | api: `OJ_LOG_LEVEL` in `.env` then `docker compose up -d api`; judge: `OJ_LOG_LEVEL` in `oj-judge.env` | recreate api container / restart oj-judge |
| Session lifetime | `OJ_JWT_EXPIRE_HOURS` in `.env` | `docker compose up -d api` |
| journald disk cap | `SystemMaxUse` in `/etc/systemd/journald.conf.d/99-oj.conf` | restart systemd-journald |
| Backup retention | `KEEP_DAYS/KEEP_MONTHS` in `/opt/oj/backup.sh` header | next backup |
| nginx | source file `deploy/nginx.conf` (mounted by the web container); sync the repo on the server | `docker compose restart web` |
| SSH policy | `/etc/ssh/sshd_config.d/00-oj-hardening.conf` | `sshd -t && systemctl reload ssh` |
| Time multipliers / memory / new language | `backend/pkg/judge/languages.yaml` + toolchain on the judge | ship a new oj-judge binary (§2) |
| Rate limits | `backend/internal/handler/middleware.go` (login/register), `submissions.go` (submit) | rebuild the api image (§2) |
| Audit hook set / allowlist logic | `deploy/lsm-audit/oj_audit.bpf.c` | recompile the loader on the target machine (see that directory's README) |
