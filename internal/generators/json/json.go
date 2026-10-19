// Package json is the generator turning each API doc into a JSON document
// described by the doc-kit JSON schema.
package json

import (
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/nodejs/doc-kit/internal/config"
	"github.com/nodejs/doc-kit/internal/doc"
	"github.com/nodejs/doc-kit/internal/fsutil"
	"github.com/nodejs/doc-kit/internal/generators"
	_ "github.com/nodejs/doc-kit/internal/generators/metadata"
	"github.com/nodejs/doc-kit/internal/jsre"
	"github.com/nodejs/doc-kit/internal/markdown"
	"github.com/nodejs/doc-kit/internal/metadata"
	"github.com/nodejs/doc-kit/internal/omap"
	"github.com/nodejs/doc-kit/internal/queries"
	"github.com/nodejs/doc-kit/internal/signature"
	"github.com/nodejs/doc-kit/internal/unist"
)

// SchemaVersion is the version of schema.json, which it must stay in sync
// with.
const SchemaVersion = "1.0.1"

// SchemaURL is where a schema version is published.
const SchemaURL = "https://doc-kit.nodejs.org/schemas/api-doc/{schemaVersion}.json"

// kinds maps the heading classifications to node kinds.
var kinds = map[string]string{
	"class":       "class",
	"ctor":        "constructor",
	"method":      "method",
	"classMethod": "staticMethod",
	"property":    "property",
	"event":       "event",
}

const sectionKind = "section"

var kindsWithTypedList = map[string]bool{"class": true, "constructor": true, "method": true, "staticMethod": true, "property": true, "event": true}

var callableKinds = map[string]bool{"constructor": true, "method": true, "staticMethod": true}

var documentTypes = map[string]bool{"module": true, "misc": true, "global": true}

var (
	displayName     = regexp.MustCompile(`displayName="([^"]*)"`)
	restMarker      = regexp.MustCompile(`^\.\.\.`)
	codeSpan        = regexp.MustCompile("(?s)^`(.*)`$")
	declaredDefault = jsre.MustCompile(`^=\s*`)
	extendsClause   = regexp.MustCompile(` extends +`)
)

func init() {
	generators.Register(&generators.Generator{
		Name:        "json",
		Description: "Generates one JSON document per API doc, described by the doc-kit JSON schema",
		DependsOn:   "metadata",
		Public:      true,
		Defaults: func(map[string]any) map[string]any {
			return map[string]any{"schemaURL": SchemaURL}
		},
		Generate: generate,
	})
}

// ResolveSchemaURL fills a generator's `schemaURL` with the schema version.
func ResolveSchemaURL(o config.Options) string {
	return config.Populate(o.String("schemaURL"), o.With("schemaVersion", SchemaVersion))
}

type dependencies struct {
	schemaURL string
	sourceURL *string
}

func generate(ctx *generators.Context, input any) (any, error) {
	entries := doc.CloneEntries(input.([]*doc.Entry))
	o := ctx.Options

	deps := dependencies{schemaURL: ResolveSchemaURL(o)}

	// Source links resolve against the repository, when one is configured
	if o.String("repository") != "" {
		url := config.Populate(config.GitHubBlobURL, o)
		deps.sourceURL = &url
	}

	// Pages other generators added to the pipeline are theirs to render
	var own []*doc.Entry

	for _, e := range entries {
		if !e.Synthetic {
			own = append(own, e)
		}
	}

	order, groups := doc.GroupByModule(own)
	documents := make([]*omap.Map, len(order))

	err := ctx.Run.Parallel(len(order), func(i int) error {
		nodes := groups[order[i]]
		head := nodes[0]

		for _, n := range nodes {
			if n.Heading.Depth == 1 {
				head = n
				break
			}
		}

		document := BuildDocument(head, nodes, deps)
		documents[i] = document

		if out := o.String("output"); out != "" {
			path, _ := document.Value("path").(string)

			return fsutil.WriteJSON(filepath.Join(out, fsutil.WithExt(path, "json")), document, o.Bool("minify"))
		}

		return nil
	})

	return documents, err
}

