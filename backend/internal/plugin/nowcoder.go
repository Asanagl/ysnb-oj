// Package plugin — Nowcoder (牛客竞赛) adapter. Nowcoder exposes JSON
// endpoints under nowcoder.com/acm/… for public judging records
// (/acm/record/list?user=… returns a paged JSON envelope). No login is
// needed for public records; the adapter reads only the public pages and
// tolerates the envelope shape drifting between {data:{records:[]}} and a
// bare array.
package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

func init() { RegisterSubmitFetcher(nkFetcher{}) }

const nowcoderBase = "https://ac.nowcoder.com"

// nkHandle is numeric for 牛客 (user id); the binding accepts either the
// numeric id or a profile URL, from which the id is extracted.
var nkIDRe = regexp.MustCompile(`(?:/acm/contest/profile/)?(\d{3,})`)

type nkFetcher struct{}

func (nkFetcher) Name() string { return "nowcoder" }

type nkRecord struct {
	ID       int64  `json:"id"`
	Problem  string `json:"problemName"`
	PID      string `json:"problemId"`
	Status   int    `json:"status"`
	Language string `json:"languageName"`
	// CreatedAt arrives as unix seconds (float in some envelopes).
	CreatedAt float64 `json:"createdAt"`
}

func (nkFetcher) FetchSubmitLog(ctx context.Context, username string, needAll bool) ([]SubmitRecord, error) {
	m := nkIDRe.FindStringSubmatch(strings.TrimSpace(username))
	if m == nil {
		return nil, fmt.Errorf("plugin/nowcoder: 请填牛客数字 ID 或个人主页链接")
	}
	page := 1
	if needAll {
		page = 0 // 0/absent = server default (first big page)
	}
	url := fmt.Sprintf("%s/acm/record/list?user=%s&page=%d&pageSize=200",
		nowcoderBase, m[1], page)
	body, err := SafeHTTPGet(ctx, url)
	if err != nil {
		return nil, err
	}
	recs, err := parseNowcoderRecords(body)
	if err != nil {
		return nil, err
	}
	out := make([]SubmitRecord, 0, len(recs))
	for _, r := range recs {
		at := int64(r.CreatedAt)
		// some envelopes report milliseconds
		if at > 1<<60/1000 {
			at /= 1000
		}
		out = append(out, SubmitRecord{
			Platform:   "nowcoder",
			ExternalID: strconv.FormatInt(r.ID, 10),
			ProblemID:  firstNonEmpty(r.PID, r.Problem),
			ProblemName: r.Problem,
			Verdict:    nkNormalizeStatus(r.Status),
			Language:   r.Language,
			At:         at,
		})
	}
	return out, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// parseNowcoderRecords accepts the historical envelope shapes: bare array,
// {"data":{...,"records":[...]}}, {"data":{"list":[...]}} — whichever the
// deployment answers with; empty payloads mean "no public records".
func parseNowcoderRecords(body []byte) ([]nkRecord, error) {
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "[") {
		var recs []nkRecord
		if err := json.Unmarshal(body, &recs); err == nil {
			return recs, nil
		}
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || len(envelope.Data) == 0 {
		return nil, fmt.Errorf("plugin/nowcoder: 无法解析记录响应（可能需要登录态或页面结构变化）")
	}
	var withRecords struct {
		Records []nkRecord `json:"records"`
		List    []nkRecord `json:"list"`
	}
	if err := json.Unmarshal(envelope.Data, &withRecords); err != nil {
		return nil, fmt.Errorf("plugin/nowcoder: parse records: %w", err)
	}
	if len(withRecords.Records) > 0 {
		return withRecords.Records, nil
	}
	return withRecords.List, nil
}

// nkNormalizeStatus maps Nowcoder numeric statuses (public record list):
// 5=AC, 6=WA, 7=RE…牛客 keeps its own table; unknown codes degrade to OTHER.
func nkNormalizeStatus(code int) string {
	switch code {
	case 5:
		return "AC"
	case 6:
		return "WA"
	case 7:
		return "RE"
	case 8:
		return "TLE"
	case 9:
		return "MLE"
	case 10:
		return "CE"
	case 11:
		return "OTHER" // compiling
	default:
		return "OTHER"
	}
}