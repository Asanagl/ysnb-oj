# YSNB OJ — technical retrospective

> A sanitized write-up for interviews and external audiences: no real server
> addresses, internal paths or credentials. This page is a condensed English
> version of the Chinese original, which goes deeper on each section.

## 1. What it is (30-second version)

I built an online judge from scratch for my campus ACM team: **a Vue 3
frontend + a Go backend + a from-scratch Linux judging sandbox**, covering a
problem bank, training lists, practice groups, ACM-style contests (freeze /
reveal / team battles), and a user-authoring review flow. About 19k lines of
code across frontend and backend, running in production on an on-campus
Linux server, with data cleanup, credential rotation, backup/restore drills
and two user manuals as delivery close-out.

Two motivations: the team genuinely needed a platform we control, and I
judged that **the technically interesting part of an OJ is the judging
sandbox** — it forces you into kernel namespaces, cgroups, seccomp, process
management and resource isolation that business systems never touch. So I
rejected both shortcuts ("fork an existing OJ" and "wrap go-judge") and built
the judging core myself.

**Quantitative snapshot**: 13.2k lines of Go + 6.2k lines of Vue/TS; three
language profiles (C++17 / Python 3 / Java 17) plus SPJ and interactive
problems; a burst of 8 concurrent users × 8 problems = 64 submissions all
AC with API latency ≤4 ms and standings computation ≤6.3 ms under load; six
E2E suites with 82 assertions all green.

## 2. Stack & key trade-offs

Every decision below answers four questions: what were the candidates, the
comparison axes, why this one, and **when I would flip to the runner-up** —
technology selection is ranking under constraints, not team loyalty.

| Layer | Choice | Runner-up rejected |
|---|---|---|
| Backend | Go 1.27 + Gin + GORM | Java/Spring Boot, Node.js |
| Database | PostgreSQL (SQLite in dev) | MySQL |
| Queue | Redis List (in-memory in dev) | Kafka/RabbitMQ, PG polling |
| Transport | gRPC bidi stream | HTTP polling, message queues |
| Realtime | WebSocket + topic auth | SSE, long polling |
| Frontend | Vue 3 + TS + Tailwind v4 + shadcn-vue + TipTap | React + AntD |
| Sandbox | from-scratch (cgroup v2 + ns + seccomp) | go-judge, isolate, Docker |
| Sessions | JWT + ban point-checks | server-side sessions |
| Deployment | single-box systemd bare metal | Docker Compose, K8s |

The full per-decision reasoning (Go vs Java/Node, PostgreSQL vs MySQL, Redis
List vs Kafka, gRPC vs HTTP polling, WebSocket vs SSE, Vue vs React, the
from-scratch sandbox vs go-judge/isolate/Docker, JWT vs server sessions,
bare metal vs K8s, and the cross-cutting principle of config-driven over
code branches) is in the Chinese original — the pattern throughout: pick the
option whose failure modes you can actually debug on this team and this
hardware.

## 3. Core difficulty ①: the from-scratch sandbox (the security core)

**Threat model**: contestant code is fully untrusted, and so are checkers
and interactors. Principles: default deny, fail-closed — minimal writable
filesystem, no network, dropped privileges, all resources capped, everything
outside the syscall allowlist gets SIGKILL.

**Design**: two-stage re-exec + five isolation layers. The parent creates a
cgroup, marshals the spec into an env var, clones fresh
PID/Mount/Net/IPC/UTS namespaces and re-executes itself; stage 2 mounts a
private filesystem view (read-only root, tmpfs overlays hiding host paths,
one writable workspace), drops to uid 65534, applies rlimits at the last
moment before execve, installs a seccomp cBPF allowlist via prctl, and
finally execs the target program. The wall-clock watchdog kills PID1, which
takes the whole process tree with it.

**A real kernel-level debug session** (the most valuable post-mortem of the
project): while tuning the memory controller, submissions started failing
with phantom runtime errors. The chain, found by bisection: (a) the cgroup
limits function was dead code — never called; (b) once wired up, writing
`memory.max` failed EACCES because cgroup v2 requires the parent's
`subgroup_control` to enable the controller first; (c) enabling it under
the `nsdelegate` mount option hung the kernel in
`mem_cgroup_css_alloc` on proxy subgroups; (d) after all that, the Go
supervisor shared the cgroup with the contestant program and got OOM-killed
first until 64 MB of headroom was added. Four layers, each individually
silent, stacking into one user-visible failure. The full write-up with
three more war stories (stage2 recursion inside test binaries, Pdeathsig vs
PID namespaces, RLIMIT_AS biting the Go runtime) is in the Chinese original
and in `judge-sandbox.md`.

## 4. Core difficulty ②: judging scheduling & reliability

Dispatch is a Redis List with lease-based at-least-once semantics: a 15-
minute lease per judged task, a requeue scanner sweeping expired leases, a
full queue rebuild from PostgreSQL at startup, and instant requeue when a
judge disconnects mid-task. Judges dial outbound over a gRPC bidi stream
(zero inbound firewall holes), stream per-case progress as each test case
finishes, and report final results with per-case verdicts and scores. The
failure-mode design goal: any component can die at any moment and the
database converges back to a consistent state.

## 5. Core difficulty ③: frontend↔backend engineering

- **Live judging status**: WebSocket topics with authorization —
  `submission:<id>` is owner-only (live judging intel must not leak to other
  contestants), `admin:daemons` is admin-only. The token rides the query
  string because browsers cannot set headers on WebSocket upgrades.
- **Standings consistency & freeze**: the server computes standings and
  broadcasts a dirty marker; the freeze mask is applied server-side so no
  endpoint can leak post-freeze verdicts.
- **Stateless JWT vs instant bans**: the ban gate re-checks the database per
  request — stateless auth with immediate revocation.
- **Large uploads**: zip testdata with path whitelisting, unified
  renumbering and per-file sha256 — no path traversal.
- **Permission model**: server-side RBAC enforced at the route layer, with
  the UI only mirroring it; privilege-escalation chains were mapped and each
  link closed.
- **Contest problems as independent copies**: a data-model decision —
  attaching a problem clones it into the contest so later edits to the bank
  original never leak into a running contest.
