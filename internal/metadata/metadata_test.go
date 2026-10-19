package metadata

import (
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nodejs/doc-kit/internal/doc"
	"github.com/nodejs/doc-kit/internal/logger"
	"github.com/nodejs/doc-kit/internal/omap"
	. "github.com/nodejs/doc-kit/internal/testutil"
	"github.com/nodejs/doc-kit/internal/unist"
)

const docPath = "fs"

func h(text string, depth int) *unist.Node {
	return Heading(text, depth)
}

func yamlNode(content string) *unist.Node {
	return U("html", "<!-- YAML\n"+content+"\n-->")
}

func stability(text string) *unist.Node {
	return U("blockquote", Paragraph(Text(text)))
}

func parse(t *testing.T, file *doc.File, typeMap map[string]string) []*doc.Entry {
	t.Helper()

	entries, err := ParseAPIDoc(file, typeMap)
	if err != nil {
		t.Fatal(err)
	}

	return entries
}

func tree(children ...*unist.Node) *unist.Node {
	return U("root", children)
}

func findChild(n *unist.Node, typ string) *unist.Node {
	for _, c := range n.Children {
		if c.Type == typ {
			return c
		}
	}

	return nil
}

func findLink(e *doc.Entry) *unist.Node {
	if p := findChild(e.Content, "paragraph"); p != nil {
		return findChild(p, "link")
	}

	return nil
}

