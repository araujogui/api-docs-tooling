package legacyhtml

import (
	"regexp"
	"testing"

	"github.com/nodejs/doc-kit/internal/config"
	"github.com/nodejs/doc-kit/internal/doc"
	"github.com/nodejs/doc-kit/internal/omap"
	"github.com/nodejs/doc-kit/internal/unist"
)

func TestLegacySlugger(t *testing.T) {
	cases := []struct{ text, api, want string }{
		{"File System", "fs", "fs_file_system"},
		{"fs.readFile(path)", "fs", "fs_fs_readfile_path"},
		{"Hello", "fs", "fs_hello"},
		{"123 test", "0num", "_0num_123_test"},
		{"DEP0001: `http.OutgoingMessage.prototype.flush`", "deprecations", "DEP0001"},
		{"DEP0190: spawning .bat and .cmd files with child_process.spawn() with shell option", "deprecations", "DEP0190"},
	}

	for _, c := range cases {
		if got := NewLegacySlugger()(c.text, c.api); got != c.want {
			t.Errorf("slug(%q, %q) = %q, want %q", c.text, c.api, got, c.want)
		}
	}

	t.Run("deduplicates identical titles with a counter", func(t *testing.T) {
		slug := NewLegacySlugger()

		for _, want := range []string{"fs_hello", "fs_hello_1", "fs_hello_2"} {
			if got := slug("Hello", "fs"); got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		}

		if got := slug("World", "fs"); got != "fs_world" {
			t.Errorf("got %q", got)
		}
	})
}

func deprecationEntry(slug string) *doc.Entry {
	text := "DEP0001: deprecated API"
	heading := &unist.Node{Type: "heading", Depth: 3, Children: []*unist.Node{unist.Text(text)}, Data: &unist.Data{Name: text, Text: text, Slug: slug}}

	return &doc.Entry{API: "deprecations", Path: "/deprecations", Basename: "deprecations", Heading: heading, Content: unist.Root(heading), Props: omap.New()}
}

func TestBuildContent(t *testing.T) {
	ids := func(html, id string) int {
		return len(regexp.MustCompile(`id="`+id+`"`).FindAllString(html, -1))
	}

	t.Run("does not duplicate a legacy anchor matching the slug", func(t *testing.T) {
		if got := BuildContent(nil, []*doc.Entry{deprecationEntry("DEP0001")}, config.Options{}); ids(got, "DEP0001") != 1 {
			t.Errorf("got %s", got)
		}
	})

	t.Run("keeps a legacy anchor differing from the slug", func(t *testing.T) {
		got := BuildContent(nil, []*doc.Entry{deprecationEntry("dep0001-deprecated-api")}, config.Options{})

		if ids(got, "DEP0001") != 1 || ids(got, "dep0001-deprecated-api") != 1 {
			t.Errorf("got %s", got)
		}
	})
}

func TestHighlightCode(t *testing.T) {
	render := func(md string) string {
		e := deprecationEntry("x")
		e.Content.Children = append(e.Content.Children, parseCode(md)...)

		return BuildContent(nil, []*doc.Entry{e}, config.Options{})
	}

	t.Run("highlights code blocks with a toolbar", func(t *testing.T) {
		got := render("```js\nconst a = 1;\n```")

		for _, want := range []string{`<pre class="shiki shiki-themes catppuccin-latte catppuccin-mocha language-js"`, `tabindex="0"`, `<span class="line">`, `<span class="code-language">js</span>`} {
			if !regexp.MustCompile(regexp.QuoteMeta(want)).MatchString(got) {
				t.Errorf("missing %s in %s", want, got)
			}
		}
	})

	t.Run("switches between adjacent CommonJS and ES module blocks", func(t *testing.T) {
		got := render("```cjs\nconst a = require('a');\n```\n\n```mjs\nimport a from 'a';\n```")

		if !regexp.MustCompile(`<pre style="[^"]+" class="shiki"><input class="js-flavor-toggle" type="checkbox" checked><code class="[^"]*language-cjs cjs"`).MatchString(got) {
			t.Errorf("got %s", got)
		}

		if !regexp.MustCompile(`<span class="code-language">javascript</span>`).MatchString(got) {
			t.Error("missing the javascript toolbar")
		}
	})
}

func parseCode(md string) []*unist.Node {
	return markdownParse(md).Children
}
