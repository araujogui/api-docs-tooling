package legacyhtml

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/nodejs/doc-kit/internal/config"
	"github.com/nodejs/doc-kit/internal/doc"
	"github.com/nodejs/doc-kit/internal/legacy"
	"github.com/nodejs/doc-kit/internal/markdown"
	"github.com/nodejs/doc-kit/internal/metadata"
	"github.com/nodejs/doc-kit/internal/omap"
	"github.com/nodejs/doc-kit/internal/queries"
	"github.com/nodejs/doc-kit/internal/unist"
)

var h = markdown.H

// parseInt mirrors JavaScript's parseInt for stability indexes ("1.1" is 1).
func parseInt(s string) string {
	end := 0

	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}

	if end == 0 {
		return "NaN"
	}

	n, _ := strconv.Atoi(s[:end])

	return strconv.Itoa(n)
}

var (
	legacyEdges  = regexp.MustCompile(`^[^a-z0-9]+|[^a-z0-9]+$`)
	legacyRuns   = regexp.MustCompile(`[^a-z0-9]+`)
	leadingDigit = regexp.MustCompile(`^\d`)
)

// NewLegacySlugger creates a slugger for the anchors of the old tooling:
// `{api}_{text}`, underscore-separated, with `_{n}` for repeats. Deprecation
// headings (`DEP0001: ...`) keep just their code.
func NewLegacySlugger() func(text, api string) string {
	counters := map[string]int{}

	return func(text, api string) string {
		if m := metadata.DeprecationHeading.FindStringSubmatch(text); m != nil {
			return m[1]
		}

		id := strings.ToLower(api + "_" + text)
		id = legacyEdges.ReplaceAllString(id, "")
		id = legacyRuns.ReplaceAllString(id, "_")
		id = leadingDigit.ReplaceAllStringFunc(id, func(d string) string { return "_" + d })

		count, ok := counters[id]

		if !ok {
			count = -1
		}

		count++
		counters[id] = count

		if count > 0 {
			return fmt.Sprintf("%s_%d", id, count)
		}

		return id
	}
}

// buildHeading replaces a heading with its HTML heading, a level deeper,
// carrying its anchors.
func buildHeading(n *unist.Node, index int, parent *unist.Node, legacySlug string) {
	data := n.Data

	if data == nil {
		data = &unist.Data{}
	}

	children := []any{n.Children}

	// Legacy anchor alias, preserving old external links
	if legacySlug != data.Slug {
		children = append(children, h("span", h("a#"+legacySlug)))
	}

	children = append(children, h("span", h("a.mark#"+data.Slug, omap.New("href", "#"+data.Slug), "#")))

	heading := h(fmt.Sprintf("h%d", n.Depth+1), children...)

	parent.Splice(index, 1)

	if data.Name != "" && data.Slug != "" && len(n.Children) > 0 {
		parent.Children = append([]*unist.Node{heading}, parent.Children...)
	}
}

// buildStability replaces a stability blockquote with its HTML element.
func buildStability(n *unist.Node, index int, parent *unist.Node) {
	el := h("div.api_stability.api_stability_"+parseInt(n.Data.Index), n.Children)
	parent.Splice(index, 1, el)
}

func joinVersions(v any) string {
	if arr, ok := v.([]any); ok {
		parts := make([]string, len(arr))

		for i, item := range arr {
			if item != nil {
				parts[i] = config.JSString(item)
			}
		}

		return strings.Join(parts, ", ")
	}

	if v == nil {
		return ""
	}

	return config.JSString(v)
}

func historyRow(change any) *unist.Node {
	c, _ := change.(*omap.Map)
	description, _ := c.Value("description").(string)

	return h("tr",
		h("td", joinVersions(c.Value("version"))),
		h("td", markdown.Parse(description, markdown.Options{})),
	)
}

// buildMetadataElement builds an entry's metadata: source link, versions,
// N-API version and history.
func buildMetadataElement(e *doc.Entry, o config.Options) *unist.Node {
	el := h("div.api_metadata")

	if link, ok := e.Prop("source_link").(string); ok {
		el.Children = append(el.Children, h("span",
			h("b", "Source Code: "),
			h("a", omap.New("href", config.Populate(config.GitHubBlobURL, o)+link), link),
		))
	}

	for _, field := range []struct{ key, label string }{{"added", "Added in: "}, {"deprecated", "Deprecated in: "}, {"removed", "Removed in: "}} {
		if e.Props.Has(field.key) {
			v := e.Prop(field.key)

			if v == nil {
				el.Children = append(el.Children, h("span", field.label))
			} else {
				el.Children = append(el.Children, h("span", field.label, joinVersions(v)))
			}
		}
	}

	if napi, ok := e.Prop("napiVersion").(float64); ok {
		el.Children = append(el.Children, h("span", h("b", "N-API Version: "), napi))
	}

	if changes, ok := e.Prop("changes").([]any); ok && len(changes) > 0 {
		rows := make([]any, len(changes))

		for i, c := range changes {
			rows[i] = historyRow(c)
		}

		el.Children = append(el.Children, h("details.changelog",
			h("summary", "History"),
			h("table",
				h("thead", h("tr", h("th", "Version"), h("th", "Changes"))),
				h("tbody", rows),
			),
		))
	}

	return el
}

// buildStabilityOverview builds the table of the modules' stability indexes.
func buildStabilityOverview(heads []*doc.Entry) *unist.Node {
	var rows []any

	for _, head := range heads {
		if head.Stability == nil {
			continue
		}

		data := head.Stability.Data
		summary := strings.SplitN(data.Description, ". ", 2)[0]

		rows = append(rows, h("tr",
			h("td.module_stability", h("a", omap.New("href", head.API+".html"), head.HeadingData().Name)),
			h("td.api_stability.api_stability_"+parseInt(data.Index), "("+data.Index+") "+summary),
		))
	}

	return h("table",
		h("thead", h("tr", h("th", "API"), h("th", "Stability"))),
		h("tbody", rows),
	)
}

// buildExtraContent builds the special content an entry's tags ask for.
func buildExtraContent(heads []*doc.Entry, e *doc.Entry) []any {
	var out []any

	for _, tag := range e.Tags() {
		if tag == "STABILITY_OVERVIEW_SLOT_BEGIN" {
			out = append(out, buildStabilityOverview(heads))
		}
	}

	return out
}

// BuildContent renders a module's entries into its page content.
func BuildContent(heads, entries []*doc.Entry, o config.Options) string {
	legacySlug := NewLegacySlugger()
	sections := make([]*unist.Node, len(entries))

	for i, e := range entries {
		unist.Visit(e.Content, queries.IsHeading, func(n *unist.Node, index int, parent *unist.Node) unist.Step {
			text := ""

			if n.Data != nil {
				text = n.Data.Text
			}

			buildHeading(n, index, parent, legacySlug(text, e.API))

			return unist.Next
		})

		unist.Visit(e.Content, queries.IsStabilityNode, func(n *unist.Node, index int, parent *unist.Node) unist.Step {
			buildStability(n, index, parent)

			return unist.SkipStep
		})

		children := []any{}

		if len(e.Content.Children) > 0 {
			children = append(children, e.Content.Children[0])
		}

		children = append(children, buildMetadataElement(e, o), buildExtraContent(heads, e))

		if len(e.Content.Children) > 1 {
			children = append(children, e.Content.Children[1:])
		}

		sections[i] = h("section", children...)
	}

	hast := markdown.ToHast(unist.Root(sections...), legacy.HastOptions)
	highlightCode(hast)

	return markdown.ToHTML(hast, markdown.HTMLOptions{AllowDangerousHTML: true})
}
