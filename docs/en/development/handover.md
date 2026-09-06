# Handover guide

> For the **incoming developer**. Goal: after ~30 minutes you can locate any
> piece of code, know how to develop, pass the gates and ship. This page is
> only a map — routes, permissions, API and judging details live in the
> linked documents.
>
> Suggested order: this page → [architecture.md](architecture.md) (overview)
> → [judge-sandbox.md](judge-sandbox.md) / [api.md](../reference/api.md) as
> needed.

## 1. What this is (60 seconds)

YSNB OJ — a **fully self-developed** online judge for a campus ACM team
(You Submit, Never Be rejected). Reusing go-judge or forking Hydro /
DOMjudge was explicitly ruled out: the judging sandbox is written from
scratch. That is both the core constraint and the core asset.

- **Backend**: Go 1.27, Gin + GORM (PostgreSQL in production / SQLite for
  self-test) + a Redis queue; API ↔ judges over a gRPC JudgeRelay bidi
  stream.
- **Frontend**: Vue 3 + TypeScript + Vite + Tailwind CSS v4 + shadcn-vue
  (reka-ui), Pinia / ECharts / TipTap / CodeMirror 6.
- **Sandbox**: cgroup v2 + namespaces + seccomp cBPF + stage2 re-exec,
  **Linux only** (which is why the backend never runs on Windows/macOS dev
  machines).
- **Production**: Debian 11 single box (3.9 GB RAM), systemd + nginx +
  PostgreSQL + Redis, API and judge co-located; SSH is key-only (password
  login disabled).

## 2. Capability map (current real surface)

- **Contest modes**: ACM + IOI (per-case partial scoring), team contests,
  freeze & reveal, practice mode, first-fail-stop.
- **Problems**: SPJ / interactive, problem lists, groups (teams / notices /
  shared lists), user authoring + review flow, solution area (unlocked by
  submitting), package import (native / DOMjudge / Hydro), external problem
  import (open to all users: regular users go through review and carry an
  "awaiting testdata" badge; setters and above import directly).
- **M5 plugin system**: three extension points ProblemSource (problem
  crawling) / SubmitLogFetcher (practice sync) / EventHook (judging event
  hooks), with built-in codeforces / luogu / atcoder / nowcoder adapters +
  a webhook hook; powers external problem import and shared export.
- **External practice sync**: an hourly ticker pulls bound users'
  multi-platform submission records; integrated into the profile page
  (binding management, stats badges, cross-platform recent AC, heatmap
  multi-platform merge, the import entry).
- **Live judging status**: submissions advance in place — queued → judging →
  full verdict name — with per-case dots lighting up (the judge streams
  each case over `CaseProgress`, best-effort); the `submission:<id>` WS
  topic is owner-only (or admin). Built on `LiveVerdictCard` +
  `useLiveSubmission`.
- **Public API**: `/public/*` + API key auth + cph bridge (Competitive
  Companion / VSCode cph imports problems and samples directly).
- **/admin console**: users, problems, contests, review, judges, plugins,
  API keys, **log viewer**.
- **Structured logs**: `internal/logx` (slog JSON → stderr → journald),
  `OJ_LOG_LEVEL` switches levels without recompiling; an INFO
  `submission judged` anchor per finished submission; the admin
  `GET /admin/logs` + the frontend log viewer page search journald directly.

## 3. Credentials & environment pointers (pointers only, zero secrets)

Iron rule: **no real passwords, keys or host addresses in the repo or
docs**. Values live only in:

- local `.zcode/` records (gitignored);
- `remote-test/.sshenv` (SSH key channel), `remote-test/.ojenv` (production
  API address & admin account), `remote-test/admin.env` — all gitignored,
  keep them out of the repo and out of chats.

Server runtime config lives in `/opt/oj/oj.env` and `/opt/oj/oj-judge.env`
(mode 600; restart the matching service after edits). **Staff changes =
full rotation**, procedure in
[maintenance.md](../operations/maintenance.md) section 8.

## 4. Code tour

### Top-level layout

```
backend/              Go backend (all service code)
  cmd/api/            API entry (HTTP :8080 / gRPC :9090)
  cmd/judge/          judge daemon entry
  cmd/cli/            in-container maintenance CLI (oj-cli, lockout rescue, see deploy.md)
  proto/oj.proto      JudgeRelay bidi protocol; pb/ is generated — do not hand-edit
  internal/           all business logic (below)
  pkg/sandbox/        the self-developed sandbox (Linux-only, the core asset)
  pkg/judge/          judging orchestration + languages.yaml (adding a language = this yaml)
frontend/             Vue 3 frontend (below)
deploy/               one-click.sh, systemd units, nginx.conf, backup/restore scripts
dist/                 dev-machine cross-compile artifacts (oj-api-linux etc.), what gets shipped
docs/                 documentation (VitePress site; README.md is the map)
remote-test/          deploy & E2E tooling + local credential env files (gitignored)
scripts/              e2e-test.sh, ws-probe.mjs, zipmake.go and friends
```

### backend/internal/ package guide

