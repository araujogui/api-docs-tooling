package signature

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/nodejs/doc-kit/internal/omap"
	. "github.com/nodejs/doc-kit/internal/testutil"
	"github.com/nodejs/doc-kit/internal/unist"
)

func TestParseNameAndOptionalStatus(t *testing.T) {
	cases := []struct {
		name, in  string
		depth     int
		wantName  string
		wantDepth int
		optional  bool
	}{
		{"simple parameter names", "param", 0, "param", 0, false},
		{"optional parameters with brackets", "[param]", 0, "param", 0, true},
		{"partial brackets at the beginning", "[param", 0, "param", 1, true},
		{"partial brackets at the end", "param]", 1, "param", 0, true},
		{"complex nested brackets", "b[", 1, "b", 2, true},
	}

	for _, c := range cases {
		name, depth, optional := ParseNameAndOptionalStatus(c.in, c.depth)

		if name != c.wantName || depth != c.wantDepth || optional != c.optional {
			t.Errorf("%s: got (%q, %d, %v)", c.name, name, depth, optional)
		}
	}
}

func TestParseDefaultValue(t *testing.T) {
	cases := []struct{ in, name, def string }{
		{"param=default", "param", "=default"},
		{"param", "param", ""},
		{"param={x: [1,2,3]}", "param", "={x: [1,2,3]}"},
		{"param=x=y=z", "param", "=x=y=z"},
	}

	for _, c := range cases {
		if name, def := ParseDefaultValue(c.in); name != c.name || def != c.def {
			t.Errorf("ParseDefaultValue(%q) = %q, %q", c.in, name, def)
		}
	}
}

func params(names ...string) []*omap.Map {
	var out []*omap.Map

	for _, n := range names {
		out = append(out, omap.New("name", n))
	}

	return out
}

func TestFindParameter(t *testing.T) {
	if got := FindParameter("first", 0, params("first", "second")); got.Value("name") != "first" {
		t.Error("should find by index")
	}

	if got := FindParameter("second", 0, params("first", "second")); got.Value("name") != "second" {
		t.Error("should search by name")
	}

	nested := []*omap.Map{omap.New("name", "options", "options", []*omap.Map{omap.New("name", "nested", "type", "string", "description", "test")})}

	if got := FindParameter("nested", 0, nested); got.Value("type") != "string" || got.Value("description") != "test" {
		t.Errorf("should find in nested options, got %v", got)
	}

	if got := FindParameter("missing", 0, nil); got.Value("name") != "missing" || got.Len() != 1 {
		t.Errorf("should default to the name, got %v", got)
	}
}

func TestParseParameters(t *testing.T) {
	cases := []struct {
		name     string
		declared []string
		markdown []*omap.Map
		want     []map[string]any
	}{
		{"simple parameters", []string{"param1", "param2"}, params("param1", "param2"), []map[string]any{{"name": "param1"}, {"name": "param2"}}},
		{"default values", []string{"param=value"}, params("param"), []map[string]any{{"name": "param", "default": "=value"}}},
		{"optional parameters", []string{"[optional]", "required"}, params("optional", "required"), []map[string]any{{"name": "optional", "optional": true}, {"name": "required"}}},
		{"brackets and default values", []string{"[param=default]"}, params("param"), []map[string]any{{"name": "param", "optional": true, "default": "=default"}}},
	}

	for _, c := range cases {
		got := ParseParameters(c.declared, c.markdown)

		if len(got) != len(c.want) {
			t.Fatalf("%s: got %d parameters", c.name, len(got))
		}

		for i, want := range c.want {
			for k, v := range want {
				if got[i].Value(k) != v {
					t.Errorf("%s: parameter %d %s = %v, want %v", c.name, i, k, got[i].Value(k), v)
				}
			}
		}
	}
}

