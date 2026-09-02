//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ysnb/oj/pkg/sandbox"
)

// runEchoTest runs /bin/echo inside the jail and checks its output; it
// exercises namespaces, mounts, privs, seccomp and output capture end to end.
// Workspaces must live under WorkRoot: stage2 covers host /tmp with a fresh
// tmpfs, so a /tmp workspace would be invisible to the bind mount.
func runEchoTest(sb *sandbox.Sandbox, workRoot string) error {
	ws, err := os.MkdirTemp(workRoot, "selftest-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(ws)
	_ = os.Chmod(ws, 0o755)
	out := filepath.Join(ws, "out.txt")
	res, err := sb.Run(&sandbox.Spec{
		Argv:         []string{"/bin/echo", "sandbox-ok"},
		Env:          []string{"PATH=/usr/bin:/bin"},
		Workspace:    ws,
		StdoutPath:   out,
		TimeLimitMS:  2000,
		FSizeLimitKB: 1024,
		Profile:      sandbox.ProfileRun,
	}, nil)
	if err != nil {
		return err
	}
	if res.Status != sandbox.StatusOK {
		return fmt.Errorf("status %s: %s", res.Status, res.Message)
	}
	raw, _ := os.ReadFile(out)
	if string(raw) != "sandbox-ok\n" {
		return errors.New("echo output mismatch: " + string(raw))
	}
	return nil
}

// runTimeoutTest runs a busy-loop with a 100ms wall limit and expects TLE.
func runTimeoutTest(sb *sandbox.Sandbox, workRoot string) error {
	ws, err := os.MkdirTemp(workRoot, "selftest-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(ws)
	_ = os.Chmod(ws, 0o755)
	sh := `i=0; while :; do i=$((i+1)); done`
	res, err := sb.Run(&sandbox.Spec{
		Argv:         []string{"/bin/sh", "-c", sh},
		Env:          []string{"PATH=/usr/bin:/bin"},
		Workspace:    ws,
		TimeLimitMS:  100,
		FSizeLimitKB: 1024,
		Profile:      sandbox.ProfileRun,
	}, nil)
	if err != nil {
		return err
	}
	if res.Status != sandbox.StatusTLE {
		return fmt.Errorf("expected TLE, got %s (%s)", res.Status, res.Message)
	}
	return nil
}
