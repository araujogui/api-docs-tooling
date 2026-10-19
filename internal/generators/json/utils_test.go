package json

import (
	stdjson "encoding/json"
	"reflect"
	"testing"

	"github.com/nodejs/doc-kit/internal/doc"
	"github.com/nodejs/doc-kit/internal/omap"
	. "github.com/nodejs/doc-kit/internal/testutil"
	"github.com/nodejs/doc-kit/internal/unist"
)

// sameJSON compares a value with the JSON it should serialize as, ignoring
// key order.
func sameJSON(t *testing.T, got any, want string) {
	t.Helper()

	data, err := omap.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}

	var a, b any

	if err := stdjson.Unmarshal(data, &a); err != nil {
		t.Fatal(err)
	}

	if err := stdjson.Unmarshal([]byte(want), &b); err != nil {
		t.Fatalf("bad expectation: %v", err)
	}

	if !reflect.DeepEqual(a, b) {
		t.Errorf("got  %s\nwant %s", data, want)
	}
}

func code(v string) *unist.Node { return U("inlineCode", v) }

func typeAnn(v string, links ...unist.TypeLink) *unist.Node {
	return U("typeAnnotation", v, P{"data": &unist.Data{Links: links}})
}

func strong(v string) *unist.Node { return U("strong", Text(v)) }

func item(children []*unist.Node, blocks ...*unist.Node) *unist.Node {
	return U("listItem", Paragraph(children...), blocks)
}

func nodes(n ...*unist.Node) []*unist.Node { return n }

func headingNode(text, name, typ string) *unist.Node {
	return &unist.Node{Type: "heading", Depth: 3, Data: &unist.Data{Text: text, Name: name, HeadType: typ}}
}

func TestToVersions(t *testing.T) {
	sameJSON(t, toVersions(nil), `[]`)
	sameJSON(t, toVersions("v1.0.0"), `["v1.0.0"]`)
	sameJSON(t, toVersions([]any{"v2.0.0", "v1.5.0"}), `["v2.0.0","v1.5.0"]`)
	sameJSON(t, toVersions("REPLACEME"), `["REPLACEME"]`)
	sameJSON(t, toVersions(float64(10)), `["10"]`)
}

func TestToNumbers(t *testing.T) {
	sameJSON(t, toNumbers(nil), `[]`)
	sameJSON(t, toNumbers(float64(3)), `[3]`)
	sameJSON(t, toNumbers([]any{"1", float64(2)}), `[1,2]`)
	sameJSON(t, toNumbers("v3"), `[null]`)
}

func TestToChanges(t *testing.T) {
	sameJSON(t, toChanges([]any{
		omap.New("version", "v1.0.0", "pr-url", "https://github.com/example/pull/1", "description", "  Trimmed.  "),
		omap.New("version", []any{"v0.2.0", "v0.1.0"}, "commit", "abc123", "description", "Old."),
	}), `[
		{"versions":["v1.0.0"],"prUrl":"https://github.com/example/pull/1","commit":null,"description":"Trimmed."},
		{"versions":["v0.2.0","v0.1.0"],"prUrl":null,"commit":"abc123","description":"Old."}
	]`)

	sameJSON(t, toChanges(nil), `[]`)
}

func stabilityNode(index, description string, children ...*unist.Node) *unist.Node {
	return U("blockquote", P{"data": &unist.Data{IsStability: true, Index: index, Description: description}}, Paragraph(children...))
}

func TestToStability(t *testing.T) {
	if toStability(nil) != nil {
		t.Error("expected nil")
	}

	sameJSON(t, toStability(stabilityNode("1.1", "",
		U("link", P{"url": "documentation.html#stability-index"}, Text("Stability: 1.1")),
		Text(" - Active development. Use "), code("other()"), Text(" instead."),
	)), `{"index":"1.1","description":"Active development. Use `+"`other()`"+` instead."}`)

	sameJSON(t, toStability(stabilityNode("2", "", Text("Stability: 2 - Stable"))), `{"index":"2","description":"Stable"}`)
	sameJSON(t, toStability(stabilityNode("0", "fallback", Text("Stability: 0"))), `{"index":"0","description":"fallback"}`)
}

