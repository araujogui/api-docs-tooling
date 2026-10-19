// Package metadata is the generator splitting parsed API docs into a flat
// list of metadata entries.
package metadata

import (
	"encoding/json"
	"fmt"

	"github.com/nodejs/doc-kit/internal/doc"
	"github.com/nodejs/doc-kit/internal/fsutil"
	"github.com/nodejs/doc-kit/internal/generators"
	_ "github.com/nodejs/doc-kit/internal/generators/ast"
	"github.com/nodejs/doc-kit/internal/metadata"
)

func init() {
	generators.Register(&generators.Generator{
		Name:        "metadata",
		Description: "generates a flattened list of API doc metadata entries",
		DependsOn:   "ast",
		Generate:    generate,
	})
}

func generate(ctx *generators.Context, input any) (any, error) {
	files := input.([]*doc.File)
	typeMap := map[string]string{}

	if location := ctx.Options.String("typeMap"); location != "" {
		data, err := fsutil.LoadFromURL(location)
		if err != nil {
			return nil, fmt.Errorf("reading the type map: %w", err)
		}

		if err := json.Unmarshal([]byte(data), &typeMap); err != nil {
			return nil, fmt.Errorf("parsing the type map: %w", err)
		}
	}

	results := make([][]*doc.Entry, len(files))

	err := ctx.Run.Parallel(len(files), func(i int) error {
		entries, err := metadata.ParseAPIDoc(files[i], typeMap)
		results[i] = entries

		return err
	})

	var entries []*doc.Entry

	for _, r := range results {
		entries = append(entries, r...)
	}

	return entries, err
}
