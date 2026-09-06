# YSNB OJ user guide

> For contestants and all regular users. Everything runs in the browser, in
> Chinese by default (the UI has light/dark themes; an English UI is on the
> roadmap — this page walks you through the Chinese labels).

## At a glance

Top navigation: **首页 Home / 题库 Problems / 提交记录 Submissions / 比赛
Contests / 题单 Lists / 小组 Groups / 个人中心 Profile** (admins also get a
后台管理 Admin console entry). The icon on the right switches theme:
**light / dark / follow system**. On narrow screens the nav collapses into a
drawer behind the logo.

「首页」(Home) is your dashboard: 「我的统计」(my stats — solved count and
verdict breakdown) on the left, 「我最近的提交」(my recent submissions) on
the right.

## Register & log in

1. Get an **invite code** from an admin (codes have usage and expiry limits).
2. Open the site, switch to the 「注册」Register tab, and fill in: username
   (3–32 chars), nickname (optional), student ID (required), password (6+
   chars), invite code.
3. After "registered" switch back to 「登录」Log in and sign in with your
   username and password.

Forgot your password? An admin can reset it. Login is rate-limited to
10 attempts/min — if you keep failing, wait a moment.

## Practice

1. Open 「题库」Problems; search by title. Each row shows time limit, memory
   limit and type (standard / special judge / interactive).
2. Open a problem: statement (Markdown, LaTeX formulas, code highlighting)
   and samples on the left; a 「提交代码」Submit panel on the right.
3. Pick a language, write or paste code, hit 「提交」Submit.
4. A 「最近提交」Recent submission live card appears below and follows the
   whole run **without any page refresh**: status advances from **排队中
   (queued) / 编译中 (compiling) / 评测中 (judging)** — with a spinner and an
   `done/total` counter while judging — to the final verdict, shown as a big
   **full verdict name** (`Accepted`, `Wrong Answer`, …). While judging, a
   strip of **per-case dots** lights up one by one (green ✓ pass / red ✗
   fail / grey pending), so you can see how far judging has got. When done
   you also get time / memory / score (IOI) stats. Contest problem pages
   have the same card.

> Lists, standings and detail pages use short codes (AC / WA / TLE…); only
> this live card uses full verdict names: AC=Accepted, WA=Wrong Answer,
> TLE=Time Limit Exceeded, MLE=Memory Limit Exceeded, RE=Runtime Error,
> CE=Compile Error, SE=System Error.

