//go:build linux

package sandbox

import (
	"os"
	"testing"
)

// TestMain routes OJ_STAGE2 re-execs to Stage2Main. The sandbox launches its
// stage2 by re-executing /proc/self/exe with the spec in the environment;
// in a TEST binary that re-exec would otherwise restart the entire test
// suite inside the sandboxed child (nested sandbox recursion — the child
// then runs its own stage2 children and the parent's Wait sees a process
// busy re-testing, not the target program). Production cmd/judge does this
// check in main(); the test binary needs it here. Stage2Main is in this
// package, hence no import needed.
func TestMain(m *testing.M) {
	if os.Getenv("OJ_STAGE2") == "1" {
		Stage2Main()
		return // never reached; Stage2Main exits
	}
	os.Exit(m.Run())
}
