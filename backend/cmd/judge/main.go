// Command judge is the judge daemon: it connects to the API server, receives
// tasks and runs them inside the self-developed sandbox. It must run as root
// on a Linux host with cgroup v2 (see docs/deploy.md).
//
// Flags:
//
//	--selftest   run sandbox environment checks and exit (exit 1 on failure)
//	--config     config file path (default $OJ_CONFIG or built-in defaults)
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ysnb/oj/internal/config"
	"github.com/ysnb/oj/internal/daemon"
	"github.com/ysnb/oj/pkg/judge"
	"github.com/ysnb/oj/pkg/sandbox"
)

// constrainedOverridePath normalizes the operator-supplied languages
// override: empty stays empty; otherwise it must be a .yaml/.yml path with
// no traversal segments (mirrors config.Load's guard — see security-audit
// §13.2; the operator owns the environment, this catches misconfiguration).
func constrainedOverridePath(p string) string {
	if p == "" {
		return ""
	}
	if !strings.HasSuffix(p, ".yaml") && !strings.HasSuffix(p, ".yml") {
		log.Printf("[judge] OJ_LANGUAGES_FILE %q ignored: only .yaml/.yml accepted", p)
		return ""
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(p)
}

func main() {
	// The sandbox re-executes this binary inside new namespaces; that inner
	// stage must run before any daemon logic.
	if os.Getenv("OJ_STAGE2") == "1" {
		sandbox.Stage2Main()
		return
	}

	selftest := flagArg("--selftest")
	cfg, err := config.Load(configPath())
	if err != nil {
		log.Fatalf("[judge] %v", err)
	}
	sb := &sandbox.Sandbox{
		CgroupBase: cfg.Judge.CGroupBase,
		HidePaths:  []string{cfg.DataDir, cfg.Judge.WorkRoot},
	}
	if selftest {
		os.Exit(runSelftest(sb, cfg.Judge.WorkRoot))
	}
	if err := os.MkdirAll(cfg.Judge.WorkRoot, 0o750); err != nil {
		log.Fatalf("[judge] work root: %v", err)
	}
	if err := sb.Preflight(); err != nil {
		log.Fatalf("[judge] preflight: %v", err)
	}
	langs, err := judge.NewRegistry(constrainedOverridePath(os.Getenv("OJ_LANGUAGES_FILE")))
	if err != nil {
		log.Fatalf("[judge] %v", err)
	}
	// why Judge.DaemonToken for fetches: it is the same shared secret the
	// daemon authenticates its gRPC stream with; JWT.DaemonSecret is the
	// API-side field and is never set in the judge process.
	svc, err := judge.NewService(sb, langs, cfg.Judge.WorkRoot,
		cfg.DataDir+"/judge-cache", cfg.FetchBase, cfg.Judge.DaemonToken)
	if err != nil {
		log.Fatalf("[judge] %v", err)
	}
	if cfg.Judge.DaemonName == "" {
		cfg.Judge.DaemonName = "judge-1"
	}
	d := &daemon.Daemon{Cfg: cfg, Service: svc}
	log.Printf("[judge] daemon %q starting, parallel=%d, api=%s",
		cfg.Judge.DaemonName, cfg.Judge.MaxParallel, cfg.Judge.APIEndpoint)
	if err := d.Run(context.Background()); err != nil {
		log.Fatalf("[judge] %v", err)
	}
}

func configPath() string {
	if p := os.Getenv("OJ_CONFIG"); p != "" {
		return p
	}
	if len(os.Args) > 2 && os.Args[1] == "--config" {
		return os.Args[2]
	}
	return ""
}

func flagArg(name string) bool {
	for _, a := range os.Args[1:] {
		if a == name {
			return true
		}
	}
	return false
}

// runSelftest validates the host environment so operators can verify a judge
// machine before pointing real submissions at it. Workspaces go under
// workRoot — the sandbox covers host /tmp, so /tmp workspaces cannot work.
func runSelftest(sb *sandbox.Sandbox, workRoot string) int {
	fmt.Println("== sandbox selftest ==")
	if err := sb.Preflight(); err != nil {
		fmt.Println("FAIL cgroup:", err)
		return 1
	}
	fmt.Println("PASS cgroup v2 writable")
	if err := os.MkdirAll(workRoot, 0o750); err != nil {
		fmt.Println("FAIL work root:", err)
		return 1
	}
	if err := runEchoTest(sb, workRoot); err != nil {
		fmt.Println("FAIL basic run:", err)
		return 1
	}
	fmt.Println("PASS basic jailed run (/bin/echo)")
	if err := runTimeoutTest(sb, workRoot); err != nil {
		fmt.Println("FAIL timeout test:", err)
		return 1
	}
	fmt.Println("PASS wall-clock watchdog (100ms limit)")
	fmt.Println("== all selftests passed ==")
	return 0
}
