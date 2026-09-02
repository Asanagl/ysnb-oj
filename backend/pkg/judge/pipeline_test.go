package judge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/ysnb/oj/pkg/sandbox"
)

// The judging sandbox is Linux-only; Service.Run calls chownWorkspace which
// uses os.Chown — a syscall that always fails on Windows (EWINDOWS). To keep
// the orchestration under test on Windows, runPipeline drives the exact same
// internal stages as Service.Run (registry lookup, compileSubmission,
// prepareTool, judgeCases, aggregate) while skipping only that chown and
// pre-seeding the tool cache so prepareTool takes its cache-hit path
// (which does no chown). On Linux the same flow is reachable via Service.Run.
func runPipeline(ctx context.Context, s *Service, task *Task) *Result {
	res := &Result{SubmissionID: task.SubmissionID}
	lang, err := s.Reg.Get(task.LanguageID)
	if err != nil {
		res.Status, res.Error = "SE", err.Error()
		return res
	}
	ws, err := os.MkdirTemp(s.WorkRoot, "judge-*")
	if err != nil {
		res.Status, res.Error = "SE", "workspace: "+err.Error()
		return res
	}
	defer os.RemoveAll(ws)
	if lang.Compile != nil {
		if !s.compileSubmission(res, ws, lang, task) {
			return res
		}
	}
	var checkerPath, interactorPath string
	if task.JudgeMode == spjMode {
		if checkerPath, err = prepareToolSeeded(s, ws, task.CheckerSource, "checker"); err != nil {
			res.Status, res.Error = "SE", "checker: "+err.Error()
			return res
		}
	}
	if task.JudgeMode == interactiveMode {
		if interactorPath, err = prepareToolSeeded(s, ws, task.InteractorSource, "interactor"); err != nil {
			res.Status, res.Error = "SE", "interactor: "+err.Error()
			return res
		}
	}
	s.judgeCases(ctx, res, ws, lang, task, checkerPath, interactorPath)
	aggregate(res)
	return res
}

// prepareToolSeeded stages a tool binary by pre-filling the BinCache with the
// artifact for source's sha, so prepareTool skips its compile-then-chown path.
func prepareToolSeeded(s *Service, ws string, source []byte, name string) (string, error) {
	sum := sha256.Sum256(source)
	key := hex.EncodeToString(sum[:])
	stub := filepath.Join(s.Cache.Root, key+".artifact")
	if err := os.WriteFile(stub, []byte("mock-tool-elf"), 0o755); err != nil {
		return "", err
	}
	if _, err := s.Cache.Put(key, stub); err != nil {
		return "", err
	}
	return s.prepareTool(ws, source, name)
}

// mockRunner is a programmable sandbox stand-in. Compile specs either fail
// (CompileStatus != "") with a stderr stub or succeed and materialize the
// artifact named by the -o flag. Non-compile specs go to RunFn; paired runs
// go to PairFn. All observed specs are recorded for assertions.
type mockRunner struct {
	CompileStatus string // "" or OK => success; any other sandbox status => compile failure
	CompileStderr string // bytes written to the compile stderr file on failure
	RunFn         func(spec *sandbox.Spec) (*sandbox.Result, error)
	PairFn        func(userSpec, interSpec *sandbox.Spec, stderr *os.File) (*sandbox.Result, *sandbox.Result, error)

	mu    sync.Mutex
	Specs []*sandbox.Spec
}

func (m *mockRunner) record(spec *sandbox.Spec) {
	m.mu.Lock()
	m.Specs = append(m.Specs, spec)
	m.mu.Unlock()
}

func (m *mockRunner) Run(spec *sandbox.Spec, files *sandbox.Files) (*sandbox.Result, error) {
	m.record(spec)
	if spec.Profile == sandbox.ProfileCompile {
		if m.CompileStatus != "" && m.CompileStatus != sandbox.StatusOK {
			if spec.StderrPath != "" && m.CompileStderr != "" {
				_ = os.WriteFile(spec.StderrPath, []byte(m.CompileStderr), 0o644)
			}
			return &sandbox.Result{Status: m.CompileStatus, Message: "mock compile " + m.CompileStatus}, nil
		}
		for i, a := range spec.Argv {
			if a == "-o" && i+1 < len(spec.Argv) {
				_ = os.WriteFile(filepath.Join(spec.Workspace, spec.Argv[i+1]), []byte("mock-elf"), 0o755)
				break
			}
		}
		return &sandbox.Result{Status: sandbox.StatusOK}, nil
	}
	if m.RunFn != nil {
		return m.RunFn(spec)
	}
	return &sandbox.Result{Status: sandbox.StatusOK}, nil
}

