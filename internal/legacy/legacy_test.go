package legacy

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nodejs/doc-kit/internal/omap"
)

func base(api any) *omap.Map {
	return omap.New(
		"type", "module", "source", "lib/fs.js", "introduced_in", "v0.10.0", "meta", omap.New(),
		"stability", 2, "stabilityText", "Stable", "classes", []any{}, "methods", []any{"readFile"},
		"properties", []any{}, "miscs", []any{}, "modules", []any{"fs"}, "globals", []any{}, "api", api,
	)
}

func decode(t *testing.T, section *omap.Map, indent int) map[string]any {
	t.Helper()

	data, err := ToJSON(section, indent)
	if err != nil {
		t.Fatal(err)
	}

	var out map[string]any

	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}

	return out
}

func TestToJSON(t *testing.T) {
	t.Run("serializes a section with all its keys", func(t *testing.T) {
		got := decode(t, base("fs"), 0)

		for _, k := range []string{"type", "methods", "modules"} {
			if _, ok := got[k]; !ok {
				t.Errorf("missing %q", k)
			}
		}
	})

	t.Run("omits modules for the index", func(t *testing.T) {
		if _, ok := decode(t, base("index"), 0)["modules"]; ok {
			t.Error("has modules")
		}
	})

	t.Run("uses the all.json keys without an api", func(t *testing.T) {
		got := decode(t, base(nil), 0)

		if _, ok := got["miscs"]; !ok {
			t.Error("missing miscs")
		}

		if _, ok := got["type"]; ok {
			t.Error("has type")
		}

		if _, ok := got["source"]; ok {
			t.Error("has source")
		}
	})

	t.Run("indents", func(t *testing.T) {
		data, _ := ToJSON(base("fs"), 2)

		if !strings.Contains(string(data), "\n") {
			t.Error("not indented")
		}
	})

	t.Run("keeps the original key order", func(t *testing.T) {
		data, _ := ToJSON(base("fs"), 0)

		if !strings.HasPrefix(string(data), `{"type":"module","source":"lib/fs.js","introduced_in":"v0.10.0","meta":{},"stability":2`) {
			t.Errorf("got %s", data)
		}
	})
}
