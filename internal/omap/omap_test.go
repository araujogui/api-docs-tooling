package omap

import "testing"

func TestMap(t *testing.T) {
	m := New("b", 1, "a", 2)
	m.Set("c", 3).Set("b", 4)

	if keys := m.Keys(); len(keys) != 3 || keys[0] != "b" || keys[1] != "a" || keys[2] != "c" {
		t.Errorf("keys %v", keys)
	}

	m.Delete("a")

	if m.Has("a") || m.Len() != 2 {
		t.Error("delete failed")
	}

	if v, _ := m.Get("b"); v != 4 {
		t.Errorf("got %v", v)
	}
}

func TestStringify(t *testing.T) {
	m := New("z", "<a & b>", "u", Undefined, "list", []any{1, Undefined, New()}, "empty", []any{}, "line", "a\u2028b")

	got, err := Stringify(m, 0)
	if err != nil {
		t.Fatal(err)
	}

	// No HTML escaping, undefined keys skipped and undefined items null, and
	// line separators written as they are, like JSON.stringify
	want := "{\"z\":\"<a & b>\",\"list\":[1,null,{}],\"empty\":[],\"line\":\"a\u2028b\"}"

	if string(got) != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}

	indented, _ := Stringify(New("a", []any{}, "b", New("c", 1)), 2)

	if string(indented) != "{\n  \"a\": [],\n  \"b\": {\n    \"c\": 1\n  }\n}" {
		t.Errorf("got %s", indented)
	}
}

func TestUnescapeLineSeparators(t *testing.T) {
	// An escaped backslash before `u2028` text is not an escape of its own
	in := []byte(`"\\u2028"`)

	if got := unescapeLineSeparators(in); string(got) != string(in) {
		t.Errorf("got %s", got)
	}
}
