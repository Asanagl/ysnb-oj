//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestConcurrentRuns reproduces the parallel-judging intermittent exit-2:
// many concurrent Run() calls of an identical spec shape (like judge cases).
// On the bug it fails with RE exit=2; on a correct sandbox all runs are OK.
func TestConcurrentRuns(t *testing.T) {
	sb := newTestSandbox(t)
	const N = 40
	wsRoot, err := os.MkdirTemp("/oj-work", "conc-")
	if err != nil {
		t.Skipf("work dir: %v", err)
	}
	defer os.RemoveAll(wsRoot)

	var wg sync.WaitGroup
	results := make([]*Result, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ws, _ := os.MkdirTemp(wsRoot, "c-")
			_ = os.Chown(ws, 65534, 65534)
			_ = os.Chmod(ws, 0o700)
			// mimic staged case dir: root-owned executable inside
			_ = os.WriteFile(filepath.Join(ws, "main.sh"), []byte("echo hi\n"), 0o755)
			spec := &Spec{
				Argv:        []string{"/bin/sh", "main.sh"},
				Workspace:   ws,
				TimeLimitMS: 5000,
				WallLimitMS: 8000,
				MemLimitKB:  256 * 1024,
				PidsLimit:   64,
				Profile:     ProfileRun,
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
		t.Fatalf("%d/%d concurrent runs failed", bad, N)
	}
}
