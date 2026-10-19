// Package markdown parses Markdown into mdast trees, renders them into HTML
// (through hast) and serializes them back into Markdown. It stands in for the
// unified/remark/rehype pipeline of the original tooling.
package markdown

import (
	"bytes"
	"sort"
	"strings"
	"sync"

	"github.com/nodejs/doc-kit/internal/unist"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Options select the syntax extensions a parse recognizes.
type Options struct {
	// GFM enables tables, strikethrough, autolink literals, task lists and
	// footnotes.
	GFM bool
	// TypeAnnotations enables `{Type}` annotations (off in MDX, where braces
	// are expressions).
	TypeAnnotations bool
}

var (
	parsersMu sync.Mutex
	parsers   = map[Options]parser.Parser{}
)

func getParser(opts Options) parser.Parser {
	parsersMu.Lock()
	defer parsersMu.Unlock()

	if p, ok := parsers[opts]; ok {
		return p
	}

	var exts []goldmark.Extender

	if opts.GFM {
		exts = append(exts, extension.Table, extension.Strikethrough, extension.Linkify, extension.TaskList)
	}

	p := goldmark.New(goldmark.WithExtensions(exts...)).Parser()

	if opts.GFM {
		p.AddOptions(
			parser.WithBlockParsers(util.Prioritized(footnoteBlockParser{}, 999)),
			parser.WithInlineParsers(util.Prioritized(footnoteParser{}, 101)),
		)
	}

	if opts.TypeAnnotations {
		p.AddOptions(typeAnnotationOption)
	}

	parsers[opts] = p

	return p
}

// Parse parses Markdown into an mdast tree.
func Parse(source string, opts Options) *unist.Node {
	src := []byte(source)
	doc := getParser(opts).Parse(text.NewReader(src))

	c := &converter{src: src, lines: lineStarts(src)}

	root := unist.Root()
	root.Children = c.blocks(doc)
	root.Position = &unist.Position{Start: unist.Point{Line: 1, Column: 1}, End: c.point(len(src))}

	return root
}

// ParseInline parses a single line of Markdown into the phrasing nodes it is
// made of (or its blocks, when it is not a lone paragraph).
func ParseInline(source string, opts Options) []*unist.Node {
	tree := Parse(source, opts)

	if len(tree.Children) == 1 && tree.Children[0].Type == "paragraph" {
		return tree.Children[0].Children
	}

	return tree.Children
}

type converter struct {
	src   []byte
	lines []int
}

func lineStarts(src []byte) []int {
	starts := []int{0}

	for i, c := range src {
		if c == '\n' {
			starts = append(starts, i+1)
		}
	}

	return starts
}

// point turns a byte offset into a line/column point.
func (c *converter) point(offset int) unist.Point {
	line := sort.Search(len(c.lines), func(i int) bool { return c.lines[i] > offset }) - 1

	if line < 0 {
		line = 0
	}

	return unist.Point{Line: line + 1, Column: offset - c.lines[line] + 1, Offset: offset}
}

func (c *converter) span(start, stop int) *unist.Position {
	return &unist.Position{Start: c.point(start), End: c.point(stop)}
}

// blockSpan is the span of a block's lines.
func (c *converter) blockSpan(n ast.Node) *unist.Position {
	start, stop := -1, -1

	ast.Walk(n, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || child.Type() != ast.TypeBlock {
			return ast.WalkContinue, nil
		}

		lines := child.Lines()

		for i := range lines.Len() {
			seg := lines.At(i)

			if start < 0 || seg.Start < start {
				start = seg.Start
			}

			if seg.Stop > stop {
				stop = seg.Stop
			}
		}

		return ast.WalkContinue, nil
	})

	if start < 0 {
		return nil
	}

	// Line segments include their line ending, which a position does not
	for stop > start && (c.src[stop-1] == '\n' || c.src[stop-1] == '\r') {
		stop--
	}

	return c.span(start, stop)
}

