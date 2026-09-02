//go:build linux

package sandbox

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// bpfInstruction and sockFprog mirror the kernel ABI structs
// (linux/filter.h: sock_filter, sock_fprog) because x/sys/unix does not
// export them for Linux; field order/sizes are part of the syscall contract.
type bpfInstruction struct {
	Code uint16
	Jt   uint8
	Jf   uint8
	K    uint32
}

type sockFprog struct {
	Len    uint16
	Filter *bpfInstruction
}

// BPF instruction classes / sizes / modes (linux/bpf_common.h).
const (
	bpfLD       uint16 = 0x00
	bpfJMP      uint16 = 0x05
	bpfRET      uint16 = 0x06
	bpfW        uint16 = 0x00
	bpfABS      uint16 = 0x20
	bpfJEQ      uint16 = 0x10
	bpfK        uint16 = 0x00
	bpfALUorJMP        = 0 // unused placeholder guard
)

func bpfStmt(code uint16, k uint32) bpfInstruction {
	return bpfInstruction{Code: uint16(code | bpfK), Jt: 0, Jf: 0, K: k}
}

func bpfJump(code uint16, k uint32, jt, jf uint8) bpfInstruction {
	return bpfInstruction{Code: code, Jt: jt, Jf: jf, K: k}
}

// seccompWhitelists maps profile → allowed syscall numbers. The filter is
// default-deny: anything not listed raises SIGSYS and the process dies.
// why whitelist per profile: compilers legitimately fork/exec their own
// sub-tools, while submitted programs have no reason to do either.
func seccompWhitelist(p Profile) []int32 {
	common := []int32{
		unix.SYS_READ, unix.SYS_WRITE, unix.SYS_OPEN, unix.SYS_CLOSE,
		unix.SYS_STAT, unix.SYS_FSTAT, unix.SYS_LSTAT, unix.SYS_POLL,
		unix.SYS_LSEEK, unix.SYS_MMAP, unix.SYS_MPROTECT, unix.SYS_MUNMAP,
		unix.SYS_BRK, unix.SYS_RT_SIGACTION, unix.SYS_RT_SIGPROCMASK,
		unix.SYS_RT_SIGRETURN, unix.SYS_IOCTL, unix.SYS_PREAD64,
		unix.SYS_PWRITE64, unix.SYS_READV, unix.SYS_WRITEV, unix.SYS_ACCESS,
		unix.SYS_PIPE, unix.SYS_PIPE2, unix.SYS_SELECT, unix.SYS_SCHED_YIELD,
		unix.SYS_MREMAP, unix.SYS_MSYNC, unix.SYS_MINCORE, unix.SYS_MADVISE,
		unix.SYS_DUP, unix.SYS_DUP2, unix.SYS_DUP3, unix.SYS_NANOSLEEP,
		unix.SYS_GETITIMER, unix.SYS_ALARM, unix.SYS_SETITIMER, unix.SYS_GETPID,
		unix.SYS_GETPPID, unix.SYS_SENDFILE, unix.SYS_SOCKETPAIR,
		unix.SYS_EXECVE, unix.SYS_EXIT, unix.SYS_WAIT4, unix.SYS_WAITID,
		unix.SYS_KILL, unix.SYS_TGKILL, unix.SYS_UNAME, unix.SYS_FCNTL,
		unix.SYS_FLOCK, unix.SYS_FSYNC, unix.SYS_FDATASYNC, unix.SYS_TRUNCATE,
		unix.SYS_FTRUNCATE, unix.SYS_GETDENTS, unix.SYS_GETDENTS64,
		unix.SYS_GETCWD, unix.SYS_CHDIR, unix.SYS_FCHDIR, unix.SYS_RENAME,
		unix.SYS_MKDIR, unix.SYS_RMDIR, unix.SYS_CREAT, unix.SYS_LINK,
		unix.SYS_UNLINK, unix.SYS_SYMLINK, unix.SYS_READLINK, unix.SYS_CHMOD,
		unix.SYS_FCHMOD, unix.SYS_UMASK, unix.SYS_GETTIMEOFDAY,
		unix.SYS_GETRLIMIT, unix.SYS_SETRLIMIT, unix.SYS_PRLIMIT64,
		unix.SYS_GETRUSAGE, unix.SYS_SYSINFO, unix.SYS_TIMES, unix.SYS_GETUID,
		unix.SYS_GETEUID, unix.SYS_GETGID, unix.SYS_GETEGID, unix.SYS_SIGALTSTACK,
		unix.SYS_STATFS, unix.SYS_FSTATFS, unix.SYS_PRCTL, unix.SYS_ARCH_PRCTL,
		unix.SYS_SET_TID_ADDRESS, unix.SYS_SCHED_GETAFFINITY, unix.SYS_GETRANDOM,
		unix.SYS_CLOCK_NANOSLEEP, unix.SYS_EXIT_GROUP, unix.SYS_EPOLL_CREATE1,
		unix.SYS_EPOLL_CTL, unix.SYS_EPOLL_WAIT, unix.SYS_EPOLL_PWAIT,
		unix.SYS_EPOLL_PWAIT2, unix.SYS_EVENTFD2, unix.SYS_TIMERFD_CREATE,
		unix.SYS_TIMERFD_SETTIME, unix.SYS_FUTEX, unix.SYS_SET_ROBUST_LIST,
		unix.SYS_GET_ROBUST_LIST, unix.SYS_OPENAT, unix.SYS_OPENAT2,
		unix.SYS_FACCESSAT, unix.SYS_FACCESSAT2, unix.SYS_UNLINKAT,
		unix.SYS_MKDIRAT, unix.SYS_RENAMEAT, unix.SYS_RENAMEAT2, unix.SYS_FCHMODAT,
		unix.SYS_FALLOCATE, unix.SYS_COPY_FILE_RANGE, unix.SYS_STATX,
		unix.SYS_RSEQ, unix.SYS_PKEY_MPROTECT, unix.SYS_MEMBARRIER,
		unix.SYS_MEMFD_CREATE, unix.SYS_GETCPU, unix.SYS_GETTID,
		unix.SYS_RESTART_SYSCALL,
		// why newfstatat + clock_getres: newfstatat is what glibc's stat()
		// and the dynamic loader use on modern kernels (the Go 1.21+ PATH
		// resolver needs it too, see stage2 notes); clock_getres is probed
		// by the JVM launcher before anything else. Missing either produces
		// an undebuggable empty-stderr RE.
		unix.SYS_NEWFSTATAT, unix.SYS_CLOCK_GETRES,
	}
	// why clone/fork only where needed: cc1/as/ld are separate processes;
	// submitted programs use threads, which plain clone covers.
	common = append(common, unix.SYS_FORK, unix.SYS_VFORK, unix.SYS_CLONE, unix.SYS_CLONE3)
	if p == ProfileCompile || p == ProfileRun {
		// why AF_UNIX socket calls: the JDK launcher initializes libjvm,
		// which creates and connects a local AF_UNIX socket during startup
		// (both javac and `java` itself) even with no network configured.
		// The sandbox runs in a network namespace with no interfaces and
		// AF_UNIX sockets require no network, so this grants no connectivity.
		common = append(common,
			unix.SYS_SOCKET, unix.SYS_CONNECT, unix.SYS_BIND,
			unix.SYS_LISTEN, unix.SYS_ACCEPT, unix.SYS_ACCEPT4,
			unix.SYS_SETSOCKOPT, unix.SYS_GETSOCKOPT,
			unix.SYS_GETSOCKNAME, unix.SYS_GETPEERNAME,
		)
	}
	return common
}

