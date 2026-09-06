# YSNB OJ admin & authoring guide

> For contest organizers, problem setters and admins. Sections follow
> "what do I want to do → where to click → what happens → verify". UI labels
> are shown in Chinese with translations inline.

## 1. Role quick reference

| Capability | user | setter | admin | super_admin |
|---|:-:|:-:|:-:|:-:|
| Practice, contests, user-authored problems (reviewed), external problem import (reviewed) | ✓ | ✓ | ✓ | ✓ |
| Direct authoring in the bank, problem package import/export, problem lists | | ✓ | ✓ | ✓ |
| Create/manage contests, in-contest judging (own contests) | | ✓ | ✓ | ✓ |
| Problem review (approve/reject), plugin registry view | | ✓ | ✓ | ✓ |
| Dashboard, judge monitor, log viewer | | | ✓ | ✓ |
| User management: ban user/setter, reset passwords, invite codes, CSV import, role user↔setter | | | ✓ | ✓ |
| API keys, webhook target management | | | ✓ | ✓ |
| Grant/revoke admin & super_admin, ban admins | | | | ✓ |

Boundaries (enforced server-side; the UI merely follows):

- Nobody can change their own role. At any moment there is **exactly one**
  super_admin — granting super_admin to someone else automatically demotes
  the previous holder to admin (succession: the new one takes over, the old
  one steps down); revoking the last one is rejected. On fresh installs the
  bootstrapped admin is the super_admin with uid 0.
- Admins only move users between user↔setter and only ban user/setter;
  granting/revoking admin and banning admins are super_admin-only.
- Bans take effect immediately: the next request from a banned user is
  rejected — login and submissions included.

## 2. Author a problem

**Prerequisite**: setter or above.

**Steps**:

1. Admin console → 「题目管理」Problem management → 「新建题目」New problem,
   which opens the full-page editor (`/problems/new`; edit on the left, live
   preview on the right). Edit existing problems via 「编辑」Edit
   (`/problems/:id/edit`).
2. Fill in: title, statement (WYSIWYG: code blocks / images / links, live
   preview), input/output description, hint (optional), tags, source; add
   samples pairwise with 「+ 添加样例」.
3. Set parameters: time limit (ms), memory (MB), visibility (hidden (author
   only) / registered only / public), judge mode (standard / SPJ /
   interactive).
4. 「保存」Save. **Test data can only be uploaded after saving** (the
   「测试数据」section appears then).
5. Upload test data (below); use 「复制题目」duplicate and 「导出 zip」export
   from the editor as needed.

**Test data**:

- Whole-package zip named `1.in/1.out`, `2.in/2.out`… — 「整包 zip 替换」
  replaces all old cases at once; or 「追加单个测试点」adds single `.in`
  (required) + `.out` pairs.
- The case list previews in/out per case and deletes single cases.
- Interactive problems may omit `.out` (the interactor judges correctness).

**Verify**: submit a correct solution (should AC) and an intentionally wrong
one (should WA); check that the data is strong enough and limits are sane.
Samples must match the statement.

**Example**: A+B — 1000 ms, 256 MB, visibility registered-only, standard
mode, 2 samples, a 10-case zip.

### SPJ problems (judge mode = special judge)

- The checker is a single C++17 source file (testlib style) — paste it into
  the text box or upload a `.cpp` with 「上传并保存」.
- Convention: `./checker <input> <answer> <output>`.
- Exit codes: `0` = AC; **any non-zero exit = WA**, with the first stderr
  line shown to the contestant (`checker exit N` when stderr is empty).
- The judge caches compiled artifacts by source sha256; changing the source
  triggers automatic recompilation.

### Interactive problems (judge mode = interactive)

- The interactor is a single C++17 source file: `./interactor <input>`, its
  stdin/stdout piped straight to the contestant's stdout/stdin, both inside
  sandboxes (the interactor is network-jailed and resource-limited too).
- Exit codes: `0` = pass (if the contestant's program TLEs/REs, the
  contestant-side status wins); `1` = WA (first stderr line as feedback);
  **any other code (including `2`) = SE** — a judge-side fault, triage via
  the log viewer (section 11).
- Deadlock protection: either process exiting kills the other; the
  interactor has a fixed 30 s watchdog.

### IOI score table (per-case scores)

- Only applies to IOI contests (ACM ignores it); upload test data first.
- No web form yet — configure via API (setter+):
  `PUT /api/v1/problems/:id/case-scores` with a body like
  `{"scores": {"1": 20, "2": 30, "3": 50}}`; `GET` on the same path shows
  the current table and total.
- Server-side validation: case numbers must exist and the total must be
  positive. Convention: make each problem total **100** — IOI standings rank
  by total score, which reads best.

