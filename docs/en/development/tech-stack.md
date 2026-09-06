# Stack & configuration

> One page for everything the OJ uses, at which version, and why.
> Architecture and data flow: `architecture.md`; judge/sandbox deep dive:
> `judge-sandbox.md`.

## Backend

| Component | Version | Purpose / why |
|---|---|---|
| Go | **1.27** (go.mod) | Single static binary; `CGO_ENABLED=0` cross-compiles the Linux judge from a Windows dev box |
| Gin | v1.10 | HTTP routing + middleware (JWT / ban gate / rate limits / body cap) |
| GORM | + glebarez/sqlite v1.11 | PostgreSQL in production, SQLite for self-test — one schema, two dialects; JSON-ish fields stored as text |
| go-redis | v9 | Redis List judge queue (RPush/BLPop) + rate-limit counters (in-memory queue for dev via the `internal/queue` abstraction; sessions are JWT, not in Redis) |
| gRPC | v1.83 | `proto/oj.proto` defines the `JudgeRelay.Connect` bidi stream (dispatch / results / heartbeat) |
| golang-jwt v5 + x/crypto | — | JWT sessions (`OJ_JWT_EXPIRE_HOURS`) + bcrypt passwords |
| gorilla/websocket | v1.5 | Realtime push: `submission:<id>`, `contest:<id>`, `admin:daemons` topics |
| x/sys | v0.47 | Sandbox plumbing: namespaces / seccomp / cgroup / rlimit syscalls |

Backend packages (`backend/`): `cmd/api`, `cmd/judge` entrypoints; `internal/`
with handler (REST), judgehub (scheduling + leases + requeue), daemon (judge
client), queue, wsq (WS broadcast), auth, model/store (GORM), config, logx
(slog JSON→stderr→journald); `pkg/sandbox` (the sandbox — Linux
implementation, stub elsewhere) and `pkg/judge` (judging orchestration, pure
logic, testable).

## Frontend

| Component | Version | Purpose |
|---|---|---|
| Vue | ^3.5 | SPA, composition API + `<script setup>` |
| TypeScript | ~6.0 | Fully typed (`vue-tsc --noEmit` as a gate) |
| Vite | ^8.2 | Build / dev server |
| Tailwind CSS | ^4.1 | Atomic styling; design tokens drive light/dark themes |
| shadcn-vue (reka-ui) | ^2.3 | Headless components + locally generated UI kit (components/ui) |
| Pinia | ^3.0 | State (auth store etc.) |
| TipTap | ^3.30 | WYSIWYG editor for statements/solutions (Editor held in a shallowRef) |
| axios | ^1.13 | API client (centralized in `src/api/client.ts`) |

## Infrastructure (production; addresses in local records)

| Item | Production reality | Notes |
|---|---|---|
| Host | Debian 11, systemd | Single box: API + judge co-located |
| Database | **PostgreSQL 13.23** | compose template defaults to postgres:16; bare metal uses Debian 11's system 13 |
| Redis | 127.0.0.1:6379 | Judge queue / rate limiting |
| Nginx | distro apt version | Static `/opt/oj/web` + reverse proxy `127.0.0.1:8080` (incl. WS upgrade) |
| API | `/opt/oj/oj-api`, `:8080`, gRPC `:9090` | systemd unit `oj-api`, env `/opt/oj/oj.env` |
| Judge | `/opt/oj/oj-judge` | systemd unit `oj-judge`, env `/opt/oj/oj-judge.env`, `OJ_MAX_PARALLEL=1` (the safe value for 3.9 GB) |
| Data dir | `/opt/oj/data` (testdata/code) | Backup root `/opt/oj/backup`; judge workspace `/oj-work` |
| Logs | journald (logx JSON→stderr) | SystemMaxUse=200M, configured in `/etc/systemd/journald.conf.d/99-oj.conf` |
| Judge toolchain | build-essential, openjdk-17-jdk-headless, python3 | New language: edit `pkg/judge/languages.yaml` first, then install the toolchain |

## Config quick reference (OJ_* env vars)

| Variable | Side | Meaning |
|---|---|---|
| `OJ_LISTEN` / `OJ_GRPC_ADDR` | API | HTTP/gRPC listen addresses |
| `OJ_MODE` | both | `dev` (SQLite + in-memory queue) / `prod` |
| `OJ_LOG_LEVEL` | both | `debug`/`info`/`warn`/`error`, default `info` |
| `OJ_DATA_DIR` | both | Testdata & code root |
| `OJ_DB_DRIVER` / `OJ_DB_DSN` | API | `postgres`/`sqlite` + DSN |
| `OJ_REDIS_ADDR` | API | Redis queue |
| `OJ_JWT_SECRET` / `OJ_JWT_EXPIRE_HOURS` | API | Signing key / session lifetime |
| `OJ_DAEMON_SECRET` | API | Judge shared secret (API side) |
| `OJ_FETCH_BASE` | both | HTTP base judges use to fetch testdata/code |
| `OJ_API_ENDPOINT` | judge | gRPC target `host:9090` |
| `OJ_DAEMON_NAME` / `OJ_DAEMON_TOKEN` | judge | Registration name / shared secret (= API-side secret) |
| `OJ_WORK_ROOT` | judge | Sandbox workspace root (never under /tmp) |
| `OJ_MAX_PARALLEL` | judge | Parallel judge slots (default 2; production 3.9 GB boxes require 1 — oversubscription triggers OOM phantom RE/SE, see `judge-sandbox.md` §2.7-5) |
| `OJ_LANGUAGES_FILE` | judge | Optional languages.yaml override (embedded by default) |

## Built-in rate limits

| Dimension | Threshold |
|---|---|
| Login | 10/min/IP |
| Register | 5/min/IP |
| Submissions | 15/min/user |
| Request body | Global 160 MB (testdata zip cap 128 MB) |