// linesValue joins a block's lines.
func (c *converter) linesValue(n ast.Node) string {
	var b bytes.Buffer

	lines := n.Lines()

	for i := range lines.Len() {
		seg := lines.At(i)
		b.WriteString(strings.Repeat(" ", seg.Padding))
		b.Write(seg.Value(c.src))
	}

	return b.String()
}

func trimFinalLineEnding(s string) string {
	s = strings.TrimSuffix(s, "\n")

	return strings.TrimSuffix(s, "\r")
}

func (c *converter) blocks(parent ast.Node) []*unist.Node {
	nodes := []*unist.Node{}

	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		if n := c.block(child); n != nil {
			nodes = append(nodes, n)
		}
	}

	return nodes
}

func (c *converter) block(n ast.Node) *unist.Node {
	var out *unist.Node

	switch node := n.(type) {
	case *ast.Paragraph, *ast.TextBlock:
		out = &unist.Node{Type: "paragraph", Children: c.inlines(n)}

		if len(out.Children) == 0 {
			return nil
		}
	case *ast.Heading:
		out = &unist.Node{Type: "heading", Depth: node.Level, Children: c.inlines(n)}
	case *ast.ThematicBreak:
		out = &unist.Node{Type: "thematicBreak"}
	case *ast.Blockquote:
		out = &unist.Node{Type: "blockquote", Children: c.blocks(n)}
	case *ast.List:
		out = c.list(node)
	case *ast.CodeBlock:
		out = &unist.Node{Type: "code", Value: trimFinalLineEnding(c.linesValue(n))}
		out.Value = strings.TrimRight(out.Value, "\n")
	case *ast.FencedCodeBlock:
		out = &unist.Node{Type: "code", Value: trimFinalLineEnding(c.linesValue(n))}

		if node.Info != nil {
			info := strings.TrimSpace(decodeString(string(node.Info.Value(c.src))))

			if info != "" {
				lang, meta, _ := strings.Cut(info, " ")

				if i := strings.IndexAny(info, " \t"); i >= 0 {
					lang, meta = info[:i], info[i+1:]
				}

				out.Lang = unist.StrPtr(lang)

				if meta = strings.TrimSpace(meta); meta != "" {
					out.Meta = unist.StrPtr(meta)
				}
			}
		}
	case *ast.HTMLBlock:
		value := c.linesValue(n)

		if node.HasClosure() {
			value += string(node.ClosureLine.Value(c.src))
		}

		out = &unist.Node{Type: "html", Value: trimFinalLineEnding(value)}
	case *ast.LinkReferenceDefinition:
		out = &unist.Node{
			Type:       "definition",
			Identifier: normalizeIdentifier(string(node.Label)),
			Label:      decodeString(string(node.Label)),
			URL:        decodeString(string(node.Destination)),
		}

		if node.Title != nil {
			out.Title = unist.StrPtr(decodeString(string(node.Title)))
		}
	case *east.Table:
		out = c.table(node)
	case *footnoteDefinitionNode:
		out = &unist.Node{
			Type:       "footnoteDefinition",
			Identifier: normalizeIdentifier(node.label),
			Label:      decodeString(node.label),
			Children:   c.blocks(n),
		}
	default:
		return nil
	}

	if out.Position == nil {
		out.Position = c.blockSpan(n)
	}

	return out
}

// normalizeIdentifier normalizes a link label like micromark does.
func normalizeIdentifier(label string) string {
	return strings.ToLower(strings.Join(strings.Fields(label), " "))
}

