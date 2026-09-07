# BPF LSM Audit Pilot Runbook (not implemented)

> Status: **planned, not yet implemented**. Decision record (2026-09-06):
> BPF LSM is NOT adopted as a judging enforcement layer — see
> [judge-sandbox.md](judge-sandbox.md) §2.5 (host-wide policy vs per-process
> fail-closed, activation requires a reboot, overlaps the existing four
> defense layers). Instead: once the kernel is upgraded to a version where
> the `bpf` LSM can be activated (e.g. Debian 12 / 6.1), pilot it in an
> **isolated environment** in **audit mode** (log-only, never blocks), then
> decide on enforcement based on observed judge-workload behavior. This
> document is the runbook for that future work.

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
