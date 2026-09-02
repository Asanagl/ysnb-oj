package judge

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ysnb/oj/pkg/sandbox"
)

// judgeCases lives in judge_parallel.go (parallel + stop-on-fail paths).

func (s *Service) judgeOne(ctx context.Context, ws string, lang *Language, task *Task,
	cr CaseRef, checkerPath, interactorPath string) CaseResult {
	out := CaseResult{Index: cr.Index, Score: cr.Score}
	input, err := s.blob(ctx, task, cr.InputSHA, cr.InputSize, cr.Index, "in")
	if err != nil {
		out.Status, out.Message = "SE", "fetch input: "+err.Error()
		return out
	}
	if err := os.WriteFile(filepath.Join(ws, "input.txt"), input, 0o644); err != nil {
		out.Status, out.Message = "SE", "write input: "+err.Error()
		return out
	}

	switch task.JudgeMode {
	case interactiveMode:
		return s.judgeInteractive(ws, lang, task, out)
	case spjMode:
		answer, err := s.blob(ctx, task, cr.AnswerSHA, cr.AnswerSize, cr.Index, "out")
		if err == nil {
			_ = os.WriteFile(filepath.Join(ws, "answer.txt"), answer, 0o644)
		} // SPJ may not need the answer; checker decides
		out = s.runPlain(ctx, ws, lang, task, out)
		if out.Status == "AC" || out.Status == "WA" {
			return s.runChecker(ws, checkerPath, out)
		}
		return out
	default:
		answer, err := s.blob(ctx, task, cr.AnswerSHA, cr.AnswerSize, cr.Index, "out")
		if err != nil {
			out.Status, out.Message = "SE", "fetch answer: "+err.Error()
			return out
		}
		if err := os.WriteFile(filepath.Join(ws, "answer.txt"), answer, 0o644); err != nil {
			out.Status, out.Message = "SE", "write answer: "+err.Error()
			return out
		}
		out = s.runPlain(ctx, ws, lang, task, out)
		if out.Status != "AC" {
			return out
		}
		return s.diffCase(ws, out)
	}
}

// runPlain executes the user program once against input.txt with file
// redirection and reports the sandbox-classified status.
func (s *Service) runPlain(ctx context.Context, ws string, lang *Language, task *Task, out CaseResult) CaseResult {
	spec := s.runSpec(ws, lang, task, "stderr.txt")
	spec.StdinPath = filepath.Join(ws, "input.txt")
	spec.StdoutPath = filepath.Join(ws, "output.txt")
	sr, err := s.Runner.Run(spec, nil)
	if err != nil {
		out.Status, out.Message = "SE", err.Error()
		return out
	}
	out.TimeMS, out.MemKB = sr.CPUTimeMS, sr.MaxRSSKB
	if sr.Status != sandbox.StatusOK {
		// why log: parallel-case REs with intact output were observed
		// (exit 2, empty stderr); the sandbox Result carries the missing
		// exit/signal detail the API-facing CaseResult drops.
		fmt.Fprintf(os.Stderr, "[judge] run ws=%s status=%s exit=%d sig=%d msg=%q wall=%dms\n",
			filepath.Base(ws), sr.Status, sr.ExitCode, sr.Signal, sr.Message, sr.WallMS)
	}
	switch sr.Status {
	case sandbox.StatusOK:
		out.Status = "AC"
	case sandbox.StatusTLE:
		out.Status, out.Message = "TLE", "time limit exceeded"
	case sandbox.StatusMLE:
		out.Status, out.Message = "MLE", "memory limit exceeded"
	case sandbox.StatusRE:
		out.Status = "RE"
		out.Message = strings.TrimSpace(sr.Message)
	case sandbox.StatusSE:
		out.Status, out.Message = "SE", sr.Message
	}
	return out
}

// diffCase compares raw bytes first, then whitespace-normalized token
// sequences (Codeforces-style default checker semantics).
func (s *Service) diffCase(ws string, out CaseResult) CaseResult {
	rawOut, err1 := os.ReadFile(filepath.Join(ws, "output.txt"))
	rawAns, err2 := os.ReadFile(filepath.Join(ws, "answer.txt"))
	if err1 != nil || err2 != nil {
		out.Status, out.Message = "SE", "read program output"
		return out
	}
	if bytes.Equal(rawOut, rawAns) || equalTokens(rawOut, rawAns) {
		return out
	}
	out.Status, out.Message = "WA", "output mismatch"
	return out
}

