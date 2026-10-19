package markdown

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/nodejs/doc-kit/internal/omap"
	"github.com/nodejs/doc-kit/internal/unist"
)

// Handler turns an mdast node into hast nodes (nil drops it).
type Handler func(s *State, node, parent *unist.Node) []*unist.Node

// HastOptions configure the mdast to hast conversion, like `remark-rehype`'s.
type HastOptions struct {
	// AllowDangerousHTML keeps raw HTML (as `raw` nodes).
	AllowDangerousHTML bool
	// PassThrough lists node types copied as they are (their children are
	// still converted).
	PassThrough []string
	// Handlers override the conversion of node types.
	Handlers map[string]Handler
}

// State is the conversion in progress.
type State struct {
	opts           HastOptions
	definitions    map[string]*unist.Node
	footnoteByID   map[string]*unist.Node
	footnoteOrder  []string
	footnoteCounts map[string]int
}

// ToHast converts an mdast tree into a hast tree, like `mdast-util-to-hast`.
func ToHast(tree *unist.Node, opts HastOptions) *unist.Node {
	s := &State{
		opts:           opts,
		definitions:    map[string]*unist.Node{},
		footnoteByID:   map[string]*unist.Node{},
		footnoteCounts: map[string]int{},
	}

	unist.Visit(tree, nil, func(n *unist.Node, _ int, _ *unist.Node) unist.Step {
		if n.Type == "definition" || n.Type == "footnoteDefinition" {
			m := s.definitions

			if n.Type == "footnoteDefinition" {
				m = s.footnoteByID
			}

			id := strings.ToUpper(n.Identifier)

			if _, ok := m[id]; !ok {
				m[id] = n
			}
		}

		return unist.Next
	})

	result := s.One(tree, nil)

	var root *unist.Node

	if len(result) == 1 && result[0].Type == "root" {
		root = result[0]
	} else {
		root = unist.Root(result...)
	}

	if foot := s.footer(); foot != nil {
		root.Children = append(root.Children, unist.Text("\n"), foot)
	}

	return root
}

// One converts a node.
func (s *State) One(node, parent *unist.Node) []*unist.Node {
	if h, ok := s.opts.Handlers[node.Type]; ok {
		return h(s, node, parent)
	}

	if h, ok := defaultHandlers[node.Type]; ok {
		return h(s, node, parent)
	}

	for _, t := range s.opts.PassThrough {
		if t == node.Type {
			c := node.ShallowCopy()

			if c.Properties != nil {
				c.Properties = c.Properties.Clone()
			}

			if node.Children != nil {
				c.Children = s.All(node)
			}

			return []*unist.Node{c}
		}
	}

	// Unknown nodes: literals become text, parents a <div>
	if node.HasValue() {
		return []*unist.Node{unist.Text(node.Value)}
	}

	return []*unist.Node{Element("div", nil, s.All(node)...)}
}

// All converts the children of a node.
func (s *State) All(parent *unist.Node) []*unist.Node {
	values := []*unist.Node{}

	for i, child := range parent.Children {
		result := s.One(child, parent)

		if len(result) == 0 {
			continue
		}

		if i > 0 && parent.Children[i-1].Type == "break" && len(result) == 1 {
			r := result[0]

			if r.Type == "text" {
				r.Value = strings.TrimLeft(r.Value, " \t")
			} else if r.Type == "element" && len(r.Children) > 0 && r.Children[0].Type == "text" {
				r.Children[0].Value = strings.TrimLeft(r.Children[0].Value, " \t")
			}
		}

		values = append(values, result...)
	}

	return values
}

// Wrap separates nodes with line endings, and surrounds them with line
// endings too when loose.
func Wrap(nodes []*unist.Node, loose bool) []*unist.Node {
	result := []*unist.Node{}

	if loose {
		result = append(result, unist.Text("\n"))
	}

	for i, n := range nodes {
		if i > 0 {
			result = append(result, unist.Text("\n"))
		}

		result = append(result, n)
	}

	if loose && len(nodes) > 0 {
		result = append(result, unist.Text("\n"))
	}

	return result
}

func one(n *unist.Node) []*unist.Node {
	return []*unist.Node{n}
}

