// Package plugin is the OJ's extension registry: compile-time Go plugins
// registered by name, grouped into three extension points that cover the
// planned growth axes without runtime loading complexity:
//
//   - ProblemSource: fetch a problem's public metadata from an external OJ
//     (题库爬取 — M5-B consumes this to import problems into the bank).
//   - SubmitLogFetcher: pull one user's submission history from an external
//     platform (刷题统计报表 — M5-D consumes this for the sync worker).
//   - EventHook: fire-and-forget sinks for judge lifecycle events (webhooks
//     for bots; registered in builtin_hooks.go).
//
// Why registration tables instead of dynamic loading: the deployment story
// is a static binary + config (docs/deploy.md), and Go plugin .so loading is
// fragile across toolchain versions. A new integration = one new file with
// an init() Register call — same philosophy as judge language profiles.
package plugin

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// ProblemMeta is the normalized result of fetching an external problem:
// statement text, public samples and limits. Hidden judging testdata is
// structurally absent — external sites only publish samples.
type ProblemMeta struct {
	Source      string  // plugin name, e.g. "codeforces"
	ExternalID  string  // platform-native key, e.g. "1900A"
	URL         string  // canonical problem URL
	Title       string  //
	StatementMD string  // markdown; HTML sources are converted by the plugin
	InputDesc   string  //
	OutputDesc  string  //
	Hint        string  //
	TimeLimitMS int     // 0 = unknown
	MemLimitMB  int     // 0 = unknown
	Tags        []string //
	Samples     []Sample //
	Notes       []string // non-fatal anomalies, surfaced to the importer UI
}

// Sample is one public example pair (mirrors model.Sample without the
// import cycle).
type Sample struct {
	Input  string
	Output string
	Note   string
}

// ProblemSource fetches public problem metadata from an external platform.
type ProblemSource interface {
	Name() string // stable plugin id, e.g. "codeforces"
	FetchProblem(ctx context.Context, externalID string) (*ProblemMeta, error)
}

// SubmitRecord is one normalized submission from an external platform.
type SubmitRecord struct {
	Platform   string // plugin name
	ExternalID string // platform-native submission id (dedup key)
	ProblemID  string // platform-native problem key
	ProblemName string
	Verdict    string // normalized: AC | WA | TLE | MLE | RE | CE | OTHER
	Language   string
	At         int64 // unix seconds
}

// SubmitLogFetcher pulls a user's submission history. needAll=false asks for
// an incremental tail (latest page or two); implementations must tolerate
// either cadence — the sync worker de-duplicates by (platform, external_id).
type SubmitLogFetcher interface {
	Name() string
	FetchSubmitLog(ctx context.Context, username string, needAll bool) ([]SubmitRecord, error)
}

// EventHook receives judge lifecycle notifications after the fact. A slow or
// failing hook must never delay judging, so implementers are invoked on a
// detached goroutine by the emitter; errors are logged, not propagated.
// The built-in webhook hook exposes SetTarget/Targets for admin management,
// so the registry also exposes the concrete type below.
type EventHook interface {
	Name() string
	// OnJudgeEvent fires for every finalized submission. Payload keys mirror
	// the WS submission:* publish (id, status, problem_id, contest_id,
	// user_id, score, at).
	OnJudgeEvent(event map[string]any)
}

var (
	regMu       sync.RWMutex
	problemSrcs = map[string]ProblemSource{}
	subFetchers = map[string]SubmitLogFetcher{}
	eventHooks  = map[string]EventHook{}
)

// RegisterProblemSource adds a problem crawler; duplicate names are rejected
// loudly (duplicate init() almost always means two plugins chose one id).
func RegisterProblemSource(p ProblemSource) {
	regMu.Lock()
	defer regMu.Unlock()
	if _, dup := problemSrcs[p.Name()]; dup {
		panic(fmt.Sprintf("plugin: duplicate problem source %q", p.Name()))
	}
	problemSrcs[p.Name()] = p
}

// RegisterSubmitFetcher adds an external-practice platform adapter.
func RegisterSubmitFetcher(p SubmitLogFetcher) {
	regMu.Lock()
	defer regMu.Unlock()
	if _, dup := subFetchers[p.Name()]; dup {
		panic(fmt.Sprintf("plugin: duplicate submit fetcher %q", p.Name()))
	}
	subFetchers[p.Name()] = p
}

// RegisterEventHook adds a judge lifecycle sink.
func RegisterEventHook(p EventHook) {
	regMu.Lock()
	defer regMu.Unlock()
	if _, dup := eventHooks[p.Name()]; dup {
		panic(fmt.Sprintf("plugin: duplicate event hook %q", p.Name()))
	}
	eventHooks[p.Name()] = p
}

// ProblemSourceNames lists registered crawlers in stable order (tests + UI).
func ProblemSourceNames() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(problemSrcs))
	for name := range problemSrcs {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// SubmitFetcherNames lists registered platform adapters in stable order.
func SubmitFetcherNames() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(subFetchers))
	for name := range subFetchers {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// ProblemSourceByID resolves one crawler by plugin id.
func ProblemSourceByID(name string) (ProblemSource, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	p, ok := problemSrcs[name]
	return p, ok
}

// SubmitFetcherByID resolves one platform adapter by plugin id.
func SubmitFetcherByID(name string) (SubmitLogFetcher, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	p, ok := subFetchers[name]
	return p, ok
}

// EventHookByID resolves one lifecycle hook by plugin id.
func EventHookByID(name string) (EventHook, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	p, ok := eventHooks[name]
	return p, ok
}

// EmitJudgeEvent fans a finalized-submission payload out to every hook on a
// detached goroutine per hook — hooks are untrusted third parties w.r.t.
// the judging latency budget.
func EmitJudgeEvent(event map[string]any) {
	regMu.RLock()
	hooks := make([]EventHook, 0, len(eventHooks))
	for _, h := range eventHooks {
		hooks = append(hooks, h)
	}
	regMu.RUnlock()
	for _, h := range hooks {
		go func(h EventHook) {
			defer func() { _ = recover() }() // a panicking hook must not kill the API
			h.OnJudgeEvent(event)
		}(h)
	}
}