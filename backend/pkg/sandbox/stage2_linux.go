//go:build linux

package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// Stage2 env keys; the parent re-executes this binary with them set.
const (
	stage2EnvSpec = "OJ_STAGE2_SPEC"
	stage2EnvHide = "OJ_STAGE2_HIDE"
	stage2EnvFlag = "OJ_STAGE2"
)

// Stage2Main is the entry used when the daemon re-executes itself inside the
// new namespaces. It assembles the private filesystem view, drops to an
// unprivileged user, applies rlimits + seccomp, then execve's the target.
// It never returns: on failure it reports over fd 3 (the parent's diag pipe)
// and exits non-zero.
func Stage2Main() {
	spec := &Spec{}
	if err := json.Unmarshal([]byte(os.Getenv(stage2EnvSpec)), spec); err != nil {
		stage2Fatal(fmt.Sprintf("stage2: bad spec: %v", err), 125)
	}
	if err := stage2Setup(spec); err != nil {
		stage2Fatal(fmt.Sprintf("stage2: %v (spec: %s)", err, spec.describe()), 125)
	}
	// why resolve BEFORE seccomp: argv0 resolution stats PATH candidates —
	// on Go ≥1.21 that is newfstatat(262), which the filter would kill the
	// whole stage2 process for. Everything after loadSeccomp must be execve
	// only; even the Go runtime's background threads race against the filter
	// here, which is why the whitelist covers futex/nanosleep/clone.
	argv0 := resolveArgv0(spec.Argv[0], spec.Env)
	if err := loadSeccomp(spec.Profile); err != nil {
		stage2Fatal(fmt.Sprintf("stage2: seccomp: %v", err), 125)
	}
	// why rlimits LAST: setrlimit applies to the calling process — us, the
	// Go-runtime stage2 supervisor. Applying RLIMIT_AS here would cap our
	// own address space (Go reserves hundreds of MB of PROT_NONE arena
	// upfront) and randomly kill stage2 at startup with a Go fatal-alloc
	// trace surfacing as phantom user-program RE. Applied just before the
	// execve instead, the limits bind to the target program: our existing
	// mappings are unaffected, and the exec'd image inherits the limits.
	if err := applyRlimits(spec); err != nil {
		stage2Fatal(fmt.Sprintf("stage2: %v", err), 125)
	}
	if err := unix.Exec(argv0, spec.Argv, spec.Env); err != nil {
		stage2Fatal(fmt.Sprintf("stage2: exec %s failed: %v", argv0, err), 126)
	}
	os.Exit(126) // unix.Exec never returns on success
}

// resolveArgv0 emulates execlp for stage2: execve performs no PATH lookup,
// and language profiles use bare toolchain names (g++, python3, java…), so
// the name is resolved against the child's PATH before exec.
func resolveArgv0(argv0 string, env []string) string {
	if strings.Contains(argv0, "/") {
		return argv0
	}
	for _, kv := range env {
		if !strings.HasPrefix(kv, "PATH=") {
			continue
		}
		for _, dir := range strings.Split(kv[len("PATH="):], ":") {
			if dir == "" {
				continue
			}
			cand := filepath.Join(dir, argv0)
			if fi, err := os.Stat(cand); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
				return cand
			}
		}
	}
	return argv0 // unchanged: execve fails with the familiar ENOENT
}

func stage2Setup(spec *Spec) error {
	if err := mountPrivateRoot(); err != nil {
		return err
	}
	if err := mountVirtualRoots(); err != nil {
		return err
	}
	// why bind before hiding: the workspace lives under the hidden WorkRoot,
	// so the bind must attach to the (still visible) host inode first; later
	// tmpfs covers over the parent cannot detach an existing mount.
	if err := mountWorkspace(spec.Workspace); err != nil {
		return err
	}
	hideHostPaths()
	if err := unix.Chdir(stage2WorkspacePath); err != nil {
		return fmt.Errorf("chdir workspace: %w", err)
	}
	if err := dropPrivileges(); err != nil {
		return err
	}
	// rlimits moved to Stage2Main's pre-exec step — see the comment there.
	return nil
}

// mountPrivateRoot makes / private (kills mount propagation — without this a
// remount inside the namespace would leak to the host) then re-binds / onto
// itself read-only, giving the sandbox a whole-host read-only view.
func mountPrivateRoot() error {
	if err := unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("make / private: %w", err)
	}
	if err := unix.Mount("/", "/", "", unix.MS_BIND|unix.MS_REC, ""); err != nil {
		return fmt.Errorf("bind /: %w", err)
	}
	flags := uintptr(unix.MS_BIND | unix.MS_REMOUNT | unix.MS_RDONLY | unix.MS_NOSUID | unix.MS_NODEV)
	if err := unix.Mount("", "/", "", flags, ""); err != nil {
		return fmt.Errorf("remount / read-only: %w", err)
	}
	return nil
}

// mountVirtualRoots replaces world-visible host mounts with namespace-local
// ones: a fresh procfs (only this PID ns), a read-only sysfs, tmpfs /run,
// writable /tmp, and a hand-populated /dev.
func mountVirtualRoots() error {
	if err := unix.Mount("proc", "/proc", "proc", unix.MS_RDONLY|unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC, ""); err != nil {
		return fmt.Errorf("mount /proc: %w", err)
	}
	if err := unix.Mount("sysfs", "/sys", "sysfs", unix.MS_RDONLY|unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC, ""); err != nil {
		return fmt.Errorf("mount /sys: %w", err)
	}
	if err := unix.Mount("tmpfs", "/run", "tmpfs", unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC, "mode=755,size=8m"); err != nil {
		return fmt.Errorf("mount /run: %w", err)
	}
	if err := unix.Mount("tmpfs", "/tmp", "tmpfs", unix.MS_NOSUID, "mode=1777,size=256m"); err != nil {
		return fmt.Errorf("mount /tmp: %w", err)
	}
	return mountDev()
}

