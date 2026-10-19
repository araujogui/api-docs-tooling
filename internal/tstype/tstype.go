// Package tstype parses TypeScript type expressions, finding the identifiers
// in them that name types. It stands in for the SWC parse the original
// tooling ran over `type $T = <value>;` statements: a value that is not a
// valid type fails to parse.
package tstype

import (
	"unicode"
	"unicode/utf8"
)

// Identifier is a type name found in a type expression. Offsets count UTF-16
// code units, as JavaScript does.
type Identifier struct {
	Start int
	End   int
	Text  string
}

// keywords are the keyword types recorded as identifiers (`null` is a
// keyword type in SWC).
var keywords = map[string]bool{
	"string": true, "number": true, "boolean": true, "any": true, "unknown": true, "never": true,
	"void": true, "undefined": true, "object": true, "bigint": true, "symbol": true, "null": true,
}

// reserved are the words that cannot name a type.
var reserved = map[string]bool{
	"break": true, "case": true, "catch": true, "class": true, "const": true, "continue": true,
	"debugger": true, "default": true, "delete": true, "do": true, "else": true, "enum": true,
	"export": true, "extends": true, "false": true, "finally": true, "for": true, "function": true,
	"if": true, "import": true, "in": true, "instanceof": true, "new": true, "return": true,
	"super": true, "switch": true, "throw": true, "true": true, "try": true, "var": true,
	"while": true, "with": true, "typeof": true, "this": true, "yield": true, "let": true,
	"static": true, "implements": true, "interface": true, "package": true, "private": true,
	"protected": true, "public": true,
}

type tokenKind int

const (
	tkEOF tokenKind = iota
	tkIdent
	tkNumber
	tkString
	tkTemplate
	tkPunct
)

type token struct {
	kind  tokenKind
	text  string
	start int // byte offsets
	end   int
	// template literal parts: the byte ranges of its `${...}` substitutions
	subs [][2]int
}

type parseError struct{}

// Parse finds the identifiers naming types in a type expression, in source
// order of their recording. ok is false when the value is not a valid type.
func Parse(value string) (ids []Identifier, ok bool) {
	toks, lexOK := lex(value)

	if !lexOK {
		return nil, false
	}

	p := &parser{src: value, toks: toks}

	defer func() {
		if r := recover(); r != nil {
			if _, isParse := r.(parseError); !isParse {
				panic(r)
			}

			ids, ok = nil, false
		}
	}()

	p.parseType()

	if p.peek().kind != tkEOF {
		p.fail()
	}

	units := utf16Units(value)

	for _, r := range p.recorded {
		ids = append(ids, Identifier{Start: units[r[0]], End: units[r[1]], Text: value[r[0]:r[1]]})
	}

	return ids, true
}

// utf16Units maps byte offsets to UTF-16 code unit offsets.
func utf16Units(s string) []int {
	units := make([]int, len(s)+1)
	n := 0

	for i, r := range s {
		size := utf8.RuneLen(r)

		for j := 0; j < size; j++ {
			units[i+j] = n
		}

		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}

	units[len(s)] = n

	return units
}

func isIdentStart(r rune) bool {
	return r == '$' || r == '_' || unicode.IsLetter(r)
}

func isIdentPart(r rune) bool {
	return isIdentStart(r) || unicode.IsDigit(r) || r == 0x200C || r == 0x200D || unicode.In(r, unicode.Mn, unicode.Mc, unicode.Pc)
}

var punctuators = []string{"...", "=>", "?.", "{", "}", "(", ")", "[", "]", "<", ">", ",", ";", ":", "?", ".", "|", "&", "=", "!", "-", "+", "*", "/", "@", "#", "%", "^", "~"}

