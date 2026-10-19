package markdown

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/nodejs/doc-kit/internal/unist"
)

// This file ports `mdast-util-to-markdown` (with the GFM and type annotation
// extensions), as `remark-stringify` runs it with its default options.

type unsafePattern struct {
	character      string
	before         string
	after          string
	atBreak        bool
	inConstruct    []string
	notInConstruct []string
	compiled       *regexp.Regexp
}

var fullPhrasingSpans = []string{"autolink", "destinationLiteral", "destinationRaw", "reference", "titleQuote", "titleApostrophe"}

var autolinkNotIn = []string{"autolink", "link", "image", "label"}

var strikethroughNotIn = []string{"autolink", "destinationLiteral", "destinationRaw", "reference", "titleQuote", "titleApostrophe"}

func c(s ...string) []string { return s }

var unsafePatterns = []*unsafePattern{
	{character: "\t", after: `[\r\n]`, inConstruct: c("phrasing")},
	{character: "\t", before: `[\r\n]`, inConstruct: c("phrasing")},
	{character: "\t", inConstruct: c("codeFencedLangGraveAccent", "codeFencedLangTilde")},
	{character: "\r", inConstruct: c("codeFencedLangGraveAccent", "codeFencedLangTilde", "codeFencedMetaGraveAccent", "codeFencedMetaTilde", "destinationLiteral", "headingAtx")},
	{character: "\n", inConstruct: c("codeFencedLangGraveAccent", "codeFencedLangTilde", "codeFencedMetaGraveAccent", "codeFencedMetaTilde", "destinationLiteral", "headingAtx")},
	{character: " ", after: `[\r\n]`, inConstruct: c("phrasing")},
	{character: " ", before: `[\r\n]`, inConstruct: c("phrasing")},
	{character: " ", inConstruct: c("codeFencedLangGraveAccent", "codeFencedLangTilde")},
	{character: "!", after: `\[`, inConstruct: c("phrasing"), notInConstruct: fullPhrasingSpans},
	{character: "\"", inConstruct: c("titleQuote")},
	{atBreak: true, character: "#"},
	{character: "#", inConstruct: c("headingAtx"), after: "(?:[\r\n]|$)"},
	{character: "&", after: `[#A-Za-z]`, inConstruct: c("phrasing")},
	{character: "'", inConstruct: c("titleApostrophe")},
	{character: "(", inConstruct: c("destinationRaw")},
	{before: `\]`, character: "(", inConstruct: c("phrasing"), notInConstruct: fullPhrasingSpans},
	{atBreak: true, before: `\d+`, character: ")"},
	{character: ")", inConstruct: c("destinationRaw")},
	{atBreak: true, character: "*", after: "(?:[ \t\r\n*])"},
	{character: "*", inConstruct: c("phrasing"), notInConstruct: fullPhrasingSpans},
	{atBreak: true, character: "+", after: "(?:[ \t\r\n])"},
	{atBreak: true, character: "-", after: "(?:[ \t\r\n-])"},
	{atBreak: true, before: `\d+`, character: ".", after: "(?:[ \t\r\n]|$)"},
	{atBreak: true, character: "<", after: `[!/?A-Za-z]`},
	{character: "<", after: `[!/?A-Za-z]`, inConstruct: c("phrasing"), notInConstruct: fullPhrasingSpans},
	{character: "<", inConstruct: c("destinationLiteral")},
	{atBreak: true, character: "="},
	{atBreak: true, character: ">"},
	{character: ">", inConstruct: c("destinationLiteral")},
	{atBreak: true, character: "["},
	{character: "[", inConstruct: c("phrasing"), notInConstruct: fullPhrasingSpans},
	{character: "[", inConstruct: c("label", "reference")},
	{character: "\\", after: `[\r\n]`, inConstruct: c("phrasing")},
	{character: "]", inConstruct: c("label", "reference")},
	{atBreak: true, character: "_"},
	{character: "_", inConstruct: c("phrasing"), notInConstruct: fullPhrasingSpans},
	{atBreak: true, character: "`"},
	{character: "`", inConstruct: c("codeFencedLangGraveAccent", "codeFencedMetaGraveAccent")},
	{character: "`", inConstruct: c("phrasing"), notInConstruct: fullPhrasingSpans},
	{atBreak: true, character: "~"},

	// GFM autolink literals
	{character: "@", before: `[+\-.\w]`, after: `[\-.\w]`, inConstruct: c("phrasing"), notInConstruct: autolinkNotIn},
	{character: ".", before: `[Ww]`, after: `[\-.\w]`, inConstruct: c("phrasing"), notInConstruct: autolinkNotIn},
	{character: ":", before: `[ps]`, after: `\/`, inConstruct: c("phrasing"), notInConstruct: autolinkNotIn},
	// GFM footnotes
	{character: "[", inConstruct: c("label", "phrasing", "reference")},
	// GFM strikethrough
	{character: "~", inConstruct: c("phrasing"), notInConstruct: strikethroughNotIn},
	// GFM tables
	{character: "\r", inConstruct: c("tableCell")},
	{character: "\n", inConstruct: c("tableCell")},
	{atBreak: true, character: "|", after: "[\t :-]"},
	{character: "|", inConstruct: c("tableCell")},
	{atBreak: true, character: ":", after: "-"},
	{atBreak: true, character: "-", after: "[:|-]"},
	// GFM task list items
	{atBreak: true, character: "-", after: "[:|-]"},
}