func TestParseAPIDoc(t *testing.T) {
	t.Run("produces one entry for a single heading", func(t *testing.T) {
		got := parse(t, &doc.File{Path: docPath, Tree: tree(h("fs", 1), Paragraph(Text("Content.")))}, nil)

		if len(got) != 1 {
			t.Errorf("got %d entries", len(got))
		}
	})

	t.Run("populates heading data with text and depth", func(t *testing.T) {
		got := parse(t, &doc.File{Path: docPath, Tree: tree(h("File System", 1))}, nil)

		if got[0].Heading.Data.Text != "File System" || got[0].Heading.Depth != 1 {
			t.Errorf("got %+v", got[0].Heading.Data)
		}
	})

	t.Run("produces one entry per heading, with their slugs", func(t *testing.T) {
		got := parse(t, &doc.File{Path: docPath, Tree: tree(
			h("First", 1), Paragraph(Text("Content A.")),
			h("Second", 1), Paragraph(Text("Content B.")),
		)}, nil)

		if len(got) != 2 || got[0].Heading.Data.Slug != "first" || got[1].Heading.Data.Slug != "second" {
			t.Errorf("got %d entries", len(got))
		}
	})

	t.Run("uses the DEP code as the slug of deprecations", func(t *testing.T) {
		got := parse(t, &doc.File{Path: "/deprecations", Tree: tree(h("DEP0001: http.OutgoingMessage.prototype.flush", 1))}, nil)

		if got[0].Heading.Data.Slug != "DEP0001" {
			t.Errorf("got %q", got[0].Heading.Data.Slug)
		}
	})

	t.Run("does not suffix a code after a similarly named heading", func(t *testing.T) {
		got := parse(t, &doc.File{Path: "/deprecations", Tree: tree(h("DEP0001", 1), h("DEP0001: deprecated API", 1))}, nil)

		if got[1].Heading.Data.Slug != "DEP0001" {
			t.Errorf("got %q", got[1].Heading.Data.Slug)
		}
	})

	t.Run("extracts YAML metadata", func(t *testing.T) {
		got := parse(t, &doc.File{Path: docPath, Tree: tree(h("oldMethod", 1), yamlNode("added: v1.0.0\ndeprecated: v2.0.0\nremoved: v3.0.0"))}, nil)

		if got[0].Prop("added") != "v1.0.0" || got[0].Prop("deprecated") != "v2.0.0" || got[0].Prop("removed") != "v3.0.0" {
			t.Errorf("got %v", got[0].Props)
		}
	})

	t.Run("extracts changes", func(t *testing.T) {
		got := parse(t, &doc.File{Path: docPath, Tree: tree(h("fs.readFile", 1), yamlNode(
			"added: v0.1.0\nchanges:\n  - version: v7.0.0\n    pr-url: https://github.com/nodejs/node/pull/7897\n    description: The callback is no longer optional.",
		))}, nil)

		changes := got[0].Prop("changes").([]any)
		change := changes[0].(*omap.Map)

		if len(changes) != 1 || change.Value("version") != "v7.0.0" || change.Value("pr-url") != "https://github.com/nodejs/node/pull/7897" {
			t.Errorf("got %v", changes)
		}
	})

	t.Run("extracts tags from a plain comment", func(t *testing.T) {
		got := parse(t, &doc.File{Path: docPath, Tree: tree(h("method", 1), U("html", "<!-- legacy -->"))}, nil)

		if !reflect.DeepEqual(got[0].Prop("tags"), []any{"legacy"}) {
			t.Errorf("got %v", got[0].Prop("tags"))
		}
	})

	t.Run("captures the stability index and description", func(t *testing.T) {
		got := parse(t, &doc.File{Path: docPath, Tree: tree(h("crypto", 1), stability("Stability: 1 - Experimental: This API is experimental."))}, nil)

		if d := got[0].Stability.Data; d.Index != "1" || d.Description != "Experimental: This API is experimental." {
			t.Errorf("got %+v", d)
		}
	})

	t.Run("ignores stability blockquotes in the documentation file", func(t *testing.T) {
		got := parse(t, &doc.File{Path: "/documentation", Tree: tree(h("Stability Index", 1), stability("Stability: 2 - Stable"))}, nil)

		if got[0].Stability != nil {
			t.Error("expected no stability")
		}
	})

	t.Run("has no stability without a blockquote", func(t *testing.T) {
		if got := parse(t, &doc.File{Path: docPath, Tree: tree(h("fs", 1))}, nil); got[0].Stability != nil {
			t.Error("expected no stability")
		}
	})

	t.Run("resolves link references to their definitions", func(t *testing.T) {
		got := parse(t, &doc.File{Path: docPath, Tree: tree(
			h("fs", 1),
			Paragraph(U("linkReference", P{"identifier": "ref", "referenceType": "full"}, Text("a link"))),
			U("definition", P{"identifier": "ref", "url": "https://example.com"}),
		)}, nil)

		if link := findLink(got[0]); link == nil || link.URL != "https://example.com" {
			t.Errorf("got %v", link)
		}

		if findChild(got[0].Content, "definition") != nil {
			t.Error("the definition should be removed")
		}
	})

	t.Run("resolves type annotations into links", func(t *testing.T) {
		got := parse(t, &doc.File{Path: docPath, Tree: tree(h("fs", 1), Paragraph(U("typeAnnotation", "string")))}, nil)
		annotation := findChild(findChild(got[0].Content, "paragraph"), "typeAnnotation")

		if len(annotation.Data.Links) != 1 || annotation.Data.Links[0].Text != "string" {
			t.Errorf("got %+v", annotation.Data)
		}
	})

	t.Run("makes type map links relative to the document", func(t *testing.T) {
		got := parse(t, &doc.File{Path: docPath, Tree: tree(h("fs", 1), Paragraph(U("typeAnnotation", "Buffer")))}, map[string]string{"Buffer": "buffer.html#class-buffer"})
		annotation := findChild(findChild(got[0].Content, "paragraph"), "typeAnnotation")

		if annotation.Data.Links[0].Href != "buffer.html#class-buffer" {
			t.Errorf("got %q", annotation.Data.Links[0].Href)
		}
	})

	t.Run("converts links to Markdown pages, keeping the hash", func(t *testing.T) {
		for url, want := range map[string]string{"events.md": "events.html", "events.md#some-section": "events.html#some-section"} {
			got := parse(t, &doc.File{Path: docPath, Tree: tree(h("fs", 1), Paragraph(U("link", P{"url": url}, Text("events"))))}, nil)

			if link := findLink(got[0]); link.URL != want {
				t.Errorf("got %q, want %q", link.URL, want)
			}
		}
	})

	t.Run("propagates the mdx flag, skipping type resolution", func(t *testing.T) {
		if got := parse(t, &doc.File{Path: docPath, Tree: tree(h("fs", 1))}, nil); got[0].MDX {
			t.Error("mdx should default to false")
		}

		got := parse(t, &doc.File{Path: docPath, MDX: true, Tree: tree(h("fs", 1), Paragraph(U("typeAnnotation", "string")))}, nil)
		annotation := findChild(findChild(got[0].Content, "paragraph"), "typeAnnotation")

		if !got[0].MDX || annotation.Data != nil {
			t.Error("mdx entries skip type resolution")
		}
	})

	t.Run("produces one entry for content without headings", func(t *testing.T) {
		if got := parse(t, &doc.File{Path: docPath, Tree: tree(Paragraph(Text("Just some text.")))}, nil); len(got) != 1 {
			t.Errorf("got %d entries", len(got))
		}
	})

	t.Run("produces nothing for an empty document", func(t *testing.T) {
		if got := parse(t, &doc.File{Path: docPath, Tree: tree()}, nil); len(got) != 0 {
			t.Errorf("got %d entries", len(got))
		}
	})

	t.Run("captures the nodes before the first heading in the first entry", func(t *testing.T) {
		got := parse(t, &doc.File{Path: docPath, Tree: tree(
			U("yaml", "layout: home"),
			h("First Heading", 1), Paragraph(Text("First content.")),
			h("Second Heading", 2), Paragraph(Text("Second content.")),
		)}, nil)

		first, second := got[0].Content.Children, got[1].Content.Children

		if len(got) != 2 || len(first) != 3 || first[0].Type != "yaml" || first[1].Type != "heading" || len(second) != 2 || second[0].Type != "heading" {
			t.Errorf("got %v, %v", first, second)
		}
	})
}