func lex(src string) ([]token, bool) {
	var toks []token

	i := 0

	for i < len(src) {
		r, size := utf8.DecodeRuneInString(src[i:])

		switch {
		case unicode.IsSpace(r) || r == 0xFEFF:
			i += size
		case r == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case r == '/' && i+1 < len(src) && src[i+1] == '*':
			end := indexFrom(src, "*/", i+2)

			if end < 0 {
				return nil, false
			}

			i = end + 2
		case isIdentStart(r) || r == '\\':
			start := i

			for i < len(src) {
				r, size := utf8.DecodeRuneInString(src[i:])

				if !isIdentPart(r) {
					break
				}

				i += size
			}

			if i == start {
				return nil, false
			}

			toks = append(toks, token{kind: tkIdent, text: src[start:i], start: start, end: i})
		case r >= '0' && r <= '9' || (r == '.' && i+1 < len(src) && src[i+1] >= '0' && src[i+1] <= '9'):
			start := i
			i = lexNumber(src, i)

			if i < 0 {
				return nil, false
			}

			toks = append(toks, token{kind: tkNumber, text: src[start:i], start: start, end: i})
		case r == '\'' || r == '"':
			start := i
			i++

			for i < len(src) && rune(src[i]) != r {
				if src[i] == '\\' {
					i++
				}

				if i < len(src) && src[i] == '\n' {
					return nil, false
				}

				i++
			}

			if i >= len(src) {
				return nil, false
			}

			i++
			toks = append(toks, token{kind: tkString, text: src[start:i], start: start, end: i})
		case r == '`':
			start := i
			var subs [][2]int
			i++

			for i < len(src) && src[i] != '`' {
				if src[i] == '\\' {
					i += 2
					continue
				}

				if src[i] == '$' && i+1 < len(src) && src[i+1] == '{' {
					depth := 0
					subStart := i + 2
					j := subStart

					for ; j < len(src); j++ {
						if src[j] == '{' {
							depth++
						} else if src[j] == '}' {
							if depth == 0 {
								break
							}

							depth--
						}
					}

					if j >= len(src) {
						return nil, false
					}

					subs = append(subs, [2]int{subStart, j})
					i = j + 1

					continue
				}

				i++
			}

			if i >= len(src) {
				return nil, false
			}

			i++
			toks = append(toks, token{kind: tkTemplate, text: src[start:i], start: start, end: i, subs: subs})
		default:
			matched := false

			for _, p := range punctuators {
				if len(src)-i >= len(p) && src[i:i+len(p)] == p {
					toks = append(toks, token{kind: tkPunct, text: p, start: i, end: i + len(p)})
					i += len(p)
					matched = true

					break
				}
			}

			if !matched {
				return nil, false
			}
		}
	}

	return append(toks, token{kind: tkEOF, start: len(src), end: len(src)}), true
}

func indexFrom(s, sub string, from int) int {
	for i := from; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}

	return -1
}

