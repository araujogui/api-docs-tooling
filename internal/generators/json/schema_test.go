package json

import (
	stdjson "encoding/json"
	"os"
	"strings"
	"testing"
)

// The schemas are published from the JavaScript package, which the documents
// must stay in sync with.
const schemaDir = "../../../packages/core/src/generators/"

func readSchema(t *testing.T, path string) map[string]any {
	t.Helper()

	data, err := os.ReadFile(schemaDir + path)
	if err != nil {
		t.Fatal(err)
	}

	var schema map[string]any

	if err := stdjson.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}

	return schema
}

func TestSchemaVersion(t *testing.T) {
	schema := readSchema(t, "json/schema.json")
	bundle := readSchema(t, "json-all/schema.json")

	for _, s := range []map[string]any{schema, bundle} {
		if id, _ := s["$id"].(string); !strings.HasSuffix(id, "/"+SchemaVersion+".json") {
			t.Errorf("$id %q is not versioned %s", id, SchemaVersion)
		}
	}

	ref := bundle["properties"].(map[string]any)["documents"].(map[string]any)["items"].(map[string]any)["$ref"]

	if ref != schema["$id"] {
		t.Errorf("the bundle references %v, not %v", ref, schema["$id"])
	}
}