func init() {
	for _, p := range unsafePatterns {
		before := ""

		if p.atBreak {
			before = `[\r\n][\t ]*`
		}

		if p.before != "" {
			before += "(?:" + p.before + ")"
		}

		expr := ""

		if before != "" {
			expr = "(" + before + ")"
		}

		expr += regexp.QuoteMeta(p.character)

		if p.after != "" {
			expr += "(?:" + p.after + ")"
		}

		p.compiled = regexp.MustCompile(expr)
	}
}

type mdInfo struct {
	before string
	after  string
	encode []string
}

type mdState struct {
	stack       []string
	indexStack  []int
	bulletCur   string
	bulletLast  string
	attention   *attentionInfo
	handlerList map[string]mdHandler
}

type attentionInfo struct {
	before bool
	after  bool
}

type mdHandler struct {
	handle func(s *mdState, node, parent *unist.Node, info mdInfo) string
	peek   func(s *mdState, node, parent *unist.Node) string
}

func (s *mdState) enter(name string) func() {
	s.stack = append(s.stack, name)

	return func() { s.stack = s.stack[:len(s.stack)-1] }
}

func listInScope(stack []string, list []string, none bool) bool {
	if len(list) == 0 {
		return none
	}

	for _, item := range list {
		for _, s := range stack {
			if s == item {
				return true
			}
		}
	}

	return false
}

func patternInScope(stack []string, p *unsafePattern) bool {
	return listInScope(stack, p.inConstruct, true) && !listInScope(stack, p.notInConstruct, false)
}

type positionInfo struct {
	before bool
	after  bool
}

var markdownPunct = regexp.MustCompile("^[!-/:-@\\[-`{-~]")

// safe escapes the characters of a value that would otherwise form Markdown
// constructs, in the current construct.
func (s *mdState) safe(input string, info mdInfo) string {
	value := info.before + input + info.after
	var positions []int
	infos := map[int]*positionInfo{}

	for _, p := range unsafePatterns {
		if !patternInScope(s.stack, p) {
			continue
		}

		for _, m := range p.compiled.FindAllStringSubmatchIndex(value, -1) {
			before := p.before != "" || p.atBreak
			after := p.after != ""
			position := m[0]

			if before {
				position += m[3] - m[2]
			}

			if existing, ok := infos[position]; ok {
				if existing.before && !before {
					existing.before = false
				}

				if existing.after && !after {
					existing.after = false
				}
			} else {
				positions = append(positions, position)
				infos[position] = &positionInfo{before: before, after: after}
			}
		}
	}

	sort.Ints(positions)

	start := len(info.before)
	end := len(value) - len(info.after)

	var result strings.Builder

	for i, position := range positions {
		if position < start || position >= end {
			continue
		}

		if (position+1 < end && i+1 < len(positions) && positions[i+1] == position+1 &&
			infos[position].after && !infos[position+1].before && !infos[position+1].after) ||
			(i > 0 && positions[i-1] == position-1 && infos[position].before &&
				!infos[position-1].before && !infos[position-1].after) {
			continue
		}

		if start != position {
			result.WriteString(escapeBackslashes(value[start:position], "\\"))
		}

		start = position

		ch := value[position : position+1]

		if markdownPunct.MatchString(ch) && !containsString(info.encode, ch) {
			result.WriteString("\\")
		} else {
			result.WriteString(encodeCharacterReference(rune(value[position])))
			start++
		}
	}

	result.WriteString(escapeBackslashes(value[start:end], info.after))

	return result.String()
}

func containsString(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}

	return false
}

