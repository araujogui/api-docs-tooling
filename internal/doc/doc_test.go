package doc

import (
	"reflect"
	"strings"
	"testing"

	"github.com/nodejs/doc-kit/internal/omap"

	. "github.com/nodejs/doc-kit/internal/testutil"
	"github.com/nodejs/doc-kit/internal/unist"
)

func entryAt(depth int) *Entry {
	return &Entry{Heading: &unist.Node{Type: "heading", Depth: depth, Data: &unist.Data{}}}
}

func TestFindParent(t *testing.T) {
	wrap := func(depths ...int) []*Hierarchized {
		var nodes []*Hierarchized

		for _, d := range depths {
			nodes = append(nodes, &Hierarchized{Entry: entryAt(d)})
		}

		return nodes
	}

	t.Run("finds a parent with a lower depth", func(t *testing.T) {
		nodes := wrap(1, 2)

		if got := FindParent(nodes[1].Entry, nodes, 0); got != nodes[0] {
			t.Errorf("got %v", got)
		}
	})

	t.Run("skips over siblings and deeper entries", func(t *testing.T) {
		nodes := wrap(2, 4, 3)

		if got := FindParent(nodes[2].Entry, nodes, 1); got != nodes[0] {
			t.Errorf("got %v", got)
		}
	})

	t.Run("returns nil when there is no parent", func(t *testing.T) {
		nodes := wrap(2)

		if got := FindParent(nodes[0].Entry, nodes, -1); got != nil {
			t.Errorf("got %v", got)
		}
	})
}

func TestBuildHierarchy(t *testing.T) {
	t.Run("returns nothing for no entries", func(t *testing.T) {
		if got := BuildHierarchy(nil); len(got) != 0 {
			t.Errorf("got %v", got)
		}
	})

	t.Run("keeps root entries at the top level", func(t *testing.T) {
		entries := []*Entry{entryAt(1), entryAt(1)}
		got := BuildHierarchy(entries)

		if len(got) != 2 || got[0].Entry != entries[0] || got[1].Entry != entries[1] {
			t.Errorf("got %v", got)
		}
	})

	t.Run("nests children under parents", func(t *testing.T) {
		entries := []*Entry{entryAt(1), entryAt(2)}
		got := BuildHierarchy(entries)

		if len(got) != 1 || len(got[0].Children) != 1 || got[0].Children[0].Entry != entries[1] {
			t.Errorf("got %v", got)
		}
	})

	t.Run("handles multiple levels", func(t *testing.T) {
		got := BuildHierarchy([]*Entry{entryAt(1), entryAt(2), entryAt(3)})

		if len(got) != 1 || len(got[0].Children[0].Children) != 1 {
			t.Errorf("got %v", got)
		}
	})

	t.Run("treats an entry without a shallower predecessor as a root", func(t *testing.T) {
		entries := []*Entry{entryAt(2), entryAt(3)}
		got := BuildHierarchy(entries)

		if len(got) != 1 || got[0].Entry != entries[0] || got[0].Children[0].Entry != entries[1] {
			t.Errorf("got %v", got)
		}
	})
}

func overloadEntry(name, typ, slug string, depth int) *Entry {
	return &Entry{Heading: &unist.Node{Type: "heading", Depth: depth, Data: &unist.Data{Name: name, HeadType: typ, Slug: slug}}}
}

func overloads(entries []*Entry) []bool {
	var out []bool

	for _, e := range entries {
		out = append(out, e.Heading.Data.IsOverload)
	}

	return out
}