func (m *mockRunner) RunPair(userSpec, interSpec *sandbox.Spec, stderr *os.File) (*sandbox.Result, *sandbox.Result, error) {
	m.record(userSpec)
	m.record(interSpec)
	if m.PairFn != nil {
		return m.PairFn(userSpec, interSpec, stderr)
	}
	return &sandbox.Result{Status: sandbox.StatusOK}, &sandbox.Result{Status: sandbox.StatusOK, ExitCode: 0}, nil
}

// checkerRan reports whether any spec invoked the given tool (e.g. ./checker).
func (m *mockRunner) sawArgv0(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, sp := range m.Specs {
		if len(sp.Argv) > 0 && sp.Argv[0] == name {
			return true
		}
	}
	return false
}

func newTestService(t *testing.T, r Runner, fetchBase string) *Service {
	t.Helper()
	reg, err := NewRegistry("")
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	s, err := NewService(r, reg, t.TempDir(), t.TempDir(), fetchBase, "daemon-token-e2e")
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	return s
}

// fetchServer serves /api/v1/internal/testdata/<pid>/<idx>/<kind> bodies from
// files ("pid/idx/kind" -> body), 404s unknown keys and records Authorization
// headers so tests can assert the daemon token was sent.
type fetchServer struct {
	*httptest.Server
	mu    sync.Mutex
	files map[string]string
	auth  []string
}

const fetchPrefix = "/api/v1/internal/testdata/"

