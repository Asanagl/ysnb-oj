# Public API reference (v1 · /api/v1/public/*)

> A read-only public API for bots, dashboards, CLI/IDE tools and cph
> integration. Fully isolated from the site's JWT API: `/public/*` is
> anonymously readable, protected endpoints use an API key.
>
> **Scope of this page**: the `/public/*` API and the M5 integration
> endpoints (plugins / external problem import / shared export / IOI /
> external practice). Logged-in business endpoints and admin endpoints
> (e.g. `GET /admin/logs`) are out of scope — admin capabilities are
> covered in `guide/admin-guide.md`.

## Authentication

| Mode | Notes |
|---|---|
| Anonymous | Call directly; all read endpoints work, sharing one 60 req/min bucket |
| API Key | Header `X-API-Key: ojk_xxx` (tools that cannot set headers may use `?api_key=`); its own bucket (60/min) with last-used auditing |

Key management: admin console → **API 密钥** (API keys) page to mint/revoke.
The full key is shown once at creation; only the hash is stored; revocation
is immediate.

Rate limiting: HTTP 429 + `{"error":"API 限流：每分钟最多 60 次请求"}`.

## Endpoints

### GET /api/v1/public/users/:id

A user's practice summary. `:id` accepts a numeric id or a username.

```bash
curl http://<host>/api/v1/public/users/admin
```

Response (excerpt):

```json
{
  "user": {"id": 1, "username": "admin", "nickname": "Admin"},
  "by_status": {"AC": 12, "WA": 3},
  "ac_problems": 10,
  "tried_problems": 14,
  "recent_ac": [
    {"problem_id": 1, "submission_id": 42, "time_ms": 14, "language": "cpp", "at": "2026-09-01T12:00:00Z"}
  ],
  "activity": [{"date": "2025-09-02", "submissions": 0, "ac": 0}]
}
```

- `recent_ac`: the **first AC** per problem, newest first, up to 20 entries
  (same semantics as the on-site profile page)
- `activity`: a contiguous 365-day daily series (zero-filled, oldest first),
  ready to feed a heatmap component

### GET /api/v1/public/problems

The public problem catalog (hidden problems and contest copies never
appear). `?q=` searches by title; `?page=&size=` paginates (size ≤ 100).

```bash
curl "http://<host>/api/v1/public/problems?page=1&size=20"
```

Response: `{"total": n, "items": [{"id","title","time_limit_ms","mem_limit_mb","judge_mode","tags":[],"sample_count"}]}`

### GET /api/v1/public/problems/:id/samples  (API key required)

The problem's **public samples** (structured JSON). The full judging test
data (the complete .in/.out set) is **never** exposed through the public
API — a fairness red line.

```bash
curl -H "X-API-Key: ojk_xxx" http://<host>/api/v1/public/problems/1/samples
```

Response: `{"problem_id","title","time_limit_ms","mem_limit_mb","samples":[{"input","output"}]}`

### GET /api/v1/public/problems/:id/cph  (API key required)

A problem JSON directly importable by **cph** (VSCode Competitive
Programming Helper):

```bash
curl -H "X-API-Key: ojk_xxx" http://<host>/api/v1/public/problems/1/cph
```

The response is cph's import format:
`{"name","group","url","timeLimit","memoryLimit","tests":[{"input","output"}]}`.
Two integration options:

1. Your tool fetches this JSON and writes it into cph's import;
2. For "type an id in cph and the problem appears": write a small helper (or
   a qqcot integration) listening for the local Competitive Companion push
   (cph listens on 127.0.0.1:4244 for browser pushes by default); on
   receiving an id, call this endpoint and reshape the JSON into a Companion
   push object. Note cph has no public remote-source protocol — a pure
   server-side push into cph is not possible.

## Error codes

| Status | Meaning |
|---|---|
| 400 | Bad parameters |
| 401 | Missing key / invalid or revoked key (protected endpoints only) |
| 404 | User or problem not found (including hidden/contest problems — existence is not disclosed) |
| 429 | Rate limited (60/min/bucket) |

## On-site integration endpoints (M5, JWT auth)

These run under the regular logged-in API (not `/public/*`), for the SPA
and bot proxies.

### Plugin system (compile-time registry)