var defaultHandlers map[string]Handler

func init() {
	defaultHandlers = map[string]Handler{
		"root": func(s *State, n, _ *unist.Node) []*unist.Node {
			return one(unist.Root(Wrap(s.All(n), false)...))
		},
		"blockquote": func(s *State, n, _ *unist.Node) []*unist.Node {
			return one(Element("blockquote", nil, Wrap(s.All(n), true)...))
		},
		"break": func(s *State, n, _ *unist.Node) []*unist.Node {
			return []*unist.Node{Element("br", nil), unist.Text("\n")}
		},
		"code":      codeHandler,
		"delete":    simple("del"),
		"emphasis":  simple("em"),
		"strong":    simple("strong"),
		"paragraph": simple("p"),
		"heading": func(s *State, n, _ *unist.Node) []*unist.Node {
			return one(Element(fmt.Sprintf("h%d", n.Depth), nil, s.All(n)...))
		},
		"footnoteReference": footnoteReferenceHandler,
		"html": func(s *State, n, _ *unist.Node) []*unist.Node {
			if s.opts.AllowDangerousHTML {
				return one(&unist.Node{Type: "raw", Value: n.Value})
			}

			return nil
		},
		"image": func(s *State, n, _ *unist.Node) []*unist.Node {
			props := omap.New("src", NormalizeURI(n.URL), "alt", n.Alt)

			if n.Title != nil {
				props.Set("title", *n.Title)
			}

			return one(Element("img", props))
		},
		"imageReference": func(s *State, n, _ *unist.Node) []*unist.Node {
			def, ok := s.definitions[strings.ToUpper(n.Identifier)]

			if !ok {
				return revert(s, n)
			}

			props := omap.New("src", NormalizeURI(def.URL), "alt", n.Alt)

			if def.Title != nil {
				props.Set("title", *def.Title)
			}

			return one(Element("img", props))
		},
		"inlineCode": func(s *State, n, _ *unist.Node) []*unist.Node {
			value := lineEndings.ReplaceAllString(n.Value, " ")

			return one(Element("code", nil, unist.Text(value)))
		},
		"link": func(s *State, n, _ *unist.Node) []*unist.Node {
			props := omap.New("href", NormalizeURI(n.URL))

			if n.Title != nil {
				props.Set("title", *n.Title)
			}

			return one(Element("a", props, s.All(n)...))
		},
		"linkReference": func(s *State, n, _ *unist.Node) []*unist.Node {
			def, ok := s.definitions[strings.ToUpper(n.Identifier)]

			if !ok {
				return revert(s, n)
			}

			props := omap.New("href", NormalizeURI(def.URL))

			if def.Title != nil {
				props.Set("title", *def.Title)
			}

			return one(Element("a", props, s.All(n)...))
		},
		"list":     listHandler,
		"listItem": listItemHandler,
		"table":    tableHandler,
		"tableRow": tableRowHandler,
		"tableCell": func(s *State, n, _ *unist.Node) []*unist.Node {
			return one(Element("td", nil, s.All(n)...))
		},
		"text": func(s *State, n, _ *unist.Node) []*unist.Node {
			return one(unist.Text(trimLines(n.Value)))
		},
		"thematicBreak": func(s *State, n, _ *unist.Node) []*unist.Node {
			return one(Element("hr", nil))
		},
		"definition":         ignore,
		"footnoteDefinition": ignore,
		"yaml":               ignore,
		"toml":               ignore,
		"typeAnnotation":     TypeAnnotationToHast,
	}
}

// revert turns an unresolved reference back into the text it was written as.
func revert(s *State, n *unist.Node) []*unist.Node {
	suffix := "]"

	switch n.ReferenceType {
	case "collapsed":
		suffix += "[]"
	case "full":
		label := n.Label

		if label == "" {
			label = n.Identifier
		}

		suffix += "[" + label + "]"
	}

	if n.Type == "imageReference" {
		return one(unist.Text("![" + n.Alt + suffix))
	}

	contents := s.All(n)

	if len(contents) > 0 && contents[0].Type == "text" {
		contents[0].Value = "[" + contents[0].Value
	} else {
		contents = append([]*unist.Node{unist.Text("[")}, contents...)
	}

	if tail := contents[len(contents)-1]; tail.Type == "text" {
		tail.Value += suffix
	} else {
		contents = append(contents, unist.Text(suffix))
	}

	return contents
}