```
internal/config/      OJ_* env assembly (config.go; add config here first)
internal/auth/        JWT sign/verify, password hashing, roles
internal/model/       every GORM table (single model.go — find fields there)
internal/store/       DB connection & auto-migration
internal/queue/       Redis submission queue
internal/handler/     all REST routes & handlers (router.go is the route map)
internal/judgehub/    judge scheduling: hub.go connection management;
                      tasks.go queue consumption, leases, requeue, persistence
internal/daemon/      judge-side gRPC client
internal/plugin/      M5 plugin system: plugin.go is the registry (init() Register,
                      same philosophy as languages.yaml), codeforces/luogu/
                      atcoder/nowcoder adapters + hooks.go webhook
internal/logx/        structured log layer: slog JSON → stderr → journald,
                      OJ_LOG_LEVEL switches levels (logx.go's header comment is the contract)
internal/external/    practice-sync persistence models (own package to avoid a
                      store→handler import cycle)
internal/public/      API key model (same reason)
internal/wsq/         topic-based WebSocket hub (submission status / standings push)
internal/sysload/     load sampling (Linux-only, build-tag isolated)
```

### Backend key files (indexed by "I want to change…")

| I want to… | Look here |
|---|---|
| Add/change REST routes | `handler/router.go` (/admin, /public groups and auth middleware) |
| Change the log viewer | `handler/admin_logs.go` — header comment is the security model (child-process argv fully isolated from the request); `admin_logs_test.go` pins it, read before extending |
| Change practice sync | `handler/external_sync.go` — hourly ticker; one platform failing only logs, never blocks others |
| Change public API / cph | `handler/public_api.go`, `handler/public_cph.go` (the cph bridge only exports samples, never full testdata); details in [api.md](../reference/api.md) |
| Change a plugin | `internal/plugin/plugin.go` + the adapter file; admin endpoints in `handler/plugins_admin.go` |
| Change scheduling / the log anchor | `internal/judgehub/tasks.go` — the INFO `submission judged` anchor line lives here |
| Change live progress streaming | `pkg/judge` `Task.OnCase` (fires on all three paths) → `internal/daemon` `CaseProgress` → `internal/judgehub/hub.go` `handleCaseProgress` (inflight ownership check then WS); best-effort, must never block judging |
| Change `submission:<id>` WS permissions | `handler/router.go` `wsTopicAuthorizer` — owner/admin only, ownership checked in DB |
| Change the sandbox / judging | Read [judge-sandbox.md](judge-sandbox.md) first; `pkg/sandbox/seccomp_sim_test.go` is the seccomp interpreter guard test |
| Add a language | only touch `pkg/judge/languages.yaml` |

### Frontend src/ layout

```
src/api/client.ts     TS wrapper for every backend endpoint (the only HTTP egress; add APIs here first)
src/router/index.ts   routes + role guards (/admin children include the logs viewer page;
                      /external is gone and redirects to /profile — practice data moved into the profile)
src/stores/auth.ts    Pinia session state
src/views/            pages: nine Admin* console pages (incl. AdminLogsView),
                      ProblemEditor / MyProblemEditor full-page authoring;
                      external binding / stats / problem import live in ProfileView
                      (the old ExternalPracticeView is deleted)
src/layouts/          MainLayout (contestants) / AdminLayout (admin console)
src/components/       CodeEditor (CodeMirror 6), MarkdownEditor (TipTap),
                      ScoreBoard, LiveVerdictCard (live verdict card: status advance +
                      per-case dots + full verdict names), ui/ (shadcn-vue kit)
src/composables/      useChart (ECharts), useTheme, useResponsive,
                      useLiveSubmission (WS submission tracking + polling fallback)
```

## 5. Day-to-day development

- **The backend never runs on a dev machine**: the sandbox needs cgroup v2 +
  namespaces, Linux only. Verify backend changes by building on Linux or
  cross-compiling and deploying.
- **Frontend local dev**: in `frontend/`, run
  `OJ_DEV_API_TARGET=<deployed address> npm run dev` to proxy `/api`
  (including WebSocket) to a deployed Linux environment; the address is in
  `remote-test/.ojenv`, never hard-coded into the repo. (`ws: true` in
  `vite.config.ts` is mandatory — verdict pushes ride WS.)
- **Gates** (all green before merge/release):
  1. frontend `npx vue-tsc --noEmit` zero errors + `npm run build`;
  2. backend (on Linux) `go build ./... && go vet ./... && go test ./...`;
  3. six remote-test E2E suites against the production API, baselines all
     green:

     | Script | Baseline |
     |---|---|
     | `m4-import-export-e2e.mjs` | 22 PASS |
     | `m4-languages-e2e.mjs` | 13 PASS |
     | `m4-spj-interactive-e2e.mjs` | 8 PASS |
     | `m5-plugins-ioi-external-e2e.mjs` | 42 PASS |
     | `public-api-e2e.mjs` | 22 PASS |
     | `stop-on-fail-e2e.mjs` | 7 PASS |

     Login rate limit is 10/min/IP — **leave ≥70 s between suites** or you
     get 429s.
