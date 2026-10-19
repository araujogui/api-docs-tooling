package jsonall

import (
	"reflect"
	"testing"

	"github.com/nodejs/doc-kit/internal/config"
	"github.com/nodejs/doc-kit/internal/generators"
	"github.com/nodejs/doc-kit/internal/generators/json"
	"github.com/nodejs/doc-kit/internal/omap"
	"github.com/nodejs/doc-kit/internal/parsers"
)

func TestGenerate(t *testing.T) {
	var documents []*omap.Map

	for _, id := range []string{"zlib", "fs", "assert", "http"} {
		documents = append(documents, omap.New("id", id, "path", "/"+id, "children", []any{}))
	}

	ctx := &generators.Context{Options: config.Options{
		"schemaURL": SchemaURL,
		"index":     []parsers.IndexEntry{{Section: "HTTP", API: "http"}, {Section: "File system", API: "fs"}},
	}}

	out, err := generate(ctx, documents)
	if err != nil {
		t.Fatal(err)
	}

	bundle := out.(*omap.Map)

	if bundle.Value("$schema") != "https://doc-kit.nodejs.org/schemas/api-doc-all/"+json.SchemaVersion+".json" {
		t.Errorf("got %v", bundle.Value("$schema"))
	}

	var ids []string

	for _, d := range bundle.Value("documents").([]*omap.Map) {
		ids = append(ids, d.Value("id").(string))
	}

	if !reflect.DeepEqual(ids, []string{"http", "fs", "assert", "zlib"}) {
		t.Errorf("got %v", ids)
	}
}
