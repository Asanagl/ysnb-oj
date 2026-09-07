// SPDX-License-Identifier: GPL-2.0
// oj-audit: BPF LSM audit pilot for the YSNB OJ judge sandbox.
// AUDIT ONLY: every hook unconditionally returns 0 (allow). Events are
// emitted via bpf_printk for judge-cgroup processes only. See
// docs/development/bpf-lsm-pilot.md.
#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

/* judge sandbox run-* cgroup ids, registered by the userspace watcher */
struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(max_entries, 1024);
	__type(key, u64);
	__type(value, u32);
} judge_cgroups SEC(".maps");

static __always_inline int judge_member(void)
{
	u64 cgid = bpf_get_current_cgroup_id();
	u32 *hit = bpf_map_lookup_elem(&judge_cgroups, &cgid);
	return hit != NULL;
}

SEC("lsm/socket_create")
int BPF_PROG(socket_create_audit, struct socket *sock, int family, int type,
	     int protocol)
{
	if (judge_member()) {
		u64 pid = bpf_get_current_pid_tgid() >> 32;
		bpf_printk("oj-audit socket family=%d type=%d pid=%u",
			   family, type, (u32)pid);
	}
	/* audit only: never block */
	return 0;
}

SEC("lsm/bprm_check_security")
int BPF_PROG(bprm_check_audit, struct linux_binprm *bprm)
{
	if (judge_member()) {
		u64 pid = bpf_get_current_pid_tgid() >> 32;
		bpf_printk("oj-audit exec pid=%u", (u32)pid);
	}
	/* audit only: never block */
	return 0;
}

SEC("lsm/ptrace_access_check")
int BPF_PROG(ptrace_access_audit, struct task_struct *task, unsigned int mode)
{
	if (judge_member()) {
		u64 pid = bpf_get_current_pid_tgid() >> 32;
		bpf_printk("oj-audit ptrace mode=%u pid=%u", mode, (u32)pid);
	}
	/* audit only: never block */
	return 0;
}

char LICENSE[] SEC("license") = "GPL";