// escapeBackslashes escapes the backslashes in a value that would otherwise
// escape the punctuation after them.
func escapeBackslashes(value, after string) string {
	whole := value + after

	var b strings.Builder

	start := 0

	for i := 0; i < len(value); i++ {
		if whole[i] == '\\' && i+1 < len(whole) && markdownPunct.MatchString(whole[i+1:i+2]) {
			b.WriteString(value[start:i])
			b.WriteString("\\")
			start = i
		}
	}

	b.WriteString(value[start:])

	return b.String()
}

func encodeCharacterReference(code rune) string {
	return fmt.Sprintf("&#x%X;", code)
}

// ToMarkdown serializes an mdast tree into Markdown.
func ToMarkdown(tree *unist.Node) string {
	s := &mdState{}
	s.handlerList = markdownHandlers

	result := s.handle(tree, nil, mdInfo{before: "\n", after: "\n"})

	if result != "" && !strings.HasSuffix(result, "\n") && !strings.HasSuffix(result, "\r") {
		result += "\n"
	}

	return result
}

func (s *mdState) handle(node, parent *unist.Node, info mdInfo) string {
	h, ok := s.handlerList[node.Type]

	if !ok {
		panic("Cannot handle unknown node `" + node.Type + "`")
	}

	return h.handle(s, node, parent, info)
}

var markdownHandlers map[string]mdHandler

func init() {
	markdownHandlers = map[string]mdHandler{
		"blockquote":         {handle: mdBlockquote},
		"break":              {handle: mdBreak},
		"code":               {handle: mdCode},
		"definition":         {handle: mdDefinition},
		"emphasis":           {handle: mdEmphasis, peek: func(*mdState, *unist.Node, *unist.Node) string { return "*" }},
		"heading":            {handle: mdHeading},
		"html":               {handle: func(_ *mdState, n, _ *unist.Node, _ mdInfo) string { return n.Value }, peek: constPeek("<")},
		"image":              {handle: mdImage, peek: constPeek("!")},
		"imageReference":     {handle: mdImageReference, peek: constPeek("!")},
		"inlineCode":         {handle: mdInlineCodeWithTable, peek: constPeek("`")},
		"link":               {handle: mdLink, peek: mdLinkPeek},
		"list":               {handle: mdList},
		"listItem":           {handle: mdListItemWithTask},
		"paragraph":          {handle: mdParagraph},
		"root":               {handle: mdRoot},
		"strong":             {handle: mdStrong, peek: constPeek("*")},
		"text":               {handle: func(s *mdState, n, _ *unist.Node, info mdInfo) string { return s.safe(n.Value, info) }},
		"thematicBreak":      {handle: func(*mdState, *unist.Node, *unist.Node, mdInfo) string { return "***" }},
		"typeAnnotation":     {handle: func(_ *mdState, n, _ *unist.Node, _ mdInfo) string { return "{" + n.Value + "}" }},
		"delete":             {handle: mdDelete, peek: constPeek("~")},
		"footnoteReference":  {handle: mdFootnoteReference, peek: constPeek("[")},
		"footnoteDefinition": {handle: mdFootnoteDefinition},
		"table":              {handle: mdTable},
		"tableRow":           {handle: mdTableRow},
		"tableCell":          {handle: mdTableCell},
	}
}

func constPeek(v string) func(*mdState, *unist.Node, *unist.Node) string {
	return func(*mdState, *unist.Node, *unist.Node) string { return v }
}

var phrasingTypes = map[string]bool{
	"break": true, "delete": true, "emphasis": true, "footnote": true, "footnoteReference": true,
	"image": true, "imageReference": true, "inlineCode": true, "inlineMath": true, "link": true,
	"linkReference": true, "mdxJsxTextElement": true, "mdxTextExpression": true, "strong": true,
	"text": true, "textDirective": true,
}

func mdRoot(s *mdState, n, _ *unist.Node, info mdInfo) string {
	for _, child := range n.Children {
		if phrasingTypes[child.Type] {
			return s.containerPhrasing(n, info)
		}
	}

	return s.containerFlow(n)
}

func mdParagraph(s *mdState, n, _ *unist.Node, info mdInfo) string {
	exit := s.enter("paragraph")
	subexit := s.enter("phrasing")
	value := s.containerPhrasing(n, info)
	subexit()
	exit()

	return value
}

func firstChar(s string) string {
	if s == "" {
		return ""
	}

	_, size := utf8.DecodeRuneInString(s)

	return s[:size]
}

func lastChar(s string) string {
	if s == "" {
		return ""
	}

	_, size := utf8.DecodeLastRuneInString(s)

	return s[len(s)-size:]
}

var trailingLineEnding = regexp.MustCompile(`(\r?\n|\r)$`)

