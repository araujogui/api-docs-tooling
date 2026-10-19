package tstype

import (
	"reflect"
	"testing"
)

func texts(ids []Identifier) []string {
	var out []string

	for _, id := range ids {
		out = append(out, id.Text)
	}

	return out
}

func TestParse(t *testing.T) {
	valid := []struct {
		value string
		want  []string
	}{
		{"string", []string{"string"}},
		{"Buffer | string", []string{"Buffer", "string"}},
		{"Promise<string>", []string{"Promise", "string"}},
		{"string[]", []string{"string"}},
		{"Array<Buffer|null>", []string{"Array", "Buffer", "null"}},
		{"vm.Module", []string{"vm.Module"}},
		{"(err: Error) => void", []string{"Error", "void"}},
		{"(err: Error, data?: Buffer) => void", []string{"Error", "Buffer", "void"}},
		{"{ buffer: Buffer, error: Error }", []string{"Buffer", "Error"}},
		{"{ [key: string]: number }", []string{"string", "number"}},
		{"{ [K in keyof T]: T[K] }", []string{"T", "T", "K"}},
		{"Record<string, {a: number}>", []string{"Record", "string", "number"}},
		{"typeof Foo", []string{"Foo"}},
		{"import('fs').Stats", []string{"Stats"}},
		{"this", []string{"this"}},
		{"'a' | 'b'", nil},
		{"-1 | 0 | 1", nil},
		{"true | undefined", []string{"undefined"}},
		{"[string, number?]", []string{"string", "number"}},
		{"[name: string, ...rest: number[]]", []string{"string", "number"}},
		{"T extends string ? A : B", []string{"T", "string", "A", "B"}},
		{"`prefix-${Name}`", []string{"Name"}},
		{"new (...args: any[]) => object", []string{"any", "object"}},
		{"<T>(value: T) => T", []string{"T", "T"}},
		{"(string | number)[]", []string{"string", "number"}},
		{"| A | B", []string{"A", "B"}},
		{"unique symbol", []string{"symbol"}},
		{"readonly string[]", []string{"string"}},
		{"Function", []string{"Function"}},
		{"integer", []string{"integer"}},
	}

	for _, c := range valid {
		ids, ok := Parse(c.value)

		if !ok {
			t.Errorf("Parse(%q) failed", c.value)
			continue
		}

		if got := texts(ids); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Parse(%q) = %q, want %q", c.value, got, c.want)
		}
	}

	invalid := []string{
		"HTTP/2 Headers Object",
		"Module Namespace Object | vm.Module",
		"req.url || '/'",
		"function",
		"string |",
		"...any",
		"?string",
		"Array<string",
		"",
	}

	for _, value := range invalid {
		if _, ok := Parse(value); ok {
			t.Errorf("Parse(%q) should fail", value)
		}
	}
}

func TestOffsets(t *testing.T) {
	ids, _ := Parse("Map<string, vm.Module>")

	want := []Identifier{{0, 3, "Map"}, {4, 10, "string"}, {12, 21, "vm.Module"}}

	if !reflect.DeepEqual(ids, want) {
		t.Errorf("got %v", ids)
	}

	// Offsets count UTF-16 code units
	ids, _ = Parse("{ 'é': Buffer }")

	if ids[0].Start != 7 || ids[0].End != 13 {
		t.Errorf("got %v", ids)
	}
}
