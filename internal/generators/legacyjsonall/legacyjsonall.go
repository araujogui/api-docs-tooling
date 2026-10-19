// Package legacyjsonall is the generator consolidating the legacy JSON files
// into one `all.json` file.
package legacyjsonall

import (
	"math"
	"path/filepath"
	"sort"

	"github.com/nodejs/doc-kit/internal/config"
	"github.com/nodejs/doc-kit/internal/fsutil"
	"github.com/nodejs/doc-kit/internal/generators"
	_ "github.com/nodejs/doc-kit/internal/generators/legacyjson"
	"github.com/nodejs/doc-kit/internal/legacy"
	"github.com/nodejs/doc-kit/internal/omap"
)

func init() {
	generators.Register(&generators.Generator{
		Name:        "legacy-json-all",
		Description: "Generates the `all.json` file from the `legacy-json` generator, which includes all the modules in one single file.",
		DependsOn:   "legacy-json",
		Public:      true,
		Defaults: func(map[string]any) map[string]any {
			return map[string]any{"minify": false, "sourceURL": "doc/api/{path}"}
		},
		Generate: generate,
	})
}

var properties = []string{"miscs", "modules", "classes", "globals", "methods"}

func generate(ctx *generators.Context, input any) (any, error) {
	o := ctx.Options
	output := omap.New()

	for _, p := range properties {
		output.Set(p, []any{})
	}

	order := map[string]float64{}

	for i, entry := range o.Index() {
		order[config.Populate(o.String("sourceURL"), config.Options{"path": entry.API + ".md"})] = float64(i)
	}

	position := func(s *omap.Map) float64 {
		source, _ := s.Value("source").(string)

		if p, ok := order[source]; ok {
			return p
		}

		return math.Inf(1)
	}

	sections := append([]*omap.Map{}, input.([]*omap.Map)...)

	sort.SliceStable(sections, func(i, j int) bool {
		return position(sections[i]) < position(sections[j])
	})

	for _, section := range sections {
		// index.json only holds navigation
		if section.Value("api") == "index" {
			continue
		}

		source := section.Value("source")

		for _, p := range properties {
			items, ok := section.Value(p).([]any)

			if !ok {
				continue
			}

			list := output.Value(p).([]any)

			for _, item := range items {
				if m, isMap := item.(*omap.Map); isMap && source != nil && source != "" {
					c := m.Clone()
					c.Set("source", source)
					item = c
				}

				list = append(list, item)
			}

			output.Set(p, list)
		}
	}

	if out := o.String("output"); out != "" {
		indent := 2

		if o.Bool("minify") {
			indent = 0
		}

		data, err := legacy.ToJSON(output, indent)
		if err != nil {
			return nil, err
		}

		if err := fsutil.WriteFile(filepath.Join(out, "all.json"), data); err != nil {
			return nil, err
		}
	}

	return output, nil
}