// buildSeccompProgram compiles the whitelist into a classic cBPF program:
// verify the 64-bit audit arch, allow listed syscall numbers, kill the rest.
func buildSeccompProgram(p Profile) []bpfInstruction {
	killAction := seccompKillAction()
	prog := []bpfInstruction{
		bpfStmt(bpfLD|bpfW|bpfABS, 4), // seccomp_data.nr is offset 0, arch offset 4
		bpfJump(bpfJMP|bpfJEQ|bpfK, unix.AUDIT_ARCH_X86_64, 1, 0),
		bpfStmt(bpfRET, uint32(killAction)),
	}
	allowed := seccompWhitelist(p)
	// Chunked linear chain. The original single chain emitted per-entry BPF
	// jump offsets in a uint8, which silently wrapped once the whitelist
	// passed 127 entries — mismatches then landed on a stale ALLOW and legal
	// syscalls (glibc stat()) landed on a stale KILL. Splitting into chunks
	// of ≤120 keeps every jt/jf within uint8; a small JA trampoline hops
	// between chunk tails.
	const chunkSize = 120
	// load seccomp_data.nr (offset 0) into the accumulator; the arch check
	// above consumed the previous load.
	prog = append(prog, bpfStmt(bpfLD|bpfW|bpfABS, 0))
	for start := 0; start < len(allowed); start += chunkSize {
		end := min(start+chunkSize, len(allowed))
		chunk := allowed[start:end]
		last := end == len(allowed)
		n := len(chunk)
		for i, nr := range chunk {
			isChunkTail := i == n-1
			// Jt: match jumps to this chunk's RET ALLOW, appended after the
			// remaining JEQs (and, for non-final chunks, after the JA).
			// Distance from entry i: (n-1-i) JEQs in between, then the ALLOW
			// itself; a non-final chunk has one more instruction (the JA).
			jt := n - 1 - i
			if last {
				jt += 1
			} else {
				jt += 2
			}
			jt-- // BPF jump targets are relative to the next instruction
			jf := 0 // mismatch falls through to the next JEQ
			if isChunkTail {
				// mismatch must leave the chunk: over ALLOW (final chunk)
				// or over JA+ALLOW (more chunks follow).
				if last {
					jf = 1
				} else {
					jf = 2
				}
			}
			prog = append(prog, bpfJump(bpfJMP|bpfJEQ|bpfK, uint32(nr), uint8(jt), uint8(jf)))
		}
		if !last {
			// trampoline over this chunk's ALLOW into the next chunk
			prog = append(prog, bpfJump(bpfJMP|bpfJA|bpfK, 2, 0, 0))
		}
		prog = append(prog, bpfStmt(bpfRET, unix.SECCOMP_RET_ALLOW))
	}
	prog = append(prog,
		bpfStmt(bpfRET, uint32(killAction)), // nr not in whitelist
	)
	return prog
}

