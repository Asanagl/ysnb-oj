//go:build linux

package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// Escape test suite: hostile programs that MUST be contained. These encode
// the security model described in docs/judge-sandbox.md — any failure here
// is a sandbox breach, not a test flake. Run as root on a Linux host with
// cgroup v2 (the judge box):
//
//	CGO_ENABLED=0 GOOS=linux go test -c -o sandbox.test ./pkg/sandbox/
//	./sandbox.test -test.run TestEscape -test.v
//
// Each case gets a fresh workspace with a bait file outside the workspace;
// the program tries a specific escape vector and we assert containment.

func runHostile(t *testing.T, name string, code string, files map[string]string) *Result {
	t.Helper()
	res, ws := runHostileKeepWs(t, name, code, files)
	t.Cleanup(func() { os.RemoveAll(ws) })
	return res
}

// readProbe writes a marker via the ONLY channel a contained program has
// (its own workspace); if the escape works the program writes the bait
// content into marker.txt.
const readBait = "/etc/oj-escape-bait"
const baitContent = "TOPSECRET"

func setupBait(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(readBait, []byte(baitContent), 0o644); err != nil {
		t.Skipf("cannot write bait (need root): %v", err)
	}
}

func assertContained(t *testing.T, name, ws string, res *Result) {
	t.Helper()
	marker := filepath.Join(ws, "marker.txt")
	data, _ := os.ReadFile(marker)
	if strings.Contains(string(data), baitContent) {
		t.Fatalf("%s: ESCAPED — read forbidden file, marker=%q", name, string(data))
	}
}

// The redundant single-vector tests below were folded into
// TestEscapeSuiteSequential; only the resource-exhaustion cases (fork bomb,
// memory bomb) and seccomp-violation case remain standalone because their
// pass criteria differ (containment time / kill signal, not marker files).

func TestEscapeForkBomb(t *testing.T) {
	// classic bash fork bomb; pids.max + wall watchdog must contain it
	r := runHostile(t, "forkbomb", `:(){ :|:& };:`+"\necho alive", nil)
	if r.WallMS > 12_000 {
		t.Fatalf("forkbomb: took %dms — watchdog failed", r.WallMS)
	}
}

func TestEscapeMemoryBomb(t *testing.T) {
	setupBait(t)
	r := runHostile(t, "membomb", `python3 -c "
a = bytearray(2*1024*1024*1024)
" 2>/dev/null || sh -c 'dd if=/dev/zero of=/dev/null bs=1M count=65536' 2>/dev/null; echo done`, nil)
	_ = r
	// mem limit is 512MB; whatever the outcome, the process must be dead
	// and fast (no 30s hang)
	if r.WallMS > 12_000 {
		t.Fatalf("membomb: took %dms", r.WallMS)
	}
}

func TestEscapeNetwork(t *testing.T) {
	setupBait(t)
	// try to reach anything: if sockets somehow exist there are no
	// interfaces in the netns; assert no connectivity marker appears
	r, ws := runHostileKeepWs(t, "net", `sh -c 'exec 3<>/dev/tcp/127.0.0.1/80' 2>/dev/null && echo NETOK > marker.txt; echo done`, nil)
	defer os.RemoveAll(ws)
	_ = r
	data, _ := os.ReadFile(filepath.Join(ws, "marker.txt"))
	if strings.Contains(string(data), "NETOK") {
		t.Fatal("net: ESCAPED — connected to host network")
	}
}

func TestEscapePathTraversal(t *testing.T) {
	setupBait(t)
	r, ws := runHostileKeepWs(t, "traversal", `cat ../../../../etc/oj-escape-bait > marker.txt 2>/dev/null; cat /etc/../etc/oj-escape-bait >> marker.txt 2>/dev/null; echo done`, nil)
	defer os.RemoveAll(ws)
	assertContained(t, "traversal", ws, r)
}