func TestParseSignature(t *testing.T) {
	cases := []struct {
		name     string
		textRaw  string
		markdown []*omap.Map
		want     []map[string]any
	}{
		{"no parameters", "`method()`", nil, nil},
		{"parameters", "`method(param1, param2)`", params("param1", "param2"), []map[string]any{{"name": "param1"}, {"name": "param2"}}},
		{"nested optional parameters", "`new Blob([sources[, options]])`", params("sources", "options"), []map[string]any{{"name": "sources", "optional": true}, {"name": "options", "optional": true}}},
		{"several levels of optionals", "`method(a[, b[, c]])`", params("a", "b", "c"), []map[string]any{{"name": "a"}, {"name": "b", "optional": true}, {"name": "c", "optional": true}}},
		{"a real-world signature", "`new Console(stdout[, stderr][, ignoreErrors])`", params("stdout", "stderr", "ignoreErrors"), []map[string]any{{"name": "stdout"}, {"name": "stderr", "optional": true}, {"name": "ignoreErrors", "optional": true}}},
		{"an un-backticked signature", "new Console(options)", params("options"), []map[string]any{{"name": "options"}}},
		{"a method without backticks", "foo(a, b)", params("a", "b"), []map[string]any{{"name": "a"}, {"name": "b"}}},
	}

	for _, c := range cases {
		got := ParseSignature(c.textRaw, c.markdown).Value("params").([]*omap.Map)

		if len(got) != len(c.want) {
			t.Fatalf("%s: got %d parameters", c.name, len(got))
		}

		for i, want := range c.want {
			for k, v := range want {
				if got[i].Value(k) != v {
					t.Errorf("%s: parameter %d %s = %v, want %v", c.name, i, k, got[i].Value(k), v)
				}
			}
		}
	}

	t.Run("extracts the return value", func(t *testing.T) {
		ret := ParseSignature("`method()`", []*omap.Map{omap.New("name", "return", "type", "string")}).Value("return").(*omap.Map)

		if ret.Value("name") != "return" || ret.Value("type") != "string" {
			t.Errorf("got %v", ret)
		}
	})
}

func TestExtractPattern(t *testing.T) {
	current := omap.New()

	if got := extractPattern("name: test description", regexp.MustCompile(`name:\s*([^.\s]+)`), "name", current); got != " description" || current.Value("name") != "test" {
		t.Errorf("got %q, %v", got, current)
	}

	current = omap.New()

	if got := extractPattern("no match", regexp.MustCompile(`missing:\s*([^.]+)`), "missing", current); got != "no match" || current.Has("missing") {
		t.Errorf("got %q, %v", got, current)
	}
}

func paragraphItem(children ...*unist.Node) *unist.Node {
	return U("listItem", Paragraph(children...))
}

func code(v string) *unist.Node    { return U("inlineCode", v) }
func typeAnn(v string) *unist.Node { return U("typeAnnotation", v) }
func strong(v string) *unist.Node  { return U("strong", Text(v)) }

func TestParseListItem(t *testing.T) {
	check := func(name string, item *unist.Node, want map[string]any) {
		got := ParseListItem(item)

		for k, v := range want {
			if got.Value(k) != v {
				t.Errorf("%s: %s = %v, want %v", name, k, got.Value(k), v)
			}
		}
	}

	check("a basic item", paragraphItem(code("param"), Text(" "), typeAnn("string"), Text(" description")),
		map[string]any{"textRaw": "`param` {string} description", "name": "param", "type": "string", "desc": "description"})

	check("a union type, verbatim", paragraphItem(code("data"), Text(" "), typeAnn("Buffer | string"), Text(" the data")),
		map[string]any{"textRaw": "`data` {Buffer | string} the data", "type": "Buffer | string"})

	check("nested braces in a type", paragraphItem(code("maps"), Text(" "), typeAnn("Record<string, {a: number}>"), Text(" the maps")),
		map[string]any{"type": "Record<string, {a: number}>", "desc": "the maps"})

	check("a prose annotation is not the type", paragraphItem(code("param"), Text(" see also "), typeAnn("string"), Text(" values")),
		map[string]any{"type": nil, "desc": "see also {string} values"})

	check("a return item", paragraphItem(Text("Returns: something")), map[string]any{"name": "return"})

	check("a default value", paragraphItem(code("flag"), Text(" "), typeAnn("boolean"), Text(" turns it on. "), strong("Default:"), Text(" "), code("false")),
		map[string]any{"type": "boolean", "default": "`false`"})
}

