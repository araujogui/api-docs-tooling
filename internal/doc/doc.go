// Package doc holds the documents the generators pass along: parsed files,
// and the metadata entries they are split into.
package doc

import (
	"regexp"
	"slices"
	"strings"

	"github.com/nodejs/doc-kit/internal/omap"
	"github.com/nodejs/doc-kit/internal/unist"
)

// File is a parsed Markdown file.
type File struct {
	// Tree is the file's mdast tree.
	Tree *unist.Node
	// Path is the file's path relative to its input, without extension
	// (e.g. `/fs`).
	Path string
	// MDX is set for files authored as MDX.
	MDX bool
	// Source is the file's path on disk.
	Source string
}

// Entry is a section of a document: a heading and its content, with the
// metadata attached to it.
type Entry struct {
	API       string
	Path      string
	Basename  string
	MDX       bool
	Synthetic bool
	// Heading is the section's heading, with its Data filled in.
	Heading *unist.Node
	// Stability is the section's stability index blockquote, if any.
	Stability *unist.Node
	// Content is the section's content (a root), heading included.
	Content *unist.Node
	// Props are the section's YAML properties, in order.
	Props *omap.Map
}

// Prop returns a YAML property (nil when unset).
func (e *Entry) Prop(key string) any {
	return e.Props.Value(key)
}

// HasProp reports whether a YAML property is set.
func (e *Entry) HasProp(key string) bool {
	return e.Props.Has(key) && e.Props.Value(key) != nil
}

// StringProp returns a string YAML property ("" when unset or not a string).
func (e *Entry) StringProp(key string) string {
	s, _ := e.Prop(key).(string)

	return s
}

// Tags returns the section's tags.
func (e *Entry) Tags() []any {
	tags, _ := e.Prop("tags").([]any)

	return tags
}

// HeadingData returns the heading's data.
func (e *Entry) HeadingData() *unist.Data {
	if e.Heading.Data == nil {
		e.Heading.Data = &unist.Data{}
	}

	return e.Heading.Data
}

// Clone deep-copies an entry, so a generator can edit it.
func (e *Entry) Clone() *Entry {
	c := *e
	c.Content = e.Content.Clone()

	// The heading and the stability index live in the content; keep pointing
	// at their copies
	c.Heading = findCopy(e.Content, c.Content, e.Heading)
	c.Stability = findCopy(e.Content, c.Content, e.Stability)

	if e.Props != nil {
		c.Props = e.Props.Clone()
	}

	return &c
}

// findCopy finds, in a copied tree, the copy of a node of the original.
func findCopy(original, copied, node *unist.Node) *unist.Node {
	if node == nil {
		return nil
	}

	if original == node {
		return copied
	}

	for i, child := range original.Children {
		if i < len(copied.Children) {
			if found := findCopy(child, copied.Children[i], node); found != nil {
				return found
			}
		}
	}

	return nil
}

// CloneEntries deep-copies entries.
func CloneEntries(entries []*Entry) []*Entry {
	out := make([]*Entry, len(entries))

	for i, e := range entries {
		out[i] = e.Clone()
	}

	return out
}

// GroupByModule groups entries by their `api`, in order of first appearance.
func GroupByModule(entries []*Entry) ([]string, map[string][]*Entry) {
	var order []string
	groups := map[string][]*Entry{}

	for _, e := range entries {
		if _, ok := groups[e.API]; !ok {
			order = append(order, e.API)
		}

		groups[e.API] = append(groups[e.API], e)
	}

	return order, groups
}

// Hierarchized is an entry with the entries nested under it.
type Hierarchized struct {
	Entry    *Entry
	Children []*Hierarchized
}

// BuildHierarchy nests entries under the closest preceding shallower
// heading.
func BuildHierarchy(entries []*Entry) []*Hierarchized {
	var roots []*Hierarchized

	nodes := make([]*Hierarchized, len(entries))

	for i, e := range entries {
		nodes[i] = &Hierarchized{Entry: e, Children: []*Hierarchized{}}
	}

	for i, node := range nodes {
		var parent *Hierarchized

		if node.Entry.Heading.Depth > 1 {
			parent = FindParent(node.Entry, nodes, i-1)
		}

		if parent != nil {
			parent.Children = append(parent.Children, node)
		} else {
			roots = append(roots, node)
		}
	}

	return roots
}

// FindParent finds the closest preceding node whose heading is shallower
// than the entry's.
func FindParent(entry *Entry, nodes []*Hierarchized, start int) *Hierarchized {
	for i := start; i >= 0; i-- {
		if nodes[i].Entry.Heading.Depth < entry.Heading.Depth {
			return nodes[i]
		}
	}

	return nil
}

var overloadableTypes = []string{"method", "ctor", "classMethod"}

// AnnotateOverloads flags the 2nd..nth heading of each run of headings
// documenting the same function, pointing them at the run's first heading.
func AnnotateOverloads(entries []*Entry) []*Entry {
	for i := 0; i < len(entries); i++ {
		heading := entries[i].Heading

		if heading.Data == nil || !slices.Contains(overloadableTypes, heading.Data.HeadType) {
			continue
		}

		end := i + 1

		for end < len(entries) && sameFunction(entries[end].Heading, heading) {
			entries[end].Heading.Data.IsOverload = true
			entries[end].Heading.Data.OverloadOf = heading.Data.Slug
			end++
		}

		i = end - 1
	}

	return entries
}

func sameFunction(a, b *unist.Node) bool {
	return a.Data != nil && b.Data != nil && a.Depth == b.Depth && a.Data.HeadType == b.Data.HeadType && a.Data.Name == b.Data.Name
}

var lineBreaks = regexp.MustCompile(`[\r\n]+`)

// EntryDescription is an entry's `llm_description`, or the text of its
// first paragraph.
func EntryDescription(e *Entry) string {
	if d := e.StringProp("llm_description"); d != "" {
		return strings.TrimSpace(d)
	}

	for _, child := range e.Content.Children {
		if child.Type == "paragraph" {
			return lineBreaks.ReplaceAllString(unist.NodeToString(child, false), "")
		}
	}

	return ""
}
