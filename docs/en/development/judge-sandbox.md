# Judge & sandbox technical reference

> For developers changing the judging core or triaging judge problems. The
> overview-level architecture and security model live in `architecture.md`;
> this page expands implementation details, parameters and triage methods.
> Code locations: judge daemon `backend/cmd/judge` + `backend/internal/daemon`;
> judging orchestration `backend/pkg/judge`; sandbox `backend/pkg/sandbox`;
> scheduling `backend/internal/judgehub`; protocol `backend/proto/oj.proto`.

## 1. The judge process (oj-judge)

### 1.1 Lifecycle

```
start → Preflight (cgroup v2 writability etc.)
      → load the language registry (embedded languages.yaml, overridable via OJ_LANGUAGES_FILE)
      → judge.NewService (sandbox + language table + workspace + blob cache + origin client)
      → daemon.Run: gRPC connection to the API (OJ_API_ENDPOINT; judges dial out by default)
```

- `--selftest`: mandatory after deploying or switching machines. Three
  checks: cgroup v2 writable → a basic jailed `/bin/echo` run → the
  wall-clock watchdog (a 100ms limit kills an infinite loop). All passing
  prints `== all selftests passed ==`.
- Registration: the judge registers with `OJ_DAEMON_NAME` (default judge-1)
  + `OJ_DAEMON_TOKEN` (= the API-side `OJ_DAEMON_SECRET`); the API stores a
  `JudgeDaemon` row for the monitor page.
- Concurrency: `OJ_MAX_PARALLEL` caps simultaneously judged tasks
  (production = 1 — the measured safe value under a 3.9 GB memory budget;
  rationale in §2.7 item 5).

### 1.2 gRPC protocol (proto/oj.proto)

```proto
service JudgeRelay {
  rpc Connect(stream DaemonMessage) returns (stream ApiMessage);
}
```

One bidi stream carries everything: registration, heartbeat (with load/mem
telemetry), task dispatch, result return, testdata download authorization.
Judges dial out, so judges need no inbound firewall holes and horizontal
scaling is native.

### 1.3 Workspace & cache layout

| Path | Contents |
|---|---|
| `$OJ_WORK_ROOT/judge-<runid>/` | per-run workspace: source, build artifacts, checker/interactor, run IO, compile_err.txt |
| `$OJ_WORK_ROOT/tool-*` | checker/interactor compile scratch (deleted after use) |
| `$OJ_DATA_DIR/judge-cache/` | testdata blob cache (by sha256); missing blobs are pulled from the API's `/internal/testdata/...` |

Failed (CE/SE) workspaces are **deliberately kept** (the log prints
`workspace kept at ...`) for post-mortem; successful runs are deleted
immediately. The kept ones accumulate over time — cleanup in the
[ops runbook](../operations/maintenance.md).

### 1.4 Judging pipeline (pkg/judge)

