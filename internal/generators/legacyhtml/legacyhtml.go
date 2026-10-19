// Package legacyhtml is the generator of the legacy HTML pages of the API
// docs, kept for retro-compatibility.
package legacyhtml

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nodejs/doc-kit/internal/collate"
	"github.com/nodejs/doc-kit/internal/config"
	"github.com/nodejs/doc-kit/internal/doc"
	"github.com/nodejs/doc-kit/internal/fsutil"
	"github.com/nodejs/doc-kit/internal/generators"
	_ "github.com/nodejs/doc-kit/internal/generators/metadata"
	"github.com/nodejs/doc-kit/internal/jsre"
	"github.com/nodejs/doc-kit/internal/legacy"
	"github.com/nodejs/doc-kit/internal/parsers"
	"github.com/nodejs/doc-kit/internal/semver"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/html"
)

var (
	//go:embed template.html
	defaultTemplate string

	//go:embed assets
	assets embed.FS
)

// Template holds a page's values for the template.
type Template struct {
	API     string
	Path    string
	Added   string
	Section string
	ToC     string
	Nav     string
	Content string
}

func init() {
	generators.Register(&generators.Generator{
		Name:        "legacy-html",
		Description: "Generates the legacy version of the API docs in HTML, with the assets and styles included as files",
		DependsOn:   "metadata",
		Public:      true,
		Defaults: func(map[string]any) map[string]any {
			return map[string]any{
				// Empty paths stand for the embedded template and assets
				"templatePath":          "",
				"additionalPathsToCopy": []any{},
				"ref":                   "main",
				"pageURL":               "{baseURL}/latest-{version}/api{path}.html",
				"editURL":               config.GitHubEditURL + "/doc/api{path}.md",
			}
		},
		Generate: generate,
	})
}

// LoadTemplate reads the page template.
func LoadTemplate(o config.Options) (string, error) {
	if path := o.String("templatePath"); path != "" {
		data, err := os.ReadFile(path)

		return string(data), err
	}

	return defaultTemplate, nil
}

// TableOfContents writes a Markdown list of entries, indented by depth.
func TableOfContents(entries []tocEntry, maxDepth int) string {
	var b strings.Builder

	for _, e := range entries {
		if e.name == "" || e.depth > maxDepth || e.depth < 1 {
			continue
		}

		b.WriteString(strings.Repeat("  ", e.depth-1) + "- " + e.label + "\n")
	}

	return b.String()
}

type tocEntry struct {
	name  string
	depth int
	label string
}

// navigationEntry links a module's page.
func navigationEntry(api, name string) tocEntry {
	return tocEntry{name: name, depth: 1, label: `<a class="nav-` + api + `" href="` + api + `.html">` + name + `</a>`}
}

// tocEntryOf links an entry's section, marked with its stability.
func tocEntryOf(e *doc.Entry) tocEntry {
	data := e.HeadingData()
	link := `<a href="` + e.API + ".html#" + data.Slug + `">` + data.Text + `</a>`

	if e.Stability != nil {
		link = `<span class="stability_` + parseInt(e.Stability.Data.Index) + `">` + link + `</span>`
	}

	return tocEntry{name: data.Name, depth: e.Heading.Depth, label: link}
}

// WrapToC wraps a table of contents for the page.
func WrapToC(toc string) string {
	if toc == "" {
		return ""
	}

	return `<details role="navigation" id="toc" open><summary>Table of contents</summary>` + toc + `</details>`
}

// buildToC builds the table of contents dropdown.
func buildToC(toc string) string {
	if toc == "" {
		return ""
	}

	return `<li class="picker-header"><a href="#toc-picker" aria-controls="toc-picker">` +
		`<span class="picker-arrow"></span>` +
		`Table of contents</a><div class="picker">` +
		`<div class="toc" tabindex="-1">` + jsre.Replace(toc, "<ul", `<ul id="toc-picker"`) + `</div></div></li>`
}

// buildNavigation builds the navigation dropdown.
func buildNavigation(nav string) string {
	return `<li class="picker-header"><a href="#gtoc-picker" aria-controls="gtoc-picker">` +
		`<span class="picker-arrow"></span>Index</a>` +
		`<div class="picker" tabindex="-1" id="gtoc-picker"><ul><li><a href="index.html">Index</a>` +
		`</li></ul><hr class="line" />` + nav + `</div></li>`
}

// VersionLabel is a version as the docs name it: `22.x`, or `0.12.x`.
func VersionLabel(v semver.Version) string {
	if v.Minor == 0 {
		return fmt.Sprintf("%d.x", v.Major)
	}

	return fmt.Sprintf("%d.%d.x", v.Major, v.Minor)
}

// CompatibleVersions are the releases since an API was introduced (all of
// them when that is unknown).
func CompatibleVersions(introduced string, releases []parsers.Release) []parsers.Release {
	major := 0

	if v := semver.Coerce(introduced); v != nil {
		major = v.Major
	}

	var out []parsers.Release

	for _, r := range releases {
		if r.Version.Major >= major {
			out = append(out, r)
		}
	}

	return out
}

// buildVersions builds the dropdown of the page in other versions.
func buildVersions(path, added string, o config.Options) string {
	var b strings.Builder

	for _, r := range CompatibleVersions(added, o.Changelog()) {
		label := VersionLabel(r.Version)
		href := config.Populate(o.String("pageURL"), o.With("path", path, "version", "v"+label))
		lts := ""

		if r.IsLts {
			lts = "<b>LTS</b>"
		}

		b.WriteString(`<li><a href="` + href + `">` + label + " " + lts + `</a></li>`)
	}

	return `<li class="picker-header"><a href="#alt-docs" aria-controls="alt-docs">` +
		`<span class="picker-arrow"></span>Other versions</a>` +
		`<div class="picker" tabindex="-1"><ol id="alt-docs">` + b.String() + `</ol></div></li>`
}

