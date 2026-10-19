package metadata

import "testing"

func TestRelative(t *testing.T) {
	cases := []struct{ to, from, want string }{
		// Absolute URLs are left alone
		{"https://example.com/a", "/fs", "https://example.com/a"},
		// Siblings from a top-level page
		{"/http", "/fs", "http"},
		{"/orama-db.json", "/index", "orama-db.json"},
		// A nested page from a top-level page
		{"/generators/web", "/index", "generators/web"},
		// Siblings within the same directory
		{"/generators/web", "/generators/ast", "web"},
		// A top-level page from a nested page
		{"/getting-started", "/generators/web", "../getting-started"},
		{"/orama-db.json", "/generators/web", "../orama-db.json"},
		// The target itself is never consumed as a common directory
		{"/fs", "/fs/promises", "../fs"},
		{"/generators", "/generators/web", "../generators"},
		{"/a/b", "/a/b/c", "../b"},
		{"/x", "/x/y/z", "../../x"},
		// The root
		{"/", "/index", "."},
		{"/", "/generators/web", ".."},
	}

	for _, c := range cases {
		if got := Relative(c.to, c.from); got != c.want {
			t.Errorf("Relative(%q, %q) = %q, want %q", c.to, c.from, got, c.want)
		}
	}
}
