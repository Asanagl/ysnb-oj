//go:build linux

package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

// Sandbox spawns jailed processes. One instance per daemon; safe for
// concurrent use.
type Sandbox struct {
	CgroupBase string
	HidePaths  []string
}

// Preflight verifies the daemon can actually jail processes: cgroup v2
// control files must exist and be writable, otherwise limits are theater.
func (s *Sandbox) Preflight() error {
	// cgroup v2 no-internal-process rule: run-XXX children only get
	// controller files (memory.max, pids.max, …) if the controllers are
	// enabled in CgroupBase's subtree_control. Delegated setups (systemd)
	// usually leave it empty — enable what we need, best-effort per
	// controller so a kernel without one still passes for the rest.
	// cpu must be in the list too: applyLimits always writes cpu.max.
	//
	// why retry: Debian 12 (systemd 252) enables controllers in the root
	// cgroup lazily during boot — a unit starting early gets EACCES on
	// this write even though root enables everything seconds later
	// (seen as all-judging-SE after the bullseye→bookworm upgrade).
	for _, ctrl := range []string{"memory", "pids", "cpu"} {
		enabled := func() bool {
			cur, err := os.ReadFile(filepath.Join(s.CgroupBase, "cgroup.subtree_control"))
			return err == nil && strings.Contains(string(cur), ctrl)
		}
		for attempt := 0; attempt < 20 && !enabled(); attempt++ {
			_ = os.WriteFile(filepath.Join(s.CgroupBase, "cgroup.subtree_control"), []byte(" "+ctrl), 0o644)
			time.Sleep(500 * time.Millisecond)
		}
	}
	probe := filepath.Join(s.CgroupBase, "preflight-"+uuid.NewString()[:8])
	if err := os.MkdirAll(probe, 0o755); err != nil {
		return fmt.Errorf("cgroup base %s unusable (need cgroup v2 + root): %w", s.CgroupBase, err)
	}
	return os.Remove(probe)
}

// Run executes one spec and classifies the outcome.
func (s *Sandbox) Run(spec *Spec, files *Files) (*Result, error) {
	return s.runProcess(spec, files, nil)
}

// RunPair wires user stdin/stdout to interactor stdout/stdin through OS
// pipes and runs both jailed programs. Pipe EOF is the natural teardown:
// when the user exits, the interactor's reads return EOF and it decides the
// verdict; when the interactor exits, the user gets EOF/SIGPIPE. For that
// to work the PARENT must drop its own pipe-end copies right after both
// children start — otherwise the interactor blocks forever on a pipe whose
// writer list includes the immortal parent (observed in cloud E2E). True
// deadlocks (both sides alive, both blocked) are covered by each side's
// own wall-clock watchdog.
func (s *Sandbox) RunPair(userSpec, interSpec *Spec, stderr *os.File) (*Result, *Result, error) {
	u2i, err := pipePair() // user stdout -> interactor stdin
	if err != nil {
		return nil, nil, err
	}
	defer closePair(u2i)
	i2u, err := pipePair() // interactor stdout -> user stdin
	if err != nil {
		return nil, nil, err
	}
	defer closePair(i2u)

	// why timeout: if a Start fails the second signal never arrives; the
	// fallback simply closes late, which is harmless.
	startedCh := make(chan struct{}, 2)
	go func() {
		for i := 0; i < 2; i++ {
			select {
			case <-startedCh:
			case <-time.After(5 * time.Second):
				return
			}
		}
		closePair(u2i)
		closePair(i2u)
	}()

	var (
		wg                sync.WaitGroup
		userRes, interRes *Result
		uerr, ierr        error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		uf := &Files{Stdin: i2u[0], Stdout: u2i[1], Stderr: stderr}
		userRes, uerr = s.runProcess(userSpec, uf, func() { startedCh <- struct{}{} })
	}()
	go func() {
		defer wg.Done()
		iff := &Files{Stdin: u2i[0], Stdout: i2u[1], Stderr: stderr}
		interRes, ierr = s.runProcess(interSpec, iff, func() { startedCh <- struct{}{} })
	}()
	wg.Wait()
	if uerr != nil {
		return nil, nil, uerr
	}
	if ierr != nil {
		return nil, nil, ierr
	}
	return userRes, interRes, nil
}