1. **Write source**: `main.cpp` / `main.py` / `Main.java` (the language
   table's source_file).
2. **Compile** (languages with a compile section): a dedicated sandbox
   profile, 60s timeout / 1GB memory / FSIZE 64MB / stack 512MB / pids 256;
   artifacts stay in the workspace.
3. **Prepare checker / interactor**: sources cached globally by sha256
   (`tool-*` compiles once, later runs hardlink into the workspace),
   `g++ -O2 -std=c++17`.
4. **Run per case**: data is fetched by sha256 and cached; each case runs in
   its own sandbox (parallel via CaseWorkers; contest submissions stop at
   the first failure — §2.8). The comparison strategy follows the problem's
   judge_mode: default (diff with trailing-whitespace tolerance) / spj
   (`./checker <in> <ans> <out>`, exit 0=AC) / interactive (bidirectional
   pipes).
5. **Aggregate**: worst verdict across cases (SKIPPED excluded); CE/RE
   semantics in the [user guide](../guide/user-guide.md) verdict table.

**Interactive problems**: the user program and the interactor each enter
their own sandbox, bridged by the parent process over pipes; either exiting
kills the other immediately; the interactor watchdog is a fixed 30s and the
user-side limit = problem limit × language multiplier. exit 0=AC / 1=WA
(first stderr line shown to the contestant) / anything else=SE. Cases may
omit `.out` (the interactor judges).

### 1.5 Language profiles (pkg/judge/languages.yaml)

Embedded defaults with file override; adding a language = a config stanza +
the toolchain, zero code:

| Field | Meaning |
|---|---|
| `source_file` | filename for the contestant's code (Java must be Main.java) |
| `compile.argv / timeout_s / mem_mb` | absent = interpreted language |
| `run.argv` | the `{MEM}` placeholder is replaced with the problem's memory limit (Java -Xmx injection) |
| `time_multiplier` | problem time-limit multiplier (Python 4×, Java 3×) |
| `mem_overhead_mb` | memory allowance (Python +128, Java +512) |
| `stack_kb` | RLIMIT_STACK |
| `as_limited` | whether RLIMIT_AS is also set (must stay off for the JVM: it reserves a huge virtual arena; the cgroup RSS cap is the real bound) |

Current three: C++17 (g++ -O2 -std=c++17 -Wall), Python 3 (python3 -B),
Java 17 (javac + java -Xss256m -Xmx{MEM}m -XX:-UsePerfData).

## 2. The sandbox (pkg/sandbox)

### 2.1 Threat model & principles

Contestant code is fully untrusted; checkers and interactors are sandboxed
too. Principles: **default deny, fail-closed** — minimal writable
filesystem, no network, dropped to uid 65534, every resource capped,
anything outside the syscall allowlist gets SIGKILL. A missing allowlist
entry manifests as "a language feature REs" (an availability problem), never
as a security hole.

### 2.2 Two-stage execution (stage2 re-exec)

`Run(spec)` on the parent side: a random runid creates the cgroup →
`json.Marshal(spec)` into the `OJ_STAGE2_SPEC` env var → `clone` fresh
**namespaces** (Mount/Pid/Net/Ipc/Uts) and re-execute our own binary into
`Stage2Main`. The child, in order:

1. `stage2Setup`: mount the private view (§2.3) → drop uid/gid 65534 →
   rlimits.
2. `resolveArgv0`: resolve tool absolute paths via PATH **before** seccomp
   (Go ≥1.21's PATH probing uses newfstatat, which the filtered view kills).
3. `loadSeccomp(profile)`: install the filter via
   `prctl(PR_SET_SECCOMP)` (not the raw seccomp(2) syscall — some cloud
   kernels return EOPNOTSUPP).
4. `execve` the target. From here any non-allowlisted syscall = SIGSYS.

### 2.3 Filesystem view

| Mount | Treatment |
|---|---|
| `/` | re-bound read-only (MS_NOSUID\|NODEV), after make-rprivate to block propagation |
| `/proc` | fresh procfs instance (read-only, this PID ns only) |
| `/sys` | fresh sysfs instance (read-only) |
| `/run` | tmpfs 8MB |
| `/tmp` | tmpfs 256MB (mode 1777) |
| `/dev` | tmpfs 16MB + manual mknod null/zero/full/random/urandom; `/dev/shm` tmpfs 64MB |
| `/home` `/root` `/var` | empty tmpfs overlays (so the read-only view cannot leak host files) |
| `/tmp/ojws` (inside the ns) | bind of host `$OJ_WORK_ROOT/judge-<runid>`, the **only writable point** (chown 65534, 0700) |

### 2.4 Resource control

| Mechanism | Contents |
|---|---|
| cgroup v2 | `memory.max` (swap off), `cpu.max`, `pids.max` (default 512 when the spec omits it) |
| RLIMIT | CORE=0; FSIZE (output cap against disk-flooding); STACK; AS (only when `as_limited`); CPU = limit+1s backstop |
| Watchdog | wall-clock deadline kills **PID1** (init = the user program), taking the whole tree; triggering it verdicts TLE |
| Verdict mapping | watchdog→TLE; cgroup OOM→MLE; SIGXCPU→TLE; SIGKILL with RSS>90% of the cap→MLE; exit 0→OK; anything else→RE (exit code/signal in the message) |

### 2.5 seccomp allowlist (the historical lessons live here)

A classic cBPF filter, policy = allowlist + SIGKILL:

- **Common section**: the regular file-IO / memory / signal / futex /
  clone(threads) / prlimit / statx set; `newfstatat` (glibc stat and the
  dynamic loader use it) and `clock_getres` (probed at every JVM start).
- **clone/fork/vfork/clone3**: allowed in both profiles (compilation needs
  cc1/as/ld children; the run profile allows the fork family for legitimate
  cases like python multiprocessing, bounded by pids.max).
- **AF_UNIX socket group** (socket/connect/bind/listen/accept/setsockopt/
  getsockname etc.): allowed in **both** compile and run profiles — the JDK
  opens a socket(AF_UNIX) at startup. The sandbox sits in a network
  namespace with no interfaces, so allowing these grants no egress; there is
  no AF_INET object to connect to at all.
- **Compile profile differences**: the seccomp allowlist is currently
  nearly identical between compile and run; resources and writable views
  match too. If the run profile is ever tightened, remember the interactive
  interactor also runs in it.

**Chunked linear chain**: the allowlist started as a single comparison chain
with each jump offset computed in uint8 — past 127 entries the offsets
wrapped, and legal calls fell into stale KILLs while illegal ones fell into
stale ALLOWS (a real incident: Java went all-RE with empty stderr,
`glibc stat()` killed by SIGSYS). The current implementation chunks at ≤120
entries, each chunk ending in a `JA +2` trampoline to the next chunk;
unbounded length is safe.

**Regression test**: `pkg/sandbox/seccomp_sim_test.go` embeds a cBPF
interpreter — verifying every allowlisted number reaches ALLOW, every
illegal number 0..459 reaches a kill action, and all jump offsets fit in
uint8. Windows dev machines cannot run it (pure Linux code); cross-compile
the test binary and run it on the judge:

```bash
CGO_ENABLED=0 GOOS=linux go test -c -o sandbox.test ./pkg/sandbox/
# after pushing to the judge: ./sandbox.test -test.v
```

**Why classic BPF instead of eBPF (read before "upgrading" the filter)**:

- The seccomp kernel interface **only accepts classic BPF**:
  `PR_SET_SECCOMP(SECCOMP_MODE_FILTER)` loads a cBPF instruction array
  (`sock_fprog`); there is no "eBPF seccomp" mode in the kernel — repeated
  upstream proposals were never merged. Docker / isolate / bubblewrap run
  on the same cBPF path.
- "cBPF is legacy, eBPF is faster" does not hold here: since kernel 3.18,
  every loaded cBPF program (including seccomp filters) is **transparently
  translated into internal eBPF and JIT-compiled**. Verified on the
  production host: `net.core.bpf_jit_enable = 1` (Debian 11 / kernel 5.10)
  — what executes is already JIT'd eBPF; cBPF is only the load format.
  Filter overhead is nanoseconds; "switching to eBPF" buys nothing in
  performance or security.
- The only true-eBPF alternative is **BPF LSM** (eBPF programs on LSM
  hooks): it needs root + `CONFIG_BPF_LSM` + the `lsm=bpf` boot parameter,
  its policy is **host-wide**, losing the per-process fail-closed allowlist
  semantics, and it is an order of magnitude more complex — not a fit for
  the OJ sandbox. Evolve the filter within this section's constraints.

### 2.6 Sandbox triage method

Start with `journalctl -u oj-api` to find the `submission judged` anchor for
the submission (one INFO line per finished judgement; SE adds an ERROR
line); without SSH, the admin log viewer page searches the same content.
Then go deeper by scene:

1. **Inspect the kept workspace**: CE/SE/**RE** keep `/oj-work/judge-<id>`
   with sources, compile_err.txt and reproduction scripts. Grep the judge
   log for `workspace kept`. Keeping RE scenes is the key to phantom
   verdicts (§2.7).
2. **SIGSYS → find the missing syscall**:
   `strace -f -o /tmp/trace.txt -p $(pidof oj-judge)`, trigger one
   submission, and find the last incomplete syscall before
   `killed by SIGSYS` — the fix is to add it to the allowlist plus a test.
3. **Compare host behavior**: `runuser -u nobody -- <cmd>` to distinguish
   "missing syscall" / "missing file" / "permission or ownership" problems.
4. **Judge machine logs**: `journalctl -u oj-judge` (compile status, per-task
   durations, blob cache misses, watchdog firings are all instrumented;
   non-OK runs additionally log exit/signal/msg).

### 2.7 War stories (read before changing the judging core)

All four were really hit while optimizing judging throughput and located
with the methodology above. Shared lesson: **any layer of the sandbox stack
(the test binary, the Go runtime, cgroup, rlimit) can detonate as a phantom
RE of the user program.**

1. **stage2 recursion in the test binary**. The sandbox's stage2 re-executes
   `/proc/self/exe`; the production judge's `main()` checks the `OJ_STAGE2`
   env var and enters Stage2Main — but the **main of a `go test` binary is
   testing.Main**, which doesn't know that variable, so the sandboxed child
   ran the whole test suite again (nesting its own stage2), and the parent's
   Wait watched a process recursively running tests with every assertion
   skating the watchdog edge. Fix: `pkg/sandbox/main_hook_test.go`'s TestMain
   checks `OJ_STAGE2` before entering the tests. **Any binary that runs
   sandbox tests must carry this hook.** Symptom: every "escape test" takes
   exactly WallLimit (watchdog kill) while all assertions pass — the child
   really did the work, just slowly.
2. **Pdeathsig vs PID namespace**. There was suspicion that Go forkExec's
   orphan check (getppid()==0) misfired inside a CLONE_NEWPID child,
   kill(pid-1) being ignored by the pidns init for unhandled signals and
   wedging waitid. Pdeathsig was removed: daemon-crash cleanup is covered by
   the lease scanner and the watchdog.
3. **rlimits biting stage2 itself**. setrlimit applies to the calling
   process — setting RLIMIT_AS during stage2 setup strangled the Go
   runtime's own multi-hundred-MB PROT_NONE arena, producing random
   "fatal error: runtime: cannot allocate memory" phantom REs. Fix: rlimits
   moved to the last moment of Stage2Main, **right before execve** — the
   limits bind to the post-exec image and leave the already-mapped Go
   runtime alone.
4. **The cgroup resource triple**. (a) `applyLimits` used to be **dead
   code** — runProcess never called it, so memory.max/cpu.max/pids.max never
   took effect and every MLE defense ran naked; (b) wiring it up, writing
   memory.max then failed EACCES — cgroup v2 rule: a child group's
   controller files are writable only when the parent's subtree_control
   enabled that controller; Preflight now enables memory/pids/cpu
   idempotently (on this box enabling controllers for proxy subgroups under
   the nsdelegate mount option hung the kernel in mem_cgroup_css_alloc —
   tests use the production base directly); (c) stage2 (the Go supervisor)
   shares the cgroup with the contestant program, so memory.max needs a
   64MB headroom or concurrent runs get the supervisor OOM-killed first.
5. **The concurrency budget is the host's memory, not a feeling**. Each
   sandbox run = a Go supervisor + tmpfs mounts + page tables ≈ hundreds of
   MB of real memory. Total judging concurrency = MaxParallel (tasks) ×
   CaseWorkers (parallel cases per task), and **that product must stay
   within the host memory budget**. On a 3.9 GB box 2×2 was enough for the
   kernel OOM-killer to manufacture phantom RE/SE. CaseWorkers defaults to
   1; when raising `OJ_MAX_PARALLEL` and CaseWorkers on bigger machines,
   budget first: `peak memory ≈ concurrency × (cgroup cap + 64MB headroom)`.

### 2.8 Judging efficiency design (algorithm level)

- **Per-case parallelism**: each case gets its own sub-workspace
  (`ws/cases/<idx>/`, build artifacts copied in); a worker pool runs
  `CaseWorkers` wide, results aggregate in case order; any case SE cancels
  sibling workers (the submission is already doomed — early exit saves
  resources).
- **First-fail-stop (contest semantics)**: `Task.StopOnFail` (proto
  `stop_on_fail`, set by judgehub when the submission is in a live contest
  and not practice) — after the first non-AC case the rest are marked
  SKIPPED without running. E2E verified: a 30-case problem WA on case 1
  actually ran 1 and skipped 29.
- **Zero-allocation streaming diff**: the default token comparison is a
  two-pointer streaming scan (no strings.Fields slice allocations),
  equivalence cross-verified against 3000 random corpora.
- **Comparison order**: full byte equality first (identical = AC, zero
  cost), degrading to the streaming token comparison.

## 3. API-side scheduling (internal/judgehub)

1. Submission validation (visibility/review/contest window/rate limit
   15/min/user) → code to disk → PENDING enqueued (Redis List; in-memory in
   dev).
2. After registration judges pull tasks by capacity → assemble the Task
   (code + limits + per-case sha manifest) → set JUDGING + a **15-minute
   lease**.
3. Results persist + WS broadcast on `submission:<id>` (contest problems
   additionally broadcast `contest:<id>`).
4. Fault tolerance: a judge disconnect immediately requeues its leased
   tasks; a background scanner reclaims expired leases; API startup runs
   `RequeuePending`.
5. Testdata delivery goes through
   `/internal/testdata/:pid/:case/:kind` (daemon-token auth, all-numeric
   path validation against traversal).

## 4. Known behavior boundaries (read before changing code)

- Contest problems are **independent copies**: setContestProblems clones the
  problem and testdata; edits never flow back; submissions must target the
  copy id (submitting to the original id 404s).
- `require_registration` defaults to true: submitting without registering is
  rejected.
- Stop services before upgrading judge binaries (systemd holds the old file;
  overwriting in place fails with Text file busy).
- Before adding a syscall to the seccomp allowlist, think through the threat
  model and **add a seccomp_sim_test case in the same commit**; any change
  to jump layout requires a full interpreter test run.
- New judge onboarding: install the toolchain → `/opt/oj/oj-judge --selftest`
  all passing → write oj-judge.env (token = the API's OJ_DAEMON_SECRET) →
  `systemctl enable --now`.
