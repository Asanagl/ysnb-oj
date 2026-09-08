#!/bin/sh
# oj-audit cgroup watcher: registers judge sandbox run-* cgroup ids into
# the BPF map so the LSM audit programs only observe judged workloads.
MAP=/sys/fs/bpf/oj_audit/judge_cgroups
while true; do
  for d in /sys/fs/cgroup/oj-judge/run-*; do
    [ -d "$d" ] || continue
    id=$(stat -c %i "$d")
    keyhex=$(python3 -c "import struct,sys;print(' '.join('%02x'%b for b in struct.pack('<Q', int(sys.argv[1]))))" "$id")
    bpftool map update pinned "$MAP" key hex $keyhex value hex 01 00 00 00 >/dev/null 2>&1 || true
  done
  sleep 1
done