func TestEscapeSeccompViolations(t *testing.T) {
	setupBait(t)
	// ptrace/mount/unshare/setuid must be SIGSYS-killed, not executed:
	// a python ptrace(PTRACE_ATTACH) attempt must end in a signal, not a
	// successful return.
	r := runHostile(t, "seccomp", `python3 -c "
import ctypes
libc = ctypes.CDLL(None, use_errno=True)
print('ptrace', libc.ptrace(16, 1, 0, 0))
" 2>&1; echo done`, nil)
	if r.Signal == 0 && r.ExitCode == 0 {
		t.Fatalf("seccomp: hostile syscall returned cleanly (exit %d signal %d)", r.ExitCode, r.Signal)
	}
}

func newTestSandbox(t *testing.T) *Sandbox {
	t.Helper()
	// why the production base: delegated child groups on this host hang on
	// nested controller enables (mem_cgroup_css_alloc under nsdelegate), so
	// tests share the daemon's already-enabled /sys/fs/cgroup/oj-judge.
	sb := &Sandbox{CgroupBase: "/sys/fs/cgroup/oj-judge", HidePaths: []string{"/tmp/esc-root"}}
	// applyLimits (wired into Run since the phantom-RE fix) needs the base
	// dir to exist — create it; the suite runs as root on the judge box.
	if err := os.MkdirAll("/sys/fs/cgroup/oj-test", 0o755); err != nil {
		t.Skipf("cgroup base: %v", err)
	}
	if err := sb.Preflight(); err != nil {
		t.Skipf("preflight (need root + cgroup v2): %v", err)
	}
	return sb
}

// runHostileKeepWs is runHostile that also returns the workspace path.
var lastWs string

func runHostileKeepWs(t *testing.T, name, code string, files map[string]string) (res *Result, ws string) {
	t.Helper()
	ws, err := os.MkdirTemp("/tmp", "esc-")
	if err != nil {
		t.Skipf("no /tmp: %v", err)
	}
	lastWs = ws
	if err := os.Chown(ws, 65534, 65534); err != nil {
		t.Skipf("need root: %v", err)
	}
	if err := os.Chmod(ws, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "main.sh"), []byte(code), 0o755); err != nil {
		t.Fatal(err)
	}
	for n, c := range files {
		_ = os.WriteFile(filepath.Join(ws, n), []byte(c), 0o644)
	}
	sb := newTestSandbox(t)
	spec := &Spec{
		Argv:        []string{"/bin/sh", "main.sh"},
		Workspace:   ws,
		TimeLimitMS: 8000,
		WallLimitMS: 10_000,
		MemLimitKB:  512 * 1024,
		PidsLimit:   64,
		Profile:     ProfileRun,
	}
	r, err := sb.Run(spec, nil)
	if err != nil {
		t.Fatalf("%s: run error: %v", name, err)
	}
	return r, ws
}

// TestEscapeSuiteSequential runs the read/write/network cases with marker
// inspection in one test to reuse the bait setup deterministically.
func TestEscapeSuiteSequential(t *testing.T) {
	setupBait(t)
	cases := []struct {
		name string
		code string
	}{
		{"read-bait", `cat /etc/oj-escape-bait > marker.txt 2>/dev/null; echo done`},
		{"read-proc1", `cat /proc/1/environ > marker.txt 2>/dev/null; echo done`},
		{"traversal", `cat ../../../../../etc/oj-escape-bait > marker.txt 2>/dev/null; echo done`},
		{"write-host", `echo pwned > /tmp/oj-pwned-host; echo done`},
		{"net-connect", `sh -c 'exec 3<>/dev/tcp/127.0.0.1/22' 2>/dev/null && echo NETOK > marker.txt; echo done`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			res, ws := runHostileKeepWs(t, tc.name, tc.code, nil)
			defer os.RemoveAll(ws)
			if res.WallMS > 12_000 {
				t.Fatalf("took %dms", res.WallMS)
			}
			data, _ := os.ReadFile(filepath.Join(ws, "marker.txt"))
			if strings.Contains(string(data), baitContent) {
				t.Fatalf("ESCAPED: marker contains bait: %q", string(data))
			}
			if strings.Contains(string(data), "NETOK") {
				t.Fatalf("ESCAPED: network connectivity")
			}
			if _, err := os.Stat("/tmp/oj-pwned-host"); err == nil {
				os.Remove("/tmp/oj-pwned-host")
				t.Fatalf("ESCAPED: wrote to host /tmp")
			}
		})
	}
	_ = unix.Getpid
	_ = time.Now
}
