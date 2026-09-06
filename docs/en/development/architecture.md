# Architecture

> A developer-facing overview: the overall shape, key decisions and data
> flows. Judge/sandbox deep dive: `judge-sandbox.md`; component versions and
> `OJ_*` config reference: `tech-stack.md`; deployment & ops:
> `../operations/deploy.md`.

## Overview

```
Browser ──Vue3 SPA (Vite dist)──► nginx edge (static hosting + /api & WS upgrade reverse proxy + security headers)
                                              │
                                ┌─────────────┴───────────────┐
                                ▼                             ▼
                     /api/v1/* (JWT + RBAC)        /api/v1/public/* public API
                     user + AdminLayout console    (anonymous reads; ojk_ keys, own quota)
                                │
                          Go API (cmd/api)
                          ├─ handler    REST: problems/submissions/contests/standings/teams/lists/jury/admin
                          ├─ judgehub   judge scheduling: queue consumption, leases, requeue-on-disconnect, result persistence
                          ├─ plugin     compile-time registry: problem crawling / practice sync / webhook event hooks
                          └─ logx       slog single-line JSON → stderr → journald
                                        (the admin "log viewer" reads it back via journalctl)
                                │
              ┌─────────────────┼──────────────────────────┐
              ▼                 ▼                          ▼
        PostgreSQL        Redis (judge queue only,    gRPC JudgeRelay bidi stream
        state source      List RPush/BLPop)           proto/oj.proto: Connect(stream⇄stream)
        of truth                                       judges dial out: register/heartbeat/tasks/results
                                                                ▼
                                                judge-daemon (cmd/judge, from scratch, sandbox included)
                                                ├─ language registry (pkg/judge/languages.yaml, config-driven)
                                                ├─ compile sandbox profiles (g++/javac)
                                                ├─ run sandbox profiles (cgroup v2 + ns + seccomp allowlist)
                                                ├─ checker / interactor compile cache (sha256)
                                                └─ testdata blob cache (sha256, origin /internal/testdata)
```

## Key decisions

| Decision | Choice | Why |
|---|---|---|
| Language | Go (CGO_ENABLED=0) | Single static binary; a Windows dev box can cross-compile the Linux judge |
| Sandbox | from-scratch `pkg/sandbox` | The user explicitly required a fully self-developed one; security model below, deep dive in `judge-sandbox.md` |
| API↔judge | gRPC `JudgeRelay.Connect` bidi stream, judges dial out | Zero inbound firewall holes on judges; register/heartbeat/tasks/results share one stream, multi-judge scaling is native |
| Queue | Redis List (prod) / in-memory (dev) | `internal/queue` abstraction (RPush/BLPop), zero deps in dev; Redis is a queue, not storage |
| Languages | data-driven `pkg/judge/languages.yaml` | Adding a language is one config stanza |
| Database | PostgreSQL (prod) / SQLite (dev) | GORM dual dialect, one schema; JSON-ish fields stored as text; version notes under "Data storage" |
| Logs | slog single-line JSON → stderr → journald, no log files | Shares the system `journalctl` ops surface; no rotation or cleanup to manage; journald `SystemMaxUse` caps disk; levels map onto PRIORITY so `journalctl -p err` filters directly |
| Plugins | compile-time registry (`init()` + `Register`), no dynamic loading | Deployment is static binaries + config; Go `.so` dynamic loading is fragile across toolchains; no dynamic linking surface — each adapter is one auditable file |
| Front/back | Vue3 SPA + nginx-served `dist`, separate API | The frontend is a pure static artifact, independently replaceable; the nginx edge is the single entry with security headers and WS upgrade; API/gRPC ports never face the internet |

## Module boundaries

