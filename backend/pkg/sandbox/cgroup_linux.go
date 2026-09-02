//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// cgroupGroup manages one v2 cgroup per run so limits are enforced by the
// kernel even if the program escapes every userspace check.
type cgroupGroup struct {
	dir string
}

func openCgroup(base string, runID string) (*cgroupGroup, error) {
	dir := filepath.Join(base, "run-"+runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("cgroup: create %s: %w", dir, err)
	}
	return &cgroupGroup{dir: dir}, nil
}

func (g *cgroupGroup) writeFile(name, content string) error {
	return os.WriteFile(filepath.Join(g.dir, name), []byte(content), 0o644)
}

// applyLimits programs memory/CPU/pid walls. Swap is disabled because swap
// would silently convert MLE into seconds of thrash.
func (g *cgroupGroup) applyLimits(spec *Spec) error {
	if spec.MemLimitKB > 0 {
		// why headroom: the cgroup hosts BOTH the user program and the
		// stage2 supervisor (a re-exec of the judge daemon — a full Go
		// runtime that reserves tens of MB of heap). Under case-level
		// parallelism several such pairs share the host; a limit sized
		// exactly to the user program lets the kernel OOM-kill stage2
		// first (Go exits 2 with a fatal alloc trace), surfacing as
		// phantom RE verdicts. 64MB covers the supervisor; the user-side
		// accounting (peak RSS, MLE classification) uses the original
		// spec value.
		limitKB := spec.MemLimitKB + stage2HeadroomKB
		if err := g.writeFile("memory.max", strconv.FormatInt(limitKB*1024, 10)); err != nil {
			return fmt.Errorf("cgroup: memory.max: %w", err)
		}
		if err := g.writeFile("memory.swap.max", "0"); err != nil {
			return fmt.Errorf("cgroup: memory.swap.max: %w", err)
		}
	}
	// why period 100ms: fine-grained enough for interactivity, coarse enough
	// to avoid throttle-storm bookkeeping on busy hosts.
	if err := g.writeFile("cpu.max", "100000 100000"); err != nil {
		return fmt.Errorf("cgroup: cpu.max: %w", err)
	}
	pids := spec.PidsLimit
	if pids <= 0 {
		pids = 512
	}
	if err := g.writeFile("pids.max", strconv.Itoa(pids)); err != nil {
		return fmt.Errorf("cgroup: pids.max: %w", err)
	}
	return nil
}

// stage2HeadroomKB is the extra memory budget for the in-cgroup supervisor.
const stage2HeadroomKB = 64 * 1024

func (g *cgroupGroup) addPid(pid int) error {
	return g.writeFile("cgroup.procs", strconv.Itoa(pid))
}

// kill terminates every process in the group. cgroup.kill (kernel >= 5.14)
// is atomic; the fallback freezes the group then SIGKILLs listed pids.
func (g *cgroupGroup) kill() {
	if err := g.writeFile("cgroup.kill", "1"); err == nil {
		return
	}
	_ = g.writeFile("cgroup.freeze", "1")
	procs, _ := os.ReadFile(filepath.Join(g.dir, "cgroup.procs"))
	for _, line := range strings.Fields(string(procs)) {
		if pid, err := strconv.Atoi(line); err == nil {
			_ = sysKill(pid)
		}
	}
	_ = g.writeFile("cgroup.freeze", "0")
}

// peakRSSKB returns the observed memory peak, when the kernel exposes it.
func (g *cgroupGroup) peakRSSKB() int64 {
	raw, err := os.ReadFile(filepath.Join(g.dir, "memory.peak"))
	if err != nil {
		return 0
	}
	v, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil {
		return 0
	}
	return v / 1024
}

// oomKills reports how many processes the kernel OOM-killed in this group;
// it is read before destroy and drives MLE classification.
func (g *cgroupGroup) oomKills() int {
	raw, err := os.ReadFile(filepath.Join(g.dir, "memory.events"))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "oom_kill ") {
			v, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "oom_kill ")))
			return v
		}
	}
	return 0
}

func (g *cgroupGroup) destroy() {
	g.kill()
	_ = os.Remove(g.dir) // busy when stragglers linger; janitor re-sweeps later
}
