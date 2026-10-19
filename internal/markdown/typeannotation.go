package markdown

import (
	"github.com/nodejs/doc-kit/internal/jsre"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// KindTypeAnnotation is the goldmark node kind of a `{...}` type annotation.
var KindTypeAnnotation = ast.NewNodeKind("TypeAnnotation")

// typeAnnotationNode is a balanced `{...}` span in text, holding the
// canonical type it annotates.
type typeAnnotationNode struct {
	ast.BaseInline
	value string
	start int
	stop  int
}

func (n *typeAnnotationNode) Kind() ast.NodeKind { return KindTypeAnnotation }

func (n *typeAnnotationNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Value": n.value}, nil)
}

var (
	// A backslash escape before ASCII punctuation (CommonMark's escapable
	// set). The raw source is captured, so legacy escapes like
	// `{string\[]}` and the mandatory `\|` inside GFM table cells are decoded
	// here.
	characterEscape = regexp.MustCompile("\\\\([!-/:-@\\[-`{-~])")
	whitespaceRun   = jsre.MustCompile(`\s+`)
)

// isTypeStart reports whether a type expression may begin with a byte: an
// identifier or numeric literal (alphanumeric or non-ASCII) or one of the
// punctuation starters: `_` and `$` (identifiers), the openers of
// parenthesized, tuple, object, generic and string-literal types, the leading
// `-` of a negative numeric literal, the leading `&`/`|` of an intersection or
// union, and `\` (a legacy escape). Prose that happens to sit between two
// braces (as in url.md's `U+007B ({), and U+007D (}).`) starts with
// punctuation no type can, so this keeps such spans out.
func isTypeStart(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
		c >= 0x80 || strings.IndexByte("_$([{<'\"`-&|\\", c) >= 0
}

// NormalizeTypeAnnotation turns the raw text between the braces into the
// canonical, single-line, unescaped type with union separators spaced.
func NormalizeTypeAnnotation(raw string) string {
	value := strings.TrimSpace(whitespaceRun.ReplaceAllString(raw, " "))
	value = characterEscape.ReplaceAllString(value, "$1")

	return spaceUnions(value)
}

// spaceUnions rewrites each single `|` union separator, with any surrounding
// whitespace, into ` | `. A `||` (logical OR in default-value prose) is left
// alone.
func spaceUnions(value string) string {
	if !strings.Contains(value, "|") {
		return value
	}

	var b strings.Builder

	for i := 0; i < len(value); i++ {
		c := value[i]

		if c == '|' && (i == 0 || value[i-1] != '|') && (i+1 >= len(value) || value[i+1] != '|') {
			// Drop the whitespace already written before the separator
			trimmed := strings.TrimRight(b.String(), " \t\n\r\f\v")
			b.Reset()
			b.WriteString(trimmed)
			b.WriteString(" | ")

			for i+1 < len(value) && isSpaceByte(value[i+1]) {
				i++
			}

			continue
		}

		b.WriteByte(c)
	}

	return b.String()
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

// typeAnnotationParser recognizes balanced `{...}` spans in text as type
// annotations. An unbalanced or empty span, or one whose content cannot start
// a type, is left as literal text, as is a `{` right after a `$` (prose about
// template literals).
type typeAnnotationParser struct{}

func (typeAnnotationParser) Trigger() []byte { return []byte{'{'} }

func (typeAnnotationParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	if block.PrecendingCharacter() == '$' {
		return nil
	}

	line, segment := block.Position()

	// The rest of the block, across lines
	var rest []byte

	for {
		l, _ := block.PeekLine()
		if l == nil {
			break
		}

		rest = append(rest, l...)
		block.AdvanceLine()
	}

	block.SetPosition(line, segment)

	depth := 0
	hasContent := false
	end := -1

	for i := 1; i < len(rest); i++ {
		c := rest[i]

		if c == '}' && depth == 0 {
			if hasContent {
				end = i
			}

			break
		}

		if c == '\n' || c == '\r' {
			continue
		}

		if !hasContent && c != ' ' && c != '\t' {
			if !isTypeStart(c) {
				return nil
			}

			hasContent = true
		}

		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
		}
	}

	if end < 0 {
		return nil
	}

	start := segment.Start
	block.Advance(end + 1)
	_, after := block.Position()

	return &typeAnnotationNode{
		value: NormalizeTypeAnnotation(string(rest[1:end])),
		start: start,
		stop:  after.Start,
	}
}

// typeAnnotationOption registers the parser ahead of the others, as micromark
// tries the construct before treating `{` as text.
var typeAnnotationOption = parser.WithInlineParsers(util.Prioritized(typeAnnotationParser{}, 50))
