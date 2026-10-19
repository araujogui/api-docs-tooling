package jsre

import "testing"

func TestTranslate(t *testing.T) {
	re := MustCompile(`^\s+(\S+)[\s:]$`)

	if !re.MatchString("\u00a0 word:") || !re.MatchString("\tword\u3000") {
		t.Error("\\s should match JavaScript's whitespace")
	}

	if re.MatchString("\u00a0wo\u00a0rd:") {
		t.Error("\\S should not match non-breaking spaces")
	}
}

func TestReplace(t *testing.T) {
	cases := []struct{ s, old, repl, want string }{
		{"a__X__b", "__X__", "c", "acb"},
		{"a__X__b", "__X__", "$&!", "a__X__!b"},
		{"a__X__b", "__X__", "$$", "a$b"},
		{"a__X__b", "__X__", "[$`|$']", "a[a|b]b"},
		{"a__X__b__X__", "__X__", "c", "acb__X__"},
	}

	for _, c := range cases {
		if got := Replace(c.s, c.old, c.repl); got != c.want {
			t.Errorf("Replace(%q, %q, %q) = %q, want %q", c.s, c.old, c.repl, got, c.want)
		}
	}

	if got := ReplaceAll("x-x-x", "x", "$&y"); got != "xy-xy-xy" {
		t.Errorf("got %q", got)
	}
}

func TestTrim(t *testing.T) {
	if got := Trim("\ufeff\u00a0 text \u2028"); got != "text" {
		t.Errorf("got %q", got)
	}
}