func (s *mdState) containerPhrasing(parent *unist.Node, info mdInfo) string {
	children := parent.Children
	var results []string
	before := info.before
	encodeAfter := ""

	s.indexStack = append(s.indexStack, -1)

	for i, child := range children {
		s.indexStack[len(s.indexStack)-1] = i

		var after string

		if i+1 < len(children) {
			next := children[i+1]
			h, ok := s.handlerList[next.Type]

			if ok {
				if h.peek != nil {
					after = firstChar(h.peek(s, next, parent))
				} else {
					after = firstChar(h.handle(s, next, parent, mdInfo{}))
				}
			}
		} else {
			after = info.after
		}

		if len(results) > 0 && (before == "\r" || before == "\n") && child.Type == "html" {
			results[len(results)-1] = trailingLineEnding.ReplaceAllString(results[len(results)-1], " ")
			before = " "
		}

		value := s.handle(child, parent, mdInfo{before: before, after: after})

		if encodeAfter != "" && encodeAfter == firstChar(value) {
			r, size := utf8.DecodeRuneInString(value)
			value = encodeCharacterReference(r) + value[size:]
		}

		encoding := s.attention
		s.attention = nil
		encodeAfter = ""

		if encoding != nil {
			if len(results) > 0 && encoding.before && before == lastChar(results[len(results)-1]) {
				last := results[len(results)-1]
				r, size := utf8.DecodeLastRuneInString(last)
				results[len(results)-1] = last[:len(last)-size] + encodeCharacterReference(r)
			}

			if encoding.after {
				encodeAfter = after
			}
		}

		results = append(results, value)
		before = lastChar(value)
	}

	s.indexStack = s.indexStack[:len(s.indexStack)-1]

	return strings.Join(results, "")
}

func (s *mdState) containerFlow(parent *unist.Node) string {
	children := parent.Children
	var b strings.Builder

	s.indexStack = append(s.indexStack, -1)

	for i, child := range children {
		s.indexStack[len(s.indexStack)-1] = i
		b.WriteString(s.handle(child, parent, mdInfo{before: "\n", after: "\n"}))

		if child.Type != "list" {
			s.bulletLast = ""
		}

		if i < len(children)-1 {
			b.WriteString(between(child, children[i+1], parent))
		}
	}

	s.indexStack = s.indexStack[:len(s.indexStack)-1]

	return b.String()
}

func between(left, right, parent *unist.Node) string {
	// joinDefaults: the children of a list or an item
	if parent.Type == "list" || parent.Type == "listItem" {
		if left.Type == "paragraph" && (left.Type == right.Type || right.Type == "definition" || (right.Type == "heading" && formatHeadingAsSetext(right))) {
			return "\n\n"
		}

		if parent.Spread {
			return "\n\n"
		}

		return "\n"
	}

	return "\n\n"
}

func indentLines(value string, mapper func(line string, index int, blank bool) string) string {
	var b strings.Builder

	start := 0
	line := 0

	for _, loc := range lineEndings.FindAllStringIndex(value, -1) {
		piece := value[start:loc[0]]
		b.WriteString(mapper(piece, line, piece == ""))
		b.WriteString(value[loc[0]:loc[1]])
		start = loc[1]
		line++
	}

	piece := value[start:]
	b.WriteString(mapper(piece, line, piece == ""))

	return b.String()
}

func mdBlockquote(s *mdState, n, _ *unist.Node, _ mdInfo) string {
	exit := s.enter("blockquote")
	value := indentLines(s.containerFlow(n), func(line string, _ int, blank bool) string {
		if blank {
			return ">" + line
		}

		return "> " + line
	})
	exit()

	return value
}

func mdBreak(s *mdState, _, _ *unist.Node, info mdInfo) string {
	for _, p := range unsafePatterns {
		if p.character == "\n" && patternInScope(s.stack, p) {
			if strings.ContainsAny(info.before, " \t") {
				return ""
			}

			return " "
		}
	}

	return "\\\n"
}

func longestStreak(value string, ch byte) int {
	best, current := 0, 0

	for i := 0; i < len(value); i++ {
		if value[i] == ch {
			current++

			if current > best {
				best = current
			}
		} else {
			current = 0
		}
	}

	return best
}

func mdCode(s *mdState, n, _ *unist.Node, _ mdInfo) string {
	raw := n.Value
	sequence := strings.Repeat("`", max(longestStreak(raw, '`')+1, 3))
	exit := s.enter("codeFenced")
	value := sequence

	if n.Lang != nil && *n.Lang != "" {
		subexit := s.enter("codeFencedLangGraveAccent")
		value += s.safe(*n.Lang, mdInfo{before: value, after: " ", encode: []string{"`"}})
		subexit()
	}

	if n.Lang != nil && *n.Lang != "" && n.Meta != nil && *n.Meta != "" {
		subexit := s.enter("codeFencedMetaGraveAccent")
		value += " "
		value += s.safe(*n.Meta, mdInfo{before: value, after: "\n", encode: []string{"`"}})
		subexit()
	}

	value += "\n"

	if raw != "" {
		value += raw + "\n"
	}

	value += sequence
	exit()

	return value
}