func (c *converter) list(node *ast.List) *unist.Node {
	out := &unist.Node{Type: "list", Ordered: node.IsOrdered(), Spread: !node.IsTight}

	if node.IsOrdered() {
		start := node.Start
		out.Start = &start
	}

	for item := node.FirstChild(); item != nil; item = item.NextSibling() {
		li := &unist.Node{Type: "listItem", Spread: false, Children: c.blocks(item)}
		li.Position = c.blockSpan(item)

		// A blank line between its direct children makes an item spread
		li.Spread = itemSpread(item, c.src)

		// A task list item's checkbox sits at the start of its paragraph
		if len(li.Children) > 0 && li.Children[0].Type == "paragraph" {
			if box := findCheckBox(item); box != nil {
				checked := box.IsChecked
				li.Checked = &checked
			}
		}

		out.Children = append(out.Children, li)
	}

	// A list is spread when a blank line separates two of its items
	out.Spread = false

	prevEnd := -1

	for item := node.FirstChild(); item != nil; item = item.NextSibling() {
		start, end := blockRange(item)

		if start < 0 {
			continue
		}

		if prevEnd >= 0 && hasBlankLine(c.src, prevEnd, start) {
			out.Spread = true
		}

		prevEnd = end
	}

	return out
}

// itemSpread reports whether a blank line separates any two direct children
// of a list item.
func itemSpread(item ast.Node, src []byte) bool {
	var prevEnd = -1

	for child := item.FirstChild(); child != nil; child = child.NextSibling() {
		start, end := blockRange(child)

		if start < 0 {
			continue
		}

		if prevEnd >= 0 && hasBlankLine(src, prevEnd, start) {
			return true
		}

		prevEnd = end
	}

	return false
}

func blockRange(n ast.Node) (int, int) {
	start, end := -1, -1

	ast.Walk(n, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || child.Type() != ast.TypeBlock {
			return ast.WalkContinue, nil
		}

		lines := child.Lines()

		for i := range lines.Len() {
			seg := lines.At(i)

			if start < 0 || seg.Start < start {
				start = seg.Start
			}

			if seg.Stop > end {
				end = seg.Stop
			}
		}

		if fenced, ok := child.(*ast.FencedCodeBlock); ok && fenced.Info != nil && fenced.Info.Segment.Start < start {
			start = fenced.Info.Segment.Start
		}

		return ast.WalkContinue, nil
	})

	return start, end
}

func hasBlankLine(src []byte, from, to int) bool {
	if from > to || to > len(src) {
		return false
	}

	between := src[from:to]
	lines := bytes.Split(between, []byte("\n"))

	// The first piece is the rest of the previous block's last line, and the
	// last piece the start of the next block's first line
	for i := 1; i < len(lines)-1; i++ {
		if len(bytes.TrimSpace(lines[i])) == 0 {
			return true
		}
	}

	return false
}

func findCheckBox(item ast.Node) *east.TaskCheckBox {
	first := item.FirstChild()

	if first == nil {
		return nil
	}

	if box, ok := first.FirstChild().(*east.TaskCheckBox); ok {
		return box
	}

	return nil
}

var alignments = map[east.Alignment]string{
	east.AlignLeft:   "left",
	east.AlignRight:  "right",
	east.AlignCenter: "center",
}

func (c *converter) table(node *east.Table) *unist.Node {
	out := &unist.Node{Type: "table"}

	for _, a := range node.Alignments {
		out.Align = append(out.Align, alignments[a])
	}

	for row := node.FirstChild(); row != nil; row = row.NextSibling() {
		tr := &unist.Node{Type: "tableRow"}

		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			td := &unist.Node{Type: "tableCell", Children: c.inlines(cell)}
			td.Position = c.blockSpan(cell)
			tr.Children = append(tr.Children, td)
		}

		tr.Position = c.blockSpan(row)
		out.Children = append(out.Children, tr)
	}

	return out
}

// inlineBuilder accumulates phrasing nodes, merging adjacent text.
type inlineBuilder struct {
	nodes []*unist.Node
	// raw text pending decoding, and where it started
	pending      strings.Builder
	pendingStart int
	pendingEnd   int
	c            *converter
}

