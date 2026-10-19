// Package jsonall is the generator bundling the `json` generator's documents
// into one `all.json` file.
package jsonall

import (
	"math"
	"path/filepath"
	"sort"

	"github.com/nodejs/doc-kit/internal/collate"
	"github.com/nodejs/doc-kit/internal/fsutil"
	"github.com/nodejs/doc-kit/internal/generators"
	"github.com/nodejs/doc-kit/internal/generators/json"
	"github.com/nodejs/doc-kit/internal/omap"
)

// SchemaURL is where a version of the bundle's schema is published.
const SchemaURL = "https://doc-kit.nodejs.org/schemas/api-doc-all/{schemaVersion}.json"

func init() {
	generators.Register(&generators.Generator{
		Name:        "json-all",
		Description: "Bundles the documents of the `json` generator into a single `all.json` file",
		DependsOn:   "json",
		Public:      true,
		Defaults: func(map[string]any) map[string]any {
			return map[string]any{"schemaURL": SchemaURL}
		},
		Generate: generate,
	})
}

func generate(ctx *generators.Context, input any) (any, error) {
	o := ctx.Options

	// Documents follow the configured index; the rest go after it, by id
	order := map[string]float64{}

	for i, entry := range o.Index() {
		if _, ok := order[entry.API]; !ok {
			order[entry.API] = float64(i)
		}
	}

	position := func(id string) float64 {
		if p, ok := order[id]; ok {
			return p
		}

		return math.Inf(1)
	}

	documents := append([]*omap.Map{}, input.([]*omap.Map)...)

	sort.SliceStable(documents, func(i, j int) bool {
		a, _ := documents[i].Value("id").(string)
		b, _ := documents[j].Value("id").(string)

		if pa, pb := position(a), position(b); pa != pb {
			return pa < pb
		}

		return collate.Compare(a, b) < 0
	})

	bundle := omap.New("$schema", json.ResolveSchemaURL(o), "documents", documents)

	if out := o.String("output"); out != "" {
		if err := fsutil.WriteJSON(filepath.Join(out, "all.json"), bundle, o.Bool("minify")); err != nil {
			return nil, err
		}
	}

	return bundle, nil
}