func newFetchServer(t *testing.T, files map[string]string) *fetchServer {
	t.Helper()
	fs := &fetchServer{files: files}
	fs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.mu.Lock()
		fs.auth = append(fs.auth, r.Header.Get("Authorization"))
		fs.mu.Unlock()
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, fetchPrefix), "/")
		if len(parts) != 3 {
			http.NotFound(w, r)
			return
		}
		body, ok := fs.files[strings.Join(parts, "/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(fs.Close)
	return fs
}

func shaOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func caseRef(idx int, in, out string) CaseRef {
	cr := CaseRef{Index: idx, InputSHA: shaOf(in), InputSize: int64(len(in))}
	if out != "" {
		cr.AnswerSHA = shaOf(out)
		cr.AnswerSize = int64(len(out))
	}
	return cr
}

func plainTask(problemID uint64, cases []CaseRef) *Task {
	return &Task{
		SubmissionID: 42, ProblemID: problemID, LanguageID: "cpp",
		Code: []byte("int main(){}"), TimeLimitMS: 1000, MemLimitMB: 256,
		JudgeMode: "plain", Cases: cases,
	}
}

func TestPipelineCompileFailureCE(t *testing.T) {
	r := &mockRunner{CompileStatus: sandbox.StatusRE, CompileStderr: "main.cpp:1:15: error: expected ';' before 'return'\n"}
	s := newTestService(t, r, "")
	res := runPipeline(context.Background(), s, plainTask(1, []CaseRef{caseRef(0, "1\n", "1\n")}))
	if res.Status != "CE" {
		t.Fatalf("status = %s, want CE (res=%+v)", res.Status, res)
	}
	if !strings.Contains(res.CompileMessage, "error: expected ';'") {
		t.Fatalf("compile message = %q, want compiler stderr", res.CompileMessage)
	}
	if len(res.Cases) != 0 {
		t.Fatalf("no case should run after CE, got %d", len(res.Cases))
	}
}

func TestPipelineCompileSE(t *testing.T) {
	// sandbox-side SE (returned status) must surface as SE, not CE
	r := &mockRunner{CompileStatus: sandbox.StatusSE}
	s := newTestService(t, r, "")
	res := runPipeline(context.Background(), s, plainTask(1, []CaseRef{caseRef(0, "1\n", "1\n")}))
	if res.Status != "SE" || res.Error == "" {
		t.Fatalf("status = %s err = %q, want SE with error", res.Status, res.Error)
	}
	if res.CompileMessage != "" {
		t.Fatalf("unexpected compile message %q", res.CompileMessage)
	}
	// infrastructure error (returned Go error) must also be SE
	r2 := &mockRunner{RunFn: func(spec *sandbox.Spec) (*sandbox.Result, error) {
		return nil, fmt.Errorf("cgroup setup exploded")
	}}
	s2 := newTestService(t, r2, newFetchServer(t, map[string]string{"1/0/in": "1\n", "1/0/out": "1\n"}).URL)
	res2 := runPipeline(context.Background(), s2, &Task{SubmissionID: 7, ProblemID: 1, LanguageID: "python3", TimeLimitMS: 1000, MemLimitMB: 256, Cases: []CaseRef{caseRef(0, "1\n", "1\n")}})
	if res2.Status != "SE" {
		t.Fatalf("status = %s, want SE propagating runner error", res2.Status)
	}
	// runner-level errors surface on the failing case; res.Error is only for
	// pre-case infrastructure faults (workspace, registry, compile)
	if !strings.Contains(res2.Cases[0].Message, "cgroup setup exploded") {
		t.Fatalf("case message = %q, want runner error", res2.Cases[0].Message)
	}
}

// echoRunner builds a runner whose user program emits outputs per input, with
// per-input CPU/mem readings; checker specs are answered by checkerFn.
func echoRunner(outputs map[string]string, resByInput map[string][2]int64, checkerFn func(spec *sandbox.Spec) (*sandbox.Result, error)) *mockRunner {
	return &mockRunner{RunFn: func(spec *sandbox.Spec) (*sandbox.Result, error) {
		if len(spec.Argv) > 0 && spec.Argv[0] == "./checker" {
			if checkerFn != nil {
				return checkerFn(spec)
			}
		}
		raw, _ := os.ReadFile(spec.StdinPath)
		in := string(raw)
		if out, ok := outputs[in]; ok && spec.StdoutPath != "" {
			if err := os.WriteFile(spec.StdoutPath, []byte(out), 0o644); err != nil {
				return nil, err
			}
		}
		st := &sandbox.Result{Status: sandbox.StatusOK, ExitCode: 0}
		if m, ok := resByInput[in]; ok {
			st.CPUTimeMS, st.MaxRSSKB = m[0], m[1]
		}
		if bad, ok := sandboxByInput[in]; ok {
			st.Status, st.Message = bad.status, bad.message
		}
		return st, nil
	}}
}

type fakeBad struct {
	status  string
	message string
}

var sandboxByInput = map[string]fakeBad{}

func TestPipelineAllACMaxResources(t *testing.T) {
	in1, in2 := "case-one-input\n", "case-two-input\n"
	ans1, ans2 := "answer-one\n", "answer-two\n"
	r := echoRunner(map[string]string{in1: ans1, in2: ans2},
		map[string][2]int64{in1: {10, 1000}, in2: {42, 250}}, nil)
	files := map[string]string{
		"7/1/in": in1, "7/1/out": ans1,
		"7/2/in": in2, "7/2/out": ans2,
	}
	fs := newFetchServer(t, files)
	s := newTestService(t, r, fs.URL)
	task := plainTask(7, []CaseRef{caseRef(1, in1, ans1), caseRef(2, in2, ans2)})
	res := runPipeline(context.Background(), s, task)
	if res.Status != "AC" {
		t.Fatalf("status = %s, want AC (%+v)", res.Status, res)
	}
	if len(res.Cases) != 2 {
		t.Fatalf("cases = %d, want 2", len(res.Cases))
	}
	for _, c := range res.Cases {
		if c.Status != "AC" {
			t.Fatalf("case %d = %s, want AC", c.Index, c.Status)
		}
	}
	if res.TimeMS != 42 || res.MemKB != 1000 {
		t.Fatalf("time=%d mem=%d, want max(10,42)=42 / max(1000,250)=1000", res.TimeMS, res.MemKB)
	}
	// daemon token must have been sent on every blob fetch
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if len(fs.auth) == 0 {
		t.Fatal("no fetch requests observed")
	}
	for _, a := range fs.auth {
		if a != "Bearer daemon-token-e2e" {
			t.Fatalf("authorization = %q, want bearer daemon token", a)
		}
	}
}

func TestPipelineTokenEquivalentAC(t *testing.T) {
	in := "3 4\n"
	ans := "7"
	r := echoRunner(map[string]string{in: "7  \n\n"}, nil, nil) // byte-unequal, token-equal
	fs := newFetchServer(t, map[string]string{"3/1/in": in, "3/1/out": ans})
	s := newTestService(t, r, fs.URL)
	res := runPipeline(context.Background(), s, plainTask(3, []CaseRef{caseRef(1, in, ans)}))
	if res.Status != "AC" {
		t.Fatalf("status = %s, want AC for whitespace-insensitive diff (%+v)", res.Status, res)
	}
	// and CRLF vs space-joined must also pass
	in2, ans2 := "1\r\n2\r\n", "1 2\n"
	r2 := echoRunner(map[string]string{in2: "1\r\n2\r\n"}, nil, nil)
	fs2 := newFetchServer(t, map[string]string{"4/1/in": in2, "4/1/out": ans2})
	s2 := newTestService(t, r2, fs2.URL)
	res2 := runPipeline(context.Background(), s2, plainTask(4, []CaseRef{caseRef(1, in2, ans2)}))
	if res2.Status != "AC" {
		t.Fatalf("status = %s, want AC for CRLF-vs-LF diff (%+v)", res2.Status, res2)
	}
}

func TestPipelineWAAndFirstFailureOrder(t *testing.T) {
	// single WA case => overall WA with default-checker message
	in, ans := "1 2\n", "3\n"
	r := echoRunner(map[string]string{in: "4\n"}, nil, nil)
	fs := newFetchServer(t, map[string]string{"5/1/in": in, "5/1/out": ans})
	s := newTestService(t, r, fs.URL)
	res := runPipeline(context.Background(), s, plainTask(5, []CaseRef{caseRef(1, in, ans)}))
	if res.Status != "WA" {
		t.Fatalf("status = %s, want WA", res.Status)
	}
	if res.Cases[0].Message != "output mismatch" {
		t.Fatalf("message = %q, want output mismatch", res.Cases[0].Message)
	}

	// out-of-order case list: the first failure by case INDEX decides overall.
	inZero, inOne, inTwo := "in-zero\n", "in-one\n", "in-two\n"
	ansAll := "ok\n"
	sandboxByInput[inOne] = fakeBad{sandbox.StatusTLE, "time limit exceeded"}
	defer delete(sandboxByInput, inOne)
	r2 := echoRunner(map[string]string{
		inZero: ansAll,   // AC
		inTwo:  "nope\n", // WA
	}, nil, nil)
	fs2 := newFetchServer(t, map[string]string{
		"6/0/in": inZero, "6/0/out": ansAll,
		"6/1/in": inOne, "6/1/out": ansAll,
		"6/2/in": inTwo, "6/2/out": ansAll,
	})
	s2 := newTestService(t, r2, fs2.URL)
	task := plainTask(6, []CaseRef{caseRef(2, inTwo, ansAll), caseRef(0, inZero, ansAll), caseRef(1, inOne, ansAll)})
	res2 := runPipeline(context.Background(), s2, task)
	// index order: 0=AC, 1=TLE, 2=WA => TLE wins despite being listed second
	if res2.Status != "TLE" {
		t.Fatalf("status = %s, want TLE (first failure by case index)", res2.Status)
	}
	if len(res2.Cases) != 3 {
		t.Fatalf("cases = %d, want 3 (no early stop)", len(res2.Cases))
	}
}

func TestPipelineUserProgramLimits(t *testing.T) {
	cases := []struct {
		name        string
		status      string
		want        string
		wantMessage string
	}{
		{"tle", sandbox.StatusTLE, "TLE", "time limit exceeded"},
		{"mle", sandbox.StatusMLE, "MLE", "memory limit exceeded"},
		{"re", sandbox.StatusRE, "RE", "runtime error: division by zero"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sandboxByInput["limit-in\n"] = fakeBad{tc.status, tc.wantMessage + "\n"}
			defer delete(sandboxByInput, "limit-in\n")
			in, ans := "limit-in\n", "x\n"
			r := echoRunner(nil, nil, nil)
			fs := newFetchServer(t, map[string]string{"9/1/in": in, "9/1/out": ans})
			s := newTestService(t, r, fs.URL)
			res := runPipeline(context.Background(), s, plainTask(9, []CaseRef{caseRef(1, in, ans)}))
			if res.Status != tc.want {
				t.Fatalf("status = %s, want %s", res.Status, tc.want)
			}
			if res.Cases[0].Message != tc.wantMessage {
				t.Fatalf("message = %q, want %q", res.Cases[0].Message, tc.wantMessage)
			}
		})
	}
}

