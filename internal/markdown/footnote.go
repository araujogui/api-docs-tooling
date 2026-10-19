package markdown

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// goldmark's footnote extension moves every definition to the end of the
// document; mdast keeps them where they are written, so these parsers do the
// same.

var (
	kindFootnoteDefinition = ast.NewNodeKind("FootnoteDefinition")
	kindFootnoteReference  = ast.NewNodeKind("FootnoteReference")
	footnoteLabelsKey      = parser.NewContextKey()
)

type footnoteDefinitionNode struct {
	ast.BaseBlock
	label string
}

func (n *footnoteDefinitionNode) Kind() ast.NodeKind { return kindFootnoteDefinition }

func (n *footnoteDefinitionNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Label": n.label}, nil)
}

type footnoteReferenceNode struct {
	ast.BaseInline
	label string
}

func (n *footnoteReferenceNode) Kind() ast.NodeKind { return kindFootnoteReference }

func (n *footnoteReferenceNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Label": n.label}, nil)
}

func footnoteLabels(pc parser.Context) map[string]bool {
	if labels, ok := pc.Get(footnoteLabelsKey).(map[string]bool); ok {
		return labels
	}

	labels := map[string]bool{}
	pc.Set(footnoteLabelsKey, labels)

	return labels
}

type footnoteBlockParser struct{}

func (footnoteBlockParser) Trigger() []byte { return []byte{'['} }

func (footnoteBlockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, segment := reader.PeekLine()
	pos := pc.BlockOffset()

	if pos < 0 || line[pos] != '[' {
		return nil, parser.NoChildren
	}

	pos++

	if pos > len(line)-1 || line[pos] != '^' {
		return nil, parser.NoChildren
	}

	open := pos + 1
	closure := util.FindClosure(line[pos+1:], '[', ']', false, false) //nolint:staticcheck

	if closure < 0 {
		return nil, parser.NoChildren
	}

	closes := pos + 1 + closure
	next := closes + 1

	if next >= len(line) || line[next] != ':' {
		return nil, parser.NoChildren
	}

	padding := segment.Padding
	label := reader.Value(text.NewSegment(segment.Start+open-padding, segment.Start+closes-padding))

	if util.IsBlank(label) {
		return nil, parser.NoChildren
	}

	node := &footnoteDefinitionNode{label: string(label)}
	footnoteLabels(pc)[normalizeIdentifier(string(label))] = true

	pos = next + 1 - padding

	if pos >= len(line) {
		reader.Advance(pos)
		return node, parser.NoChildren
	}

	reader.AdvanceAndSetPadding(pos, padding)

	return node, parser.HasChildren
}

func (footnoteBlockParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	line, _ := reader.PeekLine()

	if util.IsBlank(line) {
		return parser.Continue | parser.HasChildren
	}

	childpos, padding := util.IndentPosition(line, reader.LineOffset(), 4)

	if childpos < 0 {
		return parser.Close
	}

	reader.AdvanceAndSetPadding(childpos, padding)

	return parser.Continue | parser.HasChildren
}

func (footnoteBlockParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {}

func (footnoteBlockParser) CanInterruptParagraph() bool { return true }

func (footnoteBlockParser) CanAcceptIndentedLine() bool { return false }

type footnoteParser struct{}

func (footnoteParser) Trigger() []byte { return []byte{'['} }

func (footnoteParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, segment := block.PeekLine()

	if len(line) < 3 || line[1] != '^' {
		return nil
	}

	closure := util.FindClosure(line[2:], '[', ']', false, false) //nolint:staticcheck

	if closure < 0 {
		return nil
	}

	label := string(block.Value(text.NewSegment(segment.Start+2, segment.Start+2+closure)))

	if !footnoteLabels(pc)[normalizeIdentifier(label)] {
		return nil
	}

	block.Advance(closure + 3)

	return &footnoteReferenceNode{label: label}
}