Language resource multipliers (applied on top of each problem's limits):

| Language | Time multiplier | Extra memory | Notes |
|---|---|---|---|
| C++ 17 (g++) | 1× | +8 MB | `g++ -O2 -std=c++17` |
| Python 3 | 4× | +128 MB | interpreted; startup cost is inside the multiplier |
| Java 17 | 3× | +512 MB | JVM flags are pre-configured |

Verdict codes:

| Code | Meaning |
|---|---|
| AC | Accepted |
| WA | Wrong answer (differs from expected output, or rejected by the checker) |
| TLE | Time limit exceeded (after the language multiplier) |
| MLE | Memory limit exceeded (after the language allowance) |
| RE | Runtime error (crash, non-zero exit) |
| CE | Compile error (compiler output is visible in the submission details) |
| SE | System error (not your fault — ping an admin) |
| PENDING / COMPILING / JUDGING | Queued / compiling / judging |
| SKIPPED | This case was skipped, see below |

**First-fail-stop**: during a contest, once a case fails the rest are marked
`SKIPPED` and not run — in contest you only need pass/fail, and judge capacity
goes to other contestants. Practice submissions always run every case so you
can pinpoint the failure.

The 「提交记录」Submissions page filters by "mine only" and status; click any
row for code and per-case details. If the submission is still being judged
when you open it, the detail page **follows along automatically** (the
per-case table refreshes as results land) — no manual refresh. Submissions
are limited to 15/min per user.

## Solutions

The 「题解区」solution area sits under each problem. **You must have submitted
to that problem at least once (any verdict) to read or post**; during a live
contest its problems are fully locked, opening automatically when the contest
ends.

Click 「写题解」to post: title optional, WYSIWYG body with bold, headings,
code blocks, images and links. Every post can be 「回复」replied to; posts by
the author or admins carry a green 「官方题解」official badge and are pinned.

## Train with problem lists

1. Open 「题单」Lists — all lists are publicly browsable.
2. Open a list: each problem shows your progress — **not attempted /
   attempted / AC** — click the title to jump in.
3. If you are in a group, the group page also shows the captain's shared
   lists (see "Create & manage groups" below).

## Join a contest

1. Open 「比赛」Contests; each row shows its status: upcoming / running / ended.
2. Open a contest: countdown and announcements at the top (watch them during
   a contest — data updates and rejudges are announced there).
3. **Register before you submit**: fill a team name (optional) in the
   「报名参赛」card, pick 「正式队伍」regular or 「打星队」starred (unranked),
   and hit 「报名」Register. You can rename (Enter to save) or 「取消报名」
   cancel. Without registering you cannot submit inside the contest (practice
   after the contest is exempt).
4. Open problems from the contest's 「题目」tab. **Contest problems are
   independent copies — submit from inside the contest or your submission
   will not count toward the standings**; submitting to the same-named problem
   in the problem bank does not count.

### Reading the standings

The 「榜单」standings tab refreshes live. Cells mean different things per mode:

- **ACM**: ranked by solved count, ties broken by penalty (minutes). Green =
  solved (`+minutes`), red = wrong attempts (`-N`).
- **IOI**: ranked by total score, no penalty; each problem takes your best
  submission's score (per-case partial credit), shown directly in the cell.

Legend: `★ 打星不占名次` (starred teams are computed but unranked — the rank
cell shows ★), red names = flagged for cheating by the jury.

**Freeze**: near the end the standings freeze with a 「榜单已冻结」banner.
Post-freeze submissions show only a yellow `?N` (N pending submissions to
reveal); results are revealed and settled automatically at contest end.

Buttons for 「导出榜单 CSV」CSV export and 「投屏模式」projection mode
(full-screen auto-scrolling board) are also available.

The contest's 「提交记录」tab lists every submission in the contest; during
the freeze it masks verdicts there too — you only see pending counts.

### After the contest: practice mode

When a contest ends it switches to practice mode: a 「已结束 · 补题模式」
banner appears, problem pages carry a 「补题」badge, anyone can keep
submitting — **standings are no longer affected** — and the solution area
opens for review.

## Team contests (ICPC, three per team)

When a contest enables team mode, the registration card becomes
「组队报名（ICPC 三人一队）」:

1. Create your team and gather members in 「小组」Groups first (next
   section), and make sure you are the **captain**.
2. Back on the contest page, pick your team from the dropdown, choose regular
   or starred, and click 「以队报名」Register as a team.
3. From then on **any member's submission counts for the team**, and the
   standings aggregate one row per team.

Empty dropdown means you are not the captain of any group — create one in
「小组」first.

## Create & manage groups

**Join**: on 「小组」Groups, enter an invite code (ask the captain) and click
「加入」Join.

**Create**: click 「创建小组」, fill in name, bio and member cap (1–5, 3 for
ICPC teams). Creator becomes captain.

A group page has four blocks: member list, shared lists, group notices, and
an **internal ranking** (sorted by deduplicated AC count across shared lists).
All captain actions live there:

- Members: pull by username, remove, transfer captaincy;
- Invite code: displayed, one-click reset (old code dies instantly);
- Notices: publish/delete group notices;
- Shared lists: share your lists with the whole group, unshare anytime.

Any member can 「退出小组」leave; note **a captain leaving disbands the
group**.

## Your practice data

Open 「个人中心」Profile (click anyone's username in standings or solutions
to see their page too):

- Header stats: problems solved, problems attempted, AC rate;
- **365-day activity** heatmap: local submissions are always included; tick
  「合并平台」to overlay bound external platforms;
- 30-day trend (submits/AC), verdict pie, per-tag strength (sorted by
  attempts), verdict breakdown.

## Bind Codeforces / Luogu and other accounts

Binding and stats are integrated into 「个人中心」Profile:

1. Find the 「外站 OJ 刷题数据」card.
2. Pick a platform (Codeforces / 洛谷 Luogu / AtCoder / 牛客 Nowcoder), enter
   your account — Luogu takes the username, Nowcoder a numeric ID or profile
   link — and click 「绑定并立即同步」.
3. Afterwards the system **syncs incrementally every hour** (public data
   only); 「立即同步」refreshes on demand and 「解绑」unbinds (already-synced
   history is kept).
4. Below in the same card: per-platform submit/AC/solved badges and a
   cross-platform recent-AC list; tick the heatmap's 「合并平台」to overlay
   external records onto the heatmap.

Luogu and Nowcoder have strong anti-scraping; when a sync fails the status
column shows why — retry later.

## Import a problem from another OJ

Besides authoring from scratch, you can import public problems from external
OJs for practice:

1. Open the 「导入外站题目」card in 「个人中心」, pick a platform (built in:
   Codeforces, Luogu), enter the problem id (e.g. `1900A` / `P1001`),
   「预览题面」preview, then import.
2. An import = statement + public samples; **test data is not fetched**. Such
   problems carry a 「待补测试数据」awaiting-testdata badge and cannot be
   properly judged until a setter/admin adds data.
3. Regular users' imports enter **review** like user-authored problems:
   hidden until approved, public in the bank afterwards; setters and above
   import directly into the bank.

Statement quality: Codeforces imports include the full statement (input /
output sections, sample code blocks, renderable formulas); Luogu may fail
temporarily due to anti-scraping — retry later.

## Author a problem

1. Open 「站点地址 + `/my/problems`」for the 「我的题目」page, click
   「新建题目」.
2. In the full-page editor fill in: title, statement (WYSIWYG with live
   preview), input/output description, hint (optional), limits, and judge
   mode (standard / SPJ / interactive).
3. 「保存并提交审核」saves and submits for review. Lifecycle: **pending →
   approved / rejected (edit and resubmit)**.
4. Before approval the problem is visible only to you and does not join the
   public bank; test data and samples are configured with admin help after
   approval. Rejected problems can be edited and resubmitted, or resubmitted
   directly from the list with 「重投」.

## FAQ

| Symptom | Fix |
|---|---|
| Submission stuck in PENDING | Judges may be busy (many concurrent submissions); wait — over 5 minutes, ping an admin |
| Standings cell shows `?3` with no verdict | Freeze is on; `?N` = N post-freeze submissions awaiting reveal, resolved at contest end |
| Submission blocked, asked to register | Registration-required contests need registration first; practice after the contest is exempt |
| How to write interactive problems | Your stdin/stdout is wired straight to the judge's interactor — read what it sends, answer, and flush every round; see the SPJ & interactive convention page |
| Invite code invalid | Codes have usage/expiry limits — ask an admin for a new one |
| "Too many requests" on login | Rate limit hit (10 logins/min) — wait and retry |
| Luogu/Nowcoder sync failed | Their anti-scraping is aggressive; retry 「立即同步」later in the practice-data card |
| Imported problem cannot be judged | External imports only carry statement + samples and are badged 「待补测试数据」; wait for a setter/admin to add data |
| Do I need to refresh to see results? | No — the problem page, contest problem page and submission detail page all follow to the verdict automatically |
| Site unreachable | Network or maintenance — ping an admin |
