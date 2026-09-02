// Package plugin — Codeforces adapter. Codeforces has an official public
// read API (codeforces.com/api/*), so both the problem crawler and the
// submit-log fetcher are plain JSON clients — the most robust integration
// of the bundled set.
package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

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