func ignore(*State, *unist.Node, *unist.Node) []*unist.Node { return nil }

func simple(tag string) Handler {
	return func(s *State, n, _ *unist.Node) []*unist.Node {
		return one(Element(tag, nil, s.All(n)...))
	}
}

var (
	lineEndings = regexp.MustCompile(`\r?\n|\r`)
	langPattern = regexp.MustCompile(`^[^ \t]+`)
)

func codeHandler(s *State, n, _ *unist.Node) []*unist.Node {
	value := ""

	if n.Value != "" {
		value = n.Value + "\n"
	}

	props := omap.New()

	if n.Lang != nil {
		if lang := langPattern.FindString(*n.Lang); lang != "" {
			props.Set("className", []string{"language-" + lang})
		}
	}

	code := Element("code", props, unist.Text(value))

	if n.Meta != nil {
		code.Data = &unist.Data{Description: *n.Meta}
	}

	return one(Element("pre", nil, code))
}

// trimLines removes the spaces and tabs around line endings, like
// `trim-lines`.
func trimLines(value string) string {
	if !strings.ContainsAny(value, "\r\n") {
		return value
	}

	lines := lineSplit.FindAllStringIndex(value, -1)

	var b strings.Builder

	last := 0

	for i, loc := range lines {
		line := value[last:loc[0]]

		if i > 0 {
			line = strings.TrimLeft(line, " \t")
		}

		b.WriteString(strings.TrimRight(line, " \t"))
		b.WriteString(value[loc[0]:loc[1]])
		last = loc[1]
	}

	b.WriteString(strings.TrimLeft(value[last:], " \t"))

	return b.String()
}

var lineSplit = regexp.MustCompile(`\r?\n|\r`)

func listLoose(n *unist.Node) bool {
	if n.Type != "list" {
		return false
	}

	if n.Spread {
		return true
	}

	for _, item := range n.Children {
		if item.Spread {
			return true
		}
	}

	return false
}

func listHandler(s *State, n, _ *unist.Node) []*unist.Node {
	props := omap.New()
	results := s.All(n)

	if n.Ordered && n.Start != nil && *n.Start != 1 {
		props.Set("start", *n.Start)
	}

	for _, child := range results {
		if child.Type == "element" && child.TagName == "li" {
			if cls, ok := child.Properties.Value("className").([]string); ok && contains(cls, "task-list-item") {
				props.Set("className", []string{"contains-task-list"})
				break
			}
		}
	}

	tag := "ul"

	if n.Ordered {
		tag = "ol"
	}

	return one(Element(tag, props, Wrap(results, true)...))
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}

	return false
}

func listItemHandler(s *State, n, parent *unist.Node) []*unist.Node {
	results := s.All(n)

	loose := n.Spread

	if parent != nil {
		loose = listLoose(parent)
	}

	props := omap.New()
	children := []*unist.Node{}

	if n.Checked != nil {
		var paragraph *unist.Node

		if len(results) > 0 && results[0].Type == "element" && results[0].TagName == "p" {
			paragraph = results[0]
		} else {
			paragraph = Element("p", nil)
			results = append([]*unist.Node{paragraph}, results...)
		}

		if len(paragraph.Children) > 0 {
			paragraph.Children = append([]*unist.Node{unist.Text(" ")}, paragraph.Children...)
		}

		input := Element("input", omap.New("type", "checkbox", "checked", *n.Checked, "disabled", true))
		paragraph.Children = append([]*unist.Node{input}, paragraph.Children...)
		props.Set("className", []string{"task-list-item"})
	}

	for i, child := range results {
		isP := child.Type == "element" && child.TagName == "p"

		if loose || i != 0 || !isP {
			children = append(children, unist.Text("\n"))
		}

		if isP && !loose {
			children = append(children, child.Children...)
		} else {
			children = append(children, child)
		}
	}

	if len(results) > 0 {
		tail := results[len(results)-1]

		if loose || tail.Type != "element" || tail.TagName != "p" {
			children = append(children, unist.Text("\n"))
		}
	}

	return one(Element("li", props, children...))
}

