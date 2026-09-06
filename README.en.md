<div align="center">

# YSNB OJ

**You Submit, Never Be rejected.**

A fully self-developed online judge: Go 1.27 backend + a from-scratch Linux
judging sandbox + Vue 3 frontend + PostgreSQL / Redis. Born out of a campus
ACM team's daily training and contests — batteries included.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/Asanagl/ysnb-oj)](https://github.com/Asanagl/ysnb-oj/releases)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Vue](https://img.shields.io/badge/Vue-3-4FC08D?logo=vuedotjs&logoColor=white)](https://vuejs.org)
[![Docs](https://img.shields.io/badge/Docs-VitePress-646CFF?logo=vitepress&logoColor=white)](https://asanagl.github.io/ysnb-oj/en/)

[简体中文](README.md) | **English**

📖 [Documentation](https://asanagl.github.io/ysnb-oj/en/) · 🐛 [Issues](https://github.com/Asanagl/ysnb-oj/issues)

<img src="docs/screenshots/home-light.png" alt="Home (light theme)" width="49%"/><img src="docs/screenshots/contest-ioi-dark.png" alt="IOI standings (dark theme)" width="49%"/>

</div>

---

## Why another OJ

Plenty of open-source judges exist, but most reuse go-judge or fork the
Hydro / DOMjudge sandbox. YSNB OJ takes the harder road: the **judging sandbox
is written from scratch** (cgroup v2 + namespaces + seccomp cBPF allowlist +
stage2 re-exec), and everything a campus ACM team actually needs ships in the
box — ICPC / IOI contests, team battles, freeze & reveal, SPJ / interactive
problems, multi-platform practice aggregation, a public API and a cph bridge.

## Features

| | Highlights |
|---|---|
| 🧑‍⚖️ **Judging core** | Home-grown sandbox (read-only root, uid 65534 drop, seccomp allowlist, watchdog), compile cache, per-case parallel judging, streaming diff; languages are config-driven (C++17 / Python 3 / Java 17 built in — adding one is a yaml stanza) |
| 🏆 **Contest modes** | ACM (live scoreboard / penalty / freeze / reveal), IOI (per-case scoring + partial credit), team contests (ICPC 3-person), practice mode, first-fail-stop |
| 📝 **Problems** | Markdown + KaTeX statements, SPJ / interactive problems (testlib-style convention), problem package import/export (native / DOMjudge / Hydro), external problem crawler, user-authored problems with review flow, solution area gated by submission |
| 📈 **Training** | Problem lists (live progress), groups (invite codes / notices / shared lists / internal ranking), profile with a 365-day heatmap, multi-platform practice aggregation (Codeforces / Luogu / AtCoder / Nowcoder) |
| ⚡ **Realtime** | WebSocket topics: submission status advances in place, per-case dots light up as judging progresses, standings refresh incrementally; the submission detail page follows along automatically |
| 🔌 **Platform** | Multi-judge (gRPC bidi stream + lease scheduling), public API + cph bridge (one-click problem setup in your IDE), webhook callbacks, maintenance CLI (oj-cli), structured logs + an in-admin log viewer |
| 🔒 **Security** | Sandbox escape suite runs as routine test assets, login/register/submit rate limiting, dual-layer security headers, API ports never leave the intranet; **full test data never leaves the judge (fairness red line)** |

<details>
<summary><b>Benchmarks</b> (measured on a 2C4G cloud box, API + judge co-located)</summary>

| Item | Result |
|---|---|
| Submission burst | 8 concurrent users × 8 problems = 64 submissions injected in 40s, 64/64 AC |
| Latency under burst | API 1.9–3.9 ms; standings computation 2.1–6.3 ms |
| Judging stability | 50-case problem × 12 consecutive runs, 12/12 AC, zero phantom RE/SE |
| E2E regression | Six E2E suites, 82 assertions, all green |
| Code size | 13.2k lines of Go, 6.2k lines of Vue/TS |

</details>

## 🚀 Quick start

Prerequisites: a Linux server (Ubuntu 22.04+ / Debian 12+), Docker ≥ 24, and
cgroup v2 enabled (`stat -fc %T /sys/fs/cgroup` prints `cgroup2fs`).

```bash
# 1) Build the frontend on your dev machine, sync the whole repo
#    (including frontend/dist/) to the server
cd frontend && npm ci && npm run build

# 2) On the server, as root, run one command
bash deploy/one-click.sh
```

The script handles everything: precheck → generates `.env` with openssl
(all secrets + the initial admin password; an existing `.env` is kept) →
`docker compose up -d --build` (postgres/redis/api/web) → waits for the API →
judge self-test (runs `--selftest` if a judge is installed, otherwise prints
a 3-step install guide — the judge runs on the host, not in a container) →
installs backup/restore-drill cron → frontend smoke test. Idempotent; safe to
re-run.

Prefer systemd without Docker? The
[bare-metal route](docs/en/operations/deploy) (option B) is equally complete.

### Local development

The backend is Linux-only (the sandbox needs cgroup v2 + namespaces) and is
**not run on dev machines**; Windows/macOS only run the frontend toolchain,
with `/api` proxied to any Linux deployment:

```bash
cd frontend
npm install
OJ_DEV_API_TARGET=http://<your-deployment> npm run dev   # http://localhost:5173
```

Builds and tests run on Linux (cross-compile first on non-Linux dev boxes):

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o oj-judge ./cmd/judge
sudo ./oj-judge --selftest     # verify cgroup v2 / namespace / seccomp
cd backend && go test ./...    # core logic unit tests
bash scripts/e2e-test.sh       # self-contained API E2E
```

## 🏗️ Architecture

```
Browser ──Vue3 SPA──► Nginx ──► Go API Server ──► PostgreSQL (prod) / SQLite (self-test)
                                  │         └──► Redis (judge queue / rate limit / sessions)
                                  │  WebSocket push (submission / contest / admin:daemons)
                                  │  Compile-time plugin registry: webhook + platform adapters
                                  │ gRPC JudgeRelay (bidi stream, judges dial out)
                                  ▼
                           judge-daemon × N (from scratch, sandbox included)
                           ├─ Language registry (languages.yaml, config-driven)
                           ├─ Compile sandbox profiles (g++/javac) + cache (sha256)
                           ├─ Run sandbox (cgroup v2 + namespaces + seccomp allowlist)
                           ├─ checker / interactor (SPJ / interactive)
                           └─ Testdata blob cache (sha256)
```

## 📚 Documentation

Docs site: <https://asanagl.github.io/ysnb-oj/en/> (**English** ·
[中文](https://asanagl.github.io/ysnb-oj/))

| Doc | Contents |
|---|---|
| [User guide](docs/en/guide/user-guide.md) | Registration, submitting, live verdicts, contests, groups |
| [Admin guide](docs/en/guide/admin-guide.md) | Role matrix, authoring & review, contest ops, judge monitoring, log viewer |
| [SPJ / interactive](docs/en/guide/interactive.md) | checker / interactor calling convention |
| [Deployment](docs/en/operations/deploy.md) | One-click / Docker Compose / systemd, backup & restore drills |
| [Ops runbook](docs/en/operations/maintenance.md) | Patrols, releases, disk & cache, incident handling |
| [Architecture](docs/en/development/architecture.md) | Key decisions, judging data flow, sandbox security model |
| [Judge & sandbox](docs/en/development/judge-sandbox.md) | Judging pipeline, seccomp allowlist, war stories |
| [Public API](docs/en/reference/api.md) | /public/*, cph endpoint, API keys |
| [Handover](docs/en/development/handover.md) | Find any piece of code in 30 minutes |

## 🔒 Security

Judging security is the core design constraint of this project. The sandbox
is written from scratch (reusing go-judge or forking the Hydro / DOMjudge
sandbox was explicitly ruled out). Defense in depth: namespace isolation,
read-only root, tmpfs overlays on sensitive paths, uid 65534 drop, triple
cgroup v2 limits, rlimits, a wall-clock watchdog, and a seccomp cBPF
allowlist (default SIGKILL) — no network inside the net namespace.

The escape suite (decoy files, /proc probing, path traversal, host writes,
networking, fork bombs…) runs as routine test assets on the judge; the cBPF
program additionally has an exhaustive interpreter test. **Full judging test
data never leaves via the public API** — a fairness red line.

Audit trail and accepted residual risks:
[docs/archive/security-audit.md](docs/archive/security-audit.md).

## 🤝 Contributing

Issues and PRs are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).

## 📄 License

[MIT](LICENSE) © Asanagl
