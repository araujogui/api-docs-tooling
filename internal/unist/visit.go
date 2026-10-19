package unist

// Action tells Visit how to go on after visiting a node.
type Action int

const (
	// Continue visits the node's children, then its next sibling.
	Continue Action = iota
	// Skip does not visit the node's children.
	Skip
	// Exit stops visiting.
	Exit
)

// Step is what a visitor returns: an action, and optionally the index of the
// sibling to go on from.
type Step struct {
	Action Action
	Index  int
	HasIdx bool
}

var (
	// Next continues as usual.
	Next = Step{Action: Continue}
	// SkipStep skips the node's children.
	SkipStep = Step{Action: Skip}
	// ExitStep stops visiting.
	ExitStep = Step{Action: Exit}
)

// SkipAt skips the node's children and goes on from the sibling at index.
func SkipAt(index int) Step {
	return Step{Action: Skip, Index: index, HasIdx: true}
}

// Test reports whether a node matches.
type Test func(n *Node) bool

// IsType tests for nodes of a type.
func IsType(typ string) Test {
	return func(n *Node) bool { return n.Type == typ }
}

// Visitor is called for each matching node, with its index in its parent
// (-1 for the root) and its parent (nil for the root).
type Visitor func(n *Node, index int, parent *Node) Step

// Visit walks a tree in preorder like `unist-util-visit`, calling the visitor
// for every node the test matches (all nodes when test is nil). Visitors may
// edit their parent's children; walking goes on from the next index, or the
// index the visitor returns.
func Visit(tree *Node, test Test, visitor Visitor) {
	visit(tree, -1, nil, test, visitor)
}

func visit(node *Node, index int, parent *Node, test Test, visitor Visitor) Step {
	step := Next

	if test == nil || test(node) {
		step = visitor(node, index, parent)

		if step.Action == Exit {
			return step
		}
	}

	if step.Action != Skip && node.Children != nil {
		for i := 0; i >= 0 && i < len(node.Children); {
			child := node.Children[i]
			sub := visit(child, i, node, test, visitor)

			if sub.Action == Exit {
				return sub
			}

			if sub.HasIdx {
				i = sub.Index
			} else {
				i++
			}
		}
	}

	return step
}

// SelectAll returns every node of a type, in preorder (including the root).
func SelectAll(tree *Node, typ string) []*Node {
	var found []*Node

	Visit(tree, IsType(typ), func(n *Node, _ int, _ *Node) Step {
		found = append(found, n)

		return Next
	})

	return found
}

// FindAfter returns the first child of parent after index matching the test.
func FindAfter(parent *Node, index int, test Test) *Node {
	for i := index + 1; i < len(parent.Children); i++ {
		if test(parent.Children[i]) {
			return parent.Children[i]
		}
	}

	return nil
}

// Remove deletes every node matching the test from a tree, like
// `unist-util-remove`: a parent left empty by the removal is removed too.
// When the tree itself would be left empty, it is left as it was.
func Remove(tree *Node, test Test) *Node {
	return remove(tree, test)
}

func remove(node *Node, test Test) *Node {
	if test(node) {
		return nil
	}

	if len(node.Children) > 0 {
		position := 0

		for _, child := range node.Children {
			if kept := remove(child, test); kept != nil {
				node.Children[position] = kept
				position++
			}
		}

		if position == 0 {
			return nil
		}

		node.Children = node.Children[:position]
	}

	return node
}

// AnyOf tests for nodes matching any of the tests.
func AnyOf(tests ...Test) Test {
	return func(n *Node) bool {
		for _, t := range tests {
			if t(n) {
				return true
			}
		}

		return false
	}
}

// OneOf tests for the given nodes themselves.
func OneOf(nodes []*Node) Test {
	set := make(map[*Node]bool, len(nodes))

	for _, n := range nodes {
		set[n] = true
	}

	return func(n *Node) bool { return set[n] }
}