## 3. Import a problem package (native / DOMjudge / Hydro)

**Prerequisite**: setter or above; a package zip on hand.

**Steps**: admin console → 「题目管理」→ pick the zip → 「导入题目包」. All
three formats are auto-detected:

| Format | Signature | Imported contents |
|---|---|---|
| Native (this OJ's export) | `meta.txt` | Statement/metadata/testdata/checker.cpp/interactor.cpp (full round-trip) |
| DOMjudge | `problem.yaml` + `data/{sample,secret}` | Title/limits/`.in`/`.ans` (nested dirs flattened & renumbered)/validator source imported as checker |
| Hydro | `problem.yaml` + `problem_zh.md` + `testdata` | Title/limits/data/`check/checker.cpp` (imported as SPJ) |

- Imported problems are always 「隐藏」hidden; the editor opens automatically
  — review, then change visibility manually.
- **DOMjudge validators use a different argument convention than this OJ**
  (this OJ: `./checker <in> <ans> <out>`, exit 0 = AC) — review the checker
  logic manually after import.
- Packages where cases have `.in` but no answers are skipped entirely
  (interactive problems: use the native format, or upload cases after
  import).

**Verify**: toast 「已导入 #ID（N 个测试点，格式 X）」→ spot-check two case
previews in the editor → a correct solution should AC.

## 4. Import a statement from an external OJ

**Prerequisite**: any logged-in user can import; the platform must be in the
plugin registry (built-in statement crawlers: codeforces, luogu). Two
entries, by role:

- **Admins / setters**: admin console → 「题目管理」→ 「外部题面导入」 —
  imports land directly in the bank (no review).
- **Regular users**: 「个人中心」Profile → 「导入外站题目」 — imports enter
  the review queue (hidden until approved).

**Steps** (identical in both places):

1. Pick the source, enter the problem id (Codeforces `1900A`, Luogu
   `P1001`), click 「预览」Preview to check the statement and notes, then
   import.
2. An import = statement + public samples (Codeforces statements include
   input/output sections and sample code blocks with renderable formulas;
   Luogu may fail temporarily due to anti-scraping — retry later).
3. **No test data**: the problem carries a 「待补测试数据」awaiting-testdata
   badge (derived from: external source + zero cases; visible in "My
   problems", the review queue and the problem detail page). It cannot be
   used for official judging or contests until test data is added.

**Verify**: admin imports jump to the editor; a regular user's import shows
up in the review queue with an 「外部导入 · 待补测试数据」badge and goes
public after approval.

## 5. Export / share a problem (fairness red line)

The editor's 「导出 zip」is a **full export**: `statement.md` + `meta.txt` +
all `testdata/` + `checker.cpp`/`interactor.cpp` (SPJ/interactive sources
included automatically; the judge mode is restored on import) — enough to
recreate the problem on another YSNB OJ.

To share with outside sites/schools use the **shared export**: the same
endpoint with `?shared=1`
(`/api/v1/problems/:id/export?shared=1`). A shared package only contains the
statement + testdata generated from public samples — **no checker /
interactor and no hidden test data**.

> Red line: the two channels must not be conflated. Full test data and the
> checker never leave through the shared channel — only send `-shared`
> packages. The checker encodes your judging intent; leaking it is leaking
> the reference solution's logic.

## 6. Review user-authored problems

Problems created by regular users in 「我的题目」enter the review queue;
they stay hidden and unjudgeable until approved.

**Steps**: admin console → 「题目审核」(visible to setter and above) →
「通过」Approve or 「驳回」Reject per item.

- Approve: the problem joins the bank (registered users only) and the author
  is notified.
- Reject: the author can edit and resubmit.
- Review checklist: complete statement, correct samples, limits present, test
  data uploaded, source attribution proper. Items with an 「外部导入 ·
  待补测试数据」badge are regular users' external-OJ imports — statement and
  samples came in automatically, **add test data before approving**.

**Verify**: after approval a regular account can find and submit the problem
in the bank.

## 7. Run an ACM contest (freeze & reveal)

**Prerequisite**: setter or above; problems ready in the bank (attaching a
problem creates an independent in-contest copy; later edits to the bank
original do not affect the contest).

**Steps**:

1. Admin console → 「比赛管理」: name, description, start/end time.
2. Mode: 「ACM（罚时 + 首对即停）」.
3. Toggles: 「启用封榜」auto-freeze (with 「封榜时间」freeze time; empty =
   no auto-freeze), 「赛时手动封榜」manual freeze (disabling it rejects the
   jury's manual freeze switch), 「需要报名」registration required.
4. Pick bank problems under 「初始题目」, click 「创建（ACM 赛制）」.
5. After creation, manage problems further down: attach bank problems with
   「复制挂入（独立副本）」, or 「新建专属题」create contest-original hidden
   problems that never join the public bank; 「删除」deletes along with test
   data.

**Freeze & reveal** (contest page → 「裁判台」jury console; visible to the
creator or admins):

- During the freeze contestants see the standings as of the freeze moment;
  the jury still sees live data.
- Auto-freeze fires at 「封榜时间」; the jury's manual switch can freeze or
  unfreeze at any time.
- Reveal: enter a count in the 「滚榜揭示控制台」reveal console and teams are
  unmasked **from the last real rank upward**, their true results appearing
  as you go (pair it with projection mode for the award ceremony).

**Verify**: before the contest, register a test account and submit; check
solved counts and penalty math. After freezing, the contestant view stops
updating while the jury view stays live.

## 8. Run an IOI contest (score table)

Same flow as ACM with two differences:

1. Mode: 「IOI（测试点部分分，按总分排名）」.
2. Configure each problem's score table before the contest (end of section
   2); 100 points per problem reads best. ACM ignores the table; under IOI
   each case awards its configured partial score.

**Verify**: submit code that passes only some cases — the standings cell
should show partial score, not 0/1.

## 9. In-contest jury operations (contest page → 「裁判台」)

Jury = contest creator or admin. Entry: contest page → 「裁判台」tab.

- **Notices**: the announcement card at the top — type and 「发布」, delete
  anytime (e.g. "Problem B data updated and rejudged").
- **Adjust times**: 「比赛时间」— changing start/end takes effect
  immediately (countdown and freeze plan follow). Use for delays/extensions.
- **Registration**: view the list, switch regular/★starred, or delete.
  Starred teams don't occupy ranks — typical for coach or guest teams. In
  team contests the captain registers for the team.
- **User flags**: search and select a contestant, tick 「打星（不占名次）」
  starred or 「作弊（出榜标红）」cheating (row turns red), save to update the
  standings; 「已标记用户」lists flags for one-click undo.
- **Rejudge problem**: 「重测本题」on each problem in the 「题目」tab —
  re-queues all valid submissions of that problem (after fixing broken data;
  post an announcement too).
- **Cancel/restore/rejudge a single submission**: per-row actions in the
  「提交记录」tab; cancelled submissions score zero, restored ones can be
  「重判」rejudged.
- **CSV export**: 「导出榜单 CSV」at the top of the contest page.
- **Projection mode**: 「投屏模式」opens `/contests/:id/board` full-screen in
  a new tab, for projectors.

**Example (cheating)**: confirm a contestant copied → flag 「作弊」in the
jury console → save → their standings row turns red and drops off; to wipe
the score entirely, cancel all their submissions in 「提交记录」.

## 10. Manage users

Entry: admin console → 「用户管理」(admin and above).

- **Invite codes**: set 「可用次数」uses (1–999) and 「有效小时」valid hours
  (0–720, 0 = unlimited), then 「生成」; the list shows used/max and expiry,
  manual delete available (existing users unaffected).
- **CSV bulk import**: format
  `username,student_no,nickname,password` (password optional — a random
  10-char password is generated). A result table lists username / initial
  password / error; bad rows and duplicates (`username X already exists`)
  are flagged per-row without affecting others. **Generated passwords are
  shown once** — export and distribute immediately. 5000 rows per batch.
- **Reset password**: per-row 「重置密码」; the new password shows once in
  the dialog — hand it over offline.
- **Ban/unban**: per-row button. Admins ban user/setter; banning an admin
  needs super_admin. Bans take effect immediately.
- **Role**: per-row dropdown. Admins only user↔setter; super_admin grants /
  revokes admin and super_admin. You cannot change your own role; the system
  guarantees exactly one super_admin (auto-succession).
- **Edit**: per-row 「编辑」changes student number and nickname (username is
  immutable).

## 11. Judge status & triage

### Judge monitor (admin console → 「判题机监控」, admin+)

- A banner shows the waiting queue length; a table lists each judge's
  status, capacity, active tasks and last heartbeat (silent for 60 s →
  offline).
- WebSocket push + a 10 s poll — no manual refresh.
- 「数据总览」Dashboard (admin+) adds 8 metric cards, a 14-day submission
  trend, 24-hour judging concurrency and judge load, auto-refreshing every
  30 s.

### Log viewer (admin console → 「日志查看器」, admin+)

Reads journald for the oj-api and oj-judge systemd units:

- Process: all / API / judge; keyword filter (Enter); lines 100/200/500/1000;
  auto-refresh every 15 s or manual 「刷新」.
- Colored level badges: error red / warn orange / info green / debug grey.
- The server calls journalctl with a fixed argument set (argv never varies
  with the request — no injection surface), fetches the last 2000 records of
  both units and applies your filters server-side.
- Prerequisite: the system user running oj-api must be in the
  `systemd-journal` group (a one-time ops step, see deploy.md); otherwise the
  page reports a journalctl read failure.

### Worked example: a submission got SE

1. Grab the submission id (the number in the submissions page URL).
2. Log viewer → process 「判题机」→ keyword: that id → search.
3. Every judged submission logs an INFO `submission judged` line (status /
   time / memory); SE additionally logs an ERROR line naming the stage
   (testdata fetch failure, checker/interactor abnormal exit, sandbox error…).
4. Fix the cause and rejudge: in-contest submissions via 「重判」in
   「提交记录」; bank submissions via admin `POST /api/v1/admin/rejudge/:id`.

### Common judging faults

- Everything PENDING: judge offline → check `OJ_DAEMON_TOKEN` matches the
  API's `OJ_DAEMON_SECRET`, and `systemctl status oj-judge`.
- Queue backlog: temporarily raise the judge's `OJ_MAX_PARALLEL` (edit env,
  restart), or add a judge machine (multi-judge is native). Run
  `oj-judge --selftest` on any new judge first.
- One language all CE: missing compiler on the judge — install the
  toolchain, `systemctl restart oj-judge`.
- Everything SE: usually a full disk or an unwritable judge workspace.

## 12. Issue an API key for external tools

Entry: admin console → 「API 密钥」(admin+).

1. Enter a name (e.g. `rank-bot`, `dashboard`) → 「生成新 Key」.
2. The full key is shown once: `ojk_` prefix + 32 hex chars — copy it now.
3. Usage: header `X-API-Key: <key>` (or query param `?api_key=`).
4. The list shows each key's prefix, status, creation time and last use;
   「吊销」Revoking kills it immediately — calls using it fail at once.

Note: public read endpoints (`/api/v1/public/...`: user summary, problem
list, samples, cph bridge) work anonymously; keys are for auditing and
protected endpoints. If a key leaks, revoke it in the list.

## 13. Plugin registry / webhooks

Entry: admin console → 「插件系统」(registry visible to setter+; webhook
management needs admin).

- Lists the three compile-time plugin kinds: problem crawlers (for import —
  built-in codeforces, luogu), practice platform adapters (for the stats
  report — built-in codeforces, luogu, atcoder, nowcoder), and event hooks
  (webhooks).
- **Webhook targets**: after each judgement the server POSTs JSON
  (`id/status/score/problem_id/contest_id/user_id/at`) to the target URL —
  ideal for bots and leaderboards. Add/update by name + URL, delete by name.
- URLs pass SSRF protection: http/https only; loopback and private targets
  are rejected. Validation happens on save; invalid URLs error out without
  being stored.

## 14. Backup & restore (routine)

- `/opt/oj/backup.sh`: cron at 03:00 daily (PostgreSQL dump + data-dir
  snapshot), keeping 14 daily + 6 monthly backups; log at
  `/opt/oj/backup/backup.log`.
- `/opt/oj/restore-drill.sh <dump>`: restore drill (throwaway container or a
  temporary local DB — never production), runs automatically at 04:00 on the
  1st monthly; results in `/opt/oj/backup/drill.log`.
- The judge's blob cache is a cache — no backup needed; `/opt/oj/data`
  (testdata etc.) is the original asset.
- Manual restore steps: `../operations/deploy.md`.

## 15. Weekly checklist (suggested)

| Check | How |
|---|---|
| Services | `systemctl status oj-api oj-judge nginx` — all active |
| Backup | 03:00 backup succeeded in `backup.log`, dump size sane |
| Restore drill | After the 1st, `drill.log` ends with `drill PASSED` |
| Disk | `df -h /` (data dir + judge workspace + backups) under 80% |
| Judges | Judge monitor: online, queue at zero |
| Logs | Skim the last 200 lines in the log viewer for odd errors |
| Security updates | `apt update && apt upgrade`; env files still 600 |

## 16. Troubleshooting quick reference

| Symptom | Fix |
|---|---|
| All submissions PENDING | Judge monitor: offline? Check token and service per section 11 |
| One language all CE | Compiler missing on the judge; install and `systemctl restart oj-judge` |
| One language all SE/RE, empty errors | Sandbox seccomp kill; log viewer → judge → search `SIGSYS`, collect and report |
| One problem mass-WA (bad data?) | Fix data → 「重测本题」in the contest → post an announcement |
| Standings not refreshing | Freeze on? Refresh the page to reconnect the WebSocket |
| Log viewer errors | oj user missing from `systemd-journal` — follow deploy.md, retry |
| Site 502 | nginx and oj-api alive? nginx `client_max_body_size` ≥ 160M? |
| API keys all failing | Accidental revocation (API key page, status column) — regenerate and redistribute |