// buildGitHub builds the "Edit on GitHub" link.
func buildGitHub(url string) string {
	return `<li class="edit_on_github"><a href="` + url + `">Edit on GitHub</a></li>`
}

// replaceFirst replaces the first occurrence of old. Unlike the original's
// String.prototype.replace, `$&` and the like in the values stay as they are.
func replaceFirst(s, old, value string) string {
	return strings.Replace(s, old, value, 1)
}

// ReplaceTemplateValues fills the page template.
func ReplaceTemplateValues(template string, t Template, o config.Options, skipGitHub, skipGtocPicker bool) string {
	version := "v" + o.Version().String()

	s := replaceFirst(template, "__ID__", t.API)
	s = strings.ReplaceAll(s, "__FILENAME__", t.API)
	s = replaceFirst(s, "__SECTION__", t.Section)
	s = strings.ReplaceAll(s, "__VERSION__", version)
	s = strings.ReplaceAll(s, "__TOC__", WrapToC(t.ToC))
	s = strings.ReplaceAll(s, "__GTOC__", t.Nav)
	s = replaceFirst(s, "__CONTENT__", t.Content)
	s = strings.ReplaceAll(s, "__TOC_PICKER__", buildToC(t.ToC))

	if skipGtocPicker {
		s = strings.ReplaceAll(s, "__GTOC_PICKER__", "")
	} else {
		s = strings.ReplaceAll(s, "__GTOC_PICKER__", buildNavigation(t.Nav))
	}

	s = replaceFirst(s, "__ALTDOCS__", buildVersions(t.Path, t.Added, o))

	if skipGitHub {
		return replaceFirst(s, "__EDIT_ON_GITHUB__", "")
	}

	return replaceFirst(s, "__EDIT_ON_GITHUB__", buildGitHub(config.Populate(o.String("editURL"), o.With("path", t.Path))))
}

// MinifyHTML minifies a page.
func MinifyHTML(page string) (string, error) {
	m := minify.New()
	m.Add("text/html", &html.Minifier{KeepDocumentTags: true, KeepEndTags: true, KeepQuotes: true})

	return m.String("text/html", page)
}

// copyAssets copies the pages' assets into the output.
func copyAssets(o config.Options, out string) error {
	paths := o.Strings("additionalPathsToCopy")

	if len(paths) == 0 {
		sub, err := fs.Sub(assets, "assets")
		if err != nil {
			return err
		}

		return fs.WalkDir(sub, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}

			data, err := fs.ReadFile(sub, path)
			if err != nil {
				return err
			}

			return fsutil.WriteFile(filepath.Join(out, "assets", path), data)
		})
	}

	for _, path := range paths {
		if err := fsutil.CopyDir(path, filepath.Join(out, filepath.Base(path))); err != nil {
			return err
		}
	}

	return nil
}

func capitalize(s string) string {
	if s == "" {
		return s
	}

	return strings.ToUpper(s[:1]) + s[1:]
}

func generate(ctx *generators.Context, input any) (any, error) {
	entries := doc.CloneEntries(input.([]*doc.Entry))
	o := ctx.Options

	template, err := LoadTemplate(o)
	if err != nil {
		return nil, err
	}

	_, groups := doc.GroupByModule(entries)

	var heads []*doc.Entry

	for _, e := range entries {
		if e.Heading.Depth == 1 {
			heads = append(heads, e)
		}
	}

	sort.SliceStable(heads, func(i, j int) bool {
		return collate.Compare(heads[i].HeadingData().Name, heads[j].HeadingData().Name) < 0
	})

	var navEntries []tocEntry

	if o.HasIndex() {
		for _, entry := range o.Index() {
			navEntries = append(navEntries, navigationEntry(entry.API, entry.Section))
		}
	} else {
		for _, head := range heads {
			navEntries = append(navEntries, navigationEntry(head.API, head.HeadingData().Name))
		}
	}

	navigation := legacy.RenderMarkdown(TableOfContents(navEntries, 1))

	out := o.String("output")

	if out != "" {
		if err := copyAssets(o, out); err != nil {
			return nil, err
		}
	}

	templates := make([]Template, len(heads))

	err = ctx.Run.Parallel(len(heads), func(i int) error {
		head := heads[i]

		// Rendering edits the entries, so each page works on its own copy
		nodes := doc.CloneEntries(groups[head.API])

		var tocEntries []tocEntry

		for _, n := range nodes {
			tocEntries = append(tocEntries, tocEntryOf(n))
		}

		section := head.HeadingData().Name

		if section == "" {
			section = capitalize(head.API)
		}

		added := ""

		if v := head.Prop("introduced_in"); v != nil {
			added = config.JSString(v)
		}

		t := Template{
			API:     head.API,
			Path:    head.Path,
			Added:   added,
			Section: section,
			ToC:     legacy.RenderMarkdown(TableOfContents(tocEntries, 5)),
			Nav:     jsre.Replace(navigation, `class="nav-`+head.API+`"`, `class="nav-`+head.API+` active"`),
			Content: BuildContent(heads, nodes, o),
		}

		templates[i] = t

		if out == "" {
			return nil
		}

		page := ReplaceTemplateValues(template, t, o, false, false)

		if o.Bool("minify") {
			if page, err = MinifyHTML(page); err != nil {
				return err
			}
		}

		return fsutil.WriteFile(filepath.Join(out, t.API+".html"), []byte(page))
	})

	return templates, err
}
