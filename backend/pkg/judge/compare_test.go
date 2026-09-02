package judge

import (
	"testing"
)

// The streaming token comparison must be exactly equivalent to the old
// strings.Fields-based semantics: split on any whitespace run, compare token
// sequences. judge_test.go has the basic table; this adds tricky shapes.
func TestEqualTokensEdgeCases(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"identical", "1 2 3\n", "1 2 3\n", true},
		{"diff separators", "1  2\t3", "1 2 3", true},
		{"trailing ws", "1 2 3", "1 2 3   \n", true},
		{"leading ws", "  1 2", "1 2", true},
		{"empty vs empty", "", "", true},
		{"empty vs ws", "", "   \n  ", true},
		{"ws vs ws", "\t\n", " ", true},
		{"different token", "1 2 3", "1 2 4", false},
		{"prefix token", "12", "123", false},
		{"token split by ws", "1 23", "12 3", false},
		{"extra token a", "1 2 3", "1 2", false},
		{"extra token b", "1 2", "1 2 3", false},
		{"newline inside token", "1\n2", "1 2", true},
		{"crlf", "1\r\n2", "1 2", true},
		{"mixed whitespace runs", "1 \t \n 2", "1 2", true},
		{"empty vs nonempty", "", "1", false},
		{"nonempty vs empty", "1", "", false},
		{"long equal", "a b c d e f g h", "a b c d e f g h", true},
		{"numbers with signs", "-1 +2", "-1 +2", true},
	}
	for _, tc := range cases {
		if got := equalTokens([]byte(tc.a), []byte(tc.b)); got != tc.want {
			t.Errorf("%s: equalTokens(%q, %q) = %v, want %v", tc.name, tc.a, tc.b, got, tc.want)
		}
	}
}

// Cross-check the streaming implementation against the old Fields-based
// semantics on a deterministic pseudo-random corpus.
func TestEqualTokensAgainstReference(t *testing.T) {
	ref := func(a, b []byte) bool {
		return joinFields(a) == joinFields(b)
	}
	// xorshift corpus: generate whitespace/noise strings
	state := uint64(12345)
	next := func() uint64 { state ^= state << 13; state ^= state >> 7; state ^= state << 17; return state }
	tok := []string{"", " ", "\t", "\n", "\r\n", "1", "42", "abc", "x y", " ", "  ", "\v", "\f", "z"}
	for i := 0; i < 3000; i++ {
		gen := func() string {
			n := int(next()%8) + 1
			s := ""
			for j := 0; j < n; j++ {
				s += tok[next()%uint64(len(tok))]
			}
			return s
		}
		a, b := gen(), gen()
		if i%3 == 0 {
			b = a // force equal pair sometimes
		}
		want := ref([]byte(a), []byte(b))
		if got := equalTokens([]byte(a), []byte(b)); got != want {
			t.Fatalf("corpus %d: streaming=%v reference=%v for %q vs %q", i, got, want, a, b)
		}
	}
}

func joinFields(b []byte) string {
	out := ""
	inTok := false
	for _, c := range b {
		space := c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
		if space {
			inTok = false
			continue
		}
		if !inTok && out != "" {
			out += " "
		}
		out += string(c)
		inTok = true
	}
	return out
}