func TestBlocksToMarkdown(t *testing.T) {
	got := BlocksToMarkdown(nodes(
		Paragraph(Text("Returns a "), U("typeAnnotation", "Promise<string>"), Text(".")),
		U("code", P{"lang": "js"}, "foo();"),
	))

	if got != "Returns a {Promise<string>}.\n\n```js\nfoo();\n```" {
		t.Errorf("got %q", got)
	}

	if BlocksToMarkdown(nil) != "" {
		t.Error("expected nothing")
	}

	if got := BlocksToMarkdown(nodes(Paragraph(Text("See "), U("link", P{"url": "#foo"}, code("foo()")), Text(".")))); got != "See [`foo()`](#foo)." {
		t.Errorf("got %q", got)
	}
}

func TestExtractExamples(t *testing.T) {
	sameJSON(t, extractExamples(nodes(
		Paragraph(Text("Text")),
		U("code", P{"lang": "mjs", "meta": `displayName="ESM"`}, "import x;"),
		U("blockquote", U("code", "plain")),
	)), `[{"language":"mjs","displayName":"ESM","code":"import x;"},{"language":null,"displayName":null,"code":"plain"}]`)
}

func classifyEntry(text, typ string) *doc.Entry {
	h := headingNode(text, text, typ)
	h.Children = []*unist.Node{code(stripTicks(text))}

	return &doc.Entry{Heading: h, Props: omap.New()}
}

func stripTicks(s string) string {
	out := []rune{}

	for _, r := range s {
		if r != '`' {
			out = append(out, r)
		}
	}

	return string(out)
}

func TestClassify(t *testing.T) {
	cases := []struct{ text, typ, kind, scope string }{
		{"`foo()`", "method", "method", ""},
		{"`new Foo()`", "ctor", "constructor", ""},
		{"`Foo.bar()`", "classMethod", "staticMethod", ""},
		{"Notes", "", "section", ""},
		{"Notes", "misc", "section", ""},
		{"Example", "example", "section", ""},
		// A global override becomes a scope, the kind re-read from the heading
		{"`globalThis.foo()`", "global", "method", "global"},
	}

	for _, c := range cases {
		e := classifyEntry(c.text, c.typ)
		e.Heading.Children = []*unist.Node{Text(c.text)}

		if kind, scope := classify(e); kind != c.kind || scope != c.scope {
			t.Errorf("classify(%q, %q) = %q, %q", c.text, c.typ, kind, scope)
		}
	}
}