```
backend/
├── cmd/api              HTTP+gRPC main
├── cmd/judge            judge main (with --selftest)
├── cmd/cli              maintenance CLI
├── internal/
│   ├── handler/         REST layer (auth/problem/submission/contest/teams/lists/jury/admin)
│   │   ├── public_api.go + public_cph.go   public API (/public/*) + /admin/api-keys key management
│   │   ├── admin_logs.go                   log viewer (fixed-argv journalctl, no injection surface)
│   │   ├── plugins_admin.go                plugin registry, IOI score table (case-scores), webhook management
│   │   └── external_sync.go                practice sync worker (hourly ticker, serialized crawling)
│   ├── judgehub/        judge scheduling: queue consumption, leases, requeue-on-disconnect, result persistence
│   ├── daemon/          judge-side client (reconnect, heartbeat, concurrency gate)
│   ├── plugin/          extension registry (ProblemSource / SubmitLogFetcher / EventHook)
│   ├── external/        external submission records (practice report data source)
│   ├── public/          API key credentials (only sha256 stored)
│   ├── logx/            slog JSON → stderr → journald; OJ_LOG_LEVEL switches levels
│   ├── queue/           queue abstraction (Redis List / in-memory)
│   ├── wsq/             WebSocket topic broadcast (submission:N owner-only / contest:N / admin:daemons)
│   ├── auth/            JWT + bcrypt + RBAC middleware
│   ├── model/ store/    GORM models & migrations
│   └── config/ sysload/ config loading / host load sampling (judge monitor)
└── pkg/
    ├── sandbox/         sandbox library (Linux implementation, stub elsewhere)
    └── judge/           judging orchestration (decoupled from transport, pure logic, testable)

frontend/src/
├── layouts/             MainLayout (contestants) and AdminLayout (admin shell, role-tiered menu)
├── views/               pages (Admin*View series under AdminLayout)
├── composables/         useChart / useResponsive / useTheme / useLiveSubmission (live submission tracking)
├── lib/                 toast / confirm / utils (UI primitives)
├── api/client.ts        centralized axios wrapper
└── stores/              Pinia (auth)
```

Extension pattern: new features add an `internal/handler` group + an
`internal/model` entity; new external platforms add one file under
`internal/plugin` with `init() Register`. Teams and practice sync landed this
way; the only reserved future features are the discussion area and scripted
training plans.

## Judging data flow

1. `POST /submissions` → validation (login / visibility / language / contest
   window / registration / rate limit) → code to disk → status `PENDING`,
   enqueued (Redis List).
2. A judge registers over gRPC `JudgeRelay.Connect` (shared secret) →
   judgehub pulls the queue within its capacity → assembles the task (code +
   limits + judge_mode sources + per-case sha256 manifest; IOI problems carry
   `Score` per case) → submission set `JUDGING` + a 15-minute lease → WS
   broadcast on `submission:<id>`.
3. The judge fetches missing testdata by sha256 via `/internal/testdata` →
   compiles (cached; checker/interactor likewise) → runs each case in the
   sandbox → diff / SPJ / interactive → returns `TaskResult` (per-case
   verdicts and scores). As soon as each case finishes, the judge also
   streams a `CaseProgress` message over the same gRPC stream (best-effort:
   a dropped progress update never affects judging or the final result);
   judgehub verifies the submission is leased to that judge and relays it to
   WS.
