---
layout: home

hero:
  name: YSNB OJ
  text: You Submit, Never Be rejected.
  tagline: A fully self-developed online judge — documentation covering deployment, operations and development
  actions:
    - theme: brand
      text: Deploy an instance
      link: /en/operations/deploy
    - theme: alt
      text: Documentation map
      link: '#documentation-map'

features:
  - title: Contestants & team members
    details: Register, practice, compete (ACM/IOI), groups and problem lists, multi-platform practice aggregation
    link: /en/guide/user-guide
  - title: Admins & problem setters
    details: Authoring and review, running contests (freeze/reveal), user management, log viewer, API keys
    link: /en/guide/admin-guide
  - title: Deployment & operations
    details: Cold deploy, judge onboarding, releases, backup & restore drills, SE triage, patrols
    link: /en/operations/deploy
  - title: Developers
    details: Handover guide, architecture, judge & sandbox deep dive, stack and configuration
    link: /en/development/handover
---

## Documentation map

> Find entries by "what do I want to do"; every page opens with its intended reader.

### Guide

| Doc | Contents |
|---|---|
| [User guide](./guide/user-guide) | Registration, practicing with live verdicts, contests, groups, profile (external practice data & problem import), FAQ |
| [Admin & authoring guide](./guide/admin-guide) | Roles, authoring (standard/SPJ/interactive/IOI scoring), running contests, review, judge monitoring, log viewer, API keys, plugins |
| [SPJ & interactive convention](./guide/interactive) | checker / interactor calling convention, problem package interop |

### Deploy & operate

| Doc | Contents |
|---|---|
| [Deployment](./operations/deploy) | One-click script and systemd cold-deploy paths, judge onboarding, logs, backup drills, full env var table |
| [Ops runbook](./operations/maintenance) | Releases, patrols, SE triage, disk cleanup, incident handling, SSH hardening baseline, config change index |

### Development

| Doc | Contents |
|---|---|
| [Handover](./development/handover) | Locate any code in 30 minutes: capability map, code tour, dev gates, release flow, known pitfalls |
| [Architecture](./development/architecture) | Key decisions, module boundaries, judging data flow, sandbox security model, contest modes |
| [Stack & config](./development/tech-stack) | Dependency versions, package layout, OJ_* config quick reference, rate-limit parameters |
| [Judge & sandbox](./development/judge-sandbox) | gRPC protocol, judging pipeline, languages.yaml, seccomp/cgroup deep dive |
| [BPF LSM pilot](./development/bpf-lsm-pilot) | Planned, not implemented: audit-mode pilot runbook and the enforcement decision gate |
| [Showcase](./development/showcase) | Sanitized technical retrospective (interviews / external audiences) |

### API reference

| Doc | Contents |
|---|---|
| [Public API](./reference/api) | `/public/*` endpoints, API keys, cph integration, M5 integration endpoints |

### Archive (read-only)

[Security audit](/archive/security-audit) · [Launch readiness snapshot](/archive/launch-readiness) · [E2E chronicle](/archive/e2e-report) · [Early cloud report](/archive/cloud-test-report)

::: tip Language
The archive pages and this site's source of truth are maintained in Chinese.
English pages mirror the live documentation.
:::
