// Package handler — problem package import (自有格式 / DOMjudge / Hydro).
// The importer is deliberately tolerant: instead of three strict parsers it
// scans the zip for the layout markers each format is known to use and
// degrades gracefully (missing statement/answers become notes, not errors),
// because ICPC tool chains emit slightly different shapes per version.
package handler

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/model"
)

// caseKey is one discovered test case: the .in file path plus its paired
// answer file (same directory, same base, .ans/.out/.a extension), before
// sequential renumbering.
type caseKey struct {
	in, ans *zip.File
	name    string // base name without extension, for natural sorting
}

var judgeSrcRe = regexp.MustCompile(`(?i)(checker|validator|spj|interactor|interact)[^/]*\.(cc|cpp|cxx)$`)

// importProblem creates a new problem (owned by the caller) from an uploaded
// zip in any of three supported shapes, detected automatically:
//   - 自有格式: meta.txt + statement.md + testdata/
//   - DOMjudge: problem.yaml + statements/ + data/{sample,secret}/
//   - Hydro:    problem.yaml + problem[_zh].md + testdata/
func (s *Server) importProblem(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": "missing file field"})
		return
	}
	f, err := fileHeader.Open()
	if err != nil {
		c.JSON(400, gin.H{"error": "open upload failed"})
		return
	}
	defer f.Close()

	tmp, err := os.CreateTemp(s.Cfg.DataDir, "import-*.zip")
	if err != nil {
		c.JSON(500, gin.H{"error": "temp file failed"})
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := io.Copy(tmp, io.LimitReader(f, maxZipBytes+1)); err != nil {
		c.JSON(400, gin.H{"error": "read upload failed"})
		return
	}
	info, _ := tmp.Stat()
	if info == nil || info.Size() > maxZipBytes {
		c.JSON(400, gin.H{"error": fmt.Sprintf("zip too large (max %d MB)", maxZipBytes>>20)})
		return
	}
	zr, err := zip.NewReader(tmp, info.Size())
	if err != nil {
		c.JSON(400, gin.H{"error": "not a valid zip"})
		return
	}

	meta := scanPackage(zr)
	if len(meta.cases) == 0 {
		c.JSON(400, gin.H{"error": "zip 中未找到任何 <name>.in 测试数据文件"})
		return
	}

	claims := auth.CurrentUser(c)
	prob := &model.Problem{
		Title:         meta.title,
		StatementMD:   meta.statement,
		InputDesc:     meta.inputDesc,
		OutputDesc:    meta.outputDesc,
		Hint:          meta.hint,
		TimeLimitMS:   meta.timeMS,
		MemLimitMB:    meta.memMB,
		Visibility:    model.VisibilityHidden, // imported problems start hidden until reviewed
		JudgeMode:     meta.judgeMode,
		CheckerSource: meta.checker,
		InteractorSrc: meta.interactor,
		ReviewStatus:  ReviewApproved, // setter-created/imported problems skip review
		CreatedBy:     claims.UserID,
	}
	if strings.TrimSpace(prob.Title) == "" {
		prob.Title = "导入题目"
	}
	if err := s.DB.Create(prob).Error; err != nil {
		c.JSON(500, gin.H{"error": "create problem failed"})
		return
	}
	stored, err := s.storeImportedCases(prob, meta.cases)
	if err != nil {
		// why rollback manually: the problem row would otherwise dangle with
		// zero testdata — unusable and invisible in the editor's case panel.
		s.DB.Delete(prob)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{
		"problem": prob, "stored": stored, "format": meta.format,
		"notes": meta.notes,
	})
}

// packageMeta is everything the scanner could pull out of the zip.
type packageMeta struct {
	format     string
	title      string
	statement  string
	inputDesc  string
	outputDesc string
	hint       string
	timeMS     int
	memMB      int
	judgeMode  string
	checker    string
	interactor string
	cases      []caseKey
	notes      []string
}

