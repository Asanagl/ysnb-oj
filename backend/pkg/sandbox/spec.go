// Package sandbox executes untrusted programs inside a Linux jail built from
// cgroup v2 limits, namespaces, a read-only root view and a seccomp whitelist.
// It is deliberately cgo-free so the judge daemon cross-compiles to a static
// Linux binary from any host.
package sandbox

import (
	"errors"
	"fmt"
	"os"
)

// Profile selects policy looseness; compile may fork compilers, run cannot.
type Profile string

const (
	ProfileCompile Profile = "compile"
	ProfileRun     Profile = "run"
)

// Spec describes one sandboxed execution. Paths are host paths; the child
// sees them at identical locations behind the read-only root bind.
type Spec struct {
	Argv       []string `json:"argv"`
	Env        []string `json:"env,omitempty"`
	Workspace  string   `json:"workspace"` // host dir, bind-mounted RW, used as cwd
	StdinPath  string   `json:"stdin_path,omitempty"`
	StdoutPath string   `json:"stdout_path,omitempty"`
	StderrPath string   `json:"stderr_path,omitempty"`

	TimeLimitMS  int64   `json:"time_limit_ms"`           // CPU budget that classifies TLE
	WallLimitMS  int64   `json:"wall_limit_ms,omitempty"` // watchdog; 0 = TimeLimit+grace
	MemLimitKB   int64   `json:"mem_limit_kb"`            // cgroup memory.max; 0 = unlimited
	FSizeLimitKB int64   `json:"fsize_limit_kb,omitempty"`
	StackLimitKB int64   `json:"stack_limit_kb,omitempty"`
	AsLimitKB    int64   `json:"as_limit_kb,omitempty"` // 0 = unlimited (JVM reserves huge VA)
	PidsLimit    int     `json:"pids_limit,omitempty"`
	Profile      Profile `json:"profile"`
}

// Result reports one sandboxed execution. Err distinguishes infrastructure
// failures (SE) from program failures, which are classified in Status.
type Result struct {
	ExitCode         int    `json:"exit_code"`
	Signal           int    `json:"signal,omitempty"`
	Status           string `json:"status"` // OK | RE | TLE | MLE | SE
	WallMS           int64  `json:"wall_ms"`
	CPUTimeMS        int64  `json:"cpu_time_ms"`
	MaxRSSKB         int64  `json:"max_rss_kb"`
	KilledByWatchdog bool   `json:"killed_by_watchdog"`
	Message          string `json:"message,omitempty"`
}

// Files lets callers override the stdio redirections (needed to wire the
// pipes of an interactive judge). Callers keep ownership and must close them.
type Files struct {
	Stdin  *os.File
	Stdout *os.File
	Stderr *os.File
}

const (
	StatusOK  = "OK"
	StatusRE  = "RE"
	StatusTLE = "TLE"
	StatusMLE = "MLE"
	StatusSE  = "SE"

	// runAsUID/GID is nobody; sandboxed code never runs as root and never
	// shares a uid with host services.
	runAsUID = 65534
	runAsGID = 65534

	watchdogGraceMS = 300
)

func (s *Spec) validate() error {
	if len(s.Argv) == 0 || s.Argv[0] == "" {
		return errors.New("sandbox: argv is empty")
	}
	if s.Workspace == "" {
		return errors.New("sandbox: workspace is required")
	}
	if s.TimeLimitMS <= 0 {
		return errors.New("sandbox: time limit must be positive")
	}
	return nil
}

func (s *Spec) wallLimit() int64 {
	if s.WallLimitMS > 0 {
		return s.WallLimitMS
	}
	return s.TimeLimitMS + watchdogGraceMS
}

func (s *Spec) describe() string {
	return fmt.Sprintf("argv=%v time=%dms mem=%dKB", s.Argv, s.TimeLimitMS, s.MemLimitKB)
}
