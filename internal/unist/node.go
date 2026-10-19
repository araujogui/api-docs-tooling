// Package unist holds the syntax tree shared by the Markdown (mdast) and HTML
// (hast) stages, and the utilities that walk and edit it. The legacy
// generators mix both kinds of node in one tree, so a single node type
// carries the fields of either.
package unist

import "github.com/nodejs/doc-kit/internal/omap"

// Point is a place in a source file.
type Point struct {
	Line   int `json:"line"`
	Column int `json:"column"`
	Offset int `json:"offset"`
}

// Position is the span of source a node was parsed from.
type Position struct {
	Start Point `json:"start"`
	End   Point `json:"end"`
}

// TypeLink is a resolved identifier within a type annotation.
type TypeLink struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Text  string `json:"text"`
	Href  string `json:"href"`
}

// Data is the information later stages attach to nodes.
type Data struct {
	// Headings
	Text       string
	Name       string
	Slug       string
	HeadType   string
	Depth      int
	IsOverload bool
	OverloadOf string

	// Stability indexes
	Index       string
	Description string
	IsStability bool

	// Type annotations
	Links      []TypeLink
	TypeScript bool
	ParseError bool
	Resolved   bool
}

// Node is an mdast or hast node.
type Node struct {
	Type     string
	Children []*Node
	Position *Position
	Data     *Data

	// Literals (text, inlineCode, code, html, typeAnnotation, raw, comment)
	Value string

	// Headings
	Depth int

	// Links, images and definitions
	URL           string
	Title         *string
	Alt           string
	Identifier    string
	Label         string
	ReferenceType string

	// Code blocks
	Lang *string
	Meta *string

	// Lists and list items
	Ordered bool
	Start   *int
	Spread  bool
	Checked *bool

	// Tables
	Align []string

	// hast elements
	TagName    string
	Properties *omap.Map
}

// literalTypes are the node types that carry a `value`.
var literalTypes = map[string]bool{
	"text":           true,
	"inlineCode":     true,
	"code":           true,
	"html":           true,
	"typeAnnotation": true,
	"raw":            true,
	"comment":        true,
	"yaml":           true,
}

// HasValue reports whether a node is a literal, carrying a `value`.
func (n *Node) HasValue() bool {
	return n != nil && literalTypes[n.Type]
}

// IsParent reports whether a node can have children.
func (n *Node) IsParent() bool {
	return n != nil && !literalTypes[n.Type] && n.Type != "break" && n.Type != "thematicBreak" && n.Type != "image" && n.Type != "definition"
}

// Root creates a root node.
func Root(children ...*Node) *Node {
	return &Node{Type: "root", Children: children}
}

// Text creates a text node.
func Text(value string) *Node {
	return &Node{Type: "text", Value: value}
}

// Parent creates a parent node of a type.
func Parent(typ string, children ...*Node) *Node {
	return &Node{Type: typ, Children: children}
}

// Clone copies a node and its descendants.
func (n *Node) Clone() *Node {
	if n == nil {
		return nil
	}

	c := *n

	if n.Children != nil {
		c.Children = make([]*Node, len(n.Children))

		for i, child := range n.Children {
			c.Children[i] = child.Clone()
		}
	}

	if n.Properties != nil {
		c.Properties = n.Properties.Clone()
	}

	if n.Data != nil {
		d := *n.Data
		c.Data = &d
	}

	return &c
}

// ShallowCopy copies a node, sharing its children.
func (n *Node) ShallowCopy() *Node {
	c := *n

	return &c
}

// IndexOf returns the position of a child, or -1.
func (n *Node) IndexOf(child *Node) int {
	for i, c := range n.Children {
		if c == child {
			return i
		}
	}

	return -1
}

// Splice removes `count` children at `index` and inserts `items` there.
func (n *Node) Splice(index, count int, items ...*Node) {
	if index < 0 {
		index = 0
	}

	if index > len(n.Children) {
		index = len(n.Children)
	}

	if index+count > len(n.Children) {
		count = len(n.Children) - index
	}

	rest := append([]*Node{}, n.Children[index+count:]...)
	n.Children = append(append(n.Children[:index], items...), rest...)
}

// StrPtr returns a pointer to a string.
func StrPtr(s string) *string {
	return &s
}
