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

type cfSub struct {
	ID                  int    `json:"id"`
	ContestID           int    `json:"contestId"`
	ProblemIndex        string `json:"problemIndex"`
	ProblemName         string `json:"problemName"`
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
		out = append(out, SubmitRecord{
			Platform: "codeforces",
			ExternalID: strconv.Itoa(s.ID),
			ProblemID:  fmt.Sprintf("%d%s", s.ContestID, s.ProblemIndex),
			ProblemName: s.ProblemName,
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

// cfHTMLToText 把章节片段压成纯文本：CF 新版样例把每行输入包在
// <div class="test-example-line"> 里（转成换行），其余标签剥除，实体还原。
func cfHTMLToText(fragment string) string {
	s := strings.ReplaceAll(fragment, "</div>", "\n")
	s = strings.ReplaceAll(s, "</p>", "\n")
	s = strings.ReplaceAll(s, "<br>", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br />", "\n")
	// 去掉 section-title 之类的标题行残留
	tagRe := regexp.MustCompile(`<[^>]*>`)
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" || ln == "Input" || ln == "Output" || ln == "Note" || ln == "Examples" || ln == "Example" {
			continue
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}

var cfSamplePreRe = regexp.MustCompile(`(?s)<pre[^>]*>(.*?)</pre>`)

// cfEnrichStatement 抓题面页并填充 StatementMD（输入/输出说明 + 样例）。
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
	inputSpec := cfHTMLToText(cfSection(pageHTML, "input-specification"))
	outputSpec := cfHTMLToText(cfSection(pageHTML, "output-specification"))
	note := cfHTMLToText(cfSection(pageHTML, "note"))

	var b strings.Builder
	if inputSpec != "" {
		b.WriteString("## 输入\n\n" + inputSpec + "\n\n")
	}
	if outputSpec != "" {
		b.WriteString("## 输出\n\n" + outputSpec + "\n\n")
	}
	// 样例：sample-tests 内的 pre 交替为 输入/输出
	samplesHTML := cfSection(pageHTML, "sample-tests")
	if samplesHTML != "" {
		pre := cfSamplePreRe.FindAllStringSubmatch(samplesHTML, -1)
		b.WriteString("## 样例\n\n")
		for i, m := range pre {
			body := strings.ReplaceAll(m[1], "</div>", "\n")
			body = html.UnescapeString(strings.TrimSpace(tagStripRe.ReplaceAllString(body, "")))
			if body == "" {
				continue
			}
			if i%2 == 0 {
				b.WriteString("输入：\n```\n" + body + "\n```\n")
			} else {
				b.WriteString("输出：\n```\n" + body + "\n```\n")
			}
		}
	}
	if note != "" {
		b.WriteString("## 备注\n\n" + note + "\n")
	}
	if strings.TrimSpace(b.String()) == "" {
		return fmt.Errorf("no statement sections extracted")
	}
	meta.StatementMD = strings.TrimSpace(meta.StatementMD + "\n\n" + b.String())
	return nil
}