package markdown

import (
	"reflect"
	"strings"
	"testing"

	"github.com/nodejs/doc-kit/internal/unist"
)

func parseGFM(source string) *unist.Node {
	return Parse(source, Options{GFM: true, TypeAnnotations: true})
}

func valuesIn(source string) []string {
	var values []string

	for _, n := range unist.SelectAll(parseGFM(source), "typeAnnotation") {
		values = append(values, n.Value)
	}

	return values
}

func textsIn(source string) []string {
	var values []string

	for _, n := range unist.SelectAll(parseGFM(source), "text") {
		values = append(values, n.Value)
	}

	return values
}

func TestTypeAnnotations(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   []string
	}{
		{"a simple annotation in prose", "A {string} here.", []string{"string"}},
		{"several annotations in one paragraph", "{string} and {Buffer|Blob} and {integer}", []string{"string", "Buffer | Blob", "integer"}},
		{"union separators are spaced", "Takes {string|URL} or {string  |  URL}.", []string{"string | URL", "string | URL"}},
		{"`||` is left alone", "Defaults to {req.url || '/'}.", []string{"req.url || '/'"}},
		{"nested braces", "Takes {Record<string, {a: number}>} maps.", []string{"Record<string, {a: number}>"}},
		{"generics", "Returns {Promise<string>} always.", []string{"Promise<string>"}},
		{"arrow function types", "A {(err: Error) => void} callback.", []string{"(err: Error) => void"}},
		{"legacy character escapes", `An array {string\[]} of them.`, []string{"string[]"}},
		{"escaped pipes in table cells", "| Option | Type |\n| ------ | ---- |\n| `flag` | {string\\|number} |", []string{"string | number"}},
		{"interior line breaks", "Some {string |\nBuffer} union.", []string{"string | Buffer"}},
		{"an escaped opening brace", `Literal \{not a type} braces.`, nil},
		{"template literal prose", "Use ${foo} for interpolation.", nil},
		{"empty braces", "An empty {} object.", nil},
		{"prose that cannot start a type", "and code points U+007B ({), and U+007D (}).", nil},
		{"an annotation right after punctuation", `In batches ({Uint8Array\[]} per iteration).`, []string{"Uint8Array[]"}},
		{"an annotation right after a code span", "* `maxSessionMemory`{number} Sets the maximum", []string{"number"}},
		{"a type opening with an operator", "A {-1 | 0 | 1} literal.", []string{"-1 | 0 | 1"}},
		{"a type opening with an escape", `An {\[]} array.`, []string{"[]"}},
		{"leading whitespace inside the braces", "Headers { string | URL | Object } here.", []string{"string | URL | Object"}},
		{"code spans", "Use `{string}` literally.", nil},
		{"fenced code blocks", "```js\nconst a = {string: 1};\n```", nil},
		{"URLs in braces", "See {http://example.com} for more.", []string{"http://example.com"}},
		{"emphasis and headings", "_a {string} inside_\n\n## Head {integer}", []string{"string", "integer"}},
	}

	for _, c := range cases {
		if got := valuesIn(c.source); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}

	t.Run("prose that cannot start a type stays one text", func(t *testing.T) {
		source := "and code points U+007B ({), and U+007D (})."

		if got := textsIn(source); !reflect.DeepEqual(got, []string{source}) {
			t.Errorf("got %q", got)
		}
	})

	t.Run("an unbalanced brace stays literal text", func(t *testing.T) {
		if got := strings.Join(textsIn("An { unclosed brace."), ""); !strings.Contains(got, "{ unclosed brace.") {
			t.Errorf("got %q", got)
		}
	})

	t.Run("round-trips through stringify", func(t *testing.T) {
		got := ToMarkdown(parseGFM("Returns: {Promise<Buffer|string>} on success."))

		if !strings.Contains(got, "{Promise<Buffer | string>}") {
			t.Errorf("got %q", got)
		}
	})

	t.Run("is off in MDX", func(t *testing.T) {
		if got := unist.SelectAll(Parse("A {string} here.", Options{GFM: true}), "typeAnnotation"); len(got) != 0 {
			t.Errorf("got %v", got)
		}
	})
}
