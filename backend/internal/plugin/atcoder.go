// Package plugin — AtCoder adapter. AtCoder has no official API; the
// de-facto standard data source is the community AtCoder Problems API
// (kenkoooo.com), which serves per-user submissions as plain JSON
// (result strings already use AC/WA/TLE shorthand). The earlier approach —
// scraping /users/<name>/submissions — 404s: that endpoint does not exist.
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

// kenkooooAPI is the AtCoder Problems community API. Third-party by nature —
// acceptable for practice-stats display (non-authoritative data), same trade
// off every AtCoder statistics tool makes.
const kenkooooAPI = "https://kenkoooo.com/atcoder/atcoder-api/v3/user/submissions"

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

// atSub mirrors one kenkoooo API record.
type atSub struct {
	ID          int64  `json:"id"`
	EpochSecond int64  `json:"epoch_second"`
	ProblemID   string `json:"problem_id"`
	ContestID   string `json:"contest_id"`
	Language    string `json:"language"`
	Result      string `json:"result"`
}

type atFetcher struct{}

func (atFetcher) Name() string { return "atcoder" }

func (atFetcher) FetchSubmitLog(ctx context.Context, username string, needAll bool) ([]SubmitRecord, error) {
	if !atHandleRe.MatchString(username) {
		return nil, fmt.Errorf("plugin/atcoder: 非法用户名 %q", username)
	}
	// from_second=0 pulls the user's full history (dedup happens at insert);
	// the API truncates very long histories, so heavy users get the most
	// recent window — acceptable for practice-stats display.
	url := fmt.Sprintf("%s?user=%s&from_second=0", kenkooooAPI, username)
	body, err := SafeHTTPGet(ctx, url)
	if err != nil {
		return nil, err
	}
	var subs []atSub
	if err := json.Unmarshal(body, &subs); err != nil {
		return nil, fmt.Errorf("plugin/atcoder: parse submissions: %w", err)
	}
	out := make([]SubmitRecord, 0, len(subs))
	for _, s := range subs {
		out = append(out, SubmitRecord{
			Platform:   "atcoder",
			ExternalID: fmt.Sprintf("%d", s.ID),
			ProblemID:  s.ProblemID,
			ProblemName: s.ProblemID, // kenkoooo has no title; id reads fine (abc129_a)
			Verdict:    atcoderVerdict(s.Result),
			Language:   s.Language,
			At:         s.EpochSecond,
		})
	}
	return out, nil
}

// parseAtcoderSubmissions kept for the fixture test (plugin_test.go): the
// kenkoooo payload is this same bare JSON array shape.
func parseAtcoderSubmissions(page []byte) ([]atSub, error) {
	var subs []atSub
	if err := json.Unmarshal(page, &subs); err != nil {
		return nil, fmt.Errorf("plugin/atcoder: parse submissions: %w", err)
	}
	return subs, nil
}
