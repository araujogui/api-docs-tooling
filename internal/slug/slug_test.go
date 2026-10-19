package slug

import "testing"

func TestReplacements(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{`replaces "node.js" with "nodejs"`, "node.js", "nodejs"},
		{"is case-insensitive", "Node.JS", "nodejs"},
		{"replaces & with -and-", "a&b", "a-and-b"},
		{"preserves underscores", "foo_bar", "foo_bar"},
		{"replaces forward slashes", "foo/bar", "foo-bar"},
		{"replaces colons", "foo:bar", "foo-bar"},
		{"replaces commas", "foo,bar", "foo-bar"},
		{"replaces semicolons", "foo;bar", "foo-bar"},
		{"removes a single leading hyphen", "-foo", "foo"},
		{"preserves multiple leading hyphens", "--permission", "--permission"},
		{"preserves an all-hyphen string", "---", "---"},
		{"removes a single trailing hyphen", "foo-", "foo"},
		{"removes multiple trailing hyphens", "foo--", "foo"},
		{"replaces up to a double hyphen with one", "foo--bar", "-bar"},
		{"does not collapse leading double hyphen flags", "--allow-fs-read", "--allow-fs-read"},
	}

	for _, c := range cases {
		if got := ApplyReplacements(c.in); got != c.want {
			t.Errorf("%s: ApplyReplacements(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestNode(t *testing.T) {
	cases := map[string]string{
		"Hello World":     "hello-world",
		"child_process":   "child_process",
		"stability index": "stability-index",
		"Class: `fs.Dir`": "class-fsdir",
	}

	for in, want := range cases {
		if got := Node(in); got != want {
			t.Errorf("Node(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSlugger(t *testing.T) {
	t.Run("deduplicates the same slug with a counter", func(t *testing.T) {
		s := NewSlugger()

		for _, want := range []string{"hello", "hello-1", "hello-2"} {
			if got := s.Node("Hello"); got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		}
	})

	t.Run("does not affect different titles", func(t *testing.T) {
		s := NewSlugger()

		if s.Node("First") != "first" || s.Node("Second") != "second" {
			t.Error("unexpected slugs")
		}
	})

	t.Run("each instance has independent state", func(t *testing.T) {
		NewSlugger().Node("Hello")

		if got := NewSlugger().Node("Hello"); got != "hello" {
			t.Errorf("got %q", got)
		}
	})
}