var controlOrSpace = regexp.MustCompile("[\x00- \x7F]")

func mdDefinition(s *mdState, n, _ *unist.Node, _ mdInfo) string {
	exit := s.enter("definition")
	subexit := s.enter("label")
	value := "["
	value += s.safe(association(n), mdInfo{before: value, after: "]"})
	value += "]: "
	subexit()

	if n.URL == "" || controlOrSpace.MatchString(n.URL) {
		subexit = s.enter("destinationLiteral")
		value += "<"
		value += s.safe(n.URL, mdInfo{before: value, after: ">"})
		value += ">"
	} else {
		subexit = s.enter("destinationRaw")
		after := "\n"

		if n.Title != nil && *n.Title != "" {
			after = " "
		}

		value += s.safe(n.URL, mdInfo{before: value, after: after})
	}

	subexit()

	if n.Title != nil && *n.Title != "" {
		subexit = s.enter("titleQuote")
		value += " \""
		value += s.safe(*n.Title, mdInfo{before: value, after: "\""})
		value += "\""
		subexit()
	}

	exit()

	return value
}

func association(n *unist.Node) string {
	if n.Label != "" || n.Identifier == "" {
		return n.Label
	}

	return decodeString(n.Identifier)
}

// classifyCharacter groups a character like micromark: 1 for whitespace, 2
// for punctuation, 0 for anything else.
func classifyCharacter(ch string) int {
	if ch == "" {
		return 0
	}

	r, _ := utf8.DecodeRuneInString(ch)

	// JavaScript sees the surrogates of an astral character
	if r > 0xFFFF {
		return 0
	}

	if unicode.IsSpace(r) || r == 0xFEFF {
		return 1
	}

	if unicode.IsPunct(r) || unicode.IsSymbol(r) {
		return 2
	}

	return 0
}

func encodeInfo(outside, inside, marker string) (bool, bool) {
	outsideKind := classifyCharacter(outside)
	insideKind := classifyCharacter(inside)

	switch outsideKind {
	case 0:
		switch insideKind {
		case 0:
			if marker == "_" {
				return true, true
			}

			return false, false
		case 1:
			return true, true
		default:
			return false, true
		}
	case 1:
		if insideKind == 1 {
			return true, true
		}

		return false, false
	default:
		if insideKind == 1 {
			return true, false
		}

		return false, false
	}
}

func mdAttention(s *mdState, n *unist.Node, info mdInfo, name, marker string) string {
	exit := s.enter(name)
	before := marker
	between := s.containerPhrasing(n, mdInfo{before: before, after: marker[:1]})

	openInside, openOutside := encodeInfo(lastChar(info.before), firstChar(between), marker[:1])

	if openInside {
		r, size := utf8.DecodeRuneInString(between)
		between = encodeCharacterReference(r) + between[size:]
	}

	closeInside, closeOutside := encodeInfo(firstChar(info.after), lastChar(between), marker[:1])

	if closeInside {
		r, size := utf8.DecodeLastRuneInString(between)
		between = between[:len(between)-size] + encodeCharacterReference(r)
	}

	exit()

	s.attention = &attentionInfo{after: closeOutside, before: openOutside}

	return before + between + marker
}

func mdEmphasis(s *mdState, n, _ *unist.Node, info mdInfo) string {
	return mdAttention(s, n, info, "emphasis", "*")
}

func mdStrong(s *mdState, n, _ *unist.Node, info mdInfo) string {
	return mdAttention(s, n, info, "strong", "**")
}

func formatHeadingAsSetext(n *unist.Node) bool {
	literalWithBreak := false

	unist.Visit(n, nil, func(c *unist.Node, _ int, _ *unist.Node) unist.Step {
		if (c.HasValue() && lineEndings.MatchString(c.Value)) || c.Type == "break" {
			literalWithBreak = true

			return unist.ExitStep
		}

		return unist.Next
	})

	return (n.Depth == 0 || n.Depth < 3) && unist.ToString(n) != "" && literalWithBreak
}

var leadingSpace = regexp.MustCompile(`^[\t ]`)

