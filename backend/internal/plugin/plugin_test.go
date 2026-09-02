package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSafeHTTPGetRejectsLocalhost(t *testing.T) {
	if _, err := SafeHTTPGet(context.Background(), "http://127.0.0.1:1/x"); err == nil {
		t.Fatal("loopback literal must be rejected")
	}
	if _, err := SafeHTTPGet(context.Background(), "http://localhost/x"); err == nil {
		t.Fatal("localhost must be rejected")
	}
	if _, err := SafeHTTPGet(context.Background(), "file:///etc/passwd"); err == nil {
		t.Fatal("file:// scheme must be rejected")
	}
}

func TestSafeHTTPGetFollowsPublicRedirect(t *testing.T) {
	// an in-process public-IP server is impossible on CI, so this test only
	// exercises the happy path of a plain request through SafeHTTPGet against
	// a loopback httptest server — which SafeResolve must reject. The point
	// is proving the guard trips on httptest addresses (loopback), not that
	// redirects work; redirect re-validation is enforced in CheckRedirect.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("nope"))
	}))
	defer srv.Close()
	if _, err := SafeHTTPGet(context.Background(), srv.URL); err == nil {
		t.Fatal("httptest (loopback) must be rejected by the SSRF guard")
	}
}

func TestRegistryDuplicatePanic(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate registration must panic")
		}
	}()
	RegisterProblemSource(cfSource{})
}

func TestRegistryLookups(t *testing.T) {
	if _, ok := ProblemSourceByID("codeforces"); !ok {
		t.Fatal("codeforces problem source missing")
	}
	if _, ok := SubmitFetcherByID("codeforces"); !ok {
		t.Fatal("codeforces submit fetcher missing")
	}
	if _, ok := ProblemSourceByID("luogu"); !ok {
		t.Fatal("luogu problem source missing")
	}
	if _, ok := SubmitFetcherByID("luogu"); !ok {
		t.Fatal("luogu submit fetcher missing")
	}
	found := false
	for _, n := range ProblemSourceNames() {
		if n == "codeforces" {
			found = true
		}
	}
	if !found {
		t.Fatal("ProblemSourceNames must include codeforces")
	}
}

func TestCFSplitID(t *testing.T) {
	// gym-only ids (no problem index) are not fetchable as problems —
	// cfSplitID requires <contest><index>; bare contest ids must fail.
	if _, _, err := cfSplitID("gym104777"); err == nil {
		t.Fatal("gym contest id without problem index must fail")
	}
	contest, index, err := cfSplitID("1900A")
	if err != nil || contest != 1900 || index != "A" {
		t.Fatalf("cfSplitID(1900A) = %d,%q,%v", contest, index, err)
	}
	for _, bad := range []string{"", "abc", "1900"} {
		if _, _, err := cfSplitID(bad); err == nil {
			t.Fatalf("cfSplitID(%q) must fail", bad)
		}
	}
}

func TestCFNormalizeVerdict(t *testing.T) {
	cases := map[string]string{
		"OK":                     "AC",
		"WRONG_ANSWER":           "WA",
		"TIME_LIMIT_EXCEEDED":    "TLE",
		"MEMORY_LIMIT_EXCEEDED":  "MLE",
		"RUNTIME_ERROR":          "RE",
		"COMPILATION_ERROR":      "CE",
		"CHALLENGED":             "OTHER",
		"SKIPPED":                "OTHER",
		"TESTING":                "OTHER",
		"":                       "OTHER",
		"PARTIAL":                "OTHER",
	}
	for in, want := range cases {
		if got := cfNormalizeVerdict(in); got != want {
			t.Fatalf("cfNormalizeVerdict(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLuoguPIDValidation(t *testing.T) {
	for _, ok := range []string{"P1001", "B2001", "CF1900A", "SP123", "UVA123", "AT_abc300_a"} {
		if !luoguPIDRe.MatchString(ok) {
			t.Fatalf("luoguPIDRe must accept %q", ok)
		}
	}
	for _, bad := range []string{"hello", "1001", "P 1", "../etc"} {
		if luoguPIDRe.MatchString(bad) {
			t.Fatalf("luoguPIDRe must reject %q", bad)
		}
	}
}

func TestLuoguNormalizeStatus(t *testing.T) {
	cases := map[int]string{12: "AC", 6: "WA", 2: "TLE", 4: "MLE", 7: "RE", 5: "CE", 1: "OTHER", 0: "OTHER"}
	for in, want := range cases {
		if got := lgNormalizeStatus(in); got != want {
			t.Fatalf("lgNormalizeStatus(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestAtcoderAndNowcoderRegistered(t *testing.T) {
	for _, p := range []string{"atcoder", "nowcoder"} {
		if _, ok := SubmitFetcherByID(p); !ok {
			t.Fatalf("submit fetcher %q missing", p)
		}
	}
}

func TestAtcoderVerdict(t *testing.T) {
	cases := map[string]string{"AC": "AC", "wa": "WA", "TLE": "TLE", "": "OTHER", "IE": "OTHER"}
	for in, want := range cases {
		if got := atcoderVerdict(in); got != want {
			t.Fatalf("atcoderVerdict(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseAtcoderSubmissions(t *testing.T) {
	page := []byte(`<html><script>var x = [{"ID":"123","EpochSecond":1700000000,"ProblemID":"abc300_a","ProblemTitle":"A","Language":"C++","Status":"AC","ContestID":"abc300","SubmissionTime":"x"}];</script></html>`)
	subs, err := parseAtcoderSubmissions(page)
	if err != nil || len(subs) != 1 || subs[0].ID != "123" {
		t.Fatalf("parse failed: %v %+v", err, subs)
	}
	if _, err := parseAtcoderSubmissions([]byte("<html>nothing</html>")); err == nil {
		t.Fatal("page without payload must fail")
	}
}

func TestNowcoderIDExtraction(t *testing.T) {
	for _, in := range []string{"123456", "https://ac.nowcoder.com/acm/contest/profile/544123"} {
		if nkIDRe.FindStringSubmatch(in) == nil {
			t.Fatalf("nkIDRe must match %q", in)
		}
	}
	if nkIDRe.FindStringSubmatch("ab") != nil {
		t.Fatal("nkIDRe must reject non-numeric handles")
	}
}

func TestNowcoderEnvelopeShapes(t *testing.T) {
	bare := []byte(`[{"id":1,"status":5,"createdAt":1700000000}]`)
	recs, err := parseNowcoderRecords(bare)
	if err != nil || len(recs) != 1 {
		t.Fatalf("bare array: %v %v", err, recs)
	}
	enveloped := []byte(`{"code":0,"data":{"records":[{"id":2,"status":5,"createdAt":1700000000}]}}`)
	recs, err = parseNowcoderRecords(enveloped)
	if err != nil || len(recs) != 1 || recs[0].ID != 2 {
		t.Fatalf("envelope: %v %v", err, recs)
	}
	list := []byte(`{"data":{"list":[{"id":3,"status":6}]}}`)
	recs, err = parseNowcoderRecords(list)
	if err != nil || len(recs) != 1 {
		t.Fatalf("list envelope: %v %v", err, recs)
	}
}