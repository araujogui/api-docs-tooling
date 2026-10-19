// Package testutil builds syntax trees for tests, like `unist-builder`.
package testutil

import (
	"fmt"

	"github.com/nodejs/doc-kit/internal/unist"
)

// P are a node's properties.
type P map[string]any

// U builds a node: a string argument is its value, P its properties, and
// nodes (or slices of them) its children.
func U(typ string, args ...any) *unist.Node {
	n := &unist.Node{Type: typ}

	for _, arg := range args {
		switch a := arg.(type) {
		case string:
			n.Value = a
		case *unist.Node:
			n.Children = append(n.Children, a)
		case []*unist.Node:
			if n.Children == nil {
				n.Children = []*unist.Node{}
			}

			n.Children = append(n.Children, a...)
		case P:
			for k, v := range a {
				set(n, k, v)
			}
		default:
			panic(fmt.Sprintf("U: unexpected argument %T", arg))
		}
	}

	if n.Children == nil && !n.HasValue() && n.IsParent() {
		n.Children = []*unist.Node{}
	}

	return n
}

func set(n *unist.Node, key string, v any) {
	switch key {
	case "value":
		n.Value = v.(string)
	case "depth":
		n.Depth = v.(int)
	case "url":
		n.URL = v.(string)
	case "title":
		s := v.(string)
		n.Title = &s
	case "alt":
		n.Alt = v.(string)
	case "identifier":
		n.Identifier = v.(string)
	case "label":
		n.Label = v.(string)
	case "referenceType":
		n.ReferenceType = v.(string)
	case "lang":
		s := v.(string)
		n.Lang = &s
	case "meta":
		s := v.(string)
		n.Meta = &s
	case "ordered":
		n.Ordered = v.(bool)
	case "start":
		i := v.(int)
		n.Start = &i
	case "spread":
		n.Spread = v.(bool)
	case "checked":
		b := v.(bool)
		n.Checked = &b
	case "data":
		n.Data = v.(*unist.Data)
	case "tagName":
		n.TagName = v.(string)
	default:
		panic("U: unknown property " + key)
	}
}

// Text builds a text node.
func Text(value string) *unist.Node { return unist.Text(value) }

// Paragraph builds a paragraph.
func Paragraph(children ...*unist.Node) *unist.Node { return U("paragraph", children) }

// Heading builds a heading of some text.
func Heading(text string, depth int) *unist.Node {
	return U("heading", P{"depth": depth}, Text(text))
}
