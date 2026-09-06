// Package plugin — Codeforces adapter. Codeforces has an official public
// read API (codeforces.com/api/*), so both the problem crawler and the
// submit-log fetcher are plain JSON clients — the most robust integration
// of the bundled set.
package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

var tagStripRe = regexp.MustCompile(`<[^>]*>`)

// Codeforces contest IDs >= 100000 are gyms; their API keys differ by the
// same sign convention CWXU-Algo uses (kept for URL canonicalization).
const cfGymMinID = 100000

func init() { RegisterProblemSource(cfSource{}) }
func init() { RegisterSubmitFetcher(cfFetcher{}) }

func cfProblemURL(contestID int, index string) string {
	if contestID >= cfGymMinID {
		return fmt.Sprintf("https://codeforces.com/gym/%d/problem/%s", contestID, index)
	}
	return fmt.Sprintf("https://codeforces.com/contest/%d/problem/%s", contestID, index)
}

// --- problem source ---

type cfSource struct{}

func (cfSource) Name() string { return "codeforces" }

type cfProblemInfo struct {
	ContestID   int    `json:"contestId"`
	Index       string `json:"index"`
	Name        string `json:"name"`
	TimeLimitMS int    `json:"timeLimitMs"`
	MemLimitMB  int    `json:"memoryLimitMegabytes"`
}

