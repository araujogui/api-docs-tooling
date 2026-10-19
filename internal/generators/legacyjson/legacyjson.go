// Package legacyjson is the generator of the legacy JSON files of the API
// docs, kept for retro-compatibility.
package legacyjson

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nodejs/doc-kit/internal/config"
	"github.com/nodejs/doc-kit/internal/doc"
	"github.com/nodejs/doc-kit/internal/fsutil"
	"github.com/nodejs/doc-kit/internal/generators"
	_ "github.com/nodejs/doc-kit/internal/generators/metadata"
	"github.com/nodejs/doc-kit/internal/jsre"
	"github.com/nodejs/doc-kit/internal/legacy"
	"github.com/nodejs/doc-kit/internal/omap"
	"github.com/nodejs/doc-kit/internal/queries"
	"github.com/nodejs/doc-kit/internal/signature"
	"github.com/nodejs/doc-kit/internal/unist"
)

// SectionTypePlurals are the keys each section type is listed under.
var SectionTypePlurals = map[string]string{
	"module":      "modules",
	"misc":        "miscs",
	"class":       "classes",
	"method":      "methods",
	"property":    "properties",
	"global":      "globals",
	"example":     "examples",
	"ctor":        "signatures",
	"classMethod": "classMethods",
	"event":       "events",
	"var":         "vars",
}

// UnpromotedKeys are the keys not promoted from misc sections.
var UnpromotedKeys = []string{"textRaw", "name", "type", "desc", "miscs"}

func init() {
	generators.Register(&generators.Generator{
		Name:        "legacy-json",
		Description: "Generates the legacy version of the JSON API docs.",
		DependsOn:   "metadata",
		Public:      true,
		Defaults: func(map[string]any) map[string]any {
			return map[string]any{"ref": "main", "minify": false, "sourceURL": "doc/api/{path}"}
		},
		Generate: generate,
	})
}

func generate(ctx *generators.Context, input any) (any, error) {
	entries := input.([]*doc.Entry)
	o := ctx.Options

	_, groups := doc.GroupByModule(entries)

	var heads []*doc.Entry

	for _, e := range entries {
		if e.Heading.Depth == 1 {
			heads = append(heads, e)
		}
	}

	pathByAPI := map[string]string{}

	for _, h := range heads {
		pathByAPI[h.API] = h.Path
	}

	sections := make([]*omap.Map, len(heads))

	err := ctx.Run.Parallel(len(heads), func(i int) error {
		section := BuildSection(heads[i], groups[heads[i].API], o.String("sourceURL"))
		sections[i] = section

		if out := o.String("output"); out != "" {
			indent := 2

			if o.Bool("minify") {
				indent = 0
			}

			data, err := legacy.ToJSON(section, indent)
			if err != nil {
				return err
			}

			return fsutil.WriteFile(filepath.Join(out, fsutil.WithExt(pathByAPI[heads[i].API], "json")), data)
		}

		return nil
	})

	return sections, err
}

// jsTruthy reports whether a value is truthy in JavaScript.
func jsTruthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case float64:
		return x != 0
	case int:
		return x != 0
	}

	return v != omap.Undefined
}

// asSlice returns a value as a slice, when it is one.
func asSlice(v any) ([]any, bool) {
	switch x := v.(type) {
	case []any:
		return x, true
	case []*omap.Map:
		out := make([]any, len(x))

		for i, m := range x {
			out[i] = m
		}

		return out, true
	}

	return nil, false
}

// PromoteMiscChildren promotes a misc section's properties to its parent,
// unless the parent is a misc section too.
func PromoteMiscChildren(section, parent *omap.Map) {
	if section.Value("type") != "misc" || parent.Value("type") == "misc" {
		return
	}

	for _, key := range section.Keys() {
		if contains(UnpromotedKeys, key) {
			continue
		}

		value, _ := section.Get(key)
		existing := parent.Value(key)

		if arr, ok := asSlice(existing); ok {
			// concat spreads arrays, and appends anything else
			if items, isArr := asSlice(value); isArr {
				parent.Set(key, append(append([]any{}, arr...), items...))
			} else {
				parent.Set(key, append(append([]any{}, arr...), value))
			}

			continue
		}

		if !jsTruthy(existing) {
			parent.Set(key, value)
		}
	}
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}

	return false
}

func enforceArray(v any) []any {
	if arr, ok := v.([]any); ok {
		return arr
	}

	return []any{v}
}

// length mirrors `value?.length`: strings and arrays have one.
func length(v any) int {
	switch x := v.(type) {
	case string:
		return len(x)
	case []any:
		return len(x)
	}

	return 0
}

func createMeta(e *doc.Entry) *omap.Map {
	get := func(key string) any {
		if v, ok := e.Props.Get(key); ok && v != omap.Undefined {
			return v
		}

		return []any{}
	}

	added, napi, deprecated, removed, changes := get("added"), get("napiVersion"), get("deprecated"), get("removed"), get("changes")
	meta := omap.New()

	if length(added) > 0 {
		meta.Set("added", enforceArray(added))
	}

	meta.Set("changes", changes)

	if _, isNum := napi.(float64); isNum || length(napi) > 0 {
		meta.Set("napiVersion", enforceArray(napi))
	}

	if length(deprecated) > 0 {
		meta.Set("deprecated", enforceArray(deprecated))
	}

	if length(removed) > 0 {
		meta.Set("removed", enforceArray(removed))
	}

	if length(changes) > 0 || meta.Len() > 1 {
		return meta
	}

	return nil
}

func valueOrUndefined(v any, ok bool) any {
	if !ok {
		return omap.Undefined
	}

	return v
}

