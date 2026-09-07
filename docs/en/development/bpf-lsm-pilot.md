# BPF LSM Audit Pilot Runbook

> Status: **audit-mode pilot is LIVE** (production judge, since
> 2026-09-08; the "isolated environment" gate was waived with the owner's
> approval since audit mode has zero blast radius by construction). Debian
> 12 activates the `bpf` LSM by default — no reboot was needed; the three
> hooks (`socket_create` / `bprm_check_security` / `ptrace_access_check`)
> are attached with a judge-cgroup allowlist filter, judging verified AC,
> negative control shows zero events, and the detach/rollback drill
> passed. Tools and deployment: `deploy/lsm-audit/`. Observation period
> ≥ 2 weeks (until 2026-09-22), then §4 decides on enforcement. Original
> decision record (2026-09-06): not adopted as an enforcement layer — see
> [judge-sandbox.md](judge-sandbox.md) §2.5.

## 1. Preconditions (all met on an isolated machine, never production)

1. Linux kernel ≥ 5.7 with `bpf` active: `cat /sys/kernel/security/lsm`.
   If missing, append `bpf` at the END of the `lsm=` boot parameter
   (`capability` must come first, `bpf` must come last — hard ordering per
   kernel docs) and reboot — **isolated machines only**.
2. `bpftool`, `clang` (BPF target), and `/sys/kernel/btf/vmlinux` present
   (BTF for CO-RE).
3. Full judge toolchain with `oj-judge --selftest` passing — the control
   baseline.

## 2. Audit-mode design

- Attach BPF programs to three LSM hooks first: `socket_create`,
  `bprm_check_security`, `ptrace_access_check`. Behavior: look up the
  cgroup-id map; on a judge-cgroup hit, `bpf_printk` the event (hook name,
  pid, cgroup id, argument summary) then **unconditionally return 0
  (allow)**. The audit program never changes verdicts; blast radius is zero.
- The map is registered by the judge on every run's cgroup creation and
  cleaned up afterwards (user space via cilium/ebpf).
- Cross-validation: selftest + escape suite + E2E all green, and the logged
  hook event sequence matches `strace` observation.

## 3. Acceptance criteria

1. With the audit program attached, the full judging path (compile / run /
   SPJ / interactive) shows no difference in results or timing distribution.
2. The log can reconstruct the LSM event flow of one complete judgement.
3. Detaching restores silence; a crashing program does not affect judging.
4. Memory overhead < 10MB (negligible within the low-memory judge budget).

## 4. Gate for upgrading to enforcement (separate review, default: no)

- Audit period ≥ 2 weeks covering daily training plus at least one full
  contest workload.
- Enforcement must be implemented separately with its fail-closed semantics
  reviewed on their own; before rollout run the full escape suite plus a
  "false block" suite (legitimate language features must never be blocked)
  on the isolated machine.
- Rollback must always be available: `bpftool prog detach` fully reverts.

## 5. Implementation steps (future work order)

> Actual implementation (2026-09-08) deviated from the original plan:
> 1) Debian 12 activates the `bpf` LSM by default, so step 2 was free;
> 2) the first iteration uses a libbpf C loader (bpftool CLI cannot attach
> LSM programs) + a shell watcher + `trace_pipe` observation — the
> Go/cilium-ebpf daemon is left for formalization; 3) measured event flow:
> one C++ submission emits 3 exec events (the compile chain), zero events
> from non-judge processes. Sources and deployment steps:
> `deploy/lsm-audit/README.md`.

1. Install the target distro + full judge toolchain on the isolated machine.
2. Activate the `bpf` LSM, reboot, verify via `/sys/kernel/security/lsm`.
3. Generate the BPF header
   (`bpftool btf dump file /sys/kernel/btf/vmlinux format c > vmlinux.h`);
   compile with clang `-target bpf -g`; embed into the Go loader via bpf2go.
4. A standalone `oj-lsm-audit` command (cilium/ebpf) is validated by manual
   attach first; only then evaluate wiring it into the judge on a separate
   feature branch.
5. Accept against §3; retain the audit data as the basis for any
   enforcement decision.