// bpfJA is the low 4 bits of the JMP class code for an unconditional jump
// (linux/bpf_common.h: BPF_JA = 0x00); its offset travels in K.
const bpfJA uint16 = 0x00

// seccompKillAction prefers killing the whole process tree (kernel >= 4.14,
// where SECCOMP_RET_KILL_PROCESS exists) and degrades to per-thread kill on
// older kernels. Probing by installing a filter is not an option — filters
// are permanent — so the decision comes from the kernel version.
func seccompKillAction() uintptr {
	var uts unix.Utsname
	if err := unix.Uname(&uts); err != nil {
		return unix.SECCOMP_RET_KILL
	}
	major, minor := parseKernelVersion(uts.Release)
	if major > 4 || (major == 4 && minor >= 14) {
		return unix.SECCOMP_RET_KILL_PROCESS
	}
	return unix.SECCOMP_RET_KILL
}

func parseKernelVersion(release [65]byte) (int, int) {
	var nums []int
	cur := 0
	for _, c := range release {
		if c == 0 {
			break
		}
		if c == '.' {
			nums = append(nums, cur)
			cur = 0
			if len(nums) == 2 {
				break
			}
			continue
		}
		if c < '0' || c > '9' {
			continue
		}
		cur = cur*10 + int(c-'0')
	}
	nums = append(nums, cur)
	if len(nums) < 2 {
		return 0, 0
	}
	return nums[0], nums[1]
}

// loadSeccomp installs the profile filter; must run right before execve so
// the daemon's own syscalls are untouched.
//
// why prctl(PR_SET_SECCOMP) instead of the seccomp(2) syscall: both reach the
// identical kernel filter-attach path, but some cloud kernels (observed on a
// Debian 11 VPS during deployment testing) return EOPNOTSUPP for the raw
// seccomp(2) syscall while accepting the prctl route — so we use the prctl
// form, supported since Linux 3.17 everywhere.
func loadSeccomp(p Profile) error {
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("no_new_privs: %w", err)
	}
	prog := buildSeccompProgram(p)
	fprog := sockFprog{Len: uint16(len(prog)), Filter: &prog[0]}
	if err := unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, uintptr(unsafe.Pointer(&fprog)), 0, 0); err != nil {
		return fmt.Errorf("prctl set_seccomp: %w", err)
	}
	return nil
}
