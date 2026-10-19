package doc

import (
	"unicode/utf16"

	"github.com/nodejs/doc-kit/internal/queries"
	"github.com/nodejs/doc-kit/internal/unist"
)

// RemoveStabilityPrefix copies a stability blockquote without its
// `Stability: N - ` prefix (nil when nothing follows the prefix).
func RemoveStabilityPrefix(n *unist.Node) *unist.Node {
	var first *unist.Node

	if len(n.Children) > 0 {
		first = n.Children[0]
	}

	text := unist.ToString(first)
	start := 0

	if m := queries.StabilityIndex.FindStringSubmatch(text); m != nil {
		start = utf16Length(text) - utf16Length(m[2])
	}

	return Slice(n, start)
}

func utf16Length(s string) int {
	return len(utf16.Encode([]rune(s)))
}

// Slice copies the part of a tree from a text offset on, like
// `mdast-util-slice-markdown` does with whitespace preserved at the
// boundaries and adjacent text left unmerged.
func Slice(tree *unist.Node, start int) *unist.Node {
	if tree == nil || start < 0 {
		return nil
	}

	end := sliceLength(tree)

	if start >= end {
		return nil
	}

	return sliceNode(tree, 0, start, end)
}

func sliceLength(n *unist.Node) int {
	if n.HasValue() && n.Value != "" {
		return utf16Length(n.Value)
	}

	total := 0

	for _, c := range n.Children {
		total += sliceLength(c)
	}

	return total
}

var rangedTypes = map[string]bool{
	"emphasis": true, "strong": true, "delete": true, "underline": true,
	"link": true, "linkReference": true, "image": true, "imageReference": true,
	"paragraph": true, "heading": true, "blockquote": true, "listItem": true,
}

func sliceNode(n *unist.Node, nodeStart, start, end int) *unist.Node {
	if n.HasValue() && n.Value != "" {
		units := utf16.Encode([]rune(n.Value))
		nodeEnd := nodeStart + len(units)

		if nodeEnd <= start || nodeStart >= end {
			return nil
		}

		if nodeStart >= start && nodeEnd <= end {
			return n.ShallowCopy()
		}

		from := max(0, start-nodeStart)
		to := min(len(units), end-nodeStart)
		value := string(utf16.Decode(units[from:to]))

		if value == "" {
			return nil
		}

		c := n.ShallowCopy()
		c.Value = value

		return c
	}

	length := sliceLength(n)
	nodeEnd := nodeStart + length

	if rangedTypes[n.Type] {
		if nodeEnd <= start || nodeStart >= end {
			return nil
		}

		if n.Children == nil {
			return n.ShallowCopy()
		}

		return sliceParent(n, nodeStart, start, end)
	}

	if n.Children != nil {
		return sliceParent(n, nodeStart, start, end)
	}

	if nodeEnd > start && nodeStart < end {
		return n.ShallowCopy()
	}

	return nil
}

func sliceParent(n *unist.Node, nodeStart, start, end int) *unist.Node {
	var children []*unist.Node

	position := nodeStart

	for _, child := range n.Children {
		if result := sliceNode(child, position, start, end); result != nil {
			children = append(children, result)
		}

		position += sliceLength(child)
	}

	if len(children) == 0 {
		return nil
	}

	c := n.ShallowCopy()
	c.Children = children

	return c
}
