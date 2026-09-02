//go:build linux

package sandbox

import (
	"fmt"
	"testing"

	"golang.org/x/sys/unix"
)

// The original single-chain BPF builder computed per-entry jump offsets in a
// uint8; once the whitelist passed 127 entries the offsets wrapped and the
// filter returned wrong verdicts (glibc stat() → SIGSYS → empty-stderr CE
// for every Java compile). These tests execute the built program in a small
// interpreter and pin the verdicts for every syscall number, so any layout
// regression is caught regardless of whitelist size.
func simulate(prog []bpfInstruction, nr int32) (uint32, error) {
	const arch = uint32(unix.AUDIT_ARCH_X86_64)
	a := uint32(0)
	pc := 0
	for i := 0; i < 100000; i++ {
		if pc < 0 || pc >= len(prog) {
			return 0, fmt.Errorf("pc %d out of range", pc)
		}
		ins := prog[pc]
		switch ins.Code {
		case bpfStmtCode: // LD|W|ABS: A = seccomp_data[ins.K]
			if ins.K == 4 {
				a = arch
			} else {
				a = uint32(nr)
			}
			pc++
		case bpfJEQCode:
			if a == ins.K {
				pc = pc + 1 + int(ins.Jt)
			} else {
				pc = pc + 1 + int(ins.Jf)
			}
		case bpfJACode:
			pc = pc + 1 + int(ins.K)
		case bpfRETCode:
			return ins.K, nil
		default:
			return 0, fmt.Errorf("unknown code %#x at %d", ins.Code, pc)
		}
	}
	return 0, fmt.Errorf("no ret after 100000 steps")
}

const (
	bpfStmtCode = bpfLD | bpfW | bpfABS
	bpfJEQCode  = bpfJMP | bpfJEQ | bpfK
	bpfJACode   = bpfJMP | bpfJA | bpfK
	bpfRETCode  = bpfRET | bpfK
)

func mustVerdict(t *testing.T, prog []bpfInstruction, nr int32) uint32 {
	t.Helper()
	v, err := simulate(prog, nr)
	if err != nil {
		t.Fatalf("nr %d: %v", nr, err)
	}
	return v
}

func TestSeccompProgramAllowsWhitelisted(t *testing.T) {
	for _, p := range []Profile{ProfileCompile, ProfileRun} {
		prog := buildSeccompProgram(p)
		for _, nr := range seccompWhitelist(p) {
			if got := mustVerdict(t, prog, nr); got != unix.SECCOMP_RET_ALLOW {
				t.Fatalf("profile %s nr %d: want ALLOW got %#x", p, nr, got)
			}
		}
	}
}

func TestSeccompProgramKillsUnknown(t *testing.T) {
	known := map[int32]bool{}
	for _, p := range []Profile{ProfileCompile, ProfileRun} {
		for _, nr := range seccompWhitelist(p) {
			known[nr] = true
		}
	}
	prog := buildSeccompProgram(ProfileRun)
	killMask := uint32(unix.SECCOMP_RET_KILL_PROCESS | unix.SECCOMP_RET_KILL |
		unix.SECCOMP_RET_KILL_THREAD | unix.SECCOMP_RET_ERRNO | unix.SECCOMP_RET_TRAP |
		unix.SECCOMP_RET_ALLOW | unix.SECCOMP_RET_LOG | unix.SECCOMP_RET_TRACE)
	for nr := int32(0); nr < 460; nr++ {
		if known[nr] {
			continue
		}
		got := mustVerdict(t, prog, nr)
		if got&killMask == got && got != unix.SECCOMP_RET_ALLOW {
			continue // any kill-class action is a correct deny
		}
		t.Fatalf("nr %d not whitelisted: want KILL got %d", nr, got)
	}
}

func TestSeccompOffsetsWithinUint8(t *testing.T) {
	// Guard the chunk size directly: every jt/jf must fit uint8 even as the
	// whitelist grows. 120-entry chunks keep offsets ≤ 121.
	for _, p := range []Profile{ProfileCompile, ProfileRun} {
		prog := buildSeccompProgram(p)
		for _, ins := range prog {
			if ins.Jt > 200 || ins.Jf > 200 {
				t.Fatalf("profile %s: jump offset too large (jt=%d jf=%d)", p, ins.Jt, ins.Jf)
			}
		}
	}
}
