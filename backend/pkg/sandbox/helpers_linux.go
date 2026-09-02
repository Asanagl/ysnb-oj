//go:build linux

package sandbox

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

// diagPipe carries stage2 bootstrap errors (mount failures, bad execve) on
// fd 3 so infrastructure problems are distinguishable from program stderr.
type diagPipe struct {
	read  *os.File
	write *os.File
}

func newDiagPipe() (*diagPipe, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	return &diagPipe{read: r, write: w}, nil
}

func (d *diagPipe) close() {
	_ = d.read.Close()
	_ = d.write.Close()
}

// drain reads whatever stage2 wrote; the write end closes on exec or exit,
// so the read cannot block. Empty output means a clean exec into the target.
func (d *diagPipe) drain() string {
	_ = d.write.Close()
	raw, err := io.ReadAll(d.read)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func joinPaths(paths []string) string { return strings.Join(paths, ":") }

// fdCloser tracks files opened for redirection so runProcess can release them
// after Wait; explicitly provided Files are owned by the caller, not us.
type fdCloser struct {
	mu    sync.Mutex
	files []*os.File
}

func (c *fdCloser) add(f *os.File) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.files = append(c.files, f)
}

func (c *fdCloser) closeAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, f := range c.files {
		_ = f.Close()
	}
	c.files = nil
}

// applyRedirects wires stdio: explicit Files win (interactive wiring), then
// spec paths, then /dev/null. It returns a closer for the files we opened.
func applyRedirects(cmd *exec.Cmd, spec *Spec, files *Files) *fdCloser {
	closer := &fdCloser{}
	cmd.Stdin = devOrFile(files, "stdin", spec.StdinPath, closer)
	cmd.Stdout = devOrFile(files, "stdout", spec.StdoutPath, closer)
	cmd.Stderr = devOrFile(files, "stderr", spec.StderrPath, closer)
	return closer
}

func devOrFile(files *Files, which, path string, closer *fdCloser) *os.File {
	if files != nil {
		switch which {
		case "stdin":
			if files.Stdin != nil {
				return files.Stdin
			}
		case "stdout":
			if files.Stdout != nil {
				return files.Stdout
			}
		case "stderr":
			if files.Stderr != nil {
				return files.Stderr
			}
		}
	}
	if path == "" {
		return nil // exec.Cmd maps nil to /dev/null
	}
	var f *os.File
	var err error
	if which == "stdin" {
		f, err = os.OpenFile(path, os.O_RDONLY, 0o444)
	} else {
		f, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	}
	if err != nil {
		return nil // falls back to /dev/null; spec paths are judge-created
	}
	closer.add(f)
	return f
}

// classify turns the Wait outcome into a verdict. Precedence: watchdog TLE,
// kernel OOM MLE, SIGXCPU TLE, near-limit SIGKILL MLE, then non-zero exit RE.
func classify(spec *Spec, cmd *exec.Cmd, cg *cgroupGroup, watchdogFired bool, wallMS int64) *Result {
	res := &Result{Status: StatusOK, WallMS: wallMS}
	if cmd.ProcessState == nil {
		res.Status = StatusSE
		res.Message = "process state missing after wait"
		return res
	}
	usage, _ := cmd.ProcessState.SysUsage().(*syscall.Rusage)
	if usage != nil {
		res.CPUTimeMS = int64(usage.Utime.Sec*1000 + usage.Utime.Usec/1000 +
			usage.Stime.Sec*1000 + usage.Stime.Usec/1000)
		res.MaxRSSKB = int64(usage.Maxrss)
	}
	if peak := cg.peakRSSKB(); peak > res.MaxRSSKB {
		res.MaxRSSKB = peak
	}
	res.ExitCode = cmd.ProcessState.ExitCode()
	if status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		res.Signal = int(status.Signal())
	}

	switch {
	case watchdogFired:
		res.Status, res.KilledByWatchdog = StatusTLE, true
		res.Message = "wall clock limit exceeded"
	case cg.oomKills() > 0:
		res.Status = StatusMLE
		res.Message = "killed by kernel OOM"
	case res.Signal == int(unix.SIGXCPU):
		res.Status, res.Message = StatusTLE, "CPU time limit exceeded"
	case res.Signal == int(unix.SIGKILL) && spec.MemLimitKB > 0 && res.MaxRSSKB > spec.MemLimitKB*9/10:
		res.Status, res.Message = StatusMLE, "memory limit exceeded"
	case res.ExitCode == 0 && res.Signal == 0:
		// clean run
	default:
		res.Status = StatusRE
		res.Message = fmt.Sprintf("exit code %d, signal %d", res.ExitCode, res.Signal)
	}
	return res
}