// BuildDocument builds a module's document from its entries.
func BuildDocument(head *doc.Entry, entries []*doc.Entry, deps dependencies) *omap.Map {
	doc.AnnotateOverloads(entries)

	typ := "module"

	if t, ok := head.Prop("type").(string); ok && documentTypes[t] {
		typ = t
	}

	scope := "module"

	if typ == "global" {
		scope = "global"
	}

	// The title entry is the root; any other root is treated as its child
	roots := doc.BuildHierarchy(entries)

	var root *doc.Hierarchized

	for _, r := range roots {
		if r.Entry == head {
			root = r
			break
		}
	}

	if root == nil && len(roots) > 0 {
		root = roots[0]
	}

	children := []any{}

	if root != nil {
		for _, c := range root.Children {
			children = append(children, buildNode(c, scope))
		}
	}

	for _, r := range roots {
		if r != root {
			children = append(children, buildNode(r, scope))
		}
	}

	entry := buildEntry(head, entryBody(head))

	var module any

	if name, ok := head.Prop("name").(string); ok {
		module = name
	} else if typ == "module" {
		module = head.API
	}

	var introducedIn any

	if v := head.Prop("introduced_in"); v != nil {
		introducedIn = config.JSString(v)
	}

	var sourceLink any

	if v := head.Prop("source_link"); v != nil && truthy(v) {
		link := config.JSString(v)
		var url any

		if deps.sourceURL != nil {
			url = *deps.sourceURL + link
		}

		sourceLink = omap.New("path", link, "url", url)
	}

	document := omap.New(
		"$schema", deps.schemaURL,
		"id", head.API,
		"path", head.Path,
		"type", typ,
		"module", module,
		"title", entry.Value("title"),
		"introducedIn", introducedIn,
		"sourceLink", sourceLink,
	)

	for _, k := range entry.Keys() {
		if k != "title" {
			v, _ := entry.Get(k)
			document.Set(k, v)
		}
	}

	document.Set("children", children)

	return document
}

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return x != ""
	case bool:
		return x
	case float64:
		return x != 0 && !math.IsNaN(x)
	}

	return true
}

func isStabilityIndex(n *unist.Node) bool {
	return n.Type == "blockquote" && n.Data != nil && n.Data.IsStability
}

// entryBody is an entry's content without its heading and, when it has one,
// its stability index.
func entryBody(e *doc.Entry) []*unist.Node {
	body := []*unist.Node{}

	for _, n := range e.Content.Children {
		if queries.IsHeading(n) || (e.Stability != nil && isStabilityIndex(n)) {
			continue
		}

		body = append(body, n)
	}

	return body
}

// takeTypedItems lifts the leading typed items of the body's first typed
// list out of the body, for the kinds that turn them into data.
func takeTypedItems(body []*unist.Node, kind string) ([]*unist.Node, []*unist.Node) {
	index := -1

	if kindsWithTypedList[kind] {
		for i, n := range body {
			if queries.IsStronglyTypedList(n) {
				index = i
				break
			}
		}
	}

	if index == -1 {
		return body, []*unist.Node{}
	}

	list := body[index]
	items, rest := queries.SplitTypedItems(list)

	if kind == "class" {
		hasExtends := false

		for _, item := range items {
			if signature.ExtractListItem(item).Prefix == "Extends" {
				hasExtends = true
			}
		}

		if !hasExtends {
			return body, []*unist.Node{}
		}
	}

	out := append([]*unist.Node{}, body[:index]...)

	if len(rest) > 0 {
		c := list.ShallowCopy()
		c.Children = rest
		out = append(out, c)
	}

	return append(out, body[index+1:]...), items
}

// buildEntry builds an entry's metadata and description.
func buildEntry(e *doc.Entry, body []*unist.Node) *omap.Map {
	return omap.New(
		"title", e.HeadingData().Text,
		"stability", toStability(e.Stability),
		"added", toVersions(e.Prop("added")),
		"deprecated", toVersions(e.Prop("deprecated")),
		"removed", toVersions(e.Prop("removed")),
		"napiVersion", toNumbers(e.Prop("napiVersion")),
		"changes", toChanges(e.Prop("changes")),
		"description", BlocksToMarkdown(body),
		"summary", doc.EntryDescription(e),
		"examples", extractExamples(body),
	)
}

func enforceArray(v any) []any {
	if arr, ok := v.([]any); ok {
		return arr
	}

	return []any{v}
}

func toVersions(v any) []any {
	out := []any{}

	if v == nil {
		return out
	}

	for _, item := range enforceArray(v) {
		out = append(out, config.JSString(item))
	}

	return out
}

func toNumber(v any) any {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case bool:
		if x {
			return float64(1)
		}

		return float64(0)
	case nil:
		return float64(0)
	case string:
		s := jsre.Trim(x)

		if s == "" {
			return float64(0)
		}

		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	}

	// NaN serializes as null
	return nil
}

func toNumbers(v any) []any {
	out := []any{}

	if v == nil {
		return out
	}

	for _, item := range enforceArray(v) {
		out = append(out, toNumber(item))
	}

	return out
}

