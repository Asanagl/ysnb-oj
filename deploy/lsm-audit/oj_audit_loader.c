// SPDX-License-Identifier: GPL-2.0
// oj-audit loader: attaches the audit BPF LSM programs and pins the
// judge_cgroups map for the shell watcher. Audit-only; Ctrl-C detaches.
#include <bpf/libbpf.h>
#include <stdio.h>
#include <stdlib.h>
#include <signal.h>
#include <unistd.h>
#include "oj_audit.skel.h"

static volatile sig_atomic_t stopping;

static void on_signal(int sig) { (void)sig; stopping = 1; }

int main(void)
{
	struct oj_audit_bpf *skel;
	int err;

	libbpf_set_strict_mode(LIBBPF_STRICT_ALL);
	skel = oj_audit_bpf__open_and_load();
	if (!skel) {
		fprintf(stderr, "open_and_load failed\n");
		return 1;
	}

	struct bpf_link *l1 = bpf_program__attach_lsm(skel->progs.socket_create_audit);
	struct bpf_link *l2 = bpf_program__attach_lsm(skel->progs.bprm_check_audit);
	struct bpf_link *l3 = bpf_program__attach_lsm(skel->progs.ptrace_access_audit);
	if (!l1 || !l2 || !l3) {
		fprintf(stderr, "attach_lsm failed (l1=%p l2=%p l3=%p)\n", l1, l2, l3);
		return 1;
	}

	err = bpf_map__pin(skel->maps.judge_cgroups, "/sys/fs/bpf/oj_audit/judge_cgroups");
	if (err) {
		fprintf(stderr, "map pin failed: %d\n", err);
		return 1;
	}
	fprintf(stderr, "oj-audit attached (3 lsm hooks), map pinned\n");

	signal(SIGINT, on_signal);
	signal(SIGTERM, on_signal);
	while (!stopping)
		sleep(1);

	fprintf(stderr, "detaching\n");
	bpf_link__destroy(l1);
	bpf_link__destroy(l2);
	bpf_link__destroy(l3);
	unlink("/sys/fs/bpf/oj_audit/judge_cgroups");
	oj_audit_bpf__destroy(skel);
	return 0;
}
