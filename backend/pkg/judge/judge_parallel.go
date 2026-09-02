package judge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// judgeCases evaluates every test case. Cases run in parallel, each in its
// own per-case workspace (parallel runs sharing one workspace would clobber
// input.txt/output.txt); results are aggregated in case order. No early
// stop — richer feedback for training (contest stop-on-fail arrives via the
// scheduler setting task.StopOnFail).
//
// Concurrency budget: ALL concurrent sandbox runs on a host — task slots ×
// case workers — share its RAM (each run = a Go supervisor + tmpfs mounts +
// page tables). Oversubscribing it produces kernel-OOM kills of the
// supervisor mid-run, which surface as phantom RE/SE verdicts (verified on
// a 3.9GB box: 2 tasks × 2 cases × ~330MB cgroups + compile sandbox was
// enough). CaseWorkers therefore defaults to 1 per task slot (cases of the
// two slots interleave into ~2 concurrent runs — same total as before
// parallelism, but long-tail submissions stop straggling behind one slow
// case), and operators can raise it on RAM-fatter hosts.
func (s *Service) judgeCases(ctx context.Context, res *Result, ws string, lang *Language,
	task *Task, checkerPath, interactorPath string) {
	if len(task.Cases) == 0 {
		return
	}
	if len(task.Cases) > s.MaxCases {
		task.Cases = task.Cases[:s.MaxCases]
	}

	workers := s.CaseWorkers
	if workers <= 0 {
		workers = 1
	}
	if workers > len(task.Cases) {
		workers = len(task.Cases)
	}
	if workers == 1 || task.StopOnFail {
		// serial path: single workspace, zero staging overhead, and the
		// only correct path for stop-on-fail (waive parallel speedup to get
		// first-failure determinism and skipped-case reporting).
		for _, cr := range task.Cases {
			select {
			case <-ctx.Done():
				res.Status = "SE"
				res.Error = "judge canceled"
				return
			default:
			}
			if task.StopOnFail && caseFailed(res) {
				res.Cases = append(res.Cases, CaseResult{Index: cr.Index, Status: "SKIPPED"})
				continue
			}
			res.Cases = append(res.Cases, s.judgeOneCase(ctx, ws, lang, task, cr, checkerPath, interactorPath))
		}
		return
	}

	// parallel path: stage the compiled artifacts into per-case workspaces.
	// Copies (not hardlinks): workspace teardown is RemoveAll and the
	// compile cache's hardlinked file must never be unlink-affected from a
	// case dir.
	if err := s.stageCaseWorkspaces(ws, lang, task, checkerPath, interactorPath, len(task.Cases)); err != nil {
		res.Status, res.Error = "SE", err.Error()
		return
	}

	results := make([]CaseResult, len(task.Cases))
	idxCh := make(chan int)
	var wg sync.WaitGroup
	var once sync.Once
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range idxCh {
				cr := task.Cases[i]
				cws := filepath.Join(ws, "cases", fmt.Sprint(cr.Index))
				out := s.judgeOneCase(cancelCtx, cws, lang, task, cr, checkerPath, interactorPath)
				results[i] = out
				if out.Status == "SE" {
					// infrastructure fault: cancel sibling workers; the
					// whole submission is already doomed to SE.
					once.Do(cancel)
				}
			}
		}()
	}
	for i := range task.Cases {
		idxCh <- i
	}
	close(idxCh)
	wg.Wait()

	// preserve case order in the reported list
	for _, r := range results {
		if r.Status != "" {
			res.Cases = append(res.Cases, r)
		}
	}
	sort.SliceStable(res.Cases, func(i, j int) bool { return res.Cases[i].Index < res.Cases[j].Index })
}