func TestNormalizeYAMLSyntax(t *testing.T) {
	got := NormalizeYAMLSyntax("introduced_in=v0.1.21\nsource_link=lib/test.js\ntype=module\nname=test_module\nllm_description=This is a test module")
	want := "introduced_in: v0.1.21\nsource_link: lib/test.js\ntype: module\nname: test_module\nllm_description: This is a test module"

	if got != want {
		t.Errorf("got %q", got)
	}

	if got := NormalizeYAMLSyntax("\nintroduced_in=v0.1.21\n"); got != "introduced_in: v0.1.21" {
		t.Errorf("got %q", got)
	}
}

func TestParseYAMLIntoMetadata(t *testing.T) {
	cases := []struct {
		in   string
		want *omap.Map
	}{
		{"name: test\ntype: module\nintroduced_in: v1.0.0", omap.New("name", "test", "type", "module", "introduced_in", "v1.0.0")},
		{"name: test\ntype: module\nintroduced_in: [v1.0.0, v1.1.0]", omap.New("name", "test", "type", "module", "introduced_in", []any{"v1.0.0", "v1.1.0"})},
		{"source_link: https://github.com/nodejs/node", omap.New("source_link", "https://github.com/nodejs/node")},
		{"napiVersion: 3", omap.New("napiVersion", float64(3))},
		{"legacy", omap.New("tags", []any{"legacy"})},
	}

	for _, c := range cases {
		got, err := ParseYAMLIntoMetadata(c.in)

		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("ParseYAMLIntoMetadata(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestLookupTypeName(t *testing.T) {
	cases := []struct {
		name    string
		typeMap map[string]string
		want    string
	}{
		{"string", map[string]string{"string": "override"}, "override"},
		{"string", nil, "https://developer.mozilla.org/en-US/docs/Web/JavaScript/Data_structures#string_type"},
		{"Array", nil, "https://developer.mozilla.org/docs/Web/JavaScript/Reference/Global_Objects/Array"},
		{"AbortSignal", nil, "https://developer.mozilla.org/docs/Web/API/AbortSignal"},
		{"zlib options", map[string]string{"zlib options": "zlib.html#options"}, "zlib.html#options"},
		{"vm.Module", nil, ""},
		{"NotAThing", nil, ""},
	}

	for _, c := range cases {
		if got := LookupTypeName(c.name, c.typeMap); got != c.want {
			t.Errorf("LookupTypeName(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestResolveTypeReference(t *testing.T) {
	cases := []struct {
		name    string
		typeMap map[string]string
		want    string
	}{
		{"vm.Module", nil, "vm.html#class-vmmodule"},
		{"tls.TLSSocket", nil, "tls.html#class-tlstlssocket"},
		{"vm.constants", nil, "vm.html#vmconstants"},
		{"os.EOL", nil, "os.html#oseol"},
		{"vm.Module", map[string]string{"vm.Module": "mapped"}, "mapped"},
		{"NotAThing", nil, ""},
		{"os.constants.dlopen", nil, "os.html#osconstantsdlopen"},
		{"Buffer or fs.Stats", nil, ""},
		{"Object.<string, string>", nil, ""},
	}

	for _, c := range cases {
		if got := ResolveTypeReference(c.name, c.typeMap); got != c.want {
			t.Errorf("ResolveTypeReference(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

// capture records the warnings logged while a function runs.
func capture(fn func()) []string {
	var b strings.Builder

	logger.Default.SetOutput(&b)
	logger.Default.SetTransport(func(w io.Writer, e logger.Entry) { io.WriteString(w, e.Message+"\n") })

	defer func() {
		logger.Default.SetOutput(os.Stdout)
		logger.Default.SetTransport(logger.ConsoleTransport)
	}()

	fn()

	if b.Len() == 0 {
		return nil
	}

	return strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
}