func equalTokens(a, b []byte) bool {
	// why streaming instead of strings.Fields: the naive path materialized
	// two token slices per case (Fields allocates a slice of strings plus a
	// joined copy) — on 50-case submissions with megabyte outputs that is
	// pure allocator pressure. This scan does zero allocations and short-
	// circuits on the first differing token.
	i, j := 0, 0
	for {
		// skip whitespace runs
		for i < len(a) && isSpaceByte(a[i]) {
			i++
		}
		for j < len(b) && isSpaceByte(b[j]) {
			j++
		}
		if i == len(a) || j == len(b) {
			return i == len(a) && j == len(b)
		}
		// compare one token
		for i < len(a) && j < len(b) && !isSpaceByte(a[i]) && !isSpaceByte(b[j]) {
			if a[i] != b[j] {
				return false
			}
			i++
			j++
		}
		// token ended on exactly one side (or whitespace on both — loop continues)
		if (i < len(a) && !isSpaceByte(a[i])) || (j < len(b) && !isSpaceByte(b[j])) {
			return false
		}
	}
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

// judgeInteractive pipes the user program to the interactor inside two
// sandboxes. Interactor exit convention (docs/interactive.md): 0 = accepted,
// 1 = wrong answer, anything else = judge-side fault (SE).
func (s *Service) judgeInteractive(ws string, lang *Language,
	task *Task, out CaseResult) CaseResult {
	userSpec := s.runSpec(ws, lang, task, "stderr.txt")
	interSpec := &sandbox.Spec{
		Argv:         []string{"./interactor", "input.txt"},
		Env:          sandboxEnv(),
		Workspace:    ws,
		TimeLimitMS:  checkerTimeMS,
		WallLimitMS:  checkerTimeMS + 500,
		MemLimitKB:   checkerMemMB * 1024,
		FSizeLimitKB: 64 << 10,
		StackLimitKB: 256 << 10,
		PidsLimit:    256,
		Profile:      sandbox.ProfileRun,
	}
	ifErr := filepath.Join(ws, "interactor_err.txt")
	ifh, err := os.OpenFile(ifErr, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		out.Status, out.Message = "SE", "open interactor stderr: "+err.Error()
		return out
	}
	defer ifh.Close()

	userRes, interRes, err := s.Runner.RunPair(userSpec, interSpec, ifh)
	if err != nil {
		out.Status, out.Message = "SE", err.Error()
		return out
	}
	out.TimeMS, out.MemKB = userRes.CPUTimeMS, userRes.MaxRSSKB
	interMsg := tailFile(ifErr, maxStderrTail)
	switch {
	case interRes.Status == sandbox.StatusSE:
		out.Status, out.Message = "SE", "interactor: "+interRes.Message
	case interRes.ExitCode == 0 && userRes.Status == sandbox.StatusOK:
		out.Status = "AC"
	case interRes.ExitCode == 1:
		out.Status = "WA"
		if interMsg != "" {
			out.Message = interMsg
		}
	case interRes.ExitCode == 0:
		// interactor accepted but user program misbehaved on its own
		out.Status = mapSandboxStatus(userRes.Status)
		if out.Message == "" {
			out.Message = strings.TrimSpace(userRes.Message)
		}
	default:
		out.Status, out.Message = "SE", fmt.Sprintf("interactor exit %d", interRes.ExitCode)
	}
	return out
}

// runChecker delegates the verdict to the problem's checker binary
// (testlib convention: `checker <input> <answer> <output>`, exit 0 = AC).
func (s *Service) runChecker(ws, checkerPath string, out CaseResult) CaseResult {
	spec := &sandbox.Spec{
		Argv:         []string{"./checker", "input.txt", "answer.txt", "output.txt"},
		Env:          sandboxEnv(),
		Workspace:    ws,
		TimeLimitMS:  checkerTimeMS,
		WallLimitMS:  checkerTimeMS + 500,
		MemLimitKB:   checkerMemMB * 1024,
		FSizeLimitKB: 64 << 10,
		StackLimitKB: 256 << 10,
		PidsLimit:    256,
		Profile:      sandbox.ProfileRun,
		StderrPath:   filepath.Join(ws, "checker_err.txt"),
	}
	sr, err := s.Runner.Run(spec, nil)
	if err != nil {
		out.Status, out.Message = "SE", err.Error()
		return out
	}
	if sr.Status == sandbox.StatusSE {
		out.Status, out.Message = "SE", "checker: "+sr.Message
		return out
	}
	if sr.ExitCode == 0 {
		return out // keep AC from runPlain
	}
	out.Status = "WA"
	if msg := tailFile(filepath.Join(ws, "checker_err.txt"), maxStderrTail); msg != "" {
		out.Message = firstLine(msg)
	} else {
		out.Message = fmt.Sprintf("checker exit %d", sr.ExitCode)
	}
	return out
}

func mapSandboxStatus(st string) string {
	switch st {
	case sandbox.StatusOK:
		return "AC"
	case sandbox.StatusTLE:
		return "TLE"
	case sandbox.StatusMLE:
		return "MLE"
	case sandbox.StatusRE:
		return "RE"
	default:
		return "SE"
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// blob fetches one testdata file into the local cache (by sha256) and
// returns its bytes.
func (s *Service) blob(ctx context.Context, task *Task, sha string, size int64, index int, kind string) ([]byte, error) {
	if sha == "" {
		return nil, fmt.Errorf("empty sha for case %d %s", index, kind)
	}
	return s.HTTP.fetchBlob(ctx, s.FetchBase, s.FetchTok, s.BlobDir,
		task.ProblemID, index, kind, sha, size)
}
