package judge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ysnb/oj/pkg/sandbox"
)

// Protocol-neutral task/result shapes; the gRPC layer maps these to wire
// messages so pkg/judge stays independent of transport details.

type CaseRef struct {
	Index      int    `json:"index"`
	InputSHA   string `json:"input_sha"`
	AnswerSHA  string `json:"answer_sha,omitempty"`
	InputSize  int64  `json:"input_size"`
	AnswerSize int64  `json:"answer_size,omitempty"`
	// Score is the IOI partial credit for passing this case (0 in ACM mode;
	// the API server assigns it from the problem's per-case score table).
	Score int `json:"score,omitempty"`
}

type Task struct {
	SubmissionID     uint64    `json:"submission_id"`
	ProblemID        uint64    `json:"problem_id"`
	LanguageID       string    `json:"language_id"`
	Code             []byte    `json:"code"`
	TimeLimitMS      int64     `json:"time_limit_ms"`
	MemLimitMB       int64     `json:"mem_limit_mb"`
	JudgeMode        string    `json:"judge_mode"`
	CheckerSource    []byte    `json:"checker_source,omitempty"`
	InteractorSource []byte    `json:"interactor_source,omitempty"`
	Cases            []CaseRef `json:"cases"`
	// StopOnFail stops at the first non-AC case and reports the rest as
	// SKIPPED (contest submissions; training keeps full feedback).
	StopOnFail bool `json:"stop_on_fail,omitempty"`
}

type CaseResult struct {
	Index   int    `json:"index"`
	Status  string `json:"status"`
	TimeMS  int64  `json:"time_ms"`
	MemKB   int64  `json:"mem_kb"`
	Message string `json:"message,omitempty"`
	Score   int    `json:"score,omitempty"` // credit earned on this case (IOI)
}

type Result struct {
	SubmissionID   uint64       `json:"submission_id"`
	Status         string       `json:"status"`
	TimeMS         int64        `json:"time_ms"`
	MemKB          int64        `json:"mem_kb"`
	Score          int          `json:"score,omitempty"`
	CompileMessage string       `json:"compile_message,omitempty"`
	Cases          []CaseResult `json:"cases"`
	Error          string       `json:"error,omitempty"`
}

// Runner abstracts the sandbox so pkg/judge builds on platforms where the
// real sandbox is unavailable (the stub returns errors).
type Runner interface {
	Run(spec *sandbox.Spec, files *sandbox.Files) (*sandbox.Result, error)
	RunPair(userSpec, interSpec *sandbox.Spec, stderr *os.File) (*sandbox.Result, *sandbox.Result, error)
}

type Service struct {
	Runner    Runner
	Reg       *Registry
	Cache     *BinCache
	BlobDir   string // testdata blob cache by sha256
	WorkRoot  string // scratch workspaces root
	FetchBase string // api base for testdata download
	FetchTok  string
	HTTP      *fetchClient
	MaxCases  int
	// CaseWorkers caps per-submission case-level parallelism (0 = 1: cases
	// run serially; task slots × workers must stay within host RAM — see
	// judge_parallel.go's budget note).
	CaseWorkers int
}

func NewService(r Runner, reg *Registry, workRoot, cacheRoot, fetchBase, fetchToken string) (*Service, error) {
	cache, err := NewBinCache(filepath.Join(cacheRoot, "bin"))
	if err != nil {
		return nil, err
	}
	blobDir := filepath.Join(cacheRoot, "blobs")
	if err := os.MkdirAll(blobDir, 0o755); err != nil {
		return nil, err
	}
	return &Service{
		Runner: r, Reg: reg, Cache: cache, BlobDir: blobDir,
		WorkRoot: workRoot, FetchBase: fetchBase, FetchTok: fetchToken,
		HTTP: &fetchClient{token: fetchToken}, MaxCases: 200,
	}, nil
}

// chownWorkspace hands the scratch dir to the sandbox uid (65534) with 0700
// so only that uid and root can ever touch per-run files.
func chownWorkspace(dir string) error {
	if err := os.Chown(dir, 65534, 65534); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
}

const (
	maxOutputBytes  = 128 << 20 // 128 MB, also enforced by RLIMIT_FSIZE
	maxStderrTail   = 8 << 10
	maxTimeMS       = 120_000
	checkerTimeMS   = 30_000
	checkerMemMB    = 1024
	interactiveMode = "interactive"
	spjMode         = "spj"
)