func pipePair() ([2]*os.File, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return [2]*os.File{}, err
	}
	return [2]*os.File{r, w}, nil
}

func closePair(p [2]*os.File) {
	_ = p[0].Close()
	_ = p[1].Close()
}

// runProcess is the single execution core used by Run and RunPair.
// afterStart (optional) runs once the child is started and cgroup-attached —
// RunPair uses it to drop the parent's pipe-end copies so EOF propagates.
func (s *Sandbox) runProcess(spec *Spec, files *Files, afterStart func()) (*Result, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}
	if info, err := os.Stat(spec.Workspace); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("sandbox: workspace %s unavailable: %w", spec.Workspace, err)
	}
	runID := uuid.NewString()
	cg, err := openCgroup(s.CgroupBase, runID)
	if err != nil {
		return nil, err
	}
	defer cg.destroy()
	// why here, before Start: limits must be active when the first page is
	// faulted; attaching a cgroup without programming its walls would make
	// every limit decorative. (This call was historically missing — the
	// intermittent phantom-RE under parallel judging traced back to it.)
	if err := cg.applyLimits(spec); err != nil {
		return nil, err
	}

	stage2Spec, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	cmd, dp, err := s.buildCmd(spec, stage2Spec)
	if err != nil {
		return nil, err
	}
	defer dp.close()
	closer := applyRedirects(cmd, spec, files)
	defer closer.closeAll()

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("sandbox: start: %w", err)
	}
	if err := cg.addPid(cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill() // race: process may already be gone
		_ = cmd.Wait()
		return nil, fmt.Errorf("sandbox: cgroup attach: %w", err)
	}
	if afterStart != nil {
		afterStart()
	}

	fired, stopWatchdog := s.startWatchdog(spec, cg, cmd)
	_ = cmd.Wait() // ExitError is a normal non-zero program, not a failure
	stopWatchdog()
	wallMS := time.Since(start).Milliseconds()
	res := classify(spec, cmd, cg, fired.Load(), wallMS)
	if diag := dp.drain(); diag != "" {
		res.Status = StatusSE
		res.Message = diag
	}
	return res, nil
}

func (s *Sandbox) buildCmd(spec *Spec, specJSON []byte) (*exec.Cmd, *diagPipe, error) {
	dp, err := newDiagPipe()
	if err != nil {
		return nil, nil, err
	}
	env := []string{
		stage2EnvFlag + "=1",
		stage2EnvSpec + "=" + string(specJSON),
		stage2EnvHide + "=" + joinPaths(s.HidePaths),
	}
	cmd := &exec.Cmd{
		Path: "/proc/self/exe",
		Args: []string{"oj-stage2"},
		Env:  env,
		SysProcAttr: &syscall.SysProcAttr{
			Cloneflags: unix.CLONE_NEWNS | unix.CLONE_NEWPID |
				unix.CLONE_NEWNET | unix.CLONE_NEWIPC | unix.CLONE_NEWUTS,
			// why NO Pdeathsig: Go's forkExec orphan check compares
			// getppid() against the pre-clone parent — inside a new PID
			// namespace getppid() returns 0, so the check misfires,
			// fires kill(getpid()) on the pidns-init child, and the kernel
			// ignores non-handler signals to pidns-init; the child then
			// wedges in a pidfd waitid until the watchdog kills it. This
			// was the "every run takes the full wall limit" bug. Daemon
			// crash cleanup is covered by the lease requeue scanner and
			// the per-run watchdog instead.
		},
		ExtraFiles: []*os.File{dp.write},
	}
	return cmd, dp, nil
}

// startWatchdog enforces the wall clock. fired tells the caller afterwards
// whether TLE came from us rather than from the program itself; stop prevents
// a late fire from killing an unrelated re-used pid slot.
func (s *Sandbox) startWatchdog(spec *Spec, cg *cgroupGroup, cmd *exec.Cmd) (fired *atomic.Bool, stop func()) {
	fired = &atomic.Bool{}
	timer := time.AfterFunc(time.Duration(spec.wallLimit())*time.Millisecond, func() {
		fired.Store(true)
		cg.kill()
		_ = cmd.Process.Kill()
	})
	return fired, func() { timer.Stop() }
}

func sysKill(pid int) error {
	return unix.Kill(pid, unix.SIGKILL)
}
