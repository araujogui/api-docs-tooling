package queries

import (
	"reflect"
	"testing"

	. "github.com/nodejs/doc-kit/internal/testutil"
	"github.com/nodejs/doc-kit/internal/unist"
)

func item(children ...*unist.Node) *unist.Node {
	return U("listItem", Paragraph(children...))
}

func TestSplitTypedItems(t *testing.T) {
	typed := item(U("inlineCode", "name"), Text(" "), U("typeAnnotation", "string"))
	prose := item(Text("A prose bullet."))

	t.Run("separates the leading typed items from the rest", func(t *testing.T) {
		gotTyped, gotRest := SplitTypedItems(U("list", []*unist.Node{typed, typed, prose, typed}))

		if !reflect.DeepEqual(gotTyped, []*unist.Node{typed, typed}) || !reflect.DeepEqual(gotRest, []*unist.Node{prose, typed}) {
			t.Errorf("got %v, %v", gotTyped, gotRest)
		}
	})

	t.Run("has no rest when every item is typed", func(t *testing.T) {
		gotTyped, gotRest := SplitTypedItems(U("list", typed))

		if len(gotTyped) != 1 || len(gotRest) != 0 {
			t.Errorf("got %v, %v", gotTyped, gotRest)
		}
	})
}

func TestStandardYAMLFrontmatter(t *testing.T) {
	t.Run("matches frontmatter at the beginning of the text", func(t *testing.T) {
		m := StandardYAMLFrontmatter.FindStringSubmatch("---\nintroduced_in: v1.0.0\ntype: module\n---")

		if m == nil || m[1] != "introduced_in: v1.0.0\ntype: module" {
			t.Errorf("got %q", m)
		}
	})

	t.Run("matches frontmatter with Windows line endings", func(t *testing.T) {
		m := StandardYAMLFrontmatter.FindStringSubmatch("---\r\nintroduced_in: v1.0.0\r\n---")

		if m == nil || m[1] != "introduced_in: v1.0.0" {
			t.Errorf("got %q", m)
		}
	})

	t.Run("does not match rules not at the start", func(t *testing.T) {
		if StandardYAMLFrontmatter.MatchString("# Hello\n\n---\n\nSome text") {
			t.Error("matched")
		}
	})

	t.Run("does not match unclosed blocks", func(t *testing.T) {
		if StandardYAMLFrontmatter.MatchString("---\nintroduced_in: v1.0.0\nSome text...") {
			t.Error("matched")
		}
	})
}

func list(items ...*unist.Node) *unist.Node {
	return U("list", items)
}

func TestTypedLists(t *testing.T) {
	strong := []struct {
		name string
		node *unist.Node
		want bool
	}{
		{"a non-list node", U("paragraph"), false},
		{"an empty list", list(), false},
		{"a typed list starter", list(item(Text("Returns: foo"))), true},
		{"a direct type annotation", list(item(U("typeAnnotation", "Type"))), true},
		{"inline code, a space and a type annotation", list(item(U("inlineCode", "foo"), Text(" "), U("typeAnnotation", "Bar"))), true},
		{"inline code without a type annotation", list(item(U("inlineCode", "foo"), Text(" just prose"))), false},
		{"non-matching content", list(item(U("inlineCode", "not a valid prop"), Text(" "), U("typeAnnotation", "Bar"))), false},
	}

	for _, c := range strong {
		if got := IsStronglyTypedList(c.node); got != c.want {
			t.Errorf("IsStronglyTypedList(%s) = %v, want %v", c.name, got, c.want)
		}
	}

	loose := []struct {
		name string
		node *unist.Node
		want bool
	}{
		{"a typed list starter", list(item(Text("Returns: foo"))), true},
		{"a direct type annotation", list(item(U("typeAnnotation", "Type"))), true},
		{"inline code and text", list(item(U("inlineCode", "foo"), Text(" bar"))), true},
		{"non-matching content", list(item(U("inlineCode", "not a valid prop"), Text(" "), U("typeAnnotation", "Bar"))), false},
	}

	for _, c := range loose {
		if got := IsLooselyTypedList(c.node); got != c.want {
			t.Errorf("IsLooselyTypedList(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestIsTypedListItem(t *testing.T) {
	if IsTypedListItem(nil) || IsTypedListItem(&unist.Node{}) {
		t.Error("an empty item is not typed")
	}

	cases := []struct {
		name string
		item *unist.Node
		want bool
	}{
		{"inline code with a valid property name", item(U("inlineCode", "actual"), Text(" "), U("typeAnnotation", "Array|string")), true},
		{"a Returns prefix", item(Text("Returns: some value")), true},
		{"a direct type annotation", item(U("typeAnnotation", "Type")), true},
		{"plain prose", item(Text("Algorithm complexity: O(N*D), where:")), false},
		{"inline code with an invalid property name", item(U("inlineCode", "not a valid prop")), false},
	}

	for _, c := range cases {
		if got := IsTypedListItem(c.item); got != c.want {
			t.Errorf("IsTypedListItem(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}