func (cfSource) FetchProblem(ctx context.Context, externalID string) (*ProblemMeta, error) {
	contestID, index, err := cfSplitID(externalID)
	if err != nil {
		return nil, err
	}
	body, err := SafeHTTPGet(ctx, "https://codeforces.com/api/problemset.problems")
	if err != nil {
		return nil, err
	}
	var payload struct {
		Status string `json:"status"`
		Result struct {
			Problems []cfProblemInfo `json:"problems"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload.Status != "OK" {
		return nil, fmt.Errorf("plugin/codeforces: API status %q", payload.Status)
	}
	want := fmt.Sprintf("%d%s", contestID, index)
	for _, p := range payload.Result.Problems {
		if fmt.Sprintf("%d%s", p.ContestID, p.Index) == want {
			meta := &ProblemMeta{
				Source: "codeforces", ExternalID: want,
				URL: cfProblemURL(p.ContestID, p.Index), Title: p.Name,
				TimeLimitMS: p.TimeLimitMS, MemLimitMB: p.MemLimitMB,
				Notes: []string{
					"外部题面仅含题意与公开样例；本站测试数据需自行补充后方可用于正式评测",
				},
			}
			// 题面页抓取（best-effort）：CF 官方 API 不含题面， statements
			// 来自 HTML 页的固定结构。抓不到时保留空题面并在 Notes 提示人工补充，
			// 不让整次导入失败。
			if err := cfEnrichStatement(ctx, meta); err != nil {
				meta.Notes = append(meta.Notes, "题面页抓取失败（"+err.Error()+"），请从原题链接人工补充题面")
			}
			return meta, nil
		}
	}
	return nil, fmt.Errorf("plugin/codeforces: problem %s not found in catalog", want)
}

// cfSplitID parses "1900A" / "1900/A" / "gym104777A" (digits + letter index).
func cfSplitID(id string) (contestID int, index string, err error) {
	id = strings.TrimPrefix(strings.TrimPrefix(id, "gym"), "/")
	i := 0
	for i < len(id) && id[i] >= '0' && id[i] <= '9' {
		i++
	}
	if i == 0 || i == len(id) {
		return 0, "", fmt.Errorf("plugin/codeforces: id %q must be <contest><index> like 1900A", id)
	}
	contestID, err = strconv.Atoi(id[:i])
	if err != nil {
		return 0, "", err
	}
	index = strings.ToUpper(strings.TrimLeft(id[i:], "/"))
	return contestID, index, nil
}

// --- submit log fetcher ---

type cfFetcher struct{}

func (cfFetcher) Name() string { return "codeforces" }

// cfProblem mirrors the nested "problem" object CF returns inside every
// user.status row — the flat problemIndex/problemName fields do not exist
// at the top level (mapping them left problem_id = bare contest id and an
// empty name, which poisoned the practice stats).
type cfSub struct {
	ID                  int    `json:"id"`
	ContestID           int    `json:"contestId"`
	Problem             struct {
		ContestID int    `json:"contestId"`
		Index     string `json:"index"`
		Name      string `json:"name"`
	} `json:"problem"`
	ProgrammingLanguage string `json:"programmingLanguage"`
	Verdict             string `json:"verdict"`
	CreationTimeSeconds int64  `json:"creationTimeSeconds"`
}

func (cfFetcher) FetchSubmitLog(ctx context.Context, username string, needAll bool) ([]SubmitRecord, error) {
	url := "https://codeforces.com/api/user.status?handle=" + username + "&from=1"
	if !needAll {
		url += "&count=200"
	}
	body, err := SafeHTTPGet(ctx, url)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Status string   `json:"status"`
		Result []cfSub  `json:"result"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload.Status != "OK" {
		return nil, fmt.Errorf("plugin/codeforces: API status %q", payload.Status)
	}
	out := make([]SubmitRecord, 0, len(payload.Result))
	for _, s := range payload.Result {
		// gym rows carry their own contestId; prefer the nested problem's
		contestID := s.Problem.ContestID
		if contestID == 0 {
			contestID = s.ContestID
		}
		out = append(out, SubmitRecord{
			Platform: "codeforces",
			ExternalID: strconv.Itoa(s.ID),
			ProblemID:  fmt.Sprintf("%d%s", contestID, s.Problem.Index),
			ProblemName: s.Problem.Name,
			Verdict:    cfNormalizeVerdict(s.Verdict),
			Language:   s.ProgrammingLanguage,
			At:         s.CreationTimeSeconds,
		})
	}
	return out, nil
}

// cfNormalizeVerdict maps Codeforces verdict strings onto the site's verdict
// vocabulary; empty/unknown maps to OTHER (e.g. TESTING in-flight rows).
func cfNormalizeVerdict(v string) string {
	switch v {
	case "OK":
		return "AC"
	case "WRONG_ANSWER":
		return "WA"
	case "TIME_LIMIT_EXCEEDED":
		return "TLE"
	case "MEMORY_LIMIT_EXCEEDED":
		return "MLE"
	case "RUNTIME_ERROR":
		return "RE"
	case "COMPILATION_ERROR":
		return "CE"
	case "":
		return "OTHER"
	default:
		return "OTHER"
	}
}

// --- statement page scraping ---
//
// Codeforces 的题面只存在于 HTML 页（官方 API 不提供），页面结构多年稳定：
// 各章节是 problem-statement 下的同级 div，用已知 class 锚点切片即可提取，
// 不引入 HTML 解析依赖。抓取是 best-effort —— 失败只降级 Notes 提示。

// cfSection 按已知 class 锚点从 HTML 中切出一个章节的内部片段（到下一个
// 章节锚点或 problem-statement 结束为止）。找不到返回 ""。
func cfSection(html, class string) string {
	anchor := `<div class="` + class
	start := strings.Index(html, anchor)
	if start < 0 {
		return ""
	}
	rest := html[start:]
	end := len(rest)
	for _, next := range []string{"input-specification", "output-specification", "sample-tests", "note"} {
		if next == class {
			continue
		}
		if i := strings.Index(rest[len(anchor):], `<div class="`+next); i >= 0 {
			if len(anchor)+i < end {
				end = len(anchor) + i
			}
		}
	}
	// problem-statement 关闭兜底：若页面里其后紧跟结束标记
	if i := strings.Index(rest, `</div></div>`); i >= 0 && i < end {
		end = i
	}
	return rest[:end]
}

// MathJax 渲染残渣：CF 题面页对公式同时有 $$$...$$$ 源文本和渲染后的
// 备用 HTML（嵌套 span，剥标签会留下 "in math mode at position" 报错
// 碎片）。必须在剥标签前整块删除。mjx-container 是 MathJax 3 的另一种输出。
var (
	cfMathJaxSpanRe  = regexp.MustCompile(`(?s)<span[^>]*class="[^"]*(?:MathJax|mjx-)[^"]*"[^>]*>.*?</span>`)
	cfMjxContainerRe = regexp.MustCompile(`(?s)<mjx-container[^>]*>.*?</mjx-container>`)
	// 页内脚本/样式：script 内容是文本节点，剥标签会整段留下（实测混入
	// Codeforces.addMathJaxListener 等页内 JS）
	cfScriptStyleRe = regexp.MustCompile(`(?s)<script[^>]*>.*?</script>|<style[^>]*>.*?</style>`)
)

// stripMathJax removes rendered MathJax leftovers (nested spans need a few
// passes: inner spans surface as outer spans are removed) and page scripts.
func stripMathJax(s string) string {
	s = cfScriptStyleRe.ReplaceAllString(s, "")
	for i := 0; i < 6; i++ {
		next := cfMjxContainerRe.ReplaceAllString(s, "")
		next = cfMathJaxSpanRe.ReplaceAllString(next, "")
		if next == s {
			break
		}
		s = next
	}
	return s
}

// cfHTMLToText 把章节片段压成 Markdown 友好的纯文本：
//   - 先删除 MathJax 渲染残渣（否则剥标签后残留样式碎片污染正文）
//   - div/p 边界转段落换行（<p>/<div> 开头也转换行，段落间自然出现空行）
//   - 剥标签、实体还原
//   - Codeforces 的公式源用 $$$...$$$（三美元），站内 KaTeX 只认 $$，归一
//   - "Input/Output/Note" 等 section 标题残行剔除
func cfHTMLToText(fragment string) string {
	s := stripMathJax(fragment)
	s = strings.ReplaceAll(s, "<p>", "\n")
	s = strings.ReplaceAll(s, "<div>", "\n")
	s = strings.ReplaceAll(s, "</div>", "\n")
	s = strings.ReplaceAll(s, "</p>", "\n")
	s = strings.ReplaceAll(s, "<br>", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br />", "\n")
	s = tagStripRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	// CF 源文本的三美元 display math → 站内 KaTeX 的双美元
	s = strings.ReplaceAll(s, "$$$", "$$")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			// 空行 = Markdown 段落分隔，保留（连续空行压成一个）；
			// 首尾的空行在最后 TrimSpace 时清掉
			if n := len(out); n == 0 || out[n-1] != "" {
				out = append(out, "")
			}
			continue
		}
		if ln == "Input" || ln == "Output" || ln == "Note" || ln == "Examples" || ln == "Example" {
			continue
		}
		out = append(out, ln)
	}
	joined := strings.Join(out, "\n")
	return strings.TrimSpace(joined)
}

var cfSamplePreRe = regexp.MustCompile(`(?s)<pre[^>]*>(.*?)</pre>`)

// cfProblemStatementBlock 切出 problem-statement 整块（到页脚锚点为止）。
// 页面里 problem-statement 是最后一个主块，取到 footer/页脚标记即可。
func cfProblemStatementBlock(pageHTML string) string {
	start := strings.Index(pageHTML, `<div class="problem-statement`)
	if start < 0 {
		return ""
	}
	rest := pageHTML[start:]
	end := len(rest)
	for _, anchor := range []string{`id="footer"`, `<footer`, `class="footer`} {
		if i := strings.Index(rest, anchor); i >= 0 && i < end {
			end = i
		}
	}
	return rest[:end]
}

// cfEnrichStatement 抓题面页并填充 StatementMD：题目正文段（无 class 的
// 直接子 div，此前版本丢失）+ 输入/输出说明 + 样例代码块 + 备注。
// 一切失败都原样返回 error 由调用方降级为 Notes 提示。
func cfEnrichStatement(ctx context.Context, meta *ProblemMeta) error {
	page, err := SafeHTTPGet(ctx, meta.URL)
	if err != nil {
		return err
	}
	pageHTML := string(page)
	if !strings.Contains(pageHTML, "problem-statement") {
		return fmt.Errorf("page has no problem-statement block (anti-bot or gym page)")
	}
	block := cfProblemStatementBlock(pageHTML)
	if block == "" {
		return fmt.Errorf("problem-statement block empty")
	}
	// 题目正文：整块减去 header 与四个已知章节后的剩余
	story := block
	for _, cut := range []string{
		cfSection(block, "header"),
		cfSection(block, "input-specification"),
		cfSection(block, "output-specification"),
		cfSection(block, "sample-tests"),
		cfSection(block, "note"),
	} {
		if cut != "" {
			story = strings.Replace(story, cut, "", 1)
		}
	}
	story = cfHTMLToText(story)

	inputSpec := cfHTMLToText(cfSection(block, "input-specification"))
	outputSpec := cfHTMLToText(cfSection(block, "output-specification"))
	note := cfHTMLToText(cfSection(block, "note"))

	var b strings.Builder
	if story != "" {
		b.WriteString(story + "\n\n")
	}
	if inputSpec != "" {
		b.WriteString("## 输入\n\n" + inputSpec + "\n\n")
	}
	if outputSpec != "" {
		b.WriteString("## 输出\n\n" + outputSpec + "\n\n")
	}
	// 样例：sample-tests 内的 pre 交替为 输入/输出；代码块前后必须空行，
	// 否则 Markdown 解析器不认（行内 ``` 会被当纯文本）。
	samplesHTML := cfSection(block, "sample-tests")
	if samplesHTML != "" {
		pre := cfSamplePreRe.FindAllStringSubmatch(samplesHTML, -1)
		if len(pre) > 0 {
			b.WriteString("## 样例\n\n")
			for i, m := range pre {
				body := strings.ReplaceAll(m[1], "</div>", "\n")
				body = html.UnescapeString(strings.TrimSpace(tagStripRe.ReplaceAllString(body, "")))
				if body == "" {
					continue
				}
				label := "输入"
				if i%2 == 1 {
					label = "输出"
				}
				b.WriteString(label + "：\n\n```\n" + body + "\n```\n\n")
			}
		}
	}
	if note != "" {
		b.WriteString("## 备注\n\n" + note + "\n\n")
	}
	if strings.TrimSpace(b.String()) == "" {
		return fmt.Errorf("no statement sections extracted")
	}
	meta.StatementMD = strings.TrimSpace(b.String())
	return nil
}