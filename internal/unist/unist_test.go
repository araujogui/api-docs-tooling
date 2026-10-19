package unist

import (
	"reflect"
	"testing"
)

func para(children ...*Node) *Node { return Parent("paragraph", children...) }

func TestNodesToString(t *testing.T) {
	nodes := []*Node{para(Text("Hello, "), Parent("strong", Text("World")))}

	if got := NodesToString(nodes, false); got != "Hello, **World**" {
		t.Errorf("got %q", got)
	}

	inline := []*Node{{Type: "inlineCode", Value: "a<b>"}, Text(" and "), {Type: "typeAnnotation", Value: "T<U>"}, Parent("emphasis", Text("x\ny"))}

	if got := NodesToString(inline, true); got != "`a&lt;b&gt;` and {T&lt;U&gt;}_x y_" {
		t.Errorf("got %q", got)
	}
}

func TestToString(t *testing.T) {
	tree := Root(para(Text("a"), &Node{Type: "inlineCode", Value: "b"}, &Node{Type: "image", Alt: "c"}, &Node{Type: "break"}))

	if got := ToString(tree); got != "abc" {
		t.Errorf("got %q", got)
	}
}

func values(n *Node) []string {
	var out []string

	Visit(n, IsType("text"), func(t *Node, _ int, _ *Node) Step {
		out = append(out, t.Value)

		return Next
	})

	return out
}

func TestVisit(t *testing.T) {
	tree := Root(para(Text("a"), Text("b")), para(Text("c")))

	if got := values(tree); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("got %v", got)
	}

	t.Run("skips the children of a node", func(t *testing.T) {
		var seen []string

		Visit(tree, nil, func(n *Node, _ int, _ *Node) Step {
			seen = append(seen, n.Type)

			if n.Type == "paragraph" {
				return SkipStep
			}

			return Next
		})

		if !reflect.DeepEqual(seen, []string{"root", "paragraph", "paragraph"}) {
			t.Errorf("got %v", seen)
		}
	})

	t.Run("goes on from the next index after edits", func(t *testing.T) {
		tree := Root(para(Text("a"), Text("b"), Text("c")))

		// Removing a node moves its next sibling to its index, which is
		// then skipped, as in unist-util-visit
		Visit(tree, IsType("text"), func(n *Node, i int, parent *Node) Step {
			if n.Value == "a" {
				parent.Splice(i, 1)
			}

			return Next
		})

		if got := values(tree); !reflect.DeepEqual(got, []string{"b", "c"}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("stops on exit", func(t *testing.T) {
		count := 0

		Visit(tree, IsType("text"), func(*Node, int, *Node) Step {
			count++

			return ExitStep
		})

		if count != 1 {
			t.Errorf("visited %d", count)
		}
	})
}

func TestRemove(t *testing.T) {
	t.Run("removes matching nodes, and parents left empty", func(t *testing.T) {
		tree := Root(para(Text("x")), para(Text("y"), Text("x")))
		Remove(tree, func(n *Node) bool { return n.Type == "text" && n.Value == "x" })

		if len(tree.Children) != 1 || !reflect.DeepEqual(values(tree), []string{"y"}) {
			t.Errorf("got %v", values(tree))
		}
	})

	t.Run("leaves a tree it would empty as it is", func(t *testing.T) {
		tree := Root(para(Text("x")))
		Remove(tree, IsType("paragraph"))

		if len(tree.Children) != 1 {
			t.Errorf("got %v", tree.Children)
		}
	})
}

func TestFindAfterAndSelectAll(t *testing.T) {
	a, b := Parent("heading"), Parent("heading")
	tree := Root(a, para(Text("x")), b)

	if FindAfter(tree, 0, IsType("heading")) != b || FindAfter(tree, 2, IsType("heading")) != nil {
		t.Error("FindAfter failed")
	}

	if got := SelectAll(tree, "heading"); len(got) != 2 {
		t.Errorf("got %v", got)
	}
}
