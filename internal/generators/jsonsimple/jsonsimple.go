// Package jsonsimple is the generator of a simplified JSON version of the
// API docs, for debugging and testing rather than publishing.
package jsonsimple

import (
	"path/filepath"

	"github.com/nodejs/doc-kit/internal/doc"
	"github.com/nodejs/doc-kit/internal/fsutil"
	"github.com/nodejs/doc-kit/internal/generators"
	_ "github.com/nodejs/doc-kit/internal/generators/metadata"
	"github.com/nodejs/doc-kit/internal/omap"
	"github.com/nodejs/doc-kit/internal/queries"
	"github.com/nodejs/doc-kit/internal/unist"
)

func init() {
	generators.Register(&generators.Generator{
		Name:        "json-simple",
		Description: "Generates the simple JSON version of the API docs, and returns it as a string",
		DependsOn:   "metadata",
		Public:      true,
		Generate:    generate,
	})
}

// EntryJSON turns an entry into the object it serializes as.
func EntryJSON(e *doc.Entry) *omap.Map {
	m := omap.New("api", e.API, "path", e.Path, "mdx", e.MDX, "basename", e.Basename, "heading", unist.ToJSON(e.Heading))

	if e.Stability != nil {
		m.Set("stability", unist.ToJSON(e.Stability))
	}

	for _, k := range e.Props.Keys() {
		v, _ := e.Props.Get(k)
		m.Set(k, v)
	}

	return m.Set("content", unist.ToJSON(e.Content))
}

func generate(ctx *generators.Context, input any) (any, error) {
	entries := doc.CloneEntries(input.([]*doc.Entry))
	out := make([]any, len(entries))

	for i, e := range entries {
		unist.Remove(e.Content, unist.AnyOf(queries.IsStabilityNode, queries.IsHeading))
		out[i] = EntryJSON(e)
	}

	if dir := ctx.Options.String("output"); dir != "" {
		if err := fsutil.WriteJSON(filepath.Join(dir, "api-docs.json"), out, ctx.Options.Bool("minify")); err != nil {
			return nil, err
		}
	}

	return out, nil
}