func (b *inlineBuilder) addText(s string, start, end int) {
	if b.pending.Len() == 0 && b.pendingStart < 0 {
		b.pendingStart = start
	}

	if b.pendingStart < 0 {
		b.pendingStart = start
	}

	b.pending.WriteString(s)

	if end >= 0 {
		b.pendingEnd = end
	}
}

func (b *inlineBuilder) flush() {
	if b.pendingStart < 0 {
		return
	}

	value := b.pending.String()

	if value != "" {
		n := unist.Text(value)

		if b.pendingStart >= 0 && b.pendingEnd >= b.pendingStart {
			n.Position = b.c.span(b.pendingStart, b.pendingEnd)
		}

		b.nodes = append(b.nodes, n)
	}

	b.pending.Reset()
	b.pendingStart = -1
	b.pendingEnd = -1
}

func (b *inlineBuilder) add(n *unist.Node) {
	b.flush()
	b.nodes = append(b.nodes, n)
}

func (c *converter) inlines(parent ast.Node) []*unist.Node {
	b := &inlineBuilder{c: c, pendingStart: -1, pendingEnd: -1}

	c.collectInlines(parent, b)
	b.flush()

	nodes := b.nodes

	// A hard break ends nothing at the end of a block
	for len(nodes) > 0 && nodes[len(nodes)-1].Type == "break" {
		nodes = nodes[:len(nodes)-1]
	}

	// Nor does whitespace
	if len(nodes) > 0 && nodes[len(nodes)-1].Type == "text" {
		last := nodes[len(nodes)-1]
		last.Value = strings.TrimRight(last.Value, " \t")

		if last.Value == "" {
			nodes = nodes[:len(nodes)-1]
		}
	}

	if nodes == nil {
		nodes = []*unist.Node{}
	}

	return nodes
}

func (c *converter) collectInlines(parent ast.Node, b *inlineBuilder) {
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		c.inline(child, b)
	}
}

// children converts the phrasing content of an inline node.
func (c *converter) children(n ast.Node) []*unist.Node {
	b := &inlineBuilder{c: c, pendingStart: -1, pendingEnd: -1}

	c.collectInlines(n, b)
	b.flush()

	if b.nodes == nil {
		return []*unist.Node{}
	}

	return b.nodes
}