| Endpoint | Role | Notes |
|---|---|---|
| `GET /admin/plugins` | setter+ | Registered plugins + webhook targets |
| `GET /admin/plugins/sources` | admin+ | Platform id list (crawlers / practice adapters) |
| `GET /external/problems/sources` | any logged-in user | Importable platforms for the all-user import card (crawler names only) |
| `PUT /admin/hooks/:name` | admin+ | Set webhook target `{"url": "https://…"}` (SSRF-checked) |
| `DELETE /admin/hooks/:name` | admin+ | Remove a target |

After each judgement, the server POSTs JSON to every webhook target:
`{"id","status","score","time_ms","memory_kb","problem_id","contest_id","user_id","at"}`
(score is the IOI partial credit; always 0 in ACM mode). Third parties like
QQ bots only need to register a URL.

### External statement import (problem crawling)

| Endpoint | Role | Notes |
|---|---|---|
| `POST /external/problems/preview` | any logged-in user | `{"source":"codeforces","external_id":"1900A"}` → metadata preview, nothing stored |
| `POST /external/problems/import` | any logged-in user | Same params + optional `visibility` (default members) → creates the problem; regular users' imports enter review (`needs_review: true`), setters+ land directly in the bank |

Imported problems contain only the **statement + public samples** (full
test data cannot be fetched from external platforms). Source is annotated
`[外部训练题·平台 ID]`; regular users' imports stay hidden until approved
and carry the awaiting-testdata marker until a setter/admin adds data.

### Shared export (bank sharing, red-line protected)

`GET /problems/:id/export?shared=1` — a package **without full test data**:
statement.md + meta.txt + testdata/ generated from public samples. Hidden
data and checker.cpp never leave through this channel (fairness red line);
the on-site full export without `shared` behaves as before.

### IOI contests

- Create: `POST /contests` with `"mode": "ioi"` (immutable after creation).
  ACM contests default to acm when omitted.
- Score table: `PUT /problems/:id/case-scores` `{"scores":{"1":30,"2":70}}`
  (setter+), `GET` reads it back. Scores clone with the contest copy of the
  problem.
- Judging: each case that ACs awards its score; the submission's score is
  the sum of passed cases; verdicts follow ACM semantics (the first failure
  decides WA/TLE/…, all-pass is AC). Standings rank by the sum of each
  problem's **historical best score**.
- Field reuse: `penalty_ms` holds the **total score** in IOI contests, and a
  cell's `solved_ms` is that problem's best score; the frontend ScoreBoard
  switches headers on `contest.mode`.

### External practice stats (practice report)

| Endpoint | Notes |
|---|---|
| `GET /external/bindings` | My platform bindings + available platforms |
| `PUT /external/bindings` | Bind `{"platform":"codeforces","handle":"tourist"}`; binding triggers a first full sync |
| `POST /external/bindings/:platform/sync` | Manual incremental sync |
| `DELETE /external/bindings/:platform` | Unbind (history is kept) |
| `GET /external/report/:id` | Practice report: per-platform submit/AC/solved, a 365-day merged activity series, cross-platform recent AC |

First platforms: **codeforces / luogu / atcoder / nowcoder**. Luogu takes
the username, Nowcoder a numeric ID or profile link, AtCoder the user id.
The server syncs incrementally every hour, reading public data only — no
platform passwords or cookies are stored.

The profile heatmap (`GET /users/:id/profile`) carries a `platforms` field:
per-platform daily buckets (365 days, zero-filled) that the frontend can
overlay onto local data via multi-select.

## Python example (qqcot bot)

```python
import requests

BASE = "http://<host>/api/v1"
KEY = "ojk_xxx"

def user_summary(name_or_id):
    r = requests.get(f"{BASE}/public/users/{name_or_id}", timeout=10)
    r.raise_for_status()
    return r.json()

def problem_cph(pid):
    r = requests.get(f"{BASE}/public/problems/{pid}/cph",
                     headers={"X-API-Key": KEY}, timeout=10)
    r.raise_for_status()
    return r.json()
```

## JS example (dashboard)

```js
const res = await fetch(`/api/v1/public/users/${name}`)
const { ac_problems, activity, recent_ac } = await res.json()
```