func mdHeading(s *mdState, n, _ *unist.Node, _ mdInfo) string {
	rank := max(min(6, n.Depth), 1)

	if formatHeadingAsSetext(n) {
		exit := s.enter("headingSetext")
		subexit := s.enter("phrasing")
		value := s.containerPhrasing(n, mdInfo{before: "\n", after: "\n"})
		subexit()
		exit()

		underline := "-"

		if rank == 1 {
			underline = "="
		}

		lastLine := value[max(strings.LastIndex(value, "\r"), strings.LastIndex(value, "\n"))+1:]

		return value + "\n" + strings.Repeat(underline, utf16Len(lastLine))
	}

	sequence := strings.Repeat("#", rank)
	exit := s.enter("headingAtx")
	subexit := s.enter("phrasing")
	value := s.containerPhrasing(n, mdInfo{before: "# ", after: "\n"})

	if leadingSpace.MatchString(value) {
		value = encodeCharacterReference(rune(value[0])) + value[1:]
	}

	if value != "" {
		value = sequence + " " + value
	} else {
		value = sequence
	}

	subexit()
	exit()

	return value
}

func mdImage(s *mdState, n, _ *unist.Node, _ mdInfo) string {
	exit := s.enter("image")
	subexit := s.enter("label")
	value := "!["
	value += s.safe(n.Alt, mdInfo{before: value, after: "]"})
	value += "]("
	subexit()

	hasTitle := n.Title != nil && *n.Title != ""
	value += mdDestination(s, n.URL, hasTitle, value)

	if hasTitle {
		value += mdTitle(s, *n.Title, value)
	}

	value += ")"
	exit()

	return value
}

func mdImageReference(s *mdState, n, _ *unist.Node, _ mdInfo) string {
	exit := s.enter("imageReference")
	subexit := s.enter("label")
	value := "!["
	alt := s.safe(n.Alt, mdInfo{before: value, after: "]"})
	value += alt + "]["
	subexit()

	stack := s.stack
	s.stack = nil
	subexit = s.enter("reference")
	reference := s.safe(association(n), mdInfo{before: value, after: "]"})
	subexit()
	s.stack = stack
	exit()

	switch {
	case n.ReferenceType == "full" || alt == "" || alt != reference:
		value += reference + "]"
	case n.ReferenceType == "shortcut":
		value = value[:len(value)-1]
	default:
		value += "]"
	}

	return value
}

func mdDestination(s *mdState, url string, hasTitle bool, value string) string {
	var out string

	if (url == "" && hasTitle) || controlOrSpace.MatchString(url) {
		subexit := s.enter("destinationLiteral")
		out = "<" + s.safe(url, mdInfo{before: value + "<", after: ">"}) + ">"
		subexit()
	} else {
		subexit := s.enter("destinationRaw")
		after := ")"

		if hasTitle {
			after = " "
		}

		out = s.safe(url, mdInfo{before: value, after: after})
		subexit()
	}

	return out
}

func mdTitle(s *mdState, title, value string) string {
	subexit := s.enter("titleQuote")
	out := " \"" + s.safe(title, mdInfo{before: value + " \"", after: "\""}) + "\""
	subexit()

	return out
}

var autolinkScheme = regexp.MustCompile(`(?i)^[a-z][a-z+.-]+:`)
var autolinkUnsafe = regexp.MustCompile("[\x00- <>\x7F]")

func formatLinkAsAutolink(n *unist.Node) bool {
	raw := unist.ToString(n)

	return n.URL != "" && (n.Title == nil || *n.Title == "") && len(n.Children) == 1 &&
		n.Children[0].Type == "text" && (raw == n.URL || "mailto:"+raw == n.URL) &&
		autolinkScheme.MatchString(n.URL) && !autolinkUnsafe.MatchString(n.URL)
}

func mdLinkPeek(_ *mdState, n, _ *unist.Node) string {
	if formatLinkAsAutolink(n) {
		return "<"
	}

	return "["
}

func mdLink(s *mdState, n, _ *unist.Node, _ mdInfo) string {
	if formatLinkAsAutolink(n) {
		stack := s.stack
		s.stack = nil
		exit := s.enter("autolink")
		value := "<" + s.containerPhrasing(n, mdInfo{before: "<", after: ">"}) + ">"
		exit()
		s.stack = stack

		return value
	}

	exit := s.enter("link")
	subexit := s.enter("label")
	value := "["
	value += s.containerPhrasing(n, mdInfo{before: value, after: "]("})
	value += "]("
	subexit()

	hasTitle := n.Title != nil && *n.Title != ""
	value += mdDestination(s, n.URL, hasTitle, value)

	if hasTitle {
		value += mdTitle(s, *n.Title, value)
	}

	value += ")"
	exit()

	return value
}