func TestPipelineBlobProblemsSE(t *testing.T) {
	t.Run("sha mismatch", func(t *testing.T) {
		r := echoRunner(nil, nil, nil)
		// server serves honest bytes for 8/1/in, but the task declares a sha
		// of something else — the fetched bytes must be rejected.
		fs := newFetchServer(t, map[string]string{"8/1/in": "actual-bytes\n", "8/1/out": "actual-bytes\n"})
		s := newTestService(t, r, fs.URL)
		cr := caseRef(1, "actual-bytes\n", "actual-bytes\n")
		cr.InputSHA = shaOf("something-else") // declared != served
		res := runPipeline(context.Background(), s, plainTask(8, []CaseRef{cr}))
		if res.Status != "SE" {
			t.Fatalf("status = %s, want SE on sha mismatch", res.Status)
		}
		if !strings.Contains(res.Cases[0].Message, "sha mismatch") {
			t.Fatalf("message = %q, want sha mismatch", res.Cases[0].Message)
		}
	})
	t.Run("fetch 404", func(t *testing.T) {
		r := echoRunner(nil, nil, nil)
		fs := newFetchServer(t, map[string]string{}) // nothing served
		s := newTestService(t, r, fs.URL)
		res := runPipeline(context.Background(), s, plainTask(8, []CaseRef{caseRef(1, "x\n", "y\n")}))
		if res.Status != "SE" {
			t.Fatalf("status = %s, want SE on 404", res.Status)
		}
		if !strings.Contains(res.Cases[0].Message, "status 404") {
			t.Fatalf("message = %q, want 404 detail", res.Cases[0].Message)
		}
	})
}

