package fsutil

import "testing"

func TestWithExt(t *testing.T) {
	cases := []struct{ in, ext, want string }{
		{"file.md", "html", "file.html"},
		{"path/to/file.md", "json", "path/to/file.json"},
		{"file", "html", "file.html"},
		{"file.md", "", "file"},
		{"file.test.mjs", "js", "file.test.js"},
	}

	for _, c := range cases {
		if got := WithExt(c.in, c.ext); got != c.want {
			t.Errorf("WithExt(%q, %q) = %q, want %q", c.in, c.ext, got, c.want)
		}
	}
}
