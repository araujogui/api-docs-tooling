package markdown

import (
	"os"
	"testing"

	"github.com/nodejs/doc-kit/internal/unist"
)

// The golden files were produced by remark (remark-parse, remark-gfm and
// the type annotation plugin) with remark-rehype and rehype-stringify, or
// remark-stringify, which these functions port.

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}

func TestToHTMLGolden(t *testing.T) {
	tree := parseGFM(readFile(t, "testdata/kitchen-sink.md"))
	got := ToHTML(ToHast(tree, HastOptions{AllowDangerousHTML: true}), HTMLOptions{AllowDangerousHTML: true})

	if want := readFile(t, "testdata/kitchen-sink.html.golden"); got != want {
		t.Errorf("HTML differs from remark's:\n--- got\n%s\n--- want\n%s", got, want)
	}
}

func TestToMarkdownGolden(t *testing.T) {
	tree := parseGFM(readFile(t, "testdata/kitchen-sink.md"))

	// As the metadata stage leaves them: definitions dropped
	unist.Remove(tree, unist.IsType("definition"))

	if got, want := ToMarkdown(tree), readFile(t, "testdata/kitchen-sink.md.golden"); got != want {
		t.Errorf("Markdown differs from remark's:\n--- got\n%s\n--- want\n%s", got, want)
	}
}