func TestPipelineSPJ(t *testing.T) {
	in, ans := "1 2\n", "3\n"
	base := map[string]string{"11/1/in": in, "11/1/out": ans}
	checkerErr := func(spec *sandbox.Spec, msg string) {
		if msg != "" && spec.StderrPath != "" {
			_ = os.WriteFile(spec.StderrPath, []byte(msg), 0o644)
		}
	}
	mkRunner := func(checkerExit int, checkerStatus, msg string) *mockRunner {
		return &mockRunner{RunFn: func(spec *sandbox.Spec) (*sandbox.Result, error) {
			if spec.Argv[0] == "./checker" {
				checkerErr(spec, msg)
				return &sandbox.Result{Status: checkerStatus, ExitCode: checkerExit}, nil
			}
			return &sandbox.Result{Status: sandbox.StatusOK, ExitCode: 0}, nil
		}}
	}
	mkTask := func() *Task {
		task := plainTask(11, []CaseRef{caseRef(1, in, ans)})
		task.JudgeMode = spjMode
		task.CheckerSource = []byte("int main(){return 0;}")
		return task
	}

	t.Run("checker exit 0 keeps AC", func(t *testing.T) {
		r := mkRunner(0, sandbox.StatusOK, "")
		s := newTestService(t, r, newFetchServer(t, base).URL)
		res := runPipeline(context.Background(), s, mkTask())
		if res.Status != "AC" {
			t.Fatalf("status = %s, want AC", res.Status)
		}
		if !r.sawArgv0("./checker") {
			t.Fatal("checker was never invoked")
		}
	})
	t.Run("checker exit 1 is WA with stderr message", func(t *testing.T) {
		r := mkRunner(1, sandbox.StatusOK, "wrong answer: expected 3 got 4\nsecond line ignored\n")
		s := newTestService(t, r, newFetchServer(t, base).URL)
		res := runPipeline(context.Background(), s, mkTask())
		if res.Status != "WA" {
			t.Fatalf("status = %s, want WA", res.Status)
		}
		if res.Cases[0].Message != "wrong answer: expected 3 got 4" {
			t.Fatalf("message = %q, want first stderr line", res.Cases[0].Message)
		}
	})
	t.Run("checker exit 1 without stderr falls back to exit code", func(t *testing.T) {
		r := mkRunner(3, sandbox.StatusOK, "")
		s := newTestService(t, r, newFetchServer(t, base).URL)
		res := runPipeline(context.Background(), s, mkTask())
		if res.Status != "WA" || res.Cases[0].Message != "checker exit 3" {
			t.Fatalf("status=%s msg=%q, want WA / checker exit 3", res.Status, res.Cases[0].Message)
		}
	})
	t.Run("checker SE is SE", func(t *testing.T) {
		r := mkRunner(0, sandbox.StatusSE, "")
		s := newTestService(t, r, newFetchServer(t, base).URL)
		res := runPipeline(context.Background(), s, mkTask())
		if res.Status != "SE" || !strings.HasPrefix(res.Cases[0].Message, "checker: ") {
			t.Fatalf("status=%s msg=%q, want SE with checker prefix", res.Status, res.Cases[0].Message)
		}
	})
	t.Run("user RE skips checker", func(t *testing.T) {
		r := echoRunner(nil, nil, func(spec *sandbox.Spec) (*sandbox.Result, error) {
			return &sandbox.Result{Status: sandbox.StatusOK, ExitCode: 1}, nil // must not be reached
		})
		sandboxByInput[in] = fakeBad{sandbox.StatusRE, "segfault"}
		defer delete(sandboxByInput, in)
		s := newTestService(t, r, newFetchServer(t, base).URL)
		res := runPipeline(context.Background(), s, mkTask())
		if res.Status != "RE" {
			t.Fatalf("status = %s, want RE without checker", res.Status)
		}
		if r.sawArgv0("./checker") {
			t.Fatal("checker must not run for non-AC/WA user result")
		}
	})
}

