// Package queries holds the regular expressions and node tests the
// generators use to recognize API documentation constructs.
package queries

import (
	"github.com/nodejs/doc-kit/internal/jsre"
	"regexp"
	"strings"
	"unicode"

	"github.com/nodejs/doc-kit/internal/unist"
)

var (
	// MarkdownURL matches references to Markdown pages (see IsMarkdownURL).
	markdownURL      = regexp.MustCompile(`^([^#?]+)\.md(#.+)?$`)
	markdownURLProto = regexp.MustCompile(`^[+a-zA-Z]+:`)
	// StabilityIndex matches a stability index's text.
	StabilityIndex = jsre.MustCompile(`(?s)^Stability: ([0-5](?:\.[0-3])?)(?:\s*-\s*)?(.*)$`)
	// StabilityIndexPrefix matches a stability index's prefix.
	StabilityIndexPrefix = regexp.MustCompile(`Stability: ([0-5](?:\.[0-3])?)`)
	// YAMLInnerContent matches the content of a YAML comment block.
	YAMLInnerContent = jsre.MustCompile(`^<!--[ ]?(?:YAML([\s\S]*?)|([ \S]*?))?[ ]?-->`)
	// StandardYAMLFrontmatter matches standard Markdown YAML frontmatter.
	StandardYAMLFrontmatter = jsre.MustCompile(`^---\r?\n([\s\S]*?)\r?\n---`)
	// UnixManualPage matches references to Unix manuals.
	UnixManualPage = regexp.MustCompile(`\b([a-z.]+)\((\d)([a-z]?)\)`)
	// TypedListStarters matches a typed list item's non-property starters.
	TypedListStarters = jsre.MustCompile(`^(Returns|Extends|Type):?\s*`)
	// validJavaScriptProperty matches property names.
	validJavaScriptProperty = regexp.MustCompile(`(?i)^[.a-z0-9$_'-]+$`)
)

// RewriteMarkdownURL turns a link to a Markdown page into one to its HTML
// page, reporting whether it was one.
func RewriteMarkdownURL(url string) (string, bool) {
	if markdownURLProto.MatchString(url) {
		return url, false
	}

	m := markdownURL.FindStringSubmatch(url)

	if m == nil {
		return url, false
	}

	return m[1] + ".html" + m[2], true
}

// IsStabilityNode tests for stability index blockquotes.
func IsStabilityNode(n *unist.Node) bool {
	return n.Type == "blockquote" && StabilityIndex.MatchString(unist.NodesToString(n.Children, false))
}

// IsYAMLNode tests for YAML comment blocks.
func IsYAMLNode(n *unist.Node) bool {
	return n.Type == "html" && YAMLInnerContent.MatchString(n.Value)
}

// IsTextWithUnixManual tests for text referencing Unix manuals.
func IsTextWithUnixManual(n *unist.Node) bool {
	return n.Type == "text" && UnixManualPage.MatchString(n.Value)
}

// IsMarkdownURL tests for links to Markdown pages.
func IsMarkdownURL(n *unist.Node) bool {
	if n.Type != "link" {
		return false
	}

	_, ok := RewriteMarkdownURL(n.URL)

	return ok
}

// IsHeading tests for the headings that start entries.
func IsHeading(n *unist.Node) bool {
	return n.Type == "heading" && n.Depth >= 1 && n.Depth <= 5
}

// IsLinkReference tests for link references.
func IsLinkReference(n *unist.Node) bool {
	return n.Type == "linkReference" && n.Identifier != ""
}

// typedConfidence returns how confidently the first phrasing node of a
// paragraph looks like the start of a typed parameter: 0 (not one), 1
// (loosely typed: a property name in code) or 2 (strongly typed: a typed list
// starter or a type annotation).
func typedConfidence(first *unist.Node) int {
	if first == nil {
		return 0
	}

	value := ""

	if first.HasValue() {
		value = strings.TrimLeftFunc(first.Value, isJSSpace)
	}

	if value != "" && TypedListStarters.MatchString(value) {
		return 2
	}

	if first.Type == "typeAnnotation" {
		return 2
	}

	if first.Type == "inlineCode" && value != "" && validJavaScriptProperty.MatchString(value) {
		return 1
	}

	return 0
}

func child(n *unist.Node, i int) *unist.Node {
	if n == nil || i >= len(n.Children) {
		return nil
	}

	return n.Children[i]
}

func isJSSpace(r rune) bool {
	return unicode.IsSpace(r) || r == 0xFEFF
}

// IsTypedListItem tests for list items that look like typed parameters.
func IsTypedListItem(item *unist.Node) bool {
	return typedConfidence(child(child(item, 0), 0)) > 0
}

// TypedList returns how confidently a list looks like a typed list.
func TypedList(list *unist.Node) int {
	if list == nil || list.Type != "list" {
		return 0
	}

	return typedConfidence(child(child(child(list, 0), 0), 0))
}

// IsLooselyTypedList tests for typed lists.
func IsLooselyTypedList(n *unist.Node) bool {
	return TypedList(n) > 0
}

// IsStronglyTypedList tests for typed lists whose first item has a type.
func IsStronglyTypedList(list *unist.Node) bool {
	confidence := TypedList(list)

	if confidence == 1 {
		para := child(child(list, 0), 0)
		second, third := child(para, 1), child(para, 2)

		return second != nil && second.HasValue() && jsre.Trim(second.Value) == "" &&
			third != nil && third.Type == "typeAnnotation"
	}

	return confidence > 0
}

// SplitTypedItems splits a typed list's items into the leading typed ones
// and the rest.
func SplitTypedItems(list *unist.Node) (typed, rest []*unist.Node) {
	for i, item := range list.Children {
		if !IsTypedListItem(item) {
			return list.Children[:i], list.Children[i:]
		}
	}

	return list.Children, nil
}
