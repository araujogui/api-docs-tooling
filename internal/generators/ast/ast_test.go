package ast

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/nodejs/doc-kit/internal/doc"
	"github.com/nodejs/doc-kit/internal/unist"
)

// parseFile writes content to a file in a temporary directory and parses it.
func parseFile(t *testing.T, name, content string) *doc.File {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, name)

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := ParseFile(filepath.ToSlash(path), filepath.ToSlash(dir))
	if err != nil {
		t.Fatal(err)
	}

	return f
}

func find(n *unist.Node, typ string) *unist.Node {
	for _, c := range n.Children {
		if c.Type == typ {
			return c
		}
	}

	return nil
}

func TestMDXDetection(t *testing.T) {
	cases := []struct {
		name, file, content string
		want                bool
	}{
		{"a plain .md file is Markdown", "plain.md", "# Title\n", false},
		{"a .mdx file is MDX", "page.mdx", "# Title\n", true},
		{"frontmatter `mdx: true` opts a .md file in", "optin.md", "---\nmdx: true\n---\n\n# Title\n", true},
		{"frontmatter `mdx: false` opts a .mdx file out", "optout.mdx", "---\nmdx: false\n---\n\n# Title\n", false},
		{"invalid frontmatter falls back to the extension", "weird.mdx", "---\n: : not: valid: yaml\n---\n\n# Title\n", true},
		{"frontmatter without mdx falls back to the extension", "nokey.md", "---\nadded: v1.0.0\n---\n\n# Title\n", false},
	}

	for _, c := range cases {
		if got := parseFile(t, c.file, c.content).MDX; got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

func TestParseFile(t *testing.T) {
	t.Run("strips the extension from the relative path", func(t *testing.T) {
		if got := parseFile(t, "fs.md", "# fs\n").Path; got != "/fs" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("returns a parsed tree", func(t *testing.T) {
		tree := parseFile(t, "tree.md", "# Heading\n").Tree

		if tree.Type != "root" || tree.Children[0].Type != "heading" {
			t.Errorf("got %v", tree)
		}
	})

	yamlBlock := regexp.MustCompile(`<!-- YAML\nadded: v1\.0\.0\n-->`)

	t.Run("rewrites frontmatter into a YAML comment", func(t *testing.T) {
		html := find(parseFile(t, "meta.md", "---\nadded: v1.0.0\n---\n\n# Title\n").Tree, "html")

		if html == nil || !yamlBlock.MatchString(html.Value) {
			t.Errorf("got %v", html)
		}
	})

	t.Run("re-attaches MDX frontmatter as a leading YAML comment", func(t *testing.T) {
		first := parseFile(t, "mdxmeta.mdx", "---\nadded: v1.0.0\n---\n\n# Title\n").Tree.Children[0]

		if first.Type != "html" || !yamlBlock.MatchString(first.Value) {
			t.Errorf("got %v", first)
		}
	})

	for _, name := range []string{"stab.md", "stab.mdx"} {
		t.Run("links the stability prefix in "+name, func(t *testing.T) {
			blockquote := find(parseFile(t, name, "# fs\n\n> Stability: 2 - Stable\n").Tree, "blockquote")

			if link := find(blockquote.Children[0], "link"); link == nil || link.URL != StabilityIndexURL {
				t.Errorf("got %v", link)
			}
		})
	}
}

func TestGlob(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"c.md", "a.md", "b.md", "skip.md", "other.txt"} {
		os.WriteFile(filepath.Join(dir, name), []byte("# x\n"), 0o644)
	}

	files, err := Glob([]string{filepath.ToSlash(dir) + "/*.md"}, []string{filepath.ToSlash(dir) + "/skip.md"})
	if err != nil {
		t.Fatal(err)
	}

	var names []string

	for _, f := range files {
		names = append(names, filepath.Base(f[0]))

		if f[1] != filepath.ToSlash(dir) {
			t.Errorf("parent %q", f[1])
		}
	}

	if len(names) != 3 || names[0] != "a.md" || names[2] != "c.md" {
		t.Errorf("got %v", names)
	}
}

func TestGlobParent(t *testing.T) {
	cases := map[string]string{
		"../node/doc/api/*.md": "../node/doc/api",
		"doc/**/*.md":          "doc",
		"*.md":                 ".",
		"doc/api/fs.md":        "doc/api",
		"/abs/path/{a,b}/x.md": "/abs/path",
	}

	for in, want := range cases {
		if got := globParent(in); got != want {
			t.Errorf("globParent(%q) = %q, want %q", in, got, want)
		}
	}
}
