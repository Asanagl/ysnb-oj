//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// TestConcurrentELFRuns mirrors judge case runs exactly: a real ELF
// (compiled on-box) staged root-owned into a nobody-owned 0700 case dir,
// run with the judge run-profile spec shape, many at once. CgroupBase uses
// /sys/fs/cgroup/oj-judge (production base, subtree controllers enabled by
// ops) because delegated child groups on this host reject nested controller
// enables (mem_cgroup_css_alloc hang under nsdelegate).
func TestConcurrentELFRuns(t *testing.T) {
	sb := newTestSandbox(t)

	wsRoot, err := os.MkdirTemp("/oj-work", "elf-")
	if err != nil {
		t.Skipf("work dir: %v", err)
	}
	defer os.RemoveAll(wsRoot)

	// compile the probe binary once
	src := filepath.Join(wsRoot, "prog.c")
	bin := filepath.Join(wsRoot, "prog")
	if err := os.WriteFile(src, []byte(`#include <stdio.h>
int main(){ printf("500003500006\n"); return 0; }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("gcc", "-O2", "-o", bin, src).CombinedOutput(); err != nil {
		t.Skipf("gcc unavailable: %v %s", err, out)
	}

	const N = 24 // sized for the box: 24 concurrent ns+tmpfs is survivable
	var wg sync.WaitGroup
	results := make([]*Result, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ws, _ := os.MkdirTemp(wsRoot, "c-")
			_ = os.Chown(ws, 65534, 65534)
			_ = os.Chmod(ws, 0o700)
			// stage: root-owned executable inside nobody dir (judge shape)
			if out, err := exec.Command("cp", bin, filepath.Join(ws, "main")).CombinedOutput(); err != nil {
				results[i] = &Result{Status: StatusSE, Message: "cp: " + string(out)}
				return
			}
			_ = os.Chmod(filepath.Join(ws, "main"), 0o755)
			spec := &Spec{
				Argv:         []string{"./main"},
				Workspace:    ws,
				TimeLimitMS:  1000,
				WallLimitMS:  1500,
				MemLimitKB:   (256 + 8) * 1024,
				StackLimitKB: 262144,
				FSizeLimitKB: 128 << 10,
				PidsLimit:    256,
				Profile:      ProfileRun,
			}
			r, err := sb.Run(spec, nil)
			if err != nil {
				results[i] = &Result{Status: StatusSE, Message: err.Error()}
				return
			}
			results[i] = r
		}(i)
	}
	wg.Wait()
	bad := 0
	for i, r := range results {
		if r == nil || r.Status != StatusOK {
			bad++
			fmt.Printf("run %d: status=%s exit=%d sig=%d msg=%q wall=%dms\n",
				i, r.Status, r.ExitCode, r.Signal, r.Message, r.WallMS)
		}
	}
	if bad > 0 {
		t.Fatalf("%d/%d concurrent ELF runs failed", bad, N)
	}
}