- **Changing the judging core (especially pkg/sandbox, pkg/judge) requires**:
  1. pure-function unit tests for the logic;
  2. the seccomp interpreter test `seccomp_sim_test.go` (cross-compile and
     run on the judge, commands in [judge-sandbox.md](judge-sandbox.md));
  3. then the six E2E suites above.

## 6. Release flow (current key channel)

The command-level checklist is in
[maintenance.md](../operations/maintenance.md) section 2 — skeleton only.
**Red line: never build on the server** (3.9 GB RAM; two OOM incidents
already) — build on the dev machine, ship artifacts.

- **Frontend**: `cd frontend && npm run build`, then
  `node remote-test/m5-frontend-sync-key.mjs` (tar.gz → key channel →
  sha256 verified → atomic swap of `/opt/oj/web`, previous version kept as
  `web.old` for rollback).
- **Backend**: cross-compile `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go
  build` into `dist/oj-api-linux` / `oj-judge-linux` →
  `node remote-test/oj-deploy-binaries.mjs` (pushes to `.new` files only —
  **no swap, no restart**) → on the server `mv -f` to swap +
  `systemctl restart oj-api oj-judge` (renaming under a running process is
  safe, no need to stop first) → submit an A+B in the console to confirm the
  judging path.
- **Cold deploy on a fresh server**: `deploy/one-click.sh` (Docker Compose
  route), see [deploy.md](../operations/deploy.md).
- **Lockout rescue**: a lost super-admin password is fixed by the
  maintenance CLI `oj-cli create-superadmin`, see the oj-cli section of
  deploy.md.

## 7. Known pitfalls (previous generations already paid for these)

1. **seccomp cBPF once had a uint8 jump overflow**: allowlists >127 entries
   wrapped offsets, Java went all-RE. Fixed with chunked chains, but the
   lesson stands — **adding a syscall requires adding a
   `seccomp_sim_test.go` case**.
2. **The JDK needs socket syscalls**: javac/java open an AF_UNIX socket at
   startup; the seccomp allowlist already permits it (no NIC inside the net
   namespace, so no egress). Don't "tidy" it away.
3. **Java all-CE: check `which javac` on the judge first** — usually a
   missing JDK in the image.
4. **Contest problems are independent copies**: edits to a contest problem
   don't touch the bank original; submitting to the bank id 404s.
5. **Contests default to require_registration=true**: no registration, no
   submission — don't misread that as a bug during acceptance.
6. **The login rate limit is a feature**: running E2E suites back to back
   triggers 429s; space them ≥70 s and don't "fix" the limiter.
7. **DOMjudge validators and this OJ's checkers differ in convention**
   (this OJ: `./checker <in> <ans> <out>`, exit 0 = AC) — manually review
   after package import (see [interactive.md](../guide/interactive.md)).
8. **proto regeneration landing spot**: `protoc --go_out=. --go_opt=module=github.com/ysnb/oj`
   puts files in `pb/`; missing the module flag generates into `proto/`,
   leaving two copies and compile errors about missing fields.
9. **Don't hand-roll ssh2 exec for large transfers** (>1 MB wedges in the
   channel window): always use the ready key scripts (gzip+base64+sha256
   verified both ends). `remote-test/gen-push.js` and other password-SSH-era
   scripts are legacy — **do not use**.
10. **Vue `<script setup>` TDZ white-screen**: a `watch(..., { immediate:
    true })` written **before** the `const` it reads fires the callback at
    registration, hitting the TDZ → ReferenceError → the whole page blanks
    with no build-time error. Lesson: watches with `immediate` must come
    **after** the declarations they reference.
11. **Docs and implementation stay in sync**: any behavior change updates
    the matching page under docs/ — the next person will be misled
    otherwise.

## 8. Documentation navigation

[docs/README.md](/README) is the docs-site home and map — what each
page covers and when to open it. The frequently used ones:

- Routes/permissions/public API → [reference/api.md](../reference/api.md)
- Judge & sandbox deep dive (before touching judging) →
  [development/judge-sandbox.md](judge-sandbox.md)
- Releases/patrols/incidents/key rotation →
  [operations/maintenance.md](../operations/maintenance.md)
- Cold deploy & the env var table → [operations/deploy.md](../operations/deploy.md)
- Contestant/admin manuals → [guide/](../guide/user-guide.md)
- Historical E2E & audit records (read-only) → [archive/](/archive/e2e-report)

## 9. Todos & roadmap

Near-term, explicit:

- [ ] **TLS/HTTPS** (prerequisites and steps in deploy.md's HTTPS section)
- [ ] user-guide / admin-guide **screenshots pending** (docs/screenshots/)
- [ ] **v1.0.0 release wrap-up** (tag + release)
- [ ] **production E2E test data cleanup** (test accounts/problems/contests
      left by past rounds)

Mid-term candidates:

- [ ] discussion area (handler group pattern copies the existing modules)
- [ ] training plans (scripted; extension points reserved)
- [ ] multi-judge load balancing (native with gRPC multi-daemon — add a box
      and go)