func TestNodeName(t *testing.T) {
	e := classifyEntry("Signal events", "event")
	e.Props.Set("name", "SIGINT, SIGHUP, etc.")

	if got := nodeName(e, "event"); got != "SIGINT, SIGHUP, etc." {
		t.Errorf("got %q", got)
	}

	section := &doc.Entry{Props: omap.New(), Heading: &unist.Node{Type: "heading", Data: &unist.Data{}, Children: nodes(Text("DEP0005: "), code("Buffer()"), Text(" constructor"))}}

	if got := nodeName(section, "section"); got != "DEP0005: Buffer() constructor" {
		t.Errorf("got %q", got)
	}

	named := func(name string) *doc.Entry {
		return &doc.Entry{Props: omap.New(), Heading: &unist.Node{Type: "heading", Data: &unist.Data{Text: name, Name: name}}}
	}

	for _, c := range []struct{ name, kind, want string }{
		{"http.Server", "class", "Server"},
		{"Foo extends Bar", "class", "Foo"},
		{"buffer.Blob", "constructor", "Blob"},
		{"readFile", "method", "readFile"},
		{"close", "event", "close"},
	} {
		if got := nodeName(named(c.name), c.kind); got != c.want {
			t.Errorf("nodeName(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestTakeTypedItems(t *testing.T) {
	typed1 := item(nodes(code("name"), Text(" "), typeAnn("string")))
	typed2 := item(nodes(code("options"), Text(" "), typeAnn("Object")))
	prose := item(nodes(Text("A prose bullet that shares the list.")))
	list := U("list", typed1, typed2, prose)
	after := Paragraph(Text("After."))

	t.Run("lifts the leading typed items, leaving the rest", func(t *testing.T) {
		body, items := takeTypedItems(nodes(list, after), "method")

		if len(items) != 2 || len(body) != 2 || !reflect.DeepEqual(body[0].Children, nodes(prose)) || body[1] != after || len(list.Children) != 3 {
			t.Errorf("got %v, %v", body, items)
		}
	})

	t.Run("removes a fully typed list", func(t *testing.T) {
		before := Paragraph(Text("Before."))
		body, items := takeTypedItems(nodes(before, U("list", typed1, typed2)), "event")

		if len(items) != 2 || !reflect.DeepEqual(body, nodes(before)) {
			t.Errorf("got %v, %v", body, items)
		}
	})

	t.Run("leaves the list of a section alone", func(t *testing.T) {
		if body, items := takeTypedItems(nodes(list), "section"); len(body) != 1 || len(items) != 0 {
			t.Errorf("got %v, %v", body, items)
		}
	})

	t.Run("only lifts a class list with an Extends item", func(t *testing.T) {
		if body, items := takeTypedItems(nodes(list), "class"); len(body) != 1 || len(items) != 0 {
			t.Errorf("got %v, %v", body, items)
		}

		extendsList := U("list", item(nodes(Text("Extends: "), typeAnn("EventEmitter"))))

		if body, items := takeTypedItems(nodes(extendsList), "class"); len(body) != 0 || !reflect.DeepEqual(items, extendsList.Children) {
			t.Errorf("got %v, %v", body, items)
		}
	})

	t.Run("does nothing without a typed list", func(t *testing.T) {
		if body, items := takeTypedItems(nodes(Paragraph(Text("Only prose."))), "method"); len(body) != 1 || len(items) != 0 {
			t.Errorf("got %v, %v", body, items)
		}
	})
}

func TestEntryBody(t *testing.T) {
	heading := U("heading", P{"depth": 2}, Text("Title"))
	stability := stabilityNode("1", "Experimental", Text("Stability: 1 - Experimental"))
	prose := Paragraph(Text("Body."))

	if got := entryBody(&doc.Entry{Heading: heading, Stability: stability, Content: U("root", heading, stability, prose)}); !reflect.DeepEqual(got, nodes(prose)) {
		t.Errorf("got %v", got)
	}

	// A stability blockquote the entry does not own stays
	if got := entryBody(&doc.Entry{Heading: heading, Content: U("root", heading, stability)}); !reflect.DeepEqual(got, nodes(stability)) {
		t.Errorf("got %v", got)
	}
}

func TestBuildEntry(t *testing.T) {
	heading := U("heading", P{"depth": 2, "data": &unist.Data{Text: "`foo()`"}}, code("foo()"))
	body := nodes(Paragraph(Text("Does a thing.")), U("code", P{"lang": "js", "meta": `displayName="Usage"`}, "foo();"))

	e := &doc.Entry{Heading: heading, Content: U("root", append(nodes(heading), body...)), Props: omap.New(
		"added", "v1.0.0",
		"napiVersion", float64(3),
		"changes", []any{omap.New("version", []any{"v2.0.0", "v1.5.0"}, "description", "Changed.")},
	)}

	sameJSON(t, buildEntry(e, body), `{
		"title": "`+"`foo()`"+`",
		"stability": null,
		"added": ["v1.0.0"],
		"deprecated": [],
		"removed": [],
		"napiVersion": [3],
		"changes": [{"versions":["v2.0.0","v1.5.0"],"prUrl":null,"commit":null,"description":"Changed."}],
		"description": "Does a thing.\n\n`+"```js displayName=\\\"Usage\\\"\\nfoo();\\n```"+`",
		"summary": "Does a thing.",
		"examples": [{"language":"js","displayName":"Usage","code":"foo();"}]
	}`)

	t.Run("prefers the llm_description as the summary", func(t *testing.T) {
		h := U("heading", P{"depth": 2, "data": &unist.Data{Text: "Foo"}}, Text("Foo"))
		body := nodes(Paragraph(Text("The first paragraph.")))
		e := &doc.Entry{Heading: h, Content: U("root", append(nodes(h), body...)), Props: omap.New("llm_description", "For machines.")}

		if got := buildEntry(e, body).Value("summary"); got != "For machines." {
			t.Errorf("got %v", got)
		}
	})
}

func TestBuildSignature(t *testing.T) {
	items := nodes(
		item(nodes(code("path"), Text(" "), typeAnn("string | URL"), Text(" The file."))),
		item(nodes(code("options"), Text(" "), typeAnn("Object")),
			U("list", item(nodes(code("encoding"), Text(" "), typeAnn("string"), Text(" The encoding. "), strong("Default:"), Text(" "), code("'utf8'"), Text("."))))),
		item(nodes(Text("Returns: "), typeAnn("Promise"), Text(" Fulfills with the file."))),
	)

	sameJSON(t, buildSignature(headingNode("`fs.readFile(path[, options], callback)`", "", ""), items), `{
		"parameters": [
			{"name":"path","type":{"text":"string | URL","links":[]},"description":"The file.","default":null,"optional":false,"rest":false,"properties":[]},
			{"name":"options","type":{"text":"Object","links":[]},"description":"","default":null,"optional":true,"rest":false,"properties":[
				{"name":"encoding","type":{"text":"string","links":[]},"description":"The encoding.","default":"'utf8'","optional":true,"rest":false,"properties":[]}
			]},
			{"name":"callback","type":null,"description":"","default":null,"optional":false,"rest":false,"properties":[]}
		],
		"returns": {"type":{"text":"Promise","links":[]},"description":"Fulfills with the file."}
	}`)

	t.Run("carries the type links through", func(t *testing.T) {
		link := unist.TypeLink{Text: "Buffer", Href: "buffer.html#class-buffer", Start: 0, End: 6}
		sig := buildSignature(headingNode("`fill(buf)`", "", ""), nodes(item(nodes(code("buf"), Text(" "), typeAnn("Buffer", link)))))
		first := sig.Value("parameters").([]any)[0].(*omap.Map)

		sameJSON(t, first.Value("type").(*omap.Map).Value("links"), `[{"name":"Buffer","href":"buffer.html#class-buffer","start":0,"end":6}]`)
	})

	t.Run("marks rest parameters", func(t *testing.T) {
		sig := buildSignature(headingNode("`path.join([...paths])`", "", ""), nodes(item(nodes(code("...paths"), Text(" "), typeAnn("string")))))

		sameJSON(t, sig.Value("parameters"), `[{"name":"paths","type":{"text":"string","links":[]},"description":"","default":null,"optional":true,"rest":true,"properties":[]}]`)
	})

	t.Run("is empty without a list or parameters", func(t *testing.T) {
		sameJSON(t, buildSignature(headingNode("`emitter.close()`", "", ""), nil), `{"parameters":[],"returns":null}`)
	})
}

func TestBuildExtends(t *testing.T) {
	sameJSON(t, buildExtends(headingNode("Class: `net.Server`", "net.Server", ""), nodes(item(nodes(Text("Extends: "), typeAnn("EventEmitter"))))), `{"text":"EventEmitter","links":[]}`)
	sameJSON(t, buildExtends(headingNode("Class: `Foo extends Bar`", "Foo extends Bar", ""), nil), `{"text":"Bar","links":[]}`)

	if got := buildExtends(headingNode("Class: `Foo`", "Foo", ""), nil); got != nil {
		t.Errorf("got %v", got)
	}
}

func TestBuildPropertyType(t *testing.T) {
	typ, def, description := buildPropertyType(nodes(item(nodes(Text("Type: "), typeAnn("number"), Text(" The size. "), strong("Default:"), Text(" "), code("1")))))

	sameJSON(t, typ, `{"text":"number","links":[]}`)

	if def != "1" || description != "The size." {
		t.Errorf("got %v, %q", def, description)
	}

	if typ, def, description := buildPropertyType(nil); typ != nil || def != nil || description != "" {
		t.Error("expected nothing")
	}
}

func TestBuildEventParameters(t *testing.T) {
	got := buildEventParameters(nodes(
		item(nodes(code("code"), Text(" "), typeAnn("number"), Text(" The exit code."))),
		item(nodes(code("signal"), Text(" "), typeAnn("string | null"))),
	))

	var summary [][3]string

	for _, p := range got {
		m := p.(*omap.Map)
		summary = append(summary, [3]string{m.Value("name").(string), m.Value("type").(*omap.Map).Value("text").(string), m.Value("description").(string)})
	}

	if !reflect.DeepEqual(summary, [][3]string{{"code", "number", "The exit code."}, {"signal", "string | null", ""}}) {
		t.Errorf("got %v", summary)
	}
}