func TestExtractListItem(t *testing.T) {
	t.Run("takes a named, typed item apart", func(t *testing.T) {
		r := ExtractListItem(paragraphItem(code("path"), Text(" "), typeAnn("string | URL"), Text(" The file.")))

		if r.Name != "path" || r.Prefix != "" || r.Annotation.Value != "string | URL" || r.Default != nil || len(r.Blocks) != 0 || len(r.Items) != 0 {
			t.Errorf("got %+v", r)
		}

		if !reflect.DeepEqual(r.Text, []*unist.Node{Text("The file.")}) {
			t.Errorf("got text %v", r.Text)
		}
	})

	t.Run("recognises the special prefixes", func(t *testing.T) {
		r := ExtractListItem(paragraphItem(Text("Returns: "), typeAnn("Promise"), Text(" Fulfills on close.")))

		if r.Prefix != "Returns" || r.HasName || r.Annotation.Value != "Promise" || !reflect.DeepEqual(r.Text, []*unist.Node{Text("Fulfills on close.")}) {
			t.Errorf("got %+v", r)
		}

		r = ExtractListItem(paragraphItem(Text("Extends: "), typeAnn("EventEmitter")))

		if r.Prefix != "Extends" || r.Annotation.Value != "EventEmitter" || len(r.Text) != 0 {
			t.Errorf("got %+v", r)
		}

		r = ExtractListItem(paragraphItem(Text("Type:"), Text(" "), typeAnn("integer")))

		if r.Prefix != "Type" || r.Annotation.Value != "integer" {
			t.Errorf("got %+v", r)
		}
	})

	t.Run("keeps a prefix remainder that is not blank", func(t *testing.T) {
		r := ExtractListItem(paragraphItem(Text("Returns: description here")))

		if r.Prefix != "Returns" || !reflect.DeepEqual(r.Text, []*unist.Node{Text("description here")}) {
			t.Errorf("got %+v", r)
		}
	})

	t.Run("trims leading separators", func(t *testing.T) {
		r := ExtractListItem(paragraphItem(code("opt"), Text(" "), typeAnn("boolean"), Text(" - the option")))

		if !reflect.DeepEqual(r.Text, []*unist.Node{Text("the option")}) {
			t.Errorf("got %v", r.Text)
		}

		if r := ExtractListItem(paragraphItem(code("opt"), Text(": "))); len(r.Text) != 0 {
			t.Errorf("got %v", r.Text)
		}
	})

	t.Run("extracts the default value, leaving its marker", func(t *testing.T) {
		nodes := []*unist.Node{code("encoding"), Text(" "), typeAnn("string"), Text(" The encoding. "), strong("Default:"), Text(" "), code("'utf8'"), Text(".")}
		r := ExtractListItem(paragraphItem(nodes...))

		if r.Default == nil || *r.Default != "`'utf8'`" {
			t.Errorf("got default %v", r.Default)
		}

		want := append([]*unist.Node{Text("The encoding. ")}, nodes[4:]...)

		if !reflect.DeepEqual(r.Text, want) {
			t.Errorf("got text %v", r.Text)
		}

		if got := RemoveDefault(r.Text); !reflect.DeepEqual(got, []*unist.Node{Text("The encoding.")}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("separates the nested typed list from other blocks", func(t *testing.T) {
		nested := U("list", paragraphItem(code("flag"), Text(" "), typeAnn("string")))
		note := Paragraph(Text("A note."))
		item := U("listItem", Paragraph(code("options"), Text(" "), typeAnn("Object")), nested, note)
		r := ExtractListItem(item)

		if !reflect.DeepEqual(r.Items, nested.Children) || !reflect.DeepEqual(r.Blocks, []*unist.Node{note}) {
			t.Errorf("got %+v", r)
		}
	})

	t.Run("leaves the source untouched", func(t *testing.T) {
		source := paragraphItem(code("a"), Text(" "), typeAnn("b"), Text(" - c"))
		before := source.Clone()
		ExtractListItem(source)

		if !reflect.DeepEqual(source, before) {
			t.Error("the source changed")
		}
	})

	t.Run("handles an item without a paragraph", func(t *testing.T) {
		if r := ExtractListItem(U("listItem")); len(r.Text) != 0 || r.HasName {
			t.Errorf("got %+v", r)
		}
	})
}

func TestRemoveDefault(t *testing.T) {
	nodes := []*unist.Node{Text("No default.")}

	if got := RemoveDefault(nodes); &got[0] != &nodes[0] {
		t.Error("should return the nodes unchanged")
	}

	got := RemoveDefault([]*unist.Node{Text("Before"), Text(" "), strong("Default:"), code("1"), Text(" after")})

	if !reflect.DeepEqual(got, []*unist.Node{Text("Before")}) {
		t.Errorf("got %v", got)
	}
}