4. `finalize` persists (status/time_ms/memory_kb/score/cases, lease cleared)
   → logs INFO `submission judged` (one line per submission — the primary
   anchor for the log viewer's search by id/verdict) → WS broadcast on
   `submission:<id>`; contest submissions additionally broadcast
   `contest:<id>` (standings-dirty, incremental standings refresh); judge
   up/down broadcasts `admin:daemons` (admin-only subscription);
   `plugin.EmitJudgeEvent` fans webhooks out asynchronously on its own
   goroutine, never blocking judging. Consumers of `submission:<id>` are the
   problem page / contest problem page / submission detail page (frontend
   `useLiveSubmission`, which drives the live verdict card and the per-case
   dot strip, falling back to polling when the socket drops). The topic is
   **owner-only** (or admin) — live judging intel is never exposed to other
   contestants; ownership is checked at subscribe time in
   `handler/router.go`'s `wsTopicAuthorizer`.
5. Fault tolerance: a judge disconnecting immediately requeues its leased
   tasks as `PENDING`; `RequeuePending` at API startup rebuilds the whole
   queue, and a background `StartRequeueScanner` sweeps leases that expired —
   the database is the source of truth and the queue can be rebuilt at will.

## Data storage

- **PostgreSQL**: identical semantics on both routes — the compose template
  uses `postgres:16-alpine` (`docker-compose.yml`: postgres/redis/api/web in
  containers, judge on the host); bare-metal Debian 11 uses the distro's 13
  (tech-stack.md records 13.23). GORM dual dialect (postgres/sqlite) shares
  one schema; dev defaults to zero-dependency SQLite.
- **Redis**: judge queue only (List, RPush/BLPop) — never a cache or
  database; the queue is best-effort and rebuildable from PostgreSQL (step 5
  of the data flow).
- **Files**: testdata blobs, submission code and uploads live under
  `OJ_DATA_DIR`, addressed by sha256.

## Sandbox security model (pkg/sandbox)

Every run re-executes itself inside a **fresh set of namespaces**
(PID/Mount/Net/IPC/UTS):

- **Filesystem**: possibly-private mounts propagate, then `/` is re-bound
  read-only (NOSUID/NODEV); `/proc` and `/sys` overlaid with fresh read-only
  instances; `/tmp`, `/dev`, `/dev/shm` are capped tmpfs; `/home`, `/root`,
  `/var`, data_dir and work_root get empty tmpfs overlays to prevent leaks;
  the only writable point is the task workspace (chowned to uid 65534,
  mode 0700).
- **Identity**: root finishes mounting then `setresuid(65534)` drops
  privileges for the user program / checker / interactor.
- **Resources**: cgroup v2 `memory.max` (swap off), `cpu.max`, `pids.max`;
  RLIMITs: FSIZE (output cap), STACK, AS (per-language; JVM relies on
  cgroup), CPU (second-scale backstop) + a wall-clock watchdog (kills PID1 =
  kills the whole tree).
- **Syscalls**: seccomp cBPF allowlist, default SIGKILL. No socket family,
  ptrace, mount, unshare, setuid family; the compile profile additionally
  allows fork/vfork.
- **Interactive problems**: the user program and the interactor each run in
  their own sandbox, talking through parent-brokered pipes; either exiting
  terminates the other; the interactor convention is exit 0 = AC / 1 = WA /
  anything else = SE.

Residual risks (known, controlled): PID1 belongs to the user program, so its
orphaned children are reaped with the namespace on exit (long-running
multi-fork programs can accumulate brief zombies, capped by pids.max); the
seccomp allowlist evolves with need — a missing syscall causes a language
feature to RE, never a security hole (fail-closed). Implementation details,
parameters and triage: `judge-sandbox.md`.

## Contest modes

- **ACM (mode=acm)**: solved descending → penalty ascending; penalty = AC
  time + 20 minutes per wrong submission (CE free). Capabilities: freeze
  (`freeze_time` automatic / jury manual), starred ★ (registration
  `team_type=starred` or jury flag — row shown but unranked), cheating flags
  (row kept, excluded from scoring), reveal (a `reveal_count` scroll from
  the bottom of the real final board; ★/cheated rows stay masked).
- **IOI (mode=ioi)**: partial-credit mode. Problems keep a per-case score
  table (`TestCase.Score`, `PUT/GET /problems/:id/case-scores`, setter+);
  judging awards per case (the case must AC), and `submissions.score` stores
  the total; verdicts remain verdict-based — a partial submission can still
  be WA overall. Standings rank by total score; the frontend `ScoreBoard`
  branches on `mode=ioi` (headers "max points/score", cells show score).
  Freeze and reveal semantics match ACM.
- **Team contests**: `Contest.TeamMode` has the captain register once (with
  `TeamID` and name, capacity default 3), members snapshot into one
  registration row each, standings aggregate per team (`teamStandings`).
- **Practice mode**: post-contest submissions are flagged `IsPractice=true`
  (no registration, problems stay open), judged normally but never counted
  toward any standings and never changing freeze state.
- **Freeze mask (fairness red line)**: submissions after the freeze point
  appear on the standings only as a pending count (ACM `?n` / IOI
  `cell.Pending=1`, contributing 0); no endpoint may reveal their verdict or
  score early.

## Public API surface

`/public/*` is an anonymous read-only surface isolated from JWT
(`/public/users/:id`, `/public/problems`); machine-readable endpoints
(`/public/problems/:id/samples`, `/public/problems/:id/cph`) require an
`ojk_`-prefixed API key (`X-API-Key` header, only the sha256 stored, shown
once at creation, managed at `/admin/api-keys`, revocation keeps the audit
row). Anonymous callers share one 60 req/min bucket; each key owns its own.
**Judging test data (.in/.out) is never exposed — a fairness red line.**