func tableHandler(s *State, n, _ *unist.Node) []*unist.Node {
	rows := s.All(n)
	content := []*unist.Node{}

	if len(rows) > 0 {
		content = append(content, Element("thead", nil, Wrap(rows[:1], true)...))
		rows = rows[1:]
	}

	if len(rows) > 0 {
		content = append(content, Element("tbody", nil, Wrap(rows, true)...))
	}

	return one(Element("table", nil, Wrap(content, true)...))
}

func tableRowHandler(s *State, n, parent *unist.Node) []*unist.Node {
	rowIndex := 1

	if parent != nil {
		rowIndex = parent.IndexOf(n)
	}

	tag := "td"

	if rowIndex == 0 {
		tag = "th"
	}

	var align []string

	if parent != nil && parent.Type == "table" {
		align = parent.Align
	}

	length := len(n.Children)

	if align != nil {
		length = len(align)
	}

	cells := []*unist.Node{}

	for i := range length {
		props := omap.New()

		if align != nil && align[i] != "" {
			props.Set("align", align[i])
		}

		cell := Element(tag, props)

		if i < len(n.Children) {
			cell.Children = s.All(n.Children[i])
		}

		cells = append(cells, cell)
	}

	return one(Element("tr", nil, Wrap(cells, true)...))
}

const clobberPrefix = "user-content-"

func footnoteReferenceHandler(s *State, n, _ *unist.Node) []*unist.Node {
	id := strings.ToUpper(n.Identifier)
	safeID := NormalizeURI(strings.ToLower(id))

	counter := 0
	index := -1

	for i, existing := range s.footnoteOrder {
		if existing == id {
			index = i
		}
	}

	reuse, seen := s.footnoteCounts[id]

	if !seen {
		s.footnoteOrder = append(s.footnoteOrder, id)
		counter = len(s.footnoteOrder)
	} else {
		counter = index + 1
	}

	reuse++
	s.footnoteCounts[id] = reuse

	refID := clobberPrefix + "fnref-" + safeID

	if reuse > 1 {
		refID += fmt.Sprintf("-%d", reuse)
	}

	link := Element("a", omap.New(
		"href", "#"+clobberPrefix+"fn-"+safeID,
		"id", refID,
		"dataFootnoteRef", true,
		"ariaDescribedBy", []string{"footnote-label"},
	), unist.Text(fmt.Sprint(counter)))

	return one(Element("sup", nil, link))
}

func (s *State) footer() *unist.Node {
	var items []*unist.Node

	for ri, id := range s.footnoteOrder {
		def, ok := s.footnoteByID[id]

		if !ok {
			continue
		}

		content := s.All(def)
		safeID := NormalizeURI(strings.ToLower(strings.ToUpper(def.Identifier)))

		var backs []*unist.Node

		for rr := 1; rr <= s.footnoteCounts[id]; rr++ {
			if len(backs) > 0 {
				backs = append(backs, unist.Text(" "))
			}

			href := "#" + clobberPrefix + "fnref-" + safeID
			label := fmt.Sprintf("Back to reference %d", ri+1)

			children := []*unist.Node{unist.Text("↩")}

			if rr > 1 {
				href += fmt.Sprintf("-%d", rr)
				label += fmt.Sprintf("-%d", rr)
				children = append(children, Element("sup", nil, unist.Text(fmt.Sprint(rr))))
			}

			backs = append(backs, Element("a", omap.New(
				"href", href,
				"dataFootnoteBackref", "",
				"ariaLabel", label,
				"className", []string{"data-footnote-backref"},
			), children...))
		}

		if len(content) > 0 && content[len(content)-1].Type == "element" && content[len(content)-1].TagName == "p" {
			tail := content[len(content)-1]

			if len(tail.Children) > 0 && tail.Children[len(tail.Children)-1].Type == "text" {
				tail.Children[len(tail.Children)-1].Value += " "
			} else {
				tail.Children = append(tail.Children, unist.Text(" "))
			}

			tail.Children = append(tail.Children, backs...)
		} else {
			content = append(content, backs...)
		}

		items = append(items, Element("li", omap.New("id", clobberPrefix+"fn-"+safeID), Wrap(content, true)...))
	}

	if len(items) == 0 {
		return nil
	}

	return Element("section", omap.New("dataFootnotes", true, "className", []string{"footnotes"}),
		Element("h2", omap.New("className", []string{"sr-only"}, "id", "footnote-label"), unist.Text("Footnotes")),
		unist.Text("\n"),
		Element("ol", nil, Wrap(items, true)...),
		unist.Text("\n"),
	)
}

