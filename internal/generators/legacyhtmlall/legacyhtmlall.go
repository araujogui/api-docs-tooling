// Package legacyhtmlall is the generator of the legacy `all.html` page,
// holding every module of the `legacy-html` generator.
package legacyhtmlall

import (
	"path/filepath"
	"strings"

	"github.com/nodejs/doc-kit/internal/fsutil"
	"github.com/nodejs/doc-kit/internal/generators"
	"github.com/nodejs/doc-kit/internal/generators/legacyhtml"
	"github.com/nodejs/doc-kit/internal/legacy"
)

func init() {
	generators.Register(&generators.Generator{
		Name:        "legacy-html-all",
		Description: "Generates the `all.html` file from the `legacy-html` generator, which includes all the modules in one single file",
		DependsOn:   "legacy-html",
		Public:      true,
		Defaults: func(map[string]any) map[string]any {
			return map[string]any{"templatePath": ""}
		},
		Generate: generate,
	})
}

func generate(ctx *generators.Context, input any) (any, error) {
	o := ctx.Options

	template, err := legacyhtml.LoadTemplate(o)
	if err != nil {
		return nil, err
	}

	var tocs, contents []string

	for _, t := range input.([]legacyhtml.Template) {
		if t.API == "index" {
			continue
		}

		tocs = append(tocs, t.ToC)
		contents = append(contents, t.Content)
	}

	// The original builds its side navigation from entries without a heading
	// depth, which never make it into the list: it is always empty
	nav := legacy.RenderMarkdown("")

	page := legacyhtml.ReplaceTemplateValues(template, legacyhtml.Template{
		API:     "all",
		Path:    "all",
		Section: "All",
		ToC:     strings.Join(tocs, "\n"),
		Nav:     nav,
		Content: strings.Join(contents, "\n"),
	}, o, true, true)

	if o.Bool("minify") {
		if page, err = legacyhtml.MinifyHTML(page); err != nil {
			return nil, err
		}
	}

	if out := o.String("output"); out != "" {
		if err := fsutil.WriteFile(filepath.Join(out, "all.html"), []byte(page)); err != nil {
			return nil, err
		}
	}

	return page, nil
}
