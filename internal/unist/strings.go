package unist

import "strings"

// ToString returns the text content of a node, like `mdast-util-to-string`.
func ToString(n *Node) string {
	if n == nil {
		return ""
	}

	if n.HasValue() {
		return n.Value
	}

	if n.Type == "image" && n.Alt != "" {
		return n.Alt
	}

	return ToStringAll(n.Children)
}

// ToStringAll returns the text content of nodes.
func ToStringAll(nodes []*Node) string {
	var b strings.Builder

	for _, n := range nodes {
		b.WriteString(ToString(n))
	}

	return b.String()
}

var htmlEntities = strings.NewReplacer("<", "&lt;", ">", "&gt;")

func escapeHTMLEntities(s string) string {
	return htmlEntities.Replace(s)
}

// NodeToString turns a node back into what its source would look like, on a
// single line.
func NodeToString(n *Node, escape bool) string {
	switch n.Type {
	case "inlineCode":
		if escape {
			return "`" + escapeHTMLEntities(n.Value) + "`"
		}

		return "`" + n.Value + "`"
	case "typeAnnotation":
		if escape {
			return "{" + escapeHTMLEntities(n.Value) + "}"
		}

		return "{" + n.Value + "}"
	case "strong":
		return "**" + NodesToString(n.Children, escape) + "**"
	case "emphasis":
		return "_" + NodesToString(n.Children, escape) + "_"
	}

	if n.Children != nil {
		return NodesToString(n.Children, escape)
	}

	s := ""

	if n.HasValue() {
		s = strings.ReplaceAll(n.Value, "\n", " ")
	}

	if escape {
		return escapeHTMLEntities(s)
	}

	return s
}

// NodesToString joins nodes turned back into their source.
func NodesToString(nodes []*Node, escape bool) string {
	var b strings.Builder

	for _, n := range nodes {
		b.WriteString(NodeToString(n, escape))
	}

	return b.String()
}