// scanPackage walks the zip once and classifies it. The generic testdata scan
// (basename-paired .in/.ans|.out anywhere in the archive) covers all three
// formats, so format detection only affects metadata extraction.
func scanPackage(zr *zip.Reader) *packageMeta {
	meta := &packageMeta{timeMS: 1000, memMB: 256}
	yamlText := ""
	ownMeta := map[string]string{}

	var statementFiles, texFiles, pdfFiles []string
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() || strings.Contains(zf.Name, "__MACOSX") {
			continue
		}
		lower := strings.ToLower(zf.Name)

		switch {
		case lower == "meta.txt" || lower == "/meta.txt":
			if raw, err := readZipFile(zf, 1<<20); err == nil {
				ownMeta = parseKV(string(raw))
				meta.format = "own"
			}
		case lower == "problem.yaml":
			if raw, err := readZipFile(zf, 1<<20); err == nil {
				yamlText = string(raw)
				if meta.format == "" {
					meta.format = "hydro-or-domjudge"
				}
			}
		case strings.HasSuffix(lower, ".md") && isStatementPath(lower):
			statementFiles = append(statementFiles, zf.Name)
		case strings.HasSuffix(lower, ".tex") && isStatementPath(lower):
			texFiles = append(texFiles, zf.Name)
		case strings.HasSuffix(lower, ".pdf") && isStatementPath(lower):
			pdfFiles = append(pdfFiles, zf.Name)
		}

		if m := judgeSrcRe.FindStringSubmatch(lower); m != nil && len(zf.Name) < 200 {
			raw, err := readZipFile(zf, 512<<10)
			if err != nil {
				meta.notes = append(meta.notes, fmt.Sprintf("跳过过大的判题源码 %s", zf.Name))
				continue
			}
			src := string(raw)
			if strings.Contains(m[1], "interact") {
				meta.interactor = src
			} else if meta.checker == "" {
				meta.checker = src
			}
		}

		if strings.HasSuffix(lower, ".in") {
			key := strings.TrimSuffix(zf.Name, ".in")
			meta.cases = append(meta.cases, caseKey{in: zf, name: key})
		}
	}

	// pair each .in with its .ans/.out sibling
	var kept []caseKey
	for i := range meta.cases {
		for _, zf := range zr.File {
			lower := strings.ToLower(zf.Name)
			for _, ext := range []string{".ans", ".out", ".a"} {
				if strings.HasSuffix(lower, ext) &&
					strings.EqualFold(strings.TrimSuffix(zf.Name, ext), meta.cases[i].name) {
					meta.cases[i].ans = zf
				}
			}
		}
		if meta.cases[i].ans == nil {
			continue // kept as note below
		}
		kept = append(kept, meta.cases[i])
	}
	if len(kept) == 0 && len(meta.cases) > 0 {
		meta.notes = append(meta.notes, fmt.Sprintf("发现 %d 个 .in 但全部缺少 .ans/.out 答案文件，已全部忽略", len(meta.cases)))
	}
	meta.cases = kept
	sortCases(meta.cases)

	// metadata: own meta.txt wins, then problem.yaml
	if meta.format == "own" {
		meta.title = ownMeta["title"]
		meta.timeMS = parseIntOr(ownMeta["time_limit_ms"], meta.timeMS)
		meta.memMB = parseIntOr(ownMeta["mem_limit_mb"], meta.memMB)
		if ownMeta["judge_mode"] != "" {
			meta.judgeMode = ownMeta["judge_mode"]
		}
		if ownMeta["statement"] != "" {
			meta.statement = ownMeta["statement"] // not used; statement.md is the real one
		}
	}
	if yamlText != "" {
		y := parseFlatYAML(yamlText)
		if meta.title == "" {
			meta.title = firstNonEmpty(y["title"], y["name"])
		}
		meta.timeMS = parseDurationOr(
			y["time"], y["time_limit"], y["time_limit_s"],
			y["limits.time"], y["limits.timeout"], y["limits.time_limit"],
			meta.timeMS)
		meta.memMB = parseIntOr(firstNonEmpty(
			y["memory_limit"], y["memory_limit_mb"], y["memory"],
			y["limits.memory"], y["limits.memory_limit"]), meta.memMB)
	}

	// statement: prefer markdown, fall back to TeX (kept as source), pdf noted
	sort.Strings(statementFiles)
	switch {
	case len(statementFiles) > 0:
		for _, zf := range zr.File {
			if zf.Name == statementFiles[0] {
				if raw, err := readZipFile(zf, 4<<20); err == nil {
					meta.statement = string(raw)
				}
				break
			}
		}
	case len(texFiles) > 0:
		for _, zf := range zr.File {
			if zf.Name == texFiles[0] {
				if raw, err := readZipFile(zf, 4<<20); err == nil {
					meta.statement = "> 本题题面为 LaTeX 源码，请自行转换后编辑：\n\n```latex\n" + string(raw) + "\n```"
				}
				break
			}
		}
		meta.notes = append(meta.notes, "未找到 Markdown 题面，已将 TeX 源码放入题面字段")
	case len(pdfFiles) > 0:
		meta.notes = append(meta.notes, "题面仅为 PDF，无法自动导入，请在编辑器中补写")
	}

	// judge mode: interactor implies interactive, checker implies spj
	switch {
	case meta.interactor != "":
		meta.judgeMode = model.JudgeModeInteractive
	case meta.checker != "":
		meta.judgeMode = model.JudgeModeSPJ
	case meta.format == "own":
		if meta.judgeMode != model.JudgeModeSPJ && meta.judgeMode != model.JudgeModeInteractive {
			meta.judgeMode = model.JudgeModeDefault
		}
	default:
		meta.judgeMode = model.JudgeModeDefault
	}
	if meta.checker != "" && meta.judgeMode != model.JudgeModeInteractive {
		meta.notes = append(meta.notes, "已导入 checker 源码 — 请核对它的参数约定与本 OJ 的「checker <in> <ans> <out>」一致")
	}
	meta.timeMS = clampInt(meta.timeMS, 50, 600_000)
	meta.memMB = clampInt(meta.memMB, 16, 8192)
	return meta
}

