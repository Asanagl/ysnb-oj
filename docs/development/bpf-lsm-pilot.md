# BPF LSM 审计试点运行手册（未实施）

> 状态：**规划中，尚未实施**。决策记录（2026-09-06）：不将 BPF LSM 作为
> 判题强制层，理由见 [judge-sandbox.md](judge-sandbox.md) §2.5（宿主机全局
> 策略 vs 每进程 fail-closed、激活需重启、与既有四层防线重叠）；选择在
> 内核升级到可激活 bpf LSM 的版本（如 Debian 12 / 6.1）后，先在**隔离环境**
> 以**审计模式**试点，观察判题负载真实行为后再决定是否进入拦截评估。
> 本文是届时开工的运行手册，避免决策依据散失。

## 一、前置条件（全部在隔离环境满足，不碰生产）

1. Linux 内核 ≥ 5.7，且激活列表含 `bpf`：`cat /sys/kernel/security/lsm`。
   若不含，需在引导参数 `lsm=` 末尾追加 `bpf`（`capability` 必须居首、
   `bpf` 必须居末，内核文档硬性顺序）并重启——**只允许在隔离机做**。
2. `bpftool`、`clang`（编译 BPF 目标）、`/sys/kernel/btf/vmlinux` 存在
   （BTF，CO-RE 免适配）。
3. 判题机工具链齐全，`oj-judge --selftest` 全过——作为对照负载基线。

## 二、审计模式设计

- BPF 程序挂 LSM 钩子，首期三个：`socket_create`、`bprm_check_security`、
  `ptrace_access_check`。行为 = 查 cgroup-id map，命中判题组则
  `bpf_printk` 记录（钩子名、pid、cgroup id、参数摘要）后**无条件返回 0
  （放行）**。审计程序永远不改判结果，爆炸半径为零。
- map 由判题机在每次判题建 cgroup 时注册、结束清理（用户态经 cilium/ebpf）。
- 对照验证：selftest + 逃逸用例集 + E2E 全绿，且日志可见的钩子事件序列
  与 `strace` 观测一致。

## 三、验收标准

1. 挂载审计程序前后，判题全链路（compile / run / SPJ / 交互）结果与耗时
   分布无差异。
2. 日志能还原一次完整判题的 LSM 事件流。
3. 程序卸载即恢复无感；程序崩溃不影响判题。
4. 内存开销 < 10MB（低内存判题机预算内可忽略）。

## 四、升级为拦截模式的决策门槛（另行评估，默认不做）

- 审计期 ≥ 2 周，覆盖日常训练 + 至少 1 场完整比赛负载。
- 拦截层必须独立实现并单独评审 fail-closed 语义；上线前在隔离环境跑完整
  逃逸用例集 + "误拦"用例集（合法语言特性不得被拦）。
- 回滚手段必须随时可用：`bpftool prog detach` 即完全恢复。

## 五、实施步骤（届时开工顺序）

1. 隔离机装目标发行版 + 判题机全量工具链。
2. 激活 `bpf` LSM 并重启，`/sys/kernel/security/lsm` 确认。
3. `bpftool btf dump file /sys/kernel/btf/vmlinux format c > vmlinux.h`
   生成 BPF 头；clang `-target bpf -g` 编译审计程序；经 bpf2go 嵌入 Go loader。
4. 独立命令 `oj-lsm-audit`（cilium/ebpf）先手动挂载验证，再评估是否以
   独立 feature 分支接入判题机。
5. 按第三节逐项验收，审计数据留存为拦截模式评估的依据。
