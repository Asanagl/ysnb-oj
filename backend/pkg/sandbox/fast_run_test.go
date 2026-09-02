//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestSingleFastRun: one trivial program must return in well under the wall
// limit. Earlier escape tests showed every run taking the FULL 10s wall
// limit even for instant programs — a Wait()/watchdog interaction bug that
// parallel judging then turns into intermittent wrong verdicts.
func TestSingleFastRun(t *testing.T) {
	sb := newTestSandbox(t)
	ws, err := os.MkdirTemp("/oj-work", "fast-")
	if err != nil {
		t.Skipf("work dir: %v", err)
	}
	defer os.RemoveAll(ws)
	_ = os.Chown(ws, 65534, 65534)
	_ = os.Chmod(ws, 0o700)
	_ = os.WriteFile(filepath.Join(ws, "ok.txt"), []byte("x"), 0o644)

	spec := &Spec{
		Argv:        []string{"/bin/true"},
		Workspace:   ws,
		TimeLimitMS: 2000,
		WallLimitMS: 3000,
		MemLimitKB:  64 * 1024,
		PidsLimit:   32,
		Profile:     ProfileRun,
	}
	for i := 0; i < 5; i++ {
		t0 := time.Now()
		r, err := sb.Run(spec, nil)
		d := time.Since(t0)
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		fmt.Printf("run %d: status=%s wall=%dms dur=%v\n", i, r.Status, r.WallMS, d.Round(time.Millisecond))
		if d > 1500*time.Millisecond {
			t.Fatalf("run %d blocked for %v — Wait() missed child exit, watchdog masking", i, d)
		}
		if r.Status != StatusOK {
			t.Fatalf("run %d status %s", i, r.Status)
		}
	}
}
