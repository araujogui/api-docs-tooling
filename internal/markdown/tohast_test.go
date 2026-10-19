package markdown

import (
	"testing"

	"github.com/nodejs/doc-kit/internal/unist"
)

func TestNormalizeURI(t *testing.T) {
	cases := map[string]string{
		"https://example.com/a b": "https://example.com/a%20b",
		"#café":                   "#caf%C3%A9",
		"/path?q=1&r=2":           "/path?q=1&r=2",
		"100%":                    "100%25",
		"%2F":                     "%2F",
		"a<b>[c]":                 "a%3Cb%3E%5Bc%5D",
	}

	for in, want := range cases {
		if got := NormalizeURI(in); got != want {
			t.Errorf("NormalizeURI(%q) = %q, want %q", in, got, want)
		}
	}
}

func typeAnnotationHTML(value string, links []unist.TypeLink) string {
	n := &unist.Node{Type: "typeAnnotation", Value: value}

	if links != nil {
		n.Data = &unist.Data{Links: links}
	}

	return AllToHTML(TypeAnnotationToHast(nil, n, nil), HTMLOptions{})
}

func TestTypeAnnotationToHast(t *testing.T) {
	got := typeAnnotationHTML("Buffer|string", []unist.TypeLink{
		{Start: 0, End: 6, Text: "Buffer", Href: "buffer.html#class-buffer"},
		{Start: 7, End: 13, Text: "string", Href: "mdn.io/string"},
	})

	want := `<code class="type"><a href="buffer.html#class-buffer" class="type-link">Buffer</a>|<a href="mdn.io/string" class="type-link">string</a></code>`

	if got != want {
		t.Errorf("got %s", got)
	}

	for _, links := range [][]unist.TypeLink{{}, nil} {
		if got := typeAnnotationHTML("SomethingUnknown", links); got != `<code class="type">SomethingUnknown</code>` {
			t.Errorf("got %s", got)
		}
	}
}

func TestH(t *testing.T) {
	el := H("a.mark.other#slug", nil, "text", 3)

	if got := ToHTML(el, HTMLOptions{}); got != `<a class="mark other" id="slug">text3</a>` {
		t.Errorf("got %s", got)
	}
}

func TestSerialization(t *testing.T) {
	el := Element("input", nil)
	el.Properties.Set("type", "checkbox")
	el.Properties.Set("checked", true)
	el.Properties.Set("disabled", false)
	el.Properties.Set("dataFootnoteRef", true)
	el.Properties.Set("title", `a "quoted" & 'single'`)

	want := `<input type="checkbox" checked data-footnote-ref title="a &#x22;quoted&#x22; &#x26; &#x27;single&#x27;">`

	if got := ToHTML(el, HTMLOptions{}); got != want {
		t.Errorf("got %s", got)
	}

	if got := ToHTML(unist.Root(unist.Text("<a & b>"), &unist.Node{Type: "raw", Value: "<b>"}), HTMLOptions{AllowDangerousHTML: true}); got != "&#x3C;a &#x26; b><b>" {
		t.Errorf("got %s", got)
	}
}
