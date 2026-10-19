package metadata

import (
	"reflect"
	"testing"

	"github.com/nodejs/doc-kit/internal/unist"
)

var http2TypeMap = map[string]string{"HTTP/2 Headers Object": "http2.html#headers-object"}

const stringURL = "https://developer.mozilla.org/en-US/docs/Web/JavaScript/Data_structures#string_type"

func resolve(value string, typeMap map[string]string) *unist.Node {
	n := &unist.Node{Type: "typeAnnotation", Value: value}
	ResolveTypeAnnotations(unist.Root(n), typeMap, "test.md")

	return n
}

// linksIn resolves a value's links as text/href pairs, checking each range
// lines up with the text it links.
func linksIn(t *testing.T, value string, typeMap map[string]string) [][2]string {
	t.Helper()

	var out [][2]string

	for _, l := range resolve(value, typeMap).Data.Links {
		if value[l.Start:l.End] != l.Text {
			t.Errorf("range %d-%d of %q is not %q", l.Start, l.End, value, l.Text)
		}

		out = append(out, [2]string{l.Text, l.Href})
	}

	return out
}

func TestResolveTypeAnnotations(t *testing.T) {
	t.Run("links a type that is a whole map key", func(t *testing.T) {
		warnings := capture(func() {
			if got := linksIn(t, "HTTP/2 Headers Object", http2TypeMap); !reflect.DeepEqual(got, [][2]string{{"HTTP/2 Headers Object", "http2.html#headers-object"}}) {
				t.Errorf("got %v", got)
			}
		})

		if len(warnings) != 0 {
			t.Errorf("warned %v", warnings)
		}
	})

	t.Run("links an array of a display name", func(t *testing.T) {
		if got := linksIn(t, "HTTP/2 Headers Object[]", http2TypeMap); !reflect.DeepEqual(got, [][2]string{{"HTTP/2 Headers Object", "http2.html#headers-object"}}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("resolves module-qualified names alongside display names", func(t *testing.T) {
		want := [][2]string{
			{"Module Namespace Object", "https://tc39.github.io/ecma262/#sec-module-namespace-exotic-objects"},
			{"vm.Module", "vm.html#class-vmmodule"},
		}

		if got := linksIn(t, "Module Namespace Object | vm.Module", nil); !reflect.DeepEqual(got, want) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("keeps the resolvable parts of a partly unknown union", func(t *testing.T) {
		warnings := capture(func() {
			if got := linksIn(t, "HTTP/2 Headers Object | HTTP/2 Raw Headers", http2TypeMap); !reflect.DeepEqual(got, [][2]string{{"HTTP/2 Headers Object", "http2.html#headers-object"}}) {
				t.Errorf("got %v", got)
			}
		})

		if len(warnings) != 0 {
			t.Errorf("warned %v", warnings)
		}
	})

	t.Run("warns when no part of an unparsable type resolves", func(t *testing.T) {
		var n *unist.Node

		warnings := capture(func() { n = resolve("HTTP/2 Raw Headers", http2TypeMap) })

		if !n.Data.ParseError || len(n.Data.Links) != 0 {
			t.Errorf("got %+v", n.Data)
		}

		if !reflect.DeepEqual(warnings, []string{"Invalid type annotation: {HTTP/2 Raw Headers}"}) {
			t.Errorf("warned %v", warnings)
		}
	})

	t.Run("does not read || in prose as a union", func(t *testing.T) {
		var n *unist.Node

		warnings := capture(func() { n = resolve("req.url || '/'", nil) })

		if len(n.Data.Links) != 0 || !reflect.DeepEqual(warnings, []string{"Invalid type annotation: {req.url || '/'}"}) {
			t.Errorf("got %+v, warned %v", n.Data, warnings)
		}
	})

	t.Run("marks which values are TypeScript", func(t *testing.T) {
		cases := []struct {
			value   string
			typeMap map[string]string
			want    bool
		}{
			{"Promise<string> | null", nil, true},
			{"HTTP/2 Headers Object", http2TypeMap, false},
			{"Module Namespace Object | vm.Module", nil, false},
			{"HTTP/2 Raw Headers", http2TypeMap, false},
		}

		capture(func() {
			for _, c := range cases {
				if got := resolve(c.value, c.typeMap).Data.TypeScript; got != c.want {
					t.Errorf("%q: got %v", c.value, got)
				}
			}
		})
	})

	t.Run("parses valid TypeScript rather than splitting it", func(t *testing.T) {
		want := [][2]string{
			{"Promise", "https://developer.mozilla.org/docs/Web/JavaScript/Reference/Global_Objects/Promise"},
			{"string", stringURL},
			{"Buffer", "buffer.html#buffer"},
		}

		if got := linksIn(t, "Promise<string> | Buffer", map[string]string{"Buffer": "buffer.html#buffer"}); !reflect.DeepEqual(got, want) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("resolves every annotation in a nested tree", func(t *testing.T) {
		var nodes []*unist.Node

		for _, v := range []string{"string", "HTTP/2 Headers Object", "string", "string"} {
			nodes = append(nodes, &unist.Node{Type: "typeAnnotation", Value: v})
		}

		ResolveTypeAnnotations(unist.Root(
			unist.Parent("paragraph", nodes[:2]...),
			unist.Parent("paragraph", nodes[2:]...),
		), http2TypeMap, "test.md")

		var got [][]string

		for _, n := range nodes {
			var hrefs []string

			for _, l := range n.Data.Links {
				hrefs = append(hrefs, l.Href)
			}

			got = append(got, hrefs)
		}

		want := [][]string{{stringURL}, {"http2.html#headers-object"}, {stringURL}, {stringURL}}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v", got)
		}
	})
}