func isStatementPath(lower string) bool {
	return strings.HasPrefix(lower, "statements/") ||
		lower == "statement.md" ||
		lower == "problem.md" ||
		strings.HasPrefix(lower, "problem_zh") ||
		strings.HasPrefix(lower, "problem_en")
}

// sortCases orders by path naturally (1,2,10 rather than 1,10,2).
func sortCases(cases []caseKey) {
	sort.Slice(cases, func(i, j int) bool {
		a, b := cases[i].name, cases[j].name
		da, db := filepath.Dir(a), filepath.Dir(b)
		if da != db {
			return da < db
		}
		na, nb := filepath.Base(a), filepath.Base(b)
		return naturalLess(na, nb)
	})
}

var numRe = regexp.MustCompile(`\d+`)

func naturalLess(a, b string) bool {
	ai, bi := 0, 0
	for ai < len(a) && bi < len(b) {
		am, bm := numRe.FindStringIndex(a[ai:]), numRe.FindStringIndex(b[bi:])
		switch {
		case am != nil && bm != nil:
			x, _ := strconv.Atoi(a[ai+am[0] : ai+am[1]])
			y, _ := strconv.Atoi(b[bi+bm[0] : bi+bm[1]])
			if x != y {
				return x < y
			}
			ai += am[1]
			bi += bm[1]
		case am != nil:
			return true
		case bm != nil:
			return false
		default:
			if a[ai] != b[bi] {
				return a[ai] < b[bi]
			}
			ai++
			bi++
		}
	}
	return len(a)-ai < len(b)-bi
}