func toChanges(v any) []any {
	out := []any{}

	if v == nil {
		return out
	}

	for _, item := range enforceArray(v) {
		change, _ := item.(*omap.Map)

		nullable := func(key string) any {
			if x := change.Value(key); x != nil {
				return config.JSString(x)
			}

			return nil
		}

		description := ""

		if d := change.Value("description"); d != nil {
			description = config.JSString(d)
		}

		out = append(out, omap.New(
			"versions", toVersions(change.Value("version")),
			"prUrl", nullable("pr-url"),
			"commit", nullable("commit"),
			"description", jsre.Trim(description),
		))
	}

	return out
}

func toStability(n *unist.Node) any {
	if n == nil {
		return nil
	}

	description := n.Data.Description

	if body := doc.RemoveStabilityPrefix(n); body != nil && len(body.Children) > 0 {
		description = BlocksToMarkdown(body.Children)
	}

	return omap.New("index", n.Data.Index, "description", description)
}

// BlocksToMarkdown serializes block nodes back to Markdown.
func BlocksToMarkdown(nodes []*unist.Node) string {
	if len(nodes) == 0 {
		return ""
	}

	return jsre.Trim(markdown.ToMarkdown(unist.Root(nodes...)))
}

func extractExamples(nodes []*unist.Node) []any {
	examples := []any{}

	unist.Visit(unist.Root(nodes...), unist.IsType("code"), func(n *unist.Node, _ int, _ *unist.Node) unist.Step {
		var language, name any

		if n.Lang != nil {
			language = *n.Lang
		}

		meta := ""

		if n.Meta != nil {
			meta = *n.Meta
		}

		if m := displayName.FindStringSubmatch(meta); m != nil {
			name = m[1]
		}

		examples = append(examples, omap.New("language", language, "displayName", name, "code", n.Value))

		return unist.Next
	})

	return examples
}

// classify returns an entry's kind and, when a `global` override replaced
// its classification, the scope that sets.
func classify(e *doc.Entry) (string, string) {
	t := e.HeadingData().HeadType

	if t == "global" {
		structural := metadata.TransformNodeToHeading(e.Heading).HeadType
		kind, ok := kinds[structural]

		if !ok {
			kind = sectionKind
		}

		return kind, "global"
	}

	if kind, ok := kinds[t]; ok {
		return kind, ""
	}

	return sectionKind, ""
}

// nodeName is the bare identifier an entry documents.
func nodeName(e *doc.Entry, kind string) string {
	if name, ok := e.Prop("name").(string); ok {
		return name
	}

	name := e.HeadingData().Name

	switch kind {
	case sectionKind:
		return jsre.Trim(unist.ToStringAll(e.Heading.Children))
	case "class", "constructor":
		base := extendsClause.Split(name, -1)[0]
		parts := strings.Split(base, ".")

		return parts[len(parts)-1]
	}

	return name
}

func buildNode(h *doc.Hierarchized, scope string) *omap.Map {
	e := h.Entry
	kind, entryScope := classify(e)

	if entryScope == "" {
		entryScope = scope
	}

	body, items := takeTypedItems(entryBody(e), kind)
	properties, typeDescription := kindProperties(kind, e, items)
	entry := buildEntry(e, body)

	var overloadOf any

	if d := e.HeadingData(); d.OverloadOf != "" {
		overloadOf = d.OverloadOf
	}

	node := omap.New(
		"kind", kind,
		"id", e.HeadingData().Slug,
		"name", nodeName(e, kind),
		"title", entry.Value("title"),
		"scope", entryScope,
		"overloadOf", overloadOf,
	)

	for _, k := range []string{"stability", "added", "deprecated", "removed", "napiVersion", "changes"} {
		v, _ := entry.Get(k)
		node.Set(k, v)
	}

	for _, k := range properties.Keys() {
		v, _ := properties.Get(k)
		node.Set(k, v)
	}

	// A property's type item may describe it; that comes first
	var parts []string

	for _, d := range []string{typeDescription, entry.Value("description").(string)} {
		if d != "" {
			parts = append(parts, d)
		}
	}

	node.Set("description", strings.Join(parts, "\n\n"))
	node.Set("summary", entry.Value("summary"))
	node.Set("examples", entry.Value("examples"))

	children := []any{}

	for _, c := range h.Children {
		children = append(children, buildNode(c, scope))
	}

	node.Set("children", children)

	return node
}

func kindProperties(kind string, e *doc.Entry, items []*unist.Node) (*omap.Map, string) {
	if callableKinds[kind] {
		return omap.New("signature", buildSignature(e.Heading, items)), ""
	}

	switch kind {
	case "class":
		return omap.New("extends", buildExtends(e.Heading, items)), ""
	case "property":
		typ, def, description := buildPropertyType(items)

		return omap.New("type", typ, "default", def), description
	case "event":
		return omap.New("parameters", buildEventParameters(items)), ""
	}

	return omap.New(), ""
}