// TypeAnnotationToHast renders a resolved `typeAnnotation` as one
// `<code class="type">` whose resolved identifiers are plain `<a>` links.
func TypeAnnotationToHast(s *State, n, _ *unist.Node) []*unist.Node {
	var links []unist.TypeLink

	if n.Data != nil {
		links = n.Data.Links
	}

	return one(Element("code", omap.New("className", []string{"type"}), LinkedTypeChildren(n.Value, links)...))
}

// LinkedTypeChildren slices a type's text by its resolved link ranges into
// text segments interleaved with `<a class="type-link">` anchors.
func LinkedTypeChildren(value string, links []unist.TypeLink) []*unist.Node {
	children := []*unist.Node{}
	units := utf16Offsets(value)
	cursor := 0

	for _, l := range links {
		if l.Start > cursor {
			children = append(children, unist.Text(sliceUTF16(value, units, cursor, l.Start)))
		}

		children = append(children, Element("a",
			omap.New("href", l.Href, "className", []string{"type-link"}),
			unist.Text(sliceUTF16(value, units, l.Start, l.End))))

		cursor = l.End
	}

	if cursor < len(units)-1 {
		children = append(children, unist.Text(sliceUTF16(value, units, cursor, len(units)-1)))
	}

	return children
}

// utf16Offsets maps each UTF-16 code unit index of a string (as JavaScript
// counts them) to a byte offset; the final entry is the string's length.
func utf16Offsets(s string) []int {
	offsets := make([]int, 0, len(s)+1)

	for i, r := range s {
		offsets = append(offsets, i)

		if r >= 0x10000 {
			offsets = append(offsets, i)
		}
	}

	return append(offsets, len(s))
}

func sliceUTF16(s string, units []int, start, end int) string {
	if start < 0 {
		start = 0
	}

	if end > len(units)-1 {
		end = len(units) - 1
	}

	if start >= end {
		return ""
	}

	return s[units[start]:units[end]]
}

// NormalizeURI encodes the characters of a URL that are not safe in it, like
// `micromark-util-sanitize-uri`'s `normalizeUri`.
func NormalizeURI(value string) string {
	var b strings.Builder

	for i := 0; i < len(value); {
		c := value[i]

		if c == '%' && i+2 < len(value) && isASCIIAlnum(value[i+1]) && isASCIIAlnum(value[i+2]) {
			b.WriteString(value[i : i+3])
			i += 3

			continue
		}

		if c < 0x80 {
			if isURISafe(c) {
				b.WriteByte(c)
			} else {
				b.WriteString(encodeURIComponent(string(c)))
			}

			i++

			continue
		}

		r, size := utf8.DecodeRuneInString(value[i:])

		if r == utf8.RuneError && size == 1 {
			r = '\uFFFD'
		}

		b.WriteString(encodeURIComponent(string(r)))
		i += size
	}

	return b.String()
}

func isASCIIAlnum(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// isURISafe matches `[!#$&-;=?-Z_a-z~]`.
func isURISafe(c byte) bool {
	return c == '!' || c == '#' || c == '$' || (c >= '&' && c <= ';') || c == '=' ||
		(c >= '?' && c <= 'Z') || c == '_' || (c >= 'a' && c <= 'z') || c == '~'
}

func encodeURIComponent(s string) string {
	var b strings.Builder

	for i := 0; i < len(s); i++ {
		c := s[i]

		if isASCIIAlnum(c) || strings.IndexByte("-_.!~*'()", c) >= 0 {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}

	return b.String()
}
