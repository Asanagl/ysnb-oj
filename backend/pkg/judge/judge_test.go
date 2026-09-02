package judge

import "testing"

func TestEqualTokens(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"identical", "1 2 3\n", "1 2 3\n", true},
		{"trailing whitespace", "1 2 3   \n\n", "1 2 3\n", true},
		{"crlf vs lf", "1\r\n2\r\n", "1 2\n", true},
		{"different numbers", "1 2 3", "1 2 4", false},
		{"different count", "1 2 3", "1 2 3 4", false},
		{"empty vs space", "", "   \n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := equalTokens([]byte(tc.a), []byte(tc.b)); got != tc.want {
				t.Fatalf("equalTokens(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestAggregate(t *testing.T) {
	t.Run("all AC takes max time/mem", func(t *testing.T) {
		res := &Result{Cases: []CaseResult{
			{Index: 0, Status: "AC", TimeMS: 10, MemKB: 100},
			{Index: 1, Status: "AC", TimeMS: 42, MemKB: 250},
		}}
		aggregate(res)
		if res.Status != "AC" || res.TimeMS != 42 || res.MemKB != 250 {
			t.Fatalf("unexpected aggregate: %+v", res)
		}
	})
	t.Run("first non-AC in case order wins", func(t *testing.T) {
		res := &Result{Cases: []CaseResult{
			{Index: 2, Status: "TLE"},
			{Index: 0, Status: "WA"},
			{Index: 1, Status: "RE"},
		}}
		aggregate(res)
		if res.Status != "WA" {
			t.Fatalf("expected WA (lowest case index failing), got %s", res.Status)
		}
	})
	t.Run("SE beats everything", func(t *testing.T) {
		res := &Result{Cases: []CaseResult{
			{Index: 0, Status: "WA"},
			{Index: 1, Status: "SE"},
		}}
		aggregate(res)
		if res.Status != "SE" {
			t.Fatalf("expected SE, got %s", res.Status)
		}
	})
	t.Run("empty cases defaults AC", func(t *testing.T) {
		res := &Result{}
		aggregate(res)
		if res.Status != "AC" {
			t.Fatalf("expected AC for no cases, got %s", res.Status)
		}
	})
}

func TestRegistryLookup(t *testing.T) {
	reg, err := NewRegistry("")
	if err != nil {
		t.Fatalf("embedded registry: %v", err)
	}
	lang, err := reg.Get("cpp")
	if err != nil {
		t.Fatalf("cpp missing: %v", err)
	}
	if lang.Compile == nil || len(lang.Compile.Argv) == 0 {
		t.Fatal("cpp must have a compile profile")
	}
	// negative: unknown language must fail, not fall back silently
	if _, err := reg.Get("brainfuck"); err == nil {
		t.Fatal("unknown language should error")
	}
}

func TestBinCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cache, err := NewBinCache(dir)
	if err != nil {
		t.Fatalf("cache: %v", err)
	}
	src := dir + "/artifact"
	if err := writeFileForTest(src, []byte{0x7f, 'E', 'L', 'F'}); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.Get("k1"); ok {
		t.Fatal("empty cache must miss")
	}
	if _, err := cache.Put("k1", src); err != nil {
		t.Fatalf("put: %v", err)
	}
	p, ok := cache.Get("k1")
	if !ok {
		t.Fatal("cache must hit after put")
	}
	raw, _ := osRead(p)
	if len(raw) != 4 || raw[0] != 0x7f {
		t.Fatalf("artifact corrupted: %v", raw)
	}
}