// Run judges one submission end to end. A returned error is always
// infrastructure trouble (SE); program outcomes ride inside Result.
func (s *Service) Run(ctx context.Context, task *Task) *Result {
	res := &Result{SubmissionID: task.SubmissionID}
	lang, err := s.Reg.Get(task.LanguageID)
	if err != nil {
		res.Status = "SE"
		res.Error = err.Error()
		return res
	}
	ws, err := os.MkdirTemp(s.WorkRoot, "judge-*")
	if err != nil {
		res.Status = "SE"
		res.Error = fmt.Sprintf("workspace: %v", err)
		return res
	}
	defer func() {
		// why keep on failure: CE/SE without a live workspace is undebugable;
		// failed workspaces are the judge-side crash dump.
		if res.Status == "CE" || res.Status == "SE" || res.Status == "RE" {
			fmt.Fprintf(os.Stderr, "[judge] task %d %s: workspace kept at %s\n",
				task.SubmissionID, res.Status, ws)
			return
		}
		os.RemoveAll(ws)
	}()
	if err := chownWorkspace(ws); err != nil {
		res.Status = "SE"
		res.Error = fmt.Sprintf("workspace chown: %v", err)
		return res
	}

	// why always write the source: interpreted languages have no compile
	// step, and the runner reads main.py/main.* from the workspace; writing
	// only inside compileSubmission would starve python entirely (found in
	// cloud E2E: python exited 2 "can't open file").
	if err := os.WriteFile(filepath.Join(ws, lang.SourceFile), task.Code, 0o644); err != nil {
		res.Status, res.Error = "SE", fmt.Sprintf("write source: %v", err)
		return res
	}

	if lang.Compile != nil {
		if !s.compileSubmission(res, ws, lang, task) {
			return res
		}
	}
	var checkerPath, interactorPath string
	if task.JudgeMode == spjMode {
		if checkerPath, err = s.prepareTool(ws, task.CheckerSource, "checker"); err != nil {
			res.Status = "SE"
			res.Error = "checker: " + err.Error()
			return res
		}
	}
	if task.JudgeMode == interactiveMode {
		if interactorPath, err = s.prepareTool(ws, task.InteractorSource, "interactor"); err != nil {
			res.Status = "SE"
			res.Error = "interactor: " + err.Error()
			return res
		}
	}

	s.judgeCases(ctx, res, ws, lang, task, checkerPath, interactorPath)
	aggregate(res)
	return res
}

// compileSubmission runs the language's compile step and fills CE details
// into res; it reports whether judging should continue.
func (s *Service) compileSubmission(res *Result, ws string, lang *Language, task *Task) bool {
	spec := &sandbox.Spec{
		Argv:         lang.Compile.Argv,
		Env:          sandboxEnv(),
		Workspace:    ws,
		StderrPath:   filepath.Join(ws, "compile_err.txt"),
		TimeLimitMS:  lang.Compile.TimeoutS * 1000,
		WallLimitMS:  lang.Compile.TimeoutS*1000 + 500,
		MemLimitKB:   lang.Compile.MemMB * 1024,
		FSizeLimitKB: 64 << 10,
		StackLimitKB: 512 << 10,
		PidsLimit:    256,
		Profile:      sandbox.ProfileCompile,
	}
	sr, err := s.Runner.Run(spec, nil)
	if err != nil {
		res.Status, res.Error = "SE", err.Error()
		return false
	}
	res.CompileMessage = tailFile(filepath.Join(ws, "compile_err.txt"), maxStderrTail)
	if sr.Status != sandbox.StatusOK {
		// why log: empty CE messages are undebugable without the sandbox view;
		// this line pairs with the kept workspace for post-mortem.
		fmt.Fprintf(os.Stderr, "[judge] task %d compile status=%s msg=%q wall=%dms\n",
			task.SubmissionID, sr.Status, res.CompileMessage, sr.WallMS)
		res.Status = "CE"
		if sr.Status == sandbox.StatusSE {
			res.Status = "SE"
			res.Error = sr.Message
		}
		return false
	}
	return true
}

// prepareTool compiles checker/interactor sources once per unique source and
// stages the cached binary into the current workspace.
func (s *Service) prepareTool(ws string, source []byte, name string) (string, error) {
	if len(source) == 0 {
		return "", fmt.Errorf("%s source is empty", name)
	}
	sum := sha256.Sum256(source)
	key := hex.EncodeToString(sum[:])
	cached, ok := s.Cache.Get(key)
	if !ok {
		tws, err := os.MkdirTemp(s.WorkRoot, "tool-*")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(tws)
		if err := chownWorkspace(tws); err != nil {
			return "", err
		}
		src := filepath.Join(tws, name+".cpp")
		if err := os.WriteFile(src, source, 0o644); err != nil {
			return "", err
		}
		spec := &sandbox.Spec{
			Argv:         []string{"g++", "-O2", "-std=c++17", "-o", name, name + ".cpp"},
			Env:          sandboxEnv(),
			Workspace:    tws,
			StderrPath:   filepath.Join(tws, "cerr.txt"),
			TimeLimitMS:  60_000,
			WallLimitMS:  60_500,
			MemLimitKB:   1024 * 1024,
			FSizeLimitKB: 64 << 10,
			StackLimitKB: 512 << 10,
			PidsLimit:    256,
			Profile:      sandbox.ProfileCompile,
		}
		sr, err := s.Runner.Run(spec, nil)
		if err != nil {
			return "", err
		}
		if sr.Status != sandbox.StatusOK {
			return "", fmt.Errorf("%s compile failed: %s", name, tailFile(filepath.Join(tws, "cerr.txt"), maxStderrTail))
		}
		cached, err = s.Cache.Put(key, filepath.Join(tws, name))
		if err != nil {
			return "", err
		}
	}
	dst := filepath.Join(ws, name)
	if err := copyFile(cached, dst); err != nil {
		return "", err
	}
	if err := os.Chmod(dst, 0o755); err != nil {
		return "", err
	}
	return dst, nil
}

var memPlaceholder = regexp.MustCompile(`\{MEM\}`)

// runSpec builds the per-case run spec from the language profile and the
// problem limits (multipliers account for runtime startup overhead).
func (s *Service) runSpec(ws string, lang *Language, task *Task, stderrName string) *sandbox.Spec {
	mult := lang.Run.TimeMultiplier
	if mult <= 0 {
		mult = 1
	}
	timeMS := int64(math.Max(100, math.Min(maxTimeMS, float64(task.TimeLimitMS)*mult)))
	memKB := (task.MemLimitMB + lang.Run.MemOverheadMB) * 1024
	stackKB := lang.Run.StackKB
	if stackKB <= 0 {
		stackKB = 256 << 10
	}
	argv := make([]string, len(lang.Run.Argv))
	for i, a := range lang.Run.Argv {
		argv[i] = memPlaceholder.ReplaceAllString(a, fmt.Sprint(task.MemLimitMB))
	}
	var asKB int64
	if lang.Run.AsLimited {
		asKB = memKB
	}
	return &sandbox.Spec{
		Argv:         argv,
		Env:          sandboxEnv(),
		Workspace:    ws,
		TimeLimitMS:  timeMS,
		WallLimitMS:  timeMS + 300,
		MemLimitKB:   memKB,
		FSizeLimitKB: maxOutputBytes >> 10,
		StackLimitKB: stackKB,
		AsLimitKB:    asKB,
		PidsLimit:    512,
		Profile:      sandbox.ProfileRun,
		StderrPath:   filepath.Join(ws, stderrName),
	}
}

func sandboxEnv() []string {
	return []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=/tmp",
		"TMPDIR=/tmp",
		"LANG=C.UTF-8",
	}
}

// tailFile returns the last maxBytes bytes of a file, best-effort.
func tailFile(path string, maxBytes int64) string {
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		return ""
	}
	if int64(len(raw)) > maxBytes {
		raw = raw[len(raw)-int(maxBytes):]
	}
	return strings.TrimSpace(string(raw))
}

// aggregate collapses per-case results into the submission verdict: any SE
// (infrastructure) wins; otherwise the first non-AC case in case order
// defines the status — the ACM convention; otherwise AC with worst-case
// resource usage. Score sums per-case credit (IOI partial scoring): a case
// earns its assigned score only on AC; SKIPPED cases earn nothing. The
// status stays verdict-based (full-credit-or-not is expressed by Score) so
// existing status vocabulary keeps meaning in both modes.
func aggregate(res *Result) {
	sorted := make([]CaseResult, len(res.Cases))
	copy(sorted, res.Cases)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Index < sorted[j].Index })
	status := "AC"
	score := 0
	for _, c := range sorted {
		if c.Status == "SE" {
			status = "SE"
			break
		}
		// SKIPPED only trails a real failure (stop-on-fail), never sets it
		if status == "AC" && c.Status != "AC" && c.Status != "SKIPPED" {
			status = c.Status
		}
		if c.Status == "AC" {
			score += c.Score
		}
	}
	res.Status = status
	res.Score = score
	for _, c := range res.Cases {
		if c.TimeMS > res.TimeMS {
			res.TimeMS = c.TimeMS
		}
		if c.MemKB > res.MemKB {
			res.MemKB = c.MemKB
		}
	}
}