func lexNumber(src string, i int) int {
	isDigit := func(c byte, hex bool) bool {
		return c >= '0' && c <= '9' || c == '_' || hex && (c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F')
	}

	if src[i] == '0' && i+1 < len(src) && (src[i+1]|0x20 == 'x' || src[i+1]|0x20 == 'b' || src[i+1]|0x20 == 'o') {
		i += 2
		start := i

		for i < len(src) && isDigit(src[i], true) {
			i++
		}

		if i == start {
			return -1
		}
	} else {
		for i < len(src) && isDigit(src[i], false) {
			i++
		}

		if i < len(src) && src[i] == '.' {
			i++

			for i < len(src) && isDigit(src[i], false) {
				i++
			}
		}

		if i < len(src) && src[i]|0x20 == 'e' {
			i++

			if i < len(src) && (src[i] == '+' || src[i] == '-') {
				i++
			}

			start := i

			for i < len(src) && isDigit(src[i], false) {
				i++
			}

			if i == start {
				return -1
			}
		}
	}

	if i < len(src) && src[i] == 'n' {
		i++
	}

	// A number can't run into an identifier
	if i < len(src) {
		if r, _ := utf8.DecodeRuneInString(src[i:]); isIdentStart(r) {
			return -1
		}
	}

	return i
}

type parser struct {
	src      string
	toks     []token
	pos      int
	recorded [][2]int
}

func (p *parser) fail() {
	panic(parseError{})
}

func (p *parser) peek() token {
	return p.toks[p.pos]
}

func (p *parser) peekAt(n int) token {
	if p.pos+n >= len(p.toks) {
		return p.toks[len(p.toks)-1]
	}

	return p.toks[p.pos+n]
}

func (p *parser) next() token {
	t := p.toks[p.pos]

	if t.kind != tkEOF {
		p.pos++
	}

	return t
}

func (p *parser) is(text string) bool {
	t := p.peek()

	return (t.kind == tkPunct || t.kind == tkIdent) && t.text == text
}

func (p *parser) eat(text string) bool {
	if p.is(text) {
		p.next()

		return true
	}

	return false
}

func (p *parser) expect(text string) {
	if !p.eat(text) {
		p.fail()
	}
}

// try runs a speculative parse, undoing it when it fails.
func (p *parser) try(fn func()) (ok bool) {
	pos, recorded := p.pos, len(p.recorded)

	defer func() {
		if r := recover(); r != nil {
			if _, isParse := r.(parseError); !isParse {
				panic(r)
			}

			p.pos = pos
			p.recorded = p.recorded[:recorded]
			ok = false
		}
	}()

	fn()

	return true
}

func (p *parser) record(start, end int) {
	p.recorded = append(p.recorded, [2]int{start, end})
}

// parseType parses a type, including conditional and function types.
func (p *parser) parseType() {
	p.parseTypeWith(true)
}

func (p *parser) parseTypeWith(allowConditional bool) {
	if p.isStartOfFunctionType() {
		p.parseFunctionType()

		return
	}

	p.parseUnion()

	if allowConditional && p.is("extends") {
		p.next()
		p.parseTypeWith(false)
		p.expect("?")
		p.parseType()
		p.expect(":")
		p.parseType()
	}
}

func (p *parser) isStartOfFunctionType() bool {
	if p.is("<") {
		return true
	}

	if p.is("new") || (p.is("abstract") && p.peekAt(1).text == "new") {
		return true
	}

	if !p.is("(") {
		return false
	}

	pos, recorded := p.pos, len(p.recorded)
	ok := p.try(func() { p.parseParameters(); p.expect("=>") })
	p.pos, p.recorded = pos, p.recorded[:recorded]

	return ok
}

func (p *parser) parseFunctionType() {
	p.eat("abstract")
	p.eat("new")

	if p.is("<") {
		p.parseTypeParameters()
	}

	p.parseParameters()
	p.expect("=>")
	p.parseReturnType()
}

// parseReturnType parses a return type, which may be a type predicate.
func (p *parser) parseReturnType() {
	if p.is("asserts") && (p.peekAt(1).kind == tkIdent) && p.peekAt(1).text != "is" {
		p.next()

		if p.peek().text == "this" {
			p.next()
		} else {
			p.parseBindingIdentifier()
		}

		if p.eat("is") {
			p.parseType()
		}

		return
	}

	if (p.peek().kind == tkIdent) && p.peekAt(1).text == "is" && p.peekAt(1).kind == tkIdent {
		p.next()
		p.next()
		p.parseType()

		return
	}

	p.parseType()
}

func (p *parser) parseBindingIdentifier() {
	t := p.next()

	if t.kind != tkIdent || reserved[t.text] {
		p.fail()
	}
}

func (p *parser) parseTypeParameters() {
	p.expect("<")

	for {
		p.eat("const")
		p.eat("in")
		p.eat("out")
		p.parseBindingIdentifier()

		if p.eat("extends") {
			p.parseType()
		}

		if p.eat("=") {
			p.parseType()
		}

		if !p.eat(",") || p.is(">") {
			break
		}
	}

	p.expect(">")
}

// parseParameters parses a parenthesized parameter list.
func (p *parser) parseParameters() {
	p.expect("(")

	for !p.is(")") {
		p.eat("...")

		switch {
		case p.is("{"):
			p.parseObjectBindingPattern()
		case p.is("["):
			p.parseArrayBindingPattern()
		case p.is("this"):
			p.next()
		default:
			p.parseBindingIdentifier()
		}

		p.eat("?")

		if p.eat(":") {
			p.parseType()
		}

		if p.eat("=") {
			p.fail()
		}

		if !p.eat(",") {
			break
		}
	}

	p.expect(")")
}

func (p *parser) parseObjectBindingPattern() {
	p.expect("{")

	for !p.is("}") {
		p.eat("...")
		p.parsePropertyName()

		if p.eat(":") {
			if p.is("{") {
				p.parseObjectBindingPattern()
			} else if p.is("[") {
				p.parseArrayBindingPattern()
			} else {
				p.parseBindingIdentifier()
			}
		}

		if !p.eat(",") {
			break
		}
	}

	p.expect("}")
}

func (p *parser) parseArrayBindingPattern() {
	p.expect("[")

	for !p.is("]") {
		if p.eat(",") {
			continue
		}

		p.eat("...")

		if p.is("{") {
			p.parseObjectBindingPattern()
		} else if p.is("[") {
			p.parseArrayBindingPattern()
		} else {
			p.parseBindingIdentifier()
		}

		if !p.eat(",") {
			break
		}
	}

	p.expect("]")
}

func (p *parser) parseUnion() {
	p.eat("|")
	p.parseIntersection()

	for p.eat("|") {
		p.parseIntersection()
	}
}

func (p *parser) parseIntersection() {
	p.eat("&")
	p.parseTypeOperator()

	for p.eat("&") {
		p.parseTypeOperator()
	}
}

func (p *parser) parseTypeOperator() {
	switch {
	case p.is("keyof") && p.startsType(1), p.is("readonly") && p.startsType(1):
		p.next()
		p.parseTypeOperator()
	case p.is("unique") && p.peekAt(1).text == "symbol":
		p.next()
		p.parseTypeOperator()
	case p.is("infer") && p.peekAt(1).kind == tkIdent:
		p.next()
		p.parseBindingIdentifier()

		if p.is("extends") {
			// `infer U extends X` only binds a constraint when no
			// conditional follows
			p.try(func() {
				p.next()
				p.parseTypeWith(false)

				if p.is("?") {
					p.fail()
				}
			})
		}
	default:
		p.parsePostfix()
	}
}

// startsType reports whether the token n ahead can start a type.
func (p *parser) startsType(n int) bool {
	t := p.peekAt(n)

	switch t.kind {
	case tkEOF:
		return false
	case tkPunct:
		switch t.text {
		case "(", "[", "{", "<", "-", "|", "&", "...":
			return true
		}

		return false
	}

	return true
}

func (p *parser) parsePostfix() {
	p.parsePrimary()

	for p.is("[") {
		p.next()

		if p.eat("]") {
			continue
		}

		p.parseType()
		p.expect("]")
	}
}

func (p *parser) parsePrimary() {
	t := p.peek()

	switch t.kind {
	case tkString:
		p.next()

		return
	case tkNumber:
		p.next()

		return
	case tkTemplate:
		p.next()
		p.parseTemplateSubstitutions(t)

		return
	case tkPunct:
		switch t.text {
		case "(":
			p.next()
			p.parseType()
			p.expect(")")

			return
		case "{":
			p.parseTypeLiteralOrMapped()

			return
		case "[":
			p.parseTuple()

			return
		case "-":
			p.next()

			if p.peek().kind != tkNumber {
				p.fail()
			}

			p.next()

			return
		}

		p.fail()
	case tkIdent:
		switch t.text {
		case "true", "false":
			p.next()

			return
		case "this":
			p.next()
			p.record(t.start, t.end)

			return
		case "typeof":
			p.next()
			p.parseTypeQuery()

			return
		case "import":
			p.parseImportType()

			return
		case "void":
			p.next()
			p.record(t.start, t.end)

			return
		}

		if keywords[t.text] && !p.isQualified(1) {
			p.next()
			p.record(t.start, t.end)

			return
		}

		if reserved[t.text] {
			p.fail()
		}

		start, end := p.parseEntityName()
		p.record(start, end)

		if p.is("<") {
			p.parseTypeArguments()
		}

		return
	}

	p.fail()
}

// isQualified reports whether the token n ahead is a `.` continuing a name.
func (p *parser) isQualified(n int) bool {
	return p.peekAt(n).kind == tkPunct && p.peekAt(n).text == "."
}

// parseEntityName parses a (possibly dotted) name, returning its span.
func (p *parser) parseEntityName() (int, int) {
	first := p.next()

	if first.kind != tkIdent {
		p.fail()
	}

	end := first.end

	for p.is(".") {
		p.next()
		part := p.next()

		if part.kind != tkIdent {
			p.fail()
		}

		end = part.end
	}

	return first.start, end
}

func (p *parser) parseTypeQuery() {
	if p.is("import") {
		p.parseImportType()

		return
	}

	t := p.peek()

	if t.kind != tkIdent {
		p.fail()
	}

	start, end := p.parseEntityName()
	p.record(start, end)

	if p.is("<") {
		p.parseTypeArguments()
	}
}

func (p *parser) parseImportType() {
	p.expect("import")
	p.expect("(")

	if p.peek().kind != tkString {
		p.fail()
	}

	p.next()
	p.expect(")")

	if p.eat(".") {
		start, end := p.parseEntityName()
		p.record(start, end)
	}

	if p.is("<") {
		p.parseTypeArguments()
	}
}

func (p *parser) parseTypeArguments() {
	p.expect("<")

	for {
		p.parseType()

		if !p.eat(",") {
			break
		}
	}

	p.expect(">")
}

func (p *parser) parseTemplateSubstitutions(t token) {
	for _, sub := range t.subs {
		ids, ok := Parse(p.src[sub[0]:sub[1]])

		if !ok {
			p.fail()
		}

		// Shift the nested identifiers back into this value's offsets
		for _, id := range ids {
			startByte := sub[0] + byteOffset(p.src[sub[0]:sub[1]], id.Start)
			p.record(startByte, startByte+len(id.Text))
		}
	}
}

// byteOffset turns a UTF-16 offset into a byte offset.
func byteOffset(s string, units int) int {
	n := 0

	for i, r := range s {
		if n >= units {
			return i
		}

		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}

	return len(s)
}

func (p *parser) parseTuple() {
	p.expect("[")

	for !p.is("]") {
		rest := p.eat("...")

		// A named member: `name: T`, `name?: T`
		if p.peek().kind == tkIdent && (p.peekAt(1).text == ":" || p.peekAt(1).text == "?" && p.peekAt(2).text == ":") {
			p.next()
			p.eat("?")
			p.expect(":")
			p.parseType()
		} else {
			p.parseType()

			if !rest {
				p.eat("?")
			}
		}

		if !p.eat(",") {
			break
		}
	}

	p.expect("]")
}

func (p *parser) parseTypeLiteralOrMapped() {
	if p.isStartOfMappedType() {
		p.parseMappedType()

		return
	}

	p.expect("{")

	for !p.is("}") {
		p.parseTypeMember()

		if !p.eat(";") && !p.eat(",") {
			break
		}
	}

	p.expect("}")
}

func (p *parser) isStartOfMappedType() bool {
	n := 1

	if p.peekAt(n).text == "+" || p.peekAt(n).text == "-" {
		n++
	}

	if p.peekAt(n).text == "readonly" {
		n++
	}

	return p.peekAt(n).text == "[" && p.peekAt(n+1).kind == tkIdent && p.peekAt(n+2).text == "in"
}

func (p *parser) parseMappedType() {
	p.expect("{")

	if !p.eat("+") {
		p.eat("-")
	}

	p.eat("readonly")
	p.expect("[")
	p.parseBindingIdentifier()
	p.expect("in")
	p.parseType()

	if p.eat("as") {
		p.parseType()
	}

	p.expect("]")

	if p.eat("+") || p.eat("-") {
		p.expect("?")
	} else {
		p.eat("?")
	}

	if p.eat(":") {
		p.parseType()
	}

	p.eat(";")
	p.eat(",")
	p.expect("}")
}

// parsePropertyName parses a member's key: an identifier, string, number, or
// computed expression (whose identifiers are values, not types).
func (p *parser) parsePropertyName() {
	t := p.peek()

	switch t.kind {
	case tkIdent, tkString, tkNumber:
		p.next()
	case tkPunct:
		if t.text == "[" {
			p.next()
			p.skipExpression("]")
			p.expect("]")

			return
		}

		if t.text == "#" {
			p.next()
			p.parseBindingIdentifier()

			return
		}

		p.fail()
	default:
		p.fail()
	}
}

// skipExpression skips a simple expression (a computed key), up to a
// closing token.
func (p *parser) skipExpression(closer string) {
	depth := 0

	for {
		t := p.peek()

		if t.kind == tkEOF {
			p.fail()
		}

		if t.kind == tkPunct {
			switch t.text {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				if depth == 0 {
					if t.text != closer {
						p.fail()
					}

					return
				}

				depth--
			}
		}

		p.next()
	}
}

func (p *parser) parseTypeMember() {
	// Call and construct signatures
	if p.is("(") || p.is("<") {
		p.parseSignatureRest()

		return
	}

	if p.is("new") && (p.peekAt(1).text == "(" || p.peekAt(1).text == "<") {
		p.next()
		p.parseSignatureRest()

		return
	}

	// Modifiers, when something follows them that makes them modifiers
	for (p.is("readonly") || p.is("get") || p.is("set")) && p.isPropertyStart(1) {
		p.next()
	}

	// Index signatures: `[key: string]: T`
	if p.is("[") && p.peekAt(1).kind == tkIdent && p.peekAt(2).text == ":" {
		p.next()
		p.parseBindingIdentifier()
		p.expect(":")
		p.parseType()
		p.expect("]")
		p.expect(":")
		p.parseType()

		return
	}

	p.parsePropertyName()
	p.eat("?")

	if p.is("(") || p.is("<") {
		p.parseSignatureRest()

		return
	}

	if p.eat(":") {
		p.parseType()
	}
}

func (p *parser) isPropertyStart(n int) bool {
	t := p.peekAt(n)

	return t.kind == tkIdent || t.kind == tkString || t.kind == tkNumber || t.text == "[" || t.text == "#"
}

func (p *parser) parseSignatureRest() {
	if p.is("<") {
		p.parseTypeParameters()
	}

	p.parseParameters()

	if p.eat(":") {
		p.parseReturnType()
	}
}