// stageCaseWorkspaces creates per-case dirs under ws/cases/<idx> and copies
// the compiled program (and checker/interactor) into each, so every sandbox
// run has an isolated cwd. Returns after all dirs are ready.
func (s *Service) stageCaseWorkspaces(ws string, lang *Language, task *Task,
	checkerPath, interactorPath string, n int) error {
	bins := make([]string, 0, 3)
	if lang.Compile != nil {
		bins = append(bins, "./"+compiledName(lang))
	} else {
		bins = append(bins, lang.SourceFile)
	}
	if task.JudgeMode == spjMode && checkerPath != "" {
		bins = append(bins, "checker")
	}
	if task.JudgeMode == interactiveMode && interactorPath != "" {
		bins = append(bins, "interactor")
	}

	casesDir := filepath.Join(ws, "cases")
	if err := os.MkdirAll(casesDir, 0o755); err != nil {
		return fmt.Errorf("cases dir: %w", err)
	}
	for _, cr := range task.Cases {
		cws := filepath.Join(casesDir, fmt.Sprint(cr.Index))
		if err := os.MkdirAll(cws, 0o755); err != nil {
			return fmt.Errorf("case %d dir: %w", cr.Index, err)
		}
		if err := chownWorkspace(cws); err != nil {
			return fmt.Errorf("case %d chown: %w", cr.Index, err)
		}
		for _, b := range bins {
			src := filepath.Join(ws, b)
			dst := filepath.Join(cws, b)
			if err := copyFileIfExist(src, dst); err != nil {
				return fmt.Errorf("case %d stage %s: %w", cr.Index, b, err)
			}
		}
	}
	return nil
}

// judgeOneCase judges one case inside cws (its own workspace for parallel
// runs; the submission workspace for the serial path).
func (s *Service) judgeOneCase(ctx context.Context, cws string, lang *Language, task *Task,
	cr CaseRef, checkerPath, interactorPath string) CaseResult {
	out := CaseResult{Index: cr.Index, Score: cr.Score}
	input, err := s.blob(ctx, task, cr.InputSHA, cr.InputSize, cr.Index, "in")
	if err != nil {
		out.Status, out.Message = "SE", "fetch input: "+err.Error()
		return out
	}
	if err := os.WriteFile(filepath.Join(cws, "input.txt"), input, 0o644); err != nil {
		out.Status, out.Message = "SE", "write input: "+err.Error()
		return out
	}

	switch task.JudgeMode {
	case interactiveMode:
		return s.judgeInteractive(cws, lang, task, out)
	case spjMode:
		answer, err := s.blob(ctx, task, cr.AnswerSHA, cr.AnswerSize, cr.Index, "out")
		if err == nil {
			_ = os.WriteFile(filepath.Join(cws, "answer.txt"), answer, 0o644)
		} // SPJ may not need the answer; checker decides
		out = s.runPlain(ctx, cws, lang, task, out)
		if out.Status == "AC" || out.Status == "WA" {
			return s.runChecker(cws, checkerPath, out)
		}
		return out
	default:
		answer, err := s.blob(ctx, task, cr.AnswerSHA, cr.AnswerSize, cr.Index, "out")
		if err != nil {
			out.Status, out.Message = "SE", "fetch answer: "+err.Error()
			return out
		}
		if err := os.WriteFile(filepath.Join(cws, "answer.txt"), answer, 0o644); err != nil {
			out.Status, out.Message = "SE", "write answer: "+err.Error()
			return out
		}
		out = s.runPlain(ctx, cws, lang, task, out)
		if out.Status != "AC" {
			return out
		}
		return s.diffCase(cws, out)
	}
}

func caseFailed(res *Result) bool {
	for i := len(res.Cases) - 1; i >= 0; i-- {
		if res.Cases[i].Status != "AC" && res.Cases[i].Status != "SKIPPED" {
			return true
		}
	}
	return false
}

// compiledName derives the binary filename the compile step produces
// ("g++ -o main" → main).
func compiledName(lang *Language) string {
	for i, a := range lang.Compile.Argv {
		if a == "-o" && i+1 < len(lang.Compile.Argv) {
			return lang.Compile.Argv[i+1]
		}
	}
	return "main"
}

func copyFileIfExist(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, st.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	buf := make([]byte, 1<<20)
	for {
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if rerr != nil {
			if rerr.Error() == "EOF" {
				return nil
			}
			return rerr
		}
	}
}