func TestPipelineInteractive(t *testing.T) {
	in := "start\n"
	base := map[string]string{"12/1/in": in}
	mkTask := func() *Task {
		task := plainTask(12, []CaseRef{caseRef(1, in, "")})
		task.JudgeMode = interactiveMode
		task.InteractorSource = []byte("int main(){return 0;}")
		return task
	}
	mkRunner := func(userRes *sandbox.Result, interRes *sandbox.Result, stderrMsg string) *mockRunner {
		return &mockRunner{PairFn: func(userSpec, interSpec *sandbox.Spec, stderr *os.File) (*sandbox.Result, *sandbox.Result, error) {
			if stderrMsg != "" {
				_, _ = stderr.WriteString(stderrMsg)
			}
			return userRes, interRes, nil
		}}
	}

	t.Run("interactor 0 user OK is AC", func(t *testing.T) {
		r := mkRunner(&sandbox.Result{Status: sandbox.StatusOK, CPUTimeMS: 12, MaxRSSKB: 3400}, &sandbox.Result{Status: sandbox.StatusOK, ExitCode: 0}, "")
		s := newTestService(t, r, newFetchServer(t, base).URL)
		res := runPipeline(context.Background(), s, mkTask())
		if res.Status != "AC" {
			t.Fatalf("status = %s, want AC", res.Status)
		}
		if res.TimeMS != 12 || res.MemKB != 3400 {
			t.Fatalf("time=%d mem=%d, want user program readings", res.TimeMS, res.MemKB)
		}
		if !r.sawArgv0("./interactor") {
			t.Fatal("interactor was never invoked via RunPair")
		}
	})
	t.Run("interactor 1 is WA with stderr message", func(t *testing.T) {
		r := mkRunner(&sandbox.Result{Status: sandbox.StatusOK}, &sandbox.Result{Status: sandbox.StatusOK, ExitCode: 1}, "wa: protocol violation on move 5\n")
		s := newTestService(t, r, newFetchServer(t, base).URL)
		res := runPipeline(context.Background(), s, mkTask())
		if res.Status != "WA" || res.Cases[0].Message != "wa: protocol violation on move 5" {
			t.Fatalf("status=%s msg=%q, want WA with interactor stderr", res.Status, res.Cases[0].Message)
		}
	})
	t.Run("interactor exit 2 is SE", func(t *testing.T) {
		r := mkRunner(&sandbox.Result{Status: sandbox.StatusOK}, &sandbox.Result{Status: sandbox.StatusOK, ExitCode: 2}, "")
		s := newTestService(t, r, newFetchServer(t, base).URL)
		res := runPipeline(context.Background(), s, mkTask())
		if res.Status != "SE" || !strings.Contains(res.Cases[0].Message, "interactor exit 2") {
			t.Fatalf("status=%s msg=%q, want SE interactor exit 2", res.Status, res.Cases[0].Message)
		}
	})
	t.Run("interactor 0 but user TLE is TLE", func(t *testing.T) {
		r := mkRunner(&sandbox.Result{Status: sandbox.StatusTLE, CPUTimeMS: 1500}, &sandbox.Result{Status: sandbox.StatusOK, ExitCode: 0}, "")
		s := newTestService(t, r, newFetchServer(t, base).URL)
		res := runPipeline(context.Background(), s, mkTask())
		if res.Status != "TLE" {
			t.Fatalf("status = %s, want TLE", res.Status)
		}
	})
	t.Run("interactor SE is SE", func(t *testing.T) {
		r := mkRunner(&sandbox.Result{Status: sandbox.StatusOK}, &sandbox.Result{Status: sandbox.StatusSE, Message: "pipe exploded"}, "")
		s := newTestService(t, r, newFetchServer(t, base).URL)
		res := runPipeline(context.Background(), s, mkTask())
		if res.Status != "SE" || !strings.Contains(res.Cases[0].Message, "interactor: pipe exploded") {
			t.Fatalf("status=%s msg=%q, want SE interactor fault", res.Status, res.Cases[0].Message)
		}
	})
	t.Run("RunPair infra error is SE", func(t *testing.T) {
		r := &mockRunner{PairFn: func(userSpec, interSpec *sandbox.Spec, stderr *os.File) (*sandbox.Result, *sandbox.Result, error) {
			return nil, nil, fmt.Errorf("nameserver: cannot create pipe")
		}}
		s := newTestService(t, r, newFetchServer(t, base).URL)
		res := runPipeline(context.Background(), s, mkTask())
		if res.Status != "SE" || !strings.Contains(res.Cases[0].Message, "cannot create pipe") {
			t.Fatalf("status=%s msg=%q, want SE with runner error", res.Status, res.Cases[0].Message)
		}
	})
}

