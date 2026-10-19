package metadata

import (
	"fmt"
	"github.com/nodejs/doc-kit/internal/jsre"
	"path"
	"regexp"
	"strings"

	"github.com/nodejs/doc-kit/internal/doc"
	"github.com/nodejs/doc-kit/internal/logger"
	"github.com/nodejs/doc-kit/internal/markdown"
	"github.com/nodejs/doc-kit/internal/omap"
	"github.com/nodejs/doc-kit/internal/queries"
	"github.com/nodejs/doc-kit/internal/slug"
	"github.com/nodejs/doc-kit/internal/unist"
)

// ignoreStabilityStems are the documents defining the stability indexes,
// which are therefore not checked for them.
var ignoreStabilityStems = []string{"documentation"}

// Relative makes a URL relative to a page, like a URL resolution would (URLs
// with a scheme are left alone).
func Relative(to, from string) string {
	if strings.Contains(to, "://") {
		return to
	}

	a := splitNonEmpty(to)
	fromParts := strings.Split(from, "/")
	b := splitNonEmpty(strings.Join(fromParts[:max(len(fromParts)-1, 0)], "/"))

	// The last segment of `to` is the target itself, never a common prefix
	var file string

	if len(a) > 0 {
		file = a[len(a)-1]
		a = a[:len(a)-1]
	}

	for len(a) > 0 && len(b) > 0 && a[0] == b[0] {
		a = a[1:]
		b = b[1:]
	}

	var parts []string

	for range b {
		parts = append(parts, "..")
	}

	parts = append(parts, a...)

	if file != "" {
		parts = append(parts, file)
	}

	if rel := strings.Join(parts, "/"); rel != "" {
		return rel
	}

	return "."
}

func splitNonEmpty(s string) []string {
	var out []string

	for _, part := range strings.Split(s, "/") {
		if part != "" {
			out = append(out, part)
		}
	}

	return out
}

// ParseAPIDoc splits a parsed file into metadata entries, one per heading.
func ParseAPIDoc(file *doc.File, typeMap map[string]string) ([]*doc.Entry, error) {
	tree := file.Tree
	var entries []*doc.Entry

	slugger := slug.NewSlugger()

	// The path is unique, so a plain slug will do
	api := slug.GitHub(strings.Replace(strings.TrimPrefix(file.Path, "/"), "/", "-", 1))

	headingSlug := func(text string) string {
		if m := DeprecationHeading.FindStringSubmatch(text); m != nil {
			return m[1]
		}

		return slugger.Node(text)
	}

	definitions := unist.SelectAll(tree, "definition")
	headings := unist.SelectAll(tree, "heading")

	// Link references become plain links
	unist.Visit(tree, queries.IsLinkReference, func(n *unist.Node, _ int, _ *unist.Node) unist.Step {
		visitLinkReference(n, definitions)

		return unist.SkipStep
	})

	// The definitions aren't needed anymore
	unist.Remove(tree, unist.OneOf(definitions))

	// Type map links are relative to this page
	relativeTypeMap := make(map[string]string, len(typeMap))

	for k, v := range typeMap {
		relativeTypeMap[k] = Relative(v, file.Path)
	}

	// MDX trees hold no type annotations (`{...}` is an expression there)
	if !file.MDX {
		ResolveTypeAnnotations(tree, relativeTypeMap, file.Path)
	}

	// Links to Markdown pages point at their HTML pages
	unist.Visit(tree, queries.IsMarkdownURL, func(n *unist.Node, _ int, _ *unist.Node) unist.Step {
		n.URL, _ = queries.RewriteMarkdownURL(n.URL)

		return unist.SkipStep
	})

	// A document with content but no headings gets a hidden one
	if len(headings) == 0 && len(tree.Children) > 0 {
		tree.Children = append([]*unist.Node{{Type: "heading", Depth: 1, Children: []*unist.Node{}}}, tree.Children...)
	}

	ignoreStability := false

	for _, stem := range ignoreStabilityStems {
		if stem == api {
			ignoreStability = true
		}
	}

	var visitErr error

	unist.Visit(tree, queries.IsHeading, func(heading *unist.Node, index int, _ *unist.Node) unist.Step {
		heading.Data = TransformNodeToHeading(heading)

		entry := &doc.Entry{
			API:      api,
			Path:     file.Path,
			MDX:      file.MDX,
			Basename: path.Base(file.Path),
			Heading:  heading,
			Props:    omap.New(),
		}

		heading.Data.Slug = headingSlug(heading.Data.Text)

		// The section runs up to the next heading
		stop := len(tree.Children)

		if next := unist.FindAfter(tree, index, queries.IsHeading); next != nil {
			stop = tree.IndexOf(next)
		}

		// The first section starts at the top, to take the nodes before it
		start := index

		if len(entries) == 0 {
			start = 0
		}

		subTree := unist.Root(append([]*unist.Node{}, tree.Children[start:stop]...)...)

		unist.Visit(subTree, queries.IsStabilityNode, func(n *unist.Node, _ int, _ *unist.Node) unist.Step {
			if ignoreStability {
				visitStability(n, nil)
			} else {
				visitStability(n, entry)
			}

			return unist.SkipStep
		})

		unist.Visit(subTree, queries.IsYAMLNode, func(n *unist.Node, _ int, _ *unist.Node) unist.Step {
			if err := visitYAML(n, entry); err != nil {
				logger.Default.Warn(fmt.Sprintf("Invalid YAML metadata: %v", err), logger.Fields{File: &logger.File{Path: file.Path}})
			}

			return unist.Next
		})

		// A YAML `type` overrides the heading's
		if t, ok := entry.Prop("type").(string); ok && t != "" {
			heading.Data.HeadType = t
		}

		unist.Visit(subTree, queries.IsTextWithUnixManual, func(n *unist.Node, _ int, parent *unist.Node) unist.Step {
			visitTextWithUnixManual(n, parent)

			return unist.SkipStep
		})

		unist.Remove(subTree, queries.IsYAMLNode)

		entry.Content = subTree
		entries = append(entries, entry)

		return unist.SkipStep
	})

	return entries, visitErr
}

