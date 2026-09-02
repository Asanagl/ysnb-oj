//go:build linux

// Seccomp probe v2: SYS_seccomp vs prctl(PR_SET_SECCOMP) from Go, unbuffered.
package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

type sockFilter struct {
	Code uint16
	Jt   uint8
	Jf   uint8
	K    uint32
}

type sockFprog struct {
	Len    uint16
	Filter *sockFilter
}

const (
	sysSeccomp      = 317
	sysPrctl        = 157
	prSetNoNewPrivs = 38
	prSetSeccomp    = 22
	modeStrict      = 1
	modeFilter      = 2
	allowRet        = 0x7fff0000
)

func out(s string) { syscall.Write(1, []byte(s)) }

func main() {
	out("go seccomp probe v2\n")
	if _, _, e := syscall.Syscall(sysPrctl, prSetNoNewPrivs, 1, 0); e != 0 {
		out(fmt.Sprintf("NNP: ERRNO=%d\n", e))
		return
	}
	out("NNP: OK\n")

	// A: SYS_seccomp minimal allow
	f1 := sockFprog{Len: 1, Filter: &[]sockFilter{{Code: 0x06, K: allowRet}}[0]}
	_, _, e := syscall.Syscall(sysSeccomp, modeFilter, 0, uintptr(unsafe.Pointer(&f1)))
	out(fmt.Sprintf("A sys_seccomp filter: ERRNO=%d\n", e))

	// B: prctl(PR_SET_SECCOMP, MODE_STRICT) — installs strict mode; only
	// read/write/exit/sigreturn survive. Run LAST because it is terminal.
	_, _, e2 := syscall.Syscall(sysPrctl, prSetSeccomp, modeStrict, 0)
	out(fmt.Sprintf("B prctl strict: ERRNO=%d\n", e2))

	// C: prctl(PR_SET_SECCOMP, MODE_FILTER, &fprog) minimal allow
	_, _, e3 := syscall.Syscall(sysPrctl, prSetSeccomp, modeFilter, uintptr(unsafe.Pointer(&f1)))
	out(fmt.Sprintf("C prctl filter: ERRNO=%d\n", e3))

	// D: SYS_seccomp with GET_ACTION_AVAIL style probe (op=3? op flags)
	_, _, e4 := syscall.Syscall(sysSeccomp, 3, 0, 0)
	out(fmt.Sprintf("D sys_seccomp op3: ERRNO=%d\n", e4))

	// E: SYS_seccomp with bad operation to see errno shape
	_, _, e5 := syscall.Syscall(sysSeccomp, 99, 0, 0)
	out(fmt.Sprintf("E sys_seccomp op99: ERRNO=%d\n", e5))
	out("probe v2 done\n")
}