// storeImportedCases writes the discovered pairs under the new problem's
// testdata dir and creates TestCase rows numbered 1..N.
func (s *Server) storeImportedCases(prob *model.Problem, cases []caseKey) (int, error) {
	destDir := filepath.Join(s.Cfg.DataDir, "testdata", fmt.Sprint(prob.ID))
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return 0, err
	}
	stored := 0
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		for i, ck := range cases {
			idx := i + 1
			data, err := readZipFile(ck.in, 256<<20)
			if err != nil {
				return fmt.Errorf("case %d input: %w", idx, err)
			}
			tc := &model.TestCase{ProblemID: prob.ID, CaseIndex: idx}
			inSHA, inSize, err := writeCaseFile(filepath.Join(destDir, strconv.Itoa(idx)+".in"), data)
			if err != nil {
				return err
			}
			tc.InputSHA, tc.InputSize = inSHA, inSize
			ansData, err := readZipFile(ck.ans, 256<<20)
			if err != nil {
				return fmt.Errorf("case %d answer: %w", idx, err)
			}
			ansSHA, ansSize, err := writeCaseFile(filepath.Join(destDir, strconv.Itoa(idx)+".out"), ansData)
			if err != nil {
				return err
			}
			tc.AnswerSHA, tc.AnswerSize = ansSHA, ansSize
			if err := tx.Create(tc).Error; err != nil {
				return err
			}
			stored++
		}
		return nil
	})
	if err != nil {
		return stored, err
	}
	return stored, nil
}

func writeCaseFile(path string, data []byte) (sha string, size int64, err error) {
	if err := os.WriteFile(path, data, 0o640); err != nil {
		return "", 0, err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), int64(len(data)), nil
}

// parseFlatYAML reads the flat "key: value" subset used by problem.yaml.
// Handles three real-world shapes: plain scalars, inline flow maps
// (`limits: {timeout: 1.5, memory: 512}` — flattened as "limits.timeout"),
// and one level of block-nested maps (DOMjudge writes both). Comments,
// quotes and list lines are skipped — no YAML dependency needed.
func parseFlatYAML(text string) map[string]string {
	out := map[string]string{}
	parent := ""
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}
		indented := strings.HasPrefix(raw, " ") || strings.HasPrefix(raw, "\t")
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if key == "" || strings.Contains(key, " ") {
			continue
		}
		if indented && parent != "" {
			key = parent + "." + key
		}
		if strings.HasPrefix(val, "{") && strings.HasSuffix(val, "}") {
			for _, kv := range strings.Split(strings.Trim(val, "{}"), ",") {
				k2, v2, ok2 := strings.Cut(strings.TrimSpace(kv), ":")
				if !ok2 {
					continue
				}
				k2, v2 = strings.TrimSpace(k2), strings.Trim(strings.TrimSpace(v2), `"'`)
				if k2 != "" && !strings.Contains(k2, " ") {
					out[key+"."+k2] = v2
				}
			}
			parent = ""
			continue
		}
		out[key] = val
		if val == "" {
			parent = key // block map below: children become parent.key
		} else {
			parent = ""
		}
	}
	return out
}

func parseKV(text string) map[string]string {
	return parseFlatYAML(strings.ReplaceAll(text, "=", ": "))
}

func parseIntOr(s string, def int) int {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "mb"))
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || v <= 0 {
		return def
	}
	return v
}

// parseDurationOr accepts "1", "1s", "1.5s", "1500ms" style values (seconds
// unless suffixed) and returns milliseconds; trailing int is the fallback.
func parseDurationOr(vals ...any) int {
	def := vals[len(vals)-1]
	for _, v := range vals[:len(vals)-1] {
		s, ok := v.(string)
		if !ok {
			continue
		}
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		var num float64
		var unit string
		if _, err := fmt.Sscanf(s, "%f%s", &num, &unit); err != nil && num == 0 {
			continue
		}
		var ms int
		switch unit {
		case "", "s", "sec", "second", "seconds":
			ms = int(num * 1000)
		case "ms":
			ms = int(num)
		}
		if ms > 0 {
			return ms
		}
	}
	if d, ok := def.(int); ok {
		return d
	}
	return 1000
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