func mdInlineCode(n *unist.Node) string {
	value := n.Value
	sequence := "`"

	for {
		re := regexp.MustCompile("(^|[^`])" + sequence + "([^`]|$)")

		if !re.MatchString(value) {
			break
		}

		sequence += "`"
	}

	if strings.ContainsFunc(value, func(r rune) bool { return r != ' ' && r != '\r' && r != '\n' }) &&
		((strings.IndexAny(value[:1], " \r\n") == 0 && strings.ContainsAny(value[len(value)-1:], " \r\n")) ||
			strings.HasPrefix(value, "`") || strings.HasSuffix(value, "`")) {
		value = " " + value + " "
	}

	for _, p := range unsafePatterns {
		if !p.atBreak {
			continue
		}

		for {
			loc := p.compiled.FindStringIndex(value)

			if loc == nil {
				break
			}

			position := loc[0]

			if value[position] == '\n' && position > 0 && value[position-1] == '\r' {
				position--
			}

			value = value[:position] + " " + value[loc[0]+1:]
		}
	}

	return sequence + value + sequence
}

func mdInlineCodeWithTable(s *mdState, n, _ *unist.Node, _ mdInfo) string {
	value := mdInlineCode(n)

	if containsString(s.stack, "tableCell") {
		value = strings.ReplaceAll(value, "|", "\\|")
	}

	return value
}

func mdList(s *mdState, n, parent *unist.Node, info mdInfo) string {
	exit := s.enter("list")
	bulletCurrent := s.bulletCur

	bullet := "*"
	bulletOther := "-"

	if n.Ordered {
		bullet = "."
		bulletOther = ")"
	}

	useDifferent := parent != nil && s.bulletLast != "" && bullet == s.bulletLast

	if !n.Ordered {
		var first *unist.Node

		if len(n.Children) > 0 {
			first = n.Children[0]
		}

		st, is := s.stack, s.indexStack

		if first != nil && len(first.Children) == 0 && len(st) >= 4 &&
			st[len(st)-1] == "list" && st[len(st)-2] == "listItem" && st[len(st)-3] == "list" && st[len(st)-4] == "listItem" &&
			len(is) >= 3 && is[len(is)-1] == 0 && is[len(is)-2] == 0 && is[len(is)-3] == 0 {
			useDifferent = true
		}

		// The rule marker is `*` too
		if first != nil {
			for _, item := range n.Children {
				if len(item.Children) > 0 && item.Children[0].Type == "thematicBreak" {
					useDifferent = true
					break
				}
			}
		}
	}

	if useDifferent {
		bullet = bulletOther
	}

	s.bulletCur = bullet
	value := s.containerFlow(n)
	s.bulletLast = bullet
	s.bulletCur = bulletCurrent
	exit()

	return value
}

var taskPrefix = regexp.MustCompile(`^(?:[*+-]|\d+\.)([\r\n]| {1,3})`)

func mdListItemWithTask(s *mdState, n, parent *unist.Node, info mdInfo) string {
	checkable := n.Checked != nil && len(n.Children) > 0 && n.Children[0].Type == "paragraph"
	value := mdListItem(s, n, parent, info)

	if checkable {
		box := "[ ] "

		if *n.Checked {
			box = "[x] "
		}

		if loc := taskPrefix.FindStringIndex(value); loc != nil {
			value = value[:loc[1]] + box + value[loc[1]:]
		}
	}

	return value
}

func mdListItem(s *mdState, n, parent *unist.Node, _ mdInfo) string {
	bullet := s.bulletCur

	if bullet == "" {
		bullet = "*"
	}

	if parent != nil && parent.Type == "list" && parent.Ordered {
		start := 1

		if parent.Start != nil && *parent.Start > -1 {
			start = *parent.Start
		}

		bullet = fmt.Sprint(start+parent.IndexOf(n)) + bullet
	}

	size := len(bullet) + 1
	exit := s.enter("listItem")
	value := indentLines(s.containerFlow(n), func(line string, index int, blank bool) string {
		if index > 0 {
			if blank {
				return line
			}

			return strings.Repeat(" ", size) + line
		}

		if blank {
			return bullet + line
		}

		return bullet + strings.Repeat(" ", size-len(bullet)) + line
	})
	exit()

	return value
}

func mdDelete(s *mdState, n, _ *unist.Node, _ mdInfo) string {
	exit := s.enter("strikethrough")
	value := "~~" + s.containerPhrasing(n, mdInfo{before: "~~", after: "~"}) + "~~"
	exit()

	return value
}

func mdFootnoteReference(s *mdState, n, _ *unist.Node, _ mdInfo) string {
	exit := s.enter("footnoteReference")
	subexit := s.enter("reference")
	value := "[^" + s.safe(association(n), mdInfo{before: "[^", after: "]"})
	subexit()
	exit()

	return value + "]"
}

func mdFootnoteDefinition(s *mdState, n, _ *unist.Node, _ mdInfo) string {
	exit := s.enter("footnoteDefinition")
	subexit := s.enter("label")
	value := "[^" + s.safe(association(n), mdInfo{before: "[^", after: "]"})
	subexit()
	value += "]:"

	if len(n.Children) > 0 {
		value += " " + indentLines(s.containerFlow(n), func(line string, index int, blank bool) string {
			if index == 0 {
				return line
			}

			if blank {
				return line
			}

			return "    " + line
		})
	}

	exit()

	return value
}

func mdTableCell(s *mdState, n, _ *unist.Node, info mdInfo) string {
	exit := s.enter("tableCell")
	subexit := s.enter("phrasing")
	value := s.containerPhrasing(n, mdInfo{before: "|", after: "|"})
	subexit()
	exit()

	return value
}

func tableRowData(s *mdState, row *unist.Node) []string {
	subexit := s.enter("tableRow")
	cells := make([]string, len(row.Children))

	for i, cell := range row.Children {
		cells[i] = mdTableCell(s, cell, row, mdInfo{})
	}

	subexit()

	return cells
}

func mdTable(s *mdState, n, _ *unist.Node, _ mdInfo) string {
	subexit := s.enter("table")
	matrix := make([][]string, len(n.Children))

	for i, row := range n.Children {
		matrix[i] = tableRowData(s, row)
	}

	subexit()

	return markdownTable(matrix, n.Align)
}

func mdTableRow(s *mdState, n, _ *unist.Node, _ mdInfo) string {
	value := markdownTable([][]string{tableRowData(s, n)}, nil)

	return value[:strings.Index(value, "\n")]
}

func utf16Len(s string) int {
	n := 0

	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}

	return n
}

// markdownTable lays out a table like `markdown-table` with its default
// options.
func markdownTable(table [][]string, align []string) string {
	mostCells := 0
	var longest []int
	sizes := make([][]int, len(table))

	for r, row := range table {
		if len(row) > mostCells {
			mostCells = len(row)
		}

		sizes[r] = make([]int, len(row))

		for col, cell := range row {
			size := utf16Len(cell)
			sizes[r][col] = size

			for len(longest) <= col {
				longest = append(longest, -1)
			}

			if size > longest[col] {
				longest[col] = size
			}
		}
	}

	for len(longest) < mostCells {
		longest = append(longest, -1)
	}

	alignments := make([]byte, mostCells)

	for col := range mostCells {
		if col < len(align) && align[col] != "" {
			alignments[col] = align[col][0] | 0x20
		}
	}

	delimiter := make([]string, mostCells)
	delimiterSizes := make([]int, mostCells)

	for col := range mostCells {
		before, after := "", ""

		switch alignments[col] {
		case 'c':
			before, after = ":", ":"
		case 'l':
			before = ":"
		case 'r':
			after = ":"
		}

		size := max(1, longest[col]-len(before)-len(after))
		delimiter[col] = before + strings.Repeat("-", size) + after
		size = len(before) + size + len(after)

		if size > longest[col] {
			longest[col] = size
		}

		delimiterSizes[col] = size
	}

	cells := append([][]string{table[0]}, append([][]string{delimiter}, table[1:]...)...)
	cellSizes := append([][]int{sizes[0]}, append([][]int{delimiterSizes}, sizes[1:]...)...)

	lines := make([]string, len(cells))

	for r, row := range cells {
		var line strings.Builder

		for col := range mostCells {
			cell := ""
			size := 0

			if col < len(row) {
				cell = row[col]
				size = cellSizes[r][col]
			}

			gap := longest[col] - size
			before, after := "", ""

			switch alignments[col] {
			case 'r':
				before = strings.Repeat(" ", gap)
			case 'c':
				if gap%2 == 1 {
					before = strings.Repeat(" ", gap/2+1)
					after = strings.Repeat(" ", gap/2)
				} else {
					before = strings.Repeat(" ", gap/2)
					after = before
				}
			default:
				after = strings.Repeat(" ", gap)
			}

			if col == 0 {
				line.WriteString("|")
			}

			line.WriteString(" ")
			line.WriteString(before)
			line.WriteString(cell)
			line.WriteString(after)
			line.WriteString(" |")
		}

		lines[r] = line.String()
	}

	return strings.Join(lines, "\n")
}
