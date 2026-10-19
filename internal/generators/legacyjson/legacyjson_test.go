package legacyjson

import (
	"reflect"
	"testing"

	"github.com/nodejs/doc-kit/internal/omap"
	. "github.com/nodejs/doc-kit/internal/testutil"
	"github.com/nodejs/doc-kit/internal/unist"
)

func TestPromoteMiscChildren(t *testing.T) {
	t.Run("ignores a non-misc section", func(t *testing.T) {
		parent := omap.New("type", "text")
		PromoteMiscChildren(omap.New("type", "text", "key", "value"), parent)

		if parent.Has("key") {
			t.Error("promoted")
		}
	})

	t.Run("ignores a misc parent", func(t *testing.T) {
		parent := omap.New("type", "misc")
		PromoteMiscChildren(omap.New("type", "misc", "key", "value"), parent)

		if parent.Has("key") {
			t.Error("promoted")
		}
	})

	t.Run("ignores the unpromoted keys", func(t *testing.T) {
		section := omap.New("type", "misc", "promotableKey", "this should be promoted")

		for _, k := range UnpromotedKeys {
			if k != "type" {
				section.Set(k, "this should be ignored")
			}
		}

		parent := omap.New("type", "module")
		PromoteMiscChildren(section, parent)

		for _, k := range UnpromotedKeys {
			if k != "type" && parent.Has(k) {
				t.Errorf("%q was promoted", k)
			}
		}

		if parent.Value("promotableKey") != "this should be promoted" {
			t.Error("promotableKey was not promoted")
		}
	})

	t.Run("pushes onto a parent's array", func(t *testing.T) {
		parent := omap.New("type", "module", "someValue", []any{"foo"})
		PromoteMiscChildren(omap.New("type", "misc", "someValue", "bar"), parent)

		if !reflect.DeepEqual(parent.Value("someValue"), []any{"foo", "bar"}) {
			t.Errorf("got %v", parent.Value("someValue"))
		}
	})

	t.Run("keeps a parent's other value", func(t *testing.T) {
		parent := omap.New("type", "module", "someValue", "foo")
		PromoteMiscChildren(omap.New("type", "misc", "someValue", "bar"), parent)

		if parent.Value("someValue") != "foo" {
			t.Errorf("got %v", parent.Value("someValue"))
		}
	})

	t.Run("promotes a property the parent lacks", func(t *testing.T) {
		parent := omap.New("type", "module")
		PromoteMiscChildren(omap.New("type", "misc", "someValue", "bar"), parent)

		if parent.Value("someValue") != "bar" {
			t.Errorf("got %v", parent.Value("someValue"))
		}
	})
}

var validTypedList = []*unist.Node{U("inlineCode", "option"), Text(" "), U("typeAnnotation", "boolean"), Text(" option description")}

func TestParseList(t *testing.T) {
	t.Run("processes property sections", func(t *testing.T) {
		section := omap.New("type", "property", "name", "test")
		parseList(section, []*unist.Node{U("list", U("listItem", Paragraph(validTypedList...)))})

		if section.Value("textRaw") == nil || section.Value("type") != "boolean" {
			t.Errorf("got %v", section)
		}
	})

	t.Run("processes event sections", func(t *testing.T) {
		section := omap.New("type", "event")
		parseList(section, []*unist.Node{U("list", U("listItem", Paragraph(Text("param description"))))})

		if _, ok := section.Value("params").([]any); !ok {
			t.Errorf("got %v", section)
		}
	})

	t.Run("processes nested lists", func(t *testing.T) {
		section := omap.New("type", "event")
		nested := U("list", U("listItem", Paragraph(validTypedList...)))
		parseList(section, []*unist.Node{U("list", U("listItem", Paragraph(validTypedList...), nested))})

		first := section.Value("params").([]any)[0].(*omap.Map)

		if options := first.Value("options").([]*omap.Map); len(options) != 1 {
			t.Errorf("got %v", options)
		}
	})
}