func TestAnnotateOverloads(t *testing.T) {
	t.Run("flags every overload after the first heading of a run", func(t *testing.T) {
		entries := []*Entry{
			overloadEntry("fs.read", "method", "fsreadfd", 2),
			overloadEntry("fs.read", "method", "fsreadbuffer", 2),
			overloadEntry("fs.read", "method", "fsreadoptions", 2),
		}

		AnnotateOverloads(entries)

		if !reflect.DeepEqual(overloads(entries), []bool{false, true, true}) {
			t.Errorf("got %v", overloads(entries))
		}

		if entries[0].Heading.Data.OverloadOf != "" || entries[1].Heading.Data.OverloadOf != "fsreadfd" || entries[2].Heading.Data.OverloadOf != "fsreadfd" {
			t.Error("overloads should point at the first heading")
		}
	})

	t.Run("leaves a single signature untouched", func(t *testing.T) {
		entries := []*Entry{overloadEntry("fs.access", "method", "fsaccess", 2)}
		AnnotateOverloads(entries)

		if entries[0].Heading.Data.IsOverload {
			t.Error("flagged")
		}
	})

	t.Run("groups constructors by name", func(t *testing.T) {
		entries := []*Entry{
			overloadEntry("Buffer", "ctor", "new-bufferarray", 2),
			overloadEntry("Buffer", "ctor", "new-buffersize", 2),
		}
		AnnotateOverloads(entries)

		if !reflect.DeepEqual(overloads(entries), []bool{false, true}) {
			t.Errorf("got %v", overloads(entries))
		}
	})

	t.Run("does not group headings of different types or depths", func(t *testing.T) {
		entries := []*Entry{
			overloadEntry("Buffer", "class", "class-buffer", 2),
			overloadEntry("Buffer", "ctor", "new-bufferarray", 2),
			overloadEntry("Buffer", "ctor", "new-buffersize", 3),
		}
		AnnotateOverloads(entries)

		if !reflect.DeepEqual(overloads(entries), []bool{false, false, false}) {
			t.Errorf("got %v", overloads(entries))
		}
	})

	t.Run("starts a fresh group when a different function interrupts a run", func(t *testing.T) {
		entries := []*Entry{
			overloadEntry("fs.read", "method", "fsreadfd", 2),
			overloadEntry("fs.read", "method", "fsreadbuffer", 2),
			overloadEntry("fs.write", "method", "fswritefd", 2),
			overloadEntry("fs.write", "method", "fswritebuffer", 2),
		}
		AnnotateOverloads(entries)

		if !reflect.DeepEqual(overloads(entries), []bool{false, true, false, true}) {
			t.Errorf("got %v", overloads(entries))
		}
	})
}

func stabilityNode(children ...*unist.Node) *unist.Node {
	return U("blockquote", P{"data": &unist.Data{IsStability: true, Index: "1.1"}}, children)
}

func TestRemoveStabilityPrefix(t *testing.T) {
	t.Run("drops a linked prefix and its separator", func(t *testing.T) {
		node := stabilityNode(Paragraph(
			U("link", P{"url": "#stability-index"}, Text("Stability: 1.1")),
			Text(" - Active development. Use "),
			U("inlineCode", "other()"),
			Text(" instead."),
		))

		got := RemoveStabilityPrefix(node).Children[0].Children
		want := []*unist.Node{Text("Active development. Use "), U("inlineCode", "other()"), Text(" instead.")}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v", got)
		}
	})

	t.Run("drops a plain prefix and keeps later paragraphs", func(t *testing.T) {
		node := stabilityNode(Paragraph(Text("Stability: 2 - Stable")), Paragraph(Text("More.")))
		got := RemoveStabilityPrefix(node).Children

		if got[0].Children[0].Value != "Stable" || got[1].Children[0].Value != "More." {
			t.Errorf("got %v", got)
		}
	})

	t.Run("leaves the source untouched", func(t *testing.T) {
		node := stabilityNode(Paragraph(Text("Stability: 2 - Stable")))
		before := node.Clone()
		RemoveStabilityPrefix(node)

		if !reflect.DeepEqual(node, before) {
			t.Error("the source changed")
		}
	})
}

func paragraphEntry(children ...*unist.Node) *Entry {
	return &Entry{Content: U("root", children), Props: omap.New()}
}

func TestEntryDescription(t *testing.T) {
	t.Run("returns llm_description when available", func(t *testing.T) {
		e := paragraphEntry()
		e.Props.Set("llm_description", "LLM generated description")

		if got := EntryDescription(e); got != "LLM generated description" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("extracts the first paragraph otherwise", func(t *testing.T) {
		if got := EntryDescription(paragraphEntry(Paragraph(Text("First paragraph")))); got != "First paragraph" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("returns nothing without a paragraph", func(t *testing.T) {
		if got := EntryDescription(paragraphEntry(Heading("Title", 1))); got != "" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("removes line breaks", func(t *testing.T) {
		got := EntryDescription(paragraphEntry(Paragraph(Text("Line 1\nLine 2\r\nLine 3"))))

		if strings.ContainsAny(got, "\r\n") {
			t.Errorf("got %q", got)
		}
	})
}

func TestGroupByModule(t *testing.T) {
	order, groups := GroupByModule([]*Entry{{API: "fs"}, {API: "http"}, {API: "fs"}})

	if !reflect.DeepEqual(order, []string{"fs", "http"}) || len(groups["fs"]) != 2 || len(groups["http"]) != 1 {
		t.Errorf("got %v, %v", order, groups)
	}

	if order, _ := GroupByModule(nil); len(order) != 0 {
		t.Error("expected no groups")
	}
}