// itemDescription is the Markdown of a list item's description.
func itemDescription(item *signature.ListItem) string {
	description := signature.RemoveDefault(item.Text)

	var nodes []*unist.Node

	if len(description) > 0 {
		nodes = append(nodes, unist.Parent("paragraph", description...))
	}

	return BlocksToMarkdown(append(nodes, item.Blocks...))
}

// toDefault is a default value as authored, without its code span.
func toDefault(value *string) any {
	if value == nil {
		return nil
	}

	v := declaredDefault.ReplaceAllString(*value, "")

	return codeSpan.ReplaceAllString(v, "$1")
}

func toType(n *unist.Node) any {
	if n == nil {
		return nil
	}

	links := []any{}

	if n.Data != nil {
		for _, l := range n.Data.Links {
			links = append(links, omap.New("name", l.Text, "href", l.Href, "start", l.Start, "end", l.End))
		}
	}

	return omap.New("text", n.Value, "links", links)
}

func plainType(text string) any {
	return omap.New("text", text, "links", []any{})
}

type declaration struct {
	name     *string
	def      *string
	optional bool
}

func toParameter(item *signature.ListItem, declared declaration) *omap.Map {
	name := ""

	if declared.name != nil {
		name = *declared.name
	} else if item.HasName {
		name = item.Name
	}

	value := item.Default

	if value == nil {
		value = declared.def
	}

	properties := []any{}

	for _, nested := range item.Items {
		properties = append(properties, toParameter(signature.ExtractListItem(nested), declaration{}))
	}

	return omap.New(
		"name", restMarker.ReplaceAllString(name, ""),
		"type", toType(item.Annotation),
		"description", itemDescription(item),
		"default", toDefault(value),
		"optional", declared.optional || value != nil,
		"rest", restMarker.MatchString(name),
		"properties", properties,
	)
}

// toMarkdownParameter wraps an item the way the signature parser matches
// parameters (by name, with the nested items as options).
func toMarkdownParameter(item *signature.ListItem) *omap.Map {
	var name any

	if item.Prefix != "" {
		if item.Prefix == "Returns" {
			name = "return"
		} else {
			name = strings.ToLower(item.Prefix)
		}
	} else if item.HasName {
		name = item.Name
	}

	options := []*omap.Map{}

	for _, nested := range item.Items {
		options = append(options, toMarkdownParameter(signature.ExtractListItem(nested)))
	}

	p := omap.New()

	if name != nil {
		p.Set("name", name)
	}

	p.Set("options", options)
	p.Set("item", item)

	return p
}

var emptyItem = &signature.ListItem{Text: []*unist.Node{}, Blocks: []*unist.Node{}, Items: []*unist.Node{}}

func buildSignature(heading *unist.Node, items []*unist.Node) *omap.Map {
	params := make([]*omap.Map, len(items))

	for i, item := range items {
		params[i] = toMarkdownParameter(signature.ExtractListItem(item))
	}

	sig := signature.ParseSignature(heading.Data.Text, params)

	parameters := []any{}

	for _, declared := range sig.Value("params").([]*omap.Map) {
		item, ok := declared.Value("item").(*signature.ListItem)

		if !ok {
			item = emptyItem
		}

		d := declaration{optional: declared.Value("optional") == true}

		if name, ok := declared.Value("name").(string); ok {
			d.name = &name
		}

		if def, ok := declared.Value("default").(string); ok {
			d.def = &def
		}

		parameters = append(parameters, toParameter(item, d))
	}

	var returns any

	if ret, ok := sig.Value("return").(*omap.Map); ok {
		item := ret.Value("item").(*signature.ListItem)
		returns = omap.New("type", toType(item.Annotation), "description", itemDescription(item))
	}

	return omap.New("parameters", parameters, "returns", returns)
}

func buildExtends(heading *unist.Node, items []*unist.Node) any {
	for _, item := range items {
		if extracted := signature.ExtractListItem(item); extracted.Prefix == "Extends" {
			return toType(extracted.Annotation)
		}
	}

	parts := extendsClause.Split(heading.Data.Name, -1)

	if len(parts) > 1 && parts[1] != "" {
		return plainType(parts[1])
	}

	return nil
}

func buildPropertyType(items []*unist.Node) (any, any, string) {
	if len(items) == 0 {
		return nil, nil, ""
	}

	item := signature.ExtractListItem(items[0])

	return toType(item.Annotation), toDefault(item.Default), itemDescription(item)
}

func buildEventParameters(items []*unist.Node) []any {
	out := []any{}

	for _, item := range items {
		out = append(out, toParameter(signature.ExtractListItem(item), declaration{}))
	}

	return out
}