func TestPipelineUnknownLanguageSE(t *testing.T) {
	r := &mockRunner{}
	s := newTestService(t, r, "")
	task := plainTask(1, []CaseRef{caseRef(0, "1\n", "1\n")})
	task.LanguageID = "brainfuck"
	res := runPipeline(context.Background(), s, task)
	if res.Status != "SE" {
		t.Fatalf("status = %s, want SE", res.Status)
	}
	if !strings.Contains(res.Error, "unknown language") {
		t.Fatalf("error = %q, want unknown language detail", res.Error)
	}
	if len(res.Cases) != 0 {
		t.Fatalf("cases = %d, want 0", len(res.Cases))
	}
}

// TestServiceRunChownLimitation documents why runPipeline exists: Service.Run
// hard-fails on Windows because os.Chown is EWINDOWS there. On Linux it must
// proceed into judging, so the guard keeps each platform honest.
func TestServiceRunChownLimitation(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("documents Windows-only chown limitation; run the pipeline via Service.Run on Linux")
	}
	r := echoRunner(nil, nil, nil)
	s := newTestService(t, r, "")
	res := s.Run(context.Background(), plainTask(1, []CaseRef{caseRef(0, "1\n", "1\n")}))
	if res.Status != "SE" {
		t.Fatalf("status = %s, want SE on Windows (chown unsupported)", res.Status)
	}
	if !strings.Contains(res.Error, "chown") {
		t.Fatalf("error = %q, want chown detail", res.Error)
	}
}
