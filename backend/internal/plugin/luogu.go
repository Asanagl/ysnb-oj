// Package plugin — Luogu adapter. Luogu has no official public API; the
// data endpoints used here are the same ones its own web client calls
// (/problem/<id> returns an embedded __JS_DATA payload, /record/list.json
// lists a user's records). Responses are JSON behind a CSRF-free read path,
// but the platform rate-limits aggressively — the sync worker spaces calls
// and tolerates HTTP 420 by backing off.
package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	neturl "net/url"
	"regexp"
	"strconv"
	"strings"
)

func init() { RegisterProblemSource(lgSource{}) }
func init() { RegisterSubmitFetcher(lgFetcher{}) }

const luoguBase = "https://www.luogu.com.cn"

// luoguProblemKey validates pid shapes like P1001 / B2001 / CF1900A / SP123 /
// UVA1234 / AT_abc300_a so a bad id fails before the outbound call.
var luoguPIDRe = regexp.MustCompile(`^(P|B|CF|SP|UVA|AT_)[A-Za-z0-9_]+$`)

type lgSource struct{}

func (lgSource) Name() string { return "luogu" }

// lgProblemData mirrors the fields of __JS_DATA.problem we rely on.
type lgProblemData struct {
	PID     string `json:"pid"`
	Title   string `json:"title"`
	Content string `json:"content"` // markdown-ish Luogu flavor
	Limits  struct {
		TimeMS   int `json:"time"`
		MemMB    int `json:"memory"`
	} `json:"limits"`
	Tags []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"tags"`
	Samples [][][]string `json:"samples"` // [ [{input}], [{output}] ] oddities flattened defensively
}

func (lgSource) FetchProblem(ctx context.Context, externalID string) (*ProblemMeta, error) {
	pid := strings.ToUpper(externalID)
	if !luoguPIDRe.MatchString(pid) {
		return nil, fmt.Errorf("plugin/luogu: 非法题号 %q（期望 P1001 / B2001 / CF1900A 之类）", externalID)
	}
	body, err := SafeHTTPGet(ctx, luoguBase+"/problem/"+pid)
	if err != nil {
		return nil, err
	}
	data, err := extractJSData(body)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Problem lgProblemData `json:"problem"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("plugin/luogu: parse __JS_DATA: %w", err)
	}
	meta := &ProblemMeta{
		Source: "luogu", ExternalID: pid,
		URL:  luoguBase + "/problem/" + pid,
		Title: payload.Problem.Title, StatementMD: payload.Problem.Content,
		TimeLimitMS: payload.Problem.Limits.TimeMS, MemLimitMB: payload.Problem.Limits.MemMB,
		Notes: []string{
			"洛谷题面为洛谷社区内容，导入后仅用于站内训练；正式比赛请使用自建题目",
			"外部题面仅含公开样例；本站测试数据需自行补充后方可用于正式评测",
		},
	}
	for _, t := range payload.Problem.Tags {
		if t.Name != "" {
			meta.Tags = append(meta.Tags, t.Name)
		}
	}
	// Luogu samples JSON shape varies across pages; pull every 2-string pair
	// defensively rather than trusting one exact schema.
	for _, block := range payload.Problem.Samples {
		for _, cell := range block {
			if len(cell) >= 2 && strings.TrimSpace(cell[0]) != "" {
				meta.Samples = append(meta.Samples, Sample{Input: cell[0], Output: cell[1]})
			}
		}
	}
	return meta, nil
}

// extractJSData pulls the embedded JSON out of a Luogu page. Older pages
// embed `JSON.parse(decodeURIComponent("..."))`; current pages embed
// `JSON.parse("<base64>")` — both are handled.
func extractJSData(page []byte) ([]byte, error) {
	s := string(page)
	if m := regexp.MustCompile(`JSON\.parse\(decodeURIComponent\("((?:[^"\\]|\\.)*)"\)\)`).FindStringSubmatch(s); m != nil {
		dec, err := strconv.Unquote(`"` + m[1] + `"`)
		if err == nil {
			if raw, err := decodeURIComponentString(dec); err == nil {
				return []byte(raw), nil
			}
		}
	}
	if m := regexp.MustCompile(`JSON\.parse\("((?:[^"\\]|\\.)*)"\)`).FindStringSubmatch(s); m != nil {
		dec, err := strconv.Unquote(`"` + m[1] + `"`)
		if err == nil {
			return []byte(dec), nil
		}
	}
	return nil, fmt.Errorf("plugin/luogu: 页面中未找到 __JS_DATA（可能被风控拦截，稍后重试）")
}

// decodeURIComponentString percent-decodes a decoded-URI component; Luogu's
// legacy embedding wraps UTF-8 percent escapes.
func decodeURIComponentString(s string) (string, error) {
	return urlUnescape(s)
}

// urlUnescape is a thin alias so the helper reads as its JS counterpart at
// the call site.
func urlUnescape(s string) (string, error) { return neturl.QueryUnescape(s) }

type lgFetcher struct{}

func (lgFetcher) Name() string { return "luogu" }

type lgRecord struct {
	ID       int    `json:"id"`
	PID      string `json:"pid"`
	Status   int    `json:"status"`
	Language int    `json:"language"`
	Time     struct {
		At int64 `json:"timestamp"`
	} `json:"time"`
}

func (lgFetcher) FetchSubmitLog(ctx context.Context, username string, needAll bool) ([]SubmitRecord, error) {
	count := 50
	if needAll {
		count = 500
	}
	body, err := SafeHTTPGet(ctx, fmt.Sprintf(
		luoguBase+"/record/list?user=%s&page=1&count=%d", username, count))
	if err != nil {
		return nil, err
	}
	data, err := extractJSData(body)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Records []lgRecord `json:"recordsData"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("plugin/luogu: parse records: %w", err)
	}
	out := make([]SubmitRecord, 0, len(payload.Records))
	for _, r := range payload.Records {
		out = append(out, SubmitRecord{
			Platform:   "luogu",
			ExternalID: strconv.Itoa(r.ID),
			ProblemID:  r.PID,
			Verdict:    lgNormalizeStatus(r.Status),
			At:         r.Time.At,
		})
	}
	return out, nil
}

// lgNormalizeStatus maps Luogu numeric status codes onto site verdicts
// (12=AC, 5=CE, 6=WA, 7=RE, 2=TLE, 4=MLE per Luogu record statuses).
func lgNormalizeStatus(code int) string {
	switch code {
	case 12:
		return "AC"
	case 6:
		return "WA"
	case 2:
		return "TLE"
	case 4:
		return "MLE"
	case 7:
		return "RE"
	case 5:
		return "CE"
	default:
		return "OTHER"
	}
}