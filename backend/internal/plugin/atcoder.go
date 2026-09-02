// Package plugin — AtCoder adapter. AtCoder has no public API; the home
// page of a user (https://atcoder.jp/users/<name>) embeds a JSON payload
// (data-page="1") listing recent submissions (id, problem, verdict, time).
// Scraping is HTML-shaped but the payload is JSON, so parsing is stable as
// long as the page keeps that script block.
package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

func init() { RegisterSubmitFetcher(atFetcher{}) }

const atcoderBase = "https://atcoder.jp"

// atHandle validates AtCoder handles (alphanumeric + _ only) before use.
var atHandleRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,24}$`)

// atcoderVerdict maps the platform's "AC"/"WA"/"TLE"… shorthand (already
// close to ours) with anything unknown degrading to OTHER.
func atcoderVerdict(v string) string {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "AC":
		return "AC"
	case "WA":
		return "WA"
	case "TLE":
		return "TLE"
	case "MLE":
		return "MLE"
	case "RE":
		return "RE"
	case "CE":
		return "CE"
	default:
		return "OTHER"
	}
}

// atSub mirrors one row of the embedded submission table JSON.
type atSub struct {
	ID            string `json:"ID"`
	EpochSecond   int64  `json:"EpochSecond"`
	ProblemID     string `json:"ProblemID"`
	ProblemTitle  string `json:"ProblemTitle"`
	Language      string `json:"Language"`
	UserIsFriend  bool   `json:"UserIsFriend"`
	Status        string `json:"Status"`
	ContestID     string `json:"ContestID"`
	SubmissionTime string `json:"SubmissionTime"`
}

type atFetcher struct{}

func (atFetcher) Name() string { return "atcoder" }

func (atFetcher) FetchSubmitLog(ctx context.Context, username string, needAll bool) ([]SubmitRecord, error) {
	if !atHandleRe.MatchString(username) {
		return nil, fmt.Errorf("plugin/atcoder: 非法用户名 %q", username)
	}
	// AtCoder's profile submissions endpoint serves JSON when asked via the
	// users/<id>/submissions path? It serves HTML; the JSON rides inside a
	// script tag. use-submission "count" pages cap at 1000 — plenty for the
	// incremental tail; needAll just reads more pages.
	count := 200
	if needAll {
		count = 1000
	}
	url := fmt.Sprintf("%s/users/%s/submissions?count=%d", atcoderBase, username, count)
	body, err := SafeHTTPGet(ctx, url)
	if err != nil {
		return nil, err
	}
	subs, err := parseAtcoderSubmissions(body)
	if err != nil {
		return nil, err
	}
	out := make([]SubmitRecord, 0, len(subs))
	for _, s := range subs {
		out = append(out, SubmitRecord{
			Platform:   "atcoder",
			ExternalID: s.ID,
			ProblemID:  s.ProblemID,
			ProblemName: s.ProblemTitle,
			Verdict:    atcoderVerdict(s.Status),
			Language:   s.Language,
			At:         s.EpochSecond,
		})
	}
	return out, nil
}

// parseAtcoderSubmissions extracts the embedded JSON array from the profile
// submissions page. The page embeds it in a <script> tag as a JSON array of
// objects (not wrapped in JSON.parse) — locate the outermost [ ... ].
func parseAtcoderSubmissions(page []byte) ([]atSub, error) {
	s := string(page)
	start := strings.Index(s, "[{\"ID\":")
	if start < 0 {
		return nil, fmt.Errorf("plugin/atcoder: 页面中未找到提交列表（页面结构变化或被风控）")
	}
	// walk to the matching closing bracket of the outermost array
	depth := 0
	end := -1
	inStr := false
	esc := false
	for i := start; i < len(s) && i < start+8<<20; i++ {
		c := s[i]
		if esc {
			esc = false
			continue
		}
		switch c {
		case '\\':
			if inStr {
				esc = true
			}
		case '"':
			inStr = !inStr
		case '[':
			if !inStr {
				depth++
			}
		case ']':
			if !inStr {
				depth--
				if depth == 0 {
					end = i + 1
				}
			}
		}
		if end > 0 {
			break
		}
	}
	if end < 0 {
		return nil, fmt.Errorf("plugin/atcoder: 提交列表 JSON 未闭合")
	}
	var subs []atSub
	if err := json.Unmarshal([]byte(s[start:end]), &subs); err != nil {
		return nil, fmt.Errorf("plugin/atcoder: parse submissions: %w", err)
	}
	return subs, nil
}