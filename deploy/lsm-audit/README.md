# BPF LSM 审计试点工具（oj-audit）

> 状态：**审计模式试点运行中**（生产判题机，2026-09-08 起挂载）。
> 只记录、永不拦截——每个钩子无条件 `return 0`，事件经 cgroup 白名单
> 过滤（仅判题沙箱 run-* 组）。试点方案与验收标准见
> [docs/development/bpf-lsm-pilot.md](../docs/development/bpf-lsm-pilot.md)。

## 组成

| 文件 | 作用 |
|---|---|
| `oj_audit.bpf.c` | BPF LSM 程序：`socket_create` / `bprm_check_security` / `ptrace_access_check` 三钩子，查 `judge_cgroups` map 命中判题组则 `bpf_printk` 记录 |
| `oj_audit_loader.c` | libbpf C loader：加载并挂载三钩子（LSM 走 link 路径，bpftool CLI 不支持），钉住 map 供注册器写 |
| `oj_audit_watcher.sh` | 每秒把 `/sys/fs/cgroup/oj-judge/run-*` 的 cgroup id 注册进 map |
| 部署后产物 | `oj-audit.service` + `oj-audit-watcher.service`（开机自启，摘除即回滚） |

## 构建（目标机上执行，需 bpftool / clang / libbpf-dev / gcc）

```bash
bpftool btf dump file /sys/kernel/btf/vmlinux format c > vmlinux.h
clang -O2 -g -target bpf -D__TARGET_ARCH_x86 -I. -c oj_audit.bpf.c -o oj_audit.bpf.o
bpftool gen skeleton oj_audit.bpf.o > oj_audit.skel.h
gcc -O2 -o oj-audit-loader oj_audit_loader.c -lbpf -lelf -lz
```

## 部署 / 观测 / 回滚

```bash
# 部署（默认装到 /opt/oj/lsm-audit；单元文件见本目录 *.service）
mkdir -p /opt/oj/lsm-audit
cp oj-audit-loader oj_audit_watcher.sh /opt/oj/lsm-audit/
cp oj-audit.service oj-audit-watcher.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now oj-audit.service oj-audit-watcher.service
# 观测：判题事件流（只含判题 cgroup 的进程）
cat /sys/kernel/tracing/trace_pipe | grep oj-audit
# 回滚（随时、完全、无副作用）
systemctl disable --now oj-audit.service oj-audit-watcher.service
```

> 单元文件的两个坑（实踩）：`After=multi-user.target` + `WantedBy=multi-user.target`
> 会构成排序环，开机时 systemd 直接删掉 watcher 的启动任务——不要加；
> watcher 对 loader 用 `Wants=` 而非 `Requires=`，依赖瞬时失败时 watcher
> 仍要活着重试（map 更新在 map 出现前是无害空转）。

## 安全模型

- **审计模式硬约束**：三个钩子最后一条语句都是 `return 0`——即使程序有
  逻辑 bug 也不可能改变任何系统调用的裁决（BPF 验证器另保证无法越界/panic）。
- **过滤面**：map 里注册的 cgroup id 之外的进程在钩子内第一跳就返回，
  零开销通过；机器上非判题负载完全无感。
- **资源**：3 个 JIT 后程序 + 1 个 1024 项哈希表，内存 < 100KB。
