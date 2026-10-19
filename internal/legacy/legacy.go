// Package legacy holds what the legacy HTML and JSON generators share.
package legacy

import (
	"github.com/nodejs/doc-kit/internal/markdown"
	"github.com/nodejs/doc-kit/internal/omap"
	"github.com/nodejs/doc-kit/internal/unist"
)

// HastOptions are the mdast to hast options of the legacy generators: raw
// HTML is kept (the sources are trusted), and the hast elements they build
// into the tree pass through.
var HastOptions = markdown.HastOptions{
	AllowDangerousHTML: true,
	PassThrough:        []string{"element"},
}

// RenderHTML renders a tree (mdast, possibly holding hast elements) into an
// HTML string.
func RenderHTML(tree *unist.Node) string {
	return markdown.ToHTML(markdown.ToHast(tree, HastOptions), markdown.HTMLOptions{AllowDangerousHTML: true})
}

// RenderMarkdown parses Markdown (without extensions) and renders it.
func RenderMarkdown(source string) string {
	return RenderHTML(markdown.Parse(source, markdown.Options{}))
}

// ToJSON serializes a legacy section with its keys in the order of the
// original JSON output; the bundle (with no `api`) has an order of its own.
func ToJSON(section *omap.Map, indent int) ([]byte, error) {
	var out *omap.Map

	if !section.Has("api") || section.Value("api") == nil {
		out = pick(section, "miscs", "modules", "classes", "globals", "methods")
	} else if section.Value("api") == "index" {
		out = pick(section, "type", "source", "introduced_in", "meta", "stability", "stabilityText", "classes", "methods", "properties", "miscs", "globals")
	} else {
		out = pick(section, "type", "source", "introduced_in", "meta", "stability", "stabilityText", "classes", "methods", "properties", "miscs", "modules", "globals")
	}

	return omap.Stringify(out, indent)
}

func pick(m *omap.Map, keys ...string) *omap.Map {
	out := omap.New()

	for _, k := range keys {
		if v, ok := m.Get(k); ok {
			out.Set(k, v)
		} else {
			out.Set(k, omap.Undefined)
		}
	}

	return out
}