func (c *converter) inline(n ast.Node, b *inlineBuilder) {
	switch node := n.(type) {
	case *ast.Text:
		value := string(node.Segment.Value(c.src))

		if !node.IsRaw() {
			value = decodeString(value)
		}

		b.addText(value, node.Segment.Start, node.Segment.Stop)

		if node.SoftLineBreak() {
			b.addText("\n", node.Segment.Stop, -1)
		}

		if node.HardLineBreak() {
			br := &unist.Node{Type: "break"}
			b.add(br)
		}
	case *ast.String:
		b.addText(string(node.Value), -1, -1)
	case *ast.CodeSpan:
		var v strings.Builder

		for t := node.FirstChild(); t != nil; t = t.NextSibling() {
			if seg, ok := t.(*ast.Text); ok {
				v.Write(seg.Segment.Value(c.src))
			} else if s, ok := t.(*ast.String); ok {
				v.Write(s.Value)
			}
		}

		value := v.String()
		out := &unist.Node{Type: "inlineCode", Value: value}
		out.Position = c.inlineSpan(node)
		b.add(out)
	case *ast.Emphasis:
		typ := "emphasis"

		if node.Level == 2 {
			typ = "strong"
		}

		out := &unist.Node{Type: typ, Children: c.children(node)}
		out.Position = c.inlineSpan(node)
		b.add(out)
	case *ast.Link:
		out := &unist.Node{Type: "link", URL: decodeString(string(node.Destination)), Children: c.children(node)}

		// A link made from a reference only keeps its definition's URL
		if node.Reference == nil && node.Title != nil {
			out.Title = unist.StrPtr(decodeString(string(node.Title)))
		}

		out.Position = c.inlineSpan(node)
		b.add(out)
	case *ast.Image:
		children := c.children(node)

		// Images made from a reference stay references: nothing resolves
		// them after the definitions are dropped
		if ref := node.Reference; ref != nil {
			label := decodeString(string(ref.Value))
			out := &unist.Node{
				Type:          "imageReference",
				Alt:           unist.ToStringAll(children),
				Identifier:    normalizeIdentifier(label),
				Label:         label,
				ReferenceType: referenceTypes[ref.Type],
			}

			b.add(out)

			return
		}

		out := &unist.Node{Type: "image", URL: decodeString(string(node.Destination)), Alt: unist.ToStringAll(children)}

		if node.Title != nil {
			out.Title = unist.StrPtr(decodeString(string(node.Title)))
		}

		b.add(out)
	case *ast.AutoLink:
		label := string(node.Label(c.src))
		url := string(node.URL(c.src))

		// micromark leaves `name@1.2.3` (a package version) alone: an email
		// domain must end in a label with a letter
		if node.AutoLinkType == ast.AutoLinkEmail && !strings.HasPrefix(strings.ToLower(label), "mailto:") {
			domain := label[strings.LastIndex(label, "@")+1:]
			last := domain[strings.LastIndex(domain, ".")+1:]

			if !strings.ContainsFunc(last, func(r rune) bool { return r < 0x80 && (r|0x20 >= 'a' && r|0x20 <= 'z') }) {
				b.addText(label, -1, -1)

				return
			}
		}

		if node.AutoLinkType == ast.AutoLinkEmail && !strings.HasPrefix(strings.ToLower(url), "mailto:") {
			url = "mailto:" + url
		}

		if node.Protocol == nil && strings.HasPrefix(label, "www.") {
			url = "http://" + label
		}

		out := &unist.Node{Type: "link", URL: url, Children: []*unist.Node{unist.Text(label)}}
		b.add(out)
	case *ast.RawHTML:
		var v strings.Builder

		for i := range node.Segments.Len() {
			seg := node.Segments.At(i)
			v.Write(seg.Value(c.src))
		}

		b.add(&unist.Node{Type: "html", Value: v.String()})
	case *east.Strikethrough:
		b.add(&unist.Node{Type: "delete", Children: c.children(node)})
	case *east.TaskCheckBox:
		// Dropped; the list item records it. The space after it goes too.
		if next, ok := node.NextSibling().(*ast.Text); ok {
			v := next.Segment.Value(c.src)

			if len(v) > 0 && (v[0] == ' ' || v[0] == '\t') {
				next.Segment = next.Segment.WithStart(next.Segment.Start + 1)
			}
		}
	case *typeAnnotationNode:
		out := &unist.Node{Type: "typeAnnotation", Value: node.value}
		out.Position = c.span(node.start, node.stop)
		b.add(out)
	case *footnoteReferenceNode:
		b.add(&unist.Node{Type: "footnoteReference", Identifier: normalizeIdentifier(node.label), Label: decodeString(node.label)})
	default:
		c.collectInlines(n, b)
	}
}

var referenceTypes = map[ast.ReferenceLinkType]string{
	ast.ReferenceLinkFull:      "full",
	ast.ReferenceLinkCollapsed: "collapsed",
	ast.ReferenceLinkShortcut:  "shortcut",
}

// inlineSpan approximates an inline node's span from its text.
func (c *converter) inlineSpan(n ast.Node) *unist.Position {
	start, stop := -1, -1

	ast.Walk(n, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
		if t, ok := child.(*ast.Text); ok && entering {
			if start < 0 || t.Segment.Start < start {
				start = t.Segment.Start
			}

			if t.Segment.Stop > stop {
				stop = t.Segment.Stop
			}
		}

		return ast.WalkContinue, nil
	})

	if start < 0 {
		return nil
	}

	return c.span(start, stop)
}