func visitLinkReference(n *unist.Node, definitions []*unist.Node) {
	n.Type = "link"
	n.Title = nil

	for _, d := range definitions {
		if d.Identifier == n.Identifier {
			n.URL = d.URL

			return
		}
	}
}

var (
	leadingSpace  = jsre.MustCompile(`^\s`)
	trailingSpace = jsre.MustCompile(`\s$`)
)

// visitTextWithUnixManual turns the references to Unix manuals in a text
// node into links.
func visitTextWithUnixManual(n *unist.Node, parent *unist.Node) {
	replaced := queries.UnixManualPage.ReplaceAllStringFunc(n.Value, func(match string) string {
		m := queries.UnixManualPage.FindStringSubmatch(match)

		return TransformUnixManualToLink(m[0], m[1], m[2], m[3])
	})

	// Remark drops leading and trailing spaces, so they become entities
	replaced = leadingSpace.ReplaceAllString(replaced, "&nbsp;")
	replaced = trailingSpace.ReplaceAllString(replaced, "&nbsp;")

	tree := markdown.Parse(replaced, markdown.Options{GFM: true, TypeAnnotations: true})

	var children []*unist.Node

	if len(tree.Children) > 0 {
		children = tree.Children[0].Children
	}

	if i := parent.IndexOf(n); i >= 0 {
		parent.Splice(i, 1, children...)
	}
}

var newlines = regexp.MustCompile(`\n`)

// visitStability records a stability index on its node, and on the entry.
func visitStability(n *unist.Node, entry *doc.Entry) {
	if len(n.Children) == 0 {
		return
	}

	prefix := unist.NodesToString(n.Children[0].Children, false)
	m := queries.StabilityIndex.FindStringSubmatch(prefix)

	if m == nil {
		return
	}

	n.Data = &unist.Data{
		IsStability: true,
		Index:       m[1],
		Description: jsre.Trim(newlines.ReplaceAllString(m[2], " ")),
	}

	if entry != nil {
		entry.Stability = n
	}
}

// visitYAML merges a YAML block's properties into an entry: lists add up,
// other values replace.
func visitYAML(n *unist.Node, entry *doc.Entry) error {
	meta, err := ParseYAMLIntoMetadata(ExtractYAMLContent(n))
	if err != nil {
		return err
	}

	merge := func(key string, value any) {
		if existing, ok := entry.Props.Value(key).([]any); ok {
			switch v := value.(type) {
			case []any:
				entry.Props.Set(key, append(existing, v...))
			case string:
				for _, r := range v {
					existing = append(existing, string(r))
				}

				entry.Props.Set(key, existing)
			}

			return
		}

		entry.Props.Set(key, value)
	}

	switch m := meta.(type) {
	case *omap.Map:
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			merge(k, v)
		}
	case []any:
		for i, v := range m {
			merge(fmt.Sprint(i), v)
		}
	}

	return nil
}