func createSection(e *doc.Entry, head *unist.Node) *omap.Map {
	data := head.Data

	if data == nil {
		data = &unist.Data{}
	}

	var typ any = omap.Undefined

	if data.HeadType != "" {
		typ = data.HeadType
	}

	introduced, ok := e.Props.Get("introduced_in")

	section := omap.New(
		"textRaw", unist.NodesToString(head.Children, false),
		"name", data.Name,
		"introduced_in", valueOrUndefined(introduced, ok),
		"type", typ,
	)

	if meta := createMeta(e); meta != nil {
		section.Set("meta", meta)
	}

	return section
}

func parseStability(section *omap.Map, nodes []*unist.Node, e *doc.Entry) []*unist.Node {
	if e.Stability == nil {
		return nodes
	}

	index, err := strconv.ParseFloat(e.Stability.Data.Index, 64)

	if err == nil {
		section.Set("stability", index)
	} else {
		section.Set("stability", nil)
	}

	section.Set("stabilityText", e.Stability.Data.Description)

	if i := e.Content.IndexOf(e.Stability); i != 0 {
		// Like Array.prototype.splice, a negative index counts from the end
		at := i - 1

		if at < 0 {
			at = max(len(nodes)+at, 0)
		}

		if at < len(nodes) {
			nodes = append(nodes[:at:at], nodes[at+1:]...)
		}
	}

	return nodes
}

func toAny(params []*omap.Map) []any {
	out := make([]any, len(params))

	for i, p := range params {
		out[i] = p
	}

	return out
}

func parseList(section *omap.Map, nodes []*unist.Node) []*unist.Node {
	listIdx := -1

	for i, n := range nodes {
		if queries.IsStronglyTypedList(n) {
			listIdx = i
			break
		}
	}

	var values []*omap.Map

	if listIdx >= 0 {
		for _, item := range nodes[listIdx].Children {
			values = append(values, signature.ParseListItem(item))
		}
	}

	removeList := true
	textRaw, _ := section.Value("textRaw").(string)

	switch section.Value("type") {
	case "ctor":
		sig := signature.ParseSignature(textRaw, values)

		for _, k := range sig.Keys() {
			if !section.Has(k) {
				v, _ := sig.Get(k)

				if params, ok := v.([]*omap.Map); ok {
					v = toAny(params)
				}

				section.Set(k, v)
			}
		}
	case "classMethod", "method":
		sig := signature.ParseSignature(textRaw, values)
		sig.Set("params", toAny(sig.Value("params").([]*omap.Map)))
		section.Set("signatures", []any{sig})
	case "property":
		if len(values) > 0 {
			values[0].Delete("name")

			for _, k := range values[0].Keys() {
				v, _ := values[0].Get(k)
				section.Set(k, v)
			}
		}
	case "event":
		if values == nil {
			values = []*omap.Map{}
		}

		section.Set("params", toAny(values))
	default:
		removeList = false
	}

	if removeList && listIdx >= 0 {
		nodes = append(nodes[:listIdx:listIdx], nodes[listIdx+1:]...)
	}

	return nodes
}

func addDescription(section *omap.Map, nodes []*unist.Node) {
	if len(nodes) == 0 {
		return
	}

	rendered := legacy.RenderHTML(unist.Root(nodes...))

	if desc := section.Value("desc"); jsTruthy(desc) {
		section.Set("shortDesc", desc)
	} else {
		section.Set("shortDesc", omap.Undefined)
	}

	if rendered != "" {
		section.Set("desc", rendered)
	} else {
		section.Set("desc", omap.Undefined)
	}
}

var whitespaceRuns = jsre.MustCompile(`\s+`)

func addAdditionalMetadata(section, parent *omap.Map, heading *unist.Node) {
	typ := section.Value("type")

	if !jsTruthy(typ) || typ == "module" {
		textRaw, _ := section.Value("textRaw").(string)
		section.Set("name", whitespaceRuns.ReplaceAllString(jsre.Trim(strings.ToLower(textRaw)), "_"))
	}

	if !jsTruthy(typ) {
		name := ""

		if heading.Data != nil {
			name = heading.Data.Name
		}

		section.Set("displayName", name)

		if parent.Value("type") == "misc" {
			section.Set("type", "misc")
		} else {
			section.Set("type", "module")
		}
	}
}

func addToParent(section, parent *omap.Map) {
	typ, _ := section.Value("type").(string)
	key, ok := SectionTypePlurals[typ]

	if !ok {
		key = "properties"
	}

	existing, isArr := asSlice(parent.Value(key))

	if !isArr {
		existing = []any{}
	}

	parent.Set(key, append(existing, section))
}

func handleEntry(h *doc.Hierarchized, parent *omap.Map) {
	e := h.Entry

	if len(e.Content.Children) == 0 {
		return
	}

	heading := e.Content.Children[0]
	nodes := append([]*unist.Node{}, e.Content.Children[1:]...)
	section := createSection(e, heading)

	nodes = parseStability(section, nodes, e)
	nodes = parseList(section, nodes)
	addDescription(section, nodes)

	for _, child := range h.Children {
		handleEntry(child, section)
	}

	addAdditionalMetadata(section, parent, heading)
	addToParent(section, parent)
	PromoteMiscChildren(section, parent)
}

// BuildSection builds a module's legacy section from its entries.
func BuildSection(head *doc.Entry, entries []*doc.Entry, sourceURL string) *omap.Map {
	root := omap.New(
		"type", "module",
		"api", head.API,
		"source", config.Populate(sourceURL, config.Options{"path": head.API + ".md"}),
	)

	for _, node := range doc.BuildHierarchy(entries) {
		handleEntry(node, root)
	}

	return root
}
