package signature

import (
	"github.com/nodejs/doc-kit/internal/jsre"
	"regexp"
	"strings"

	"github.com/nodejs/doc-kit/internal/omap"
	"github.com/nodejs/doc-kit/internal/queries"
	"github.com/nodejs/doc-kit/internal/unist"
)

// ListItem is a typed list item taken apart, nothing stringified, so each
// consumer can render the description its own way.
type ListItem struct {
	// Name is the parameter name (the leading code span), if any.
	Name string
	// HasName is set when the item has a name.
	HasName bool
	// Prefix is the special starter (`Returns`, `Extends`, `Type`), if any.
	Prefix string
	// Annotation is the leading `typeAnnotation` node, if any.
	Annotation *unist.Node
	// Text is the rest of the first paragraph: the description, with any
	// `**Default:**` marker still in place.
	Text []*unist.Node
	// Blocks are the item's further blocks, without its nested typed list.
	Blocks []*unist.Node
	// Default is the `**Default:**` value as written, if any.
	Default *string
	// Items are the items of the nested typed list, if any.
	Items []*unist.Node
}

var (
	defaultMarker = regexp.MustCompile(`(?i)^default:$`)
	trailingDot   = regexp.MustCompile(`\.$`)
)

// replaceLeadingText replaces the first node's text, or drops it when
// nothing is left.
func replaceLeadingText(nodes []*unist.Node, value string) []*unist.Node {
	if jsre.Trim(value) != "" {
		c := nodes[0].ShallowCopy()
		c.Value = value
		nodes[0] = c

		return nodes
	}

	return nodes[1:]
}

// ExtractListItem takes a typed list item apart:
// “`name` {Type} Description. **Default:** `value`”, or one of the
// `Returns:`, `Extends:` and `Type:` forms.
func ExtractListItem(item *unist.Node) *ListItem {
	result := &ListItem{Text: []*unist.Node{}, Blocks: []*unist.Node{}, Items: []*unist.Node{}}

	var rest []*unist.Node

	if len(item.Children) > 0 {
		paragraph := item.Children[0]
		result.Text = append(result.Text, paragraph.Children...)
		rest = item.Children[1:]
	}

	text := result.Text

	if len(text) > 0 {
		first := text[0]

		if first.Type == "inlineCode" {
			result.Name = strings.TrimRight(first.Value, " \t\n\r\f\v")
			result.HasName = true
			text = text[1:]
		} else if first.Type == "text" {
			if m := queries.TypedListStarters.FindStringSubmatch(first.Value); m != nil {
				result.Prefix = m[1]
				text = replaceLeadingText(text, first.Value[len(m[0]):])
			}
		}
	}

	// The whitespace between the name and the type
	if len(text) > 0 && text[0].Type == "text" && jsre.Trim(text[0].Value) == "" {
		text = text[1:]
	}

	if len(text) > 0 && text[0].Type == "typeAnnotation" {
		result.Annotation = text[0]
		text = text[1:]
	}

	// Leading separators: `- description`, `: description`
	if len(text) > 0 && text[0].Type == "text" {
		text = replaceLeadingText(text, TrimmablePadding.ReplaceAllString(text[0].Value, ""))
	}

	result.Text = text

	if m := DefaultExpression.FindStringSubmatch(unist.NodesToString(text, false)); m != nil {
		d := trailingDot.ReplaceAllString(jsre.Trim(m[1]), "")
		result.Default = &d
	}

	for _, n := range rest {
		if len(result.Items) == 0 && queries.IsLooselyTypedList(n) {
			result.Items = n.Children
		} else {
			result.Blocks = append(result.Blocks, n)
		}
	}

	return result
}

// RemoveDefault drops the `**Default:**` marker, and everything after it,
// from an item's description.
func RemoveDefault(nodes []*unist.Node) []*unist.Node {
	index := -1

	for i, n := range nodes {
		if n.Type == "strong" && defaultMarker.MatchString(jsre.Trim(unist.NodesToString(n.Children, false))) {
			index = i
			break
		}
	}

	if index == -1 {
		return nodes
	}

	kept := append([]*unist.Node{}, nodes[:index]...)

	// The whitespace separating the description from its default
	if len(kept) > 0 && kept[len(kept)-1].Type == "text" {
		last := kept[len(kept)-1]
		value := strings.TrimRight(last.Value, " \t\n\r\f\v")
		kept = kept[:len(kept)-1]

		if value != "" {
			c := last.ShallowCopy()
			c.Value = value
			kept = append(kept, c)
		}
	}

	return kept
}

var (
	whitespaceRuns = jsre.MustCompile(`\s+`)
	htmlComments   = regexp.MustCompile(`(?s)<!--.*?-->`)
	leadingSpaces  = jsre.MustCompile(`^\s+`)
)

// extractPattern moves a pattern's (trimmed, undotted) first group out of
// the text and into a key of the parameter.
func extractPattern(text string, pattern *regexp.Regexp, key string, current *omap.Map) string {
	m := pattern.FindStringSubmatchIndex(text)

	if m == nil || m[2] < 0 {
		return text
	}

	value := trailingDot.ReplaceAllString(jsre.Trim(text[m[2]:m[3]]), "")

	if value == "" {
		return text
	}

	current.Set(key, value)

	return text[:m[0]] + text[m[1]:]
}

// ParseListItem parses a typed list item into a legacy parameter: its raw
// text, name, type, default, description and options.
func ParseListItem(item *unist.Node) *omap.Map {
	current := omap.New()

	var subList *unist.Node

	for _, n := range item.Children {
		if queries.IsLooselyTypedList(n) {
			subList = n
			break
		}
	}

	var others []*unist.Node

	for _, n := range item.Children {
		if n != subList {
			others = append(others, n)
		}
	}

	textRaw := htmlComments.ReplaceAllString(whitespaceRuns.ReplaceAllString(unist.NodesToString(others, false), " "), "")
	current.Set("textRaw", textRaw)

	text := textRaw

	if m := queries.TypedListStarters.FindStringSubmatch(text); m != nil {
		if m[1] == "Returns" {
			current.Set("name", "return")
		} else {
			current.Set("name", strings.ToLower(m[1]))
		}

		text = text[len(m[0]):]
	} else {
		text = extractPattern(text, NameExpression, "name", current)
	}

	// The type is the item's typeAnnotation node, when its serialized form
	// heads the remaining text
	var annotation *unist.Node

	for _, n := range item.Children {
		if n.Type == "paragraph" {
			for _, c := range n.Children {
				if c.Type == "typeAnnotation" {
					annotation = c
					break
				}
			}

			break
		}
	}

	if annotation != nil && strings.HasPrefix(text, "{"+annotation.Value+"}") {
		current.Set("type", annotation.Value)
		text = leadingSpaces.ReplaceAllString(text[len(annotation.Value)+2:], "")
	}

	text = extractPattern(text, DefaultExpression, "default", current)

	if desc := jsre.Trim(LeadingHyphen.ReplaceAllString(text, "")); desc != "" {
		current.Set("desc", desc)
	} else {
		current.Set("desc", omap.Undefined)
	}

	if subList != nil {
		options := make([]*omap.Map, len(subList.Children))

		for i, child := range subList.Children {
			options[i] = ParseListItem(child)
		}

		current.Set("options", options)
	}

	return current
}