func mountDev() error {
	if err := unix.Mount("tmpfs", "/dev", "tmpfs", unix.MS_NOSUID, "mode=755,size=16m"); err != nil {
		return fmt.Errorf("mount /dev: %w", err)
	}
	// why: /dev is mounted without MS_NODEV so these char nodes are usable.
	devs := []struct {
		name  string
		major uint32
		minor uint32
	}{
		{"null", 1, 3}, {"zero", 1, 5}, {"full", 1, 7},
		{"random", 1, 8}, {"urandom", 1, 9},
	}
	for _, d := range devs {
		dev := unix.Mkdev(d.major, d.minor)
		if err := unix.Mknod("/dev/"+d.name, unix.S_IFCHR|0o666, int(dev)); err != nil {
			return fmt.Errorf("mknod /dev/%s: %w", d.name, err)
		}
	}
	if err := os.Mkdir("/dev/shm", 0o777); err != nil {
		return fmt.Errorf("mkdir /dev/shm: %w", err)
	}
	if err := unix.Mount("tmpfs", "/dev/shm", "tmpfs", unix.MS_NOSUID|unix.MS_NODEV, "mode=1777,size=64m"); err != nil {
		return fmt.Errorf("mount /dev/shm: %w", err)
	}
	return nil
}

// hideHostPaths covers sensitive host directories with an empty inaccessible
// tmpfs so sandboxed code cannot read other users' data through the RO view.
func hideHostPaths() {
	hides := []string{"/home", "/root", "/var"}
	if extra := os.Getenv(stage2EnvHide); extra != "" {
		hides = append(hides, strings.Split(extra, ":")...)
	}
	seen := map[string]bool{}
	for _, p := range hides {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		_ = unix.Mount("tmpfs", p, "tmpfs", unix.MS_RDONLY|unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC, "mode=000,size=4k")
	}
}

// stage2WorkspacePath is the in-namespace mount point of the per-run
// workspace: inside the private /tmp tmpfs, so each run gets its own view
// and sibling run directories under WorkRoot stay hidden.
const stage2WorkspacePath = "/tmp/ojws"

// mountWorkspace binds the host workspace directory to the in-namespace
// path /tmp/ojws and makes it writable — the only writable location outside
// /tmp and /dev/shm.
func mountWorkspace(hostPath string) error {
	if err := os.Mkdir(stage2WorkspacePath, 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", stage2WorkspacePath, err)
	}
	if err := unix.Mount(hostPath, stage2WorkspacePath, "", unix.MS_BIND, ""); err != nil {
		return fmt.Errorf("bind workspace %s: %w", hostPath, err)
	}
	if err := unix.Mount("", stage2WorkspacePath, "", unix.MS_BIND|unix.MS_REMOUNT, ""); err != nil {
		return fmt.Errorf("remount workspace rw: %w", err)
	}
	return nil
}

func dropPrivileges() error {
	if err := unix.Setgroups([]int{}); err != nil {
		return fmt.Errorf("setgroups: %w", err)
	}
	if err := unix.Setresgid(runAsGID, runAsGID, runAsGID); err != nil {
		return fmt.Errorf("setresgid: %w", err)
	}
	if err := unix.Setresuid(runAsUID, runAsUID, runAsUID); err != nil {
		return fmt.Errorf("setresuid: %w", err)
	}
	return nil
}

func applyRlimits(spec *Spec) error {
	set := func(res int, kb int64) error {
		if kb <= 0 {
			return nil
		}
		return unix.Setrlimit(res, &unix.Rlimit{Cur: uint64(kb * 1024), Max: uint64(kb * 1024)})
	}
	if err := set(unix.RLIMIT_CORE, 0); err != nil {
		return fmt.Errorf("rlimit core: %w", err)
	}
	if err := set(unix.RLIMIT_FSIZE, spec.FSizeLimitKB); err != nil {
		return fmt.Errorf("rlimit fsize: %w", err)
	}
	if err := set(unix.RLIMIT_STACK, spec.StackLimitKB); err != nil {
		return fmt.Errorf("rlimit stack: %w", err)
	}
	if err := set(unix.RLIMIT_AS, spec.AsLimitKB); err != nil {
		return fmt.Errorf("rlimit as: %w", err)
	}
	// CPU backstop in seconds; the wall-clock watchdog remains the primary
	// TLE trigger, this catches spinning threads that hide from the timer.
	cpuSec := spec.TimeLimitMS/1000 + 1
	return unix.Setrlimit(unix.RLIMIT_CPU, &unix.Rlimit{Cur: uint64(cpuSec), Max: uint64(cpuSec)})
}

// stage2Fatal prefers fd 3, the parent's diag pipe, so bootstrap failures
// surface as SE instead of being mistaken for the program's stderr.
func stage2Fatal(msg string, code int) {
	if f := os.NewFile(3, "stage2-diag"); f != nil {
		_, _ = f.WriteString(msg + "\n")
		_ = f.Close()
	}
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(code)
}
