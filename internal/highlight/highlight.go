// Package highlight highlights code with chroma, producing the dual-theme
// (light and dark) markup Shiki produces: a `<pre class="shiki">` whose
// tokens are colored for the light theme, with the dark theme's colors in
// `--shiki-dark` custom properties.
package highlight

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/nodejs/doc-kit/internal/omap"
	"github.com/nodejs/doc-kit/internal/unist"
)

// Theme names, as Shiki knows the themes.
const (
	LightTheme = "catppuccin-latte"
	DarkTheme  = "catppuccin-mocha"
)

var (
	light = styles.Get(LightTheme)
	dark  = styles.Get(DarkTheme)
)

// aliases maps the language names the docs use to chroma's lexers, where
// chroma doesn't know them.
var aliases = map[string]string{
	"mjs":          "javascript",
	"cjs":          "javascript",
	"shellscript":  "bash",
	"shell":        "bash",
	"zsh":          "bash",
	"console":      "bash session",
	"shellsession": "bash session",
	"text":         "plaintext",
	"txt":          "plaintext",
	"plain":        "plaintext",
}

// lexer resolves a language, falling back to plain text for unknown ones.
func lexer(lang string) chroma.Lexer {
	lang = strings.ToLower(lang)

	if alias, ok := aliases[lang]; ok {
		lang = alias
	}

	if l := lexers.Get(lang); l != nil {
		return chroma.Coalesce(l)
	}

	return lexers.Fallback
}

func hex(c chroma.Colour) string {
	return strings.ToUpper(c.String())
}

// colors returns the style of a token in both themes.
func tokenStyle(t chroma.TokenType) string {
	l, d := light.Get(t), dark.Get(t)

	var b strings.Builder

	b.WriteString("color:" + hex(l.Colour))

	if l.Italic == chroma.Yes {
		b.WriteString(";--shiki-light-font-style:italic")
	}

	if l.Bold == chroma.Yes {
		b.WriteString(";--shiki-light-font-weight:bold")
	}

	b.WriteString(";--shiki-dark:" + hex(d.Colour))

	if d.Italic == chroma.Yes {
		b.WriteString(";--shiki-dark-font-style:italic")
	}

	if d.Bold == chroma.Yes {
		b.WriteString(";--shiki-dark-font-weight:bold")
	}

	return b.String()
}

// PreStyle is the style of a highlighted block: the themes' backgrounds and
// foregrounds.
func PreStyle() string {
	lb, db := light.Get(chroma.Background), dark.Get(chroma.Background)

	return "background-color:" + lb.Background.String() + ";--shiki-dark-bg:" + db.Background.String() +
		";color:" + lb.Colour.String() + ";--shiki-dark:" + db.Colour.String()
}

type span struct {
	style string
	text  string
}

// Highlight highlights code into a `<pre>` element (with a `<code>` of line
// spans).
func Highlight(code, lang string) *unist.Node {
	var lines [][]span

	current := []span{}
	pendingSpace := ""

	push := func(style, text string) {
		text = pendingSpace + text
		pendingSpace = ""

		if n := len(current); n > 0 && current[n-1].style == style {
			current[n-1].text += text
		} else {
			current = append(current, span{style, text})
		}
	}

	iterator, err := lexer(lang).Tokenise(nil, code)

	if err != nil {
		iterator, _ = lexers.Fallback.Tokenise(nil, code)
	}

	for _, tok := range iterator.Tokens() {
		parts := strings.Split(tok.Value, "\n")

		for i, part := range parts {
			if i > 0 {
				if pendingSpace != "" {
					push(tokenStyle(chroma.Text), "")
				}

				lines = append(lines, current)
				current = []span{}
			}

			if part == "" {
				continue
			}

			// Whitespace joins the token after it, as in Shiki's output
			if strings.TrimSpace(part) == "" && tok.Type != chroma.LiteralString {
				pendingSpace += part
				continue
			}

			push(tokenStyle(tok.Type), part)
		}
	}

	if pendingSpace != "" {
		push(tokenStyle(chroma.Text), "")
	}

	lines = append(lines, current)

	codeEl := &unist.Node{Type: "element", TagName: "code", Properties: omap.New(), Children: []*unist.Node{}}

	for i, line := range lines {
		if i > 0 {
			codeEl.Children = append(codeEl.Children, unist.Text("\n"))
		}

		lineEl := &unist.Node{Type: "element", TagName: "span", Properties: omap.New("class", "line"), Children: []*unist.Node{}}

		for _, s := range line {
			lineEl.Children = append(lineEl.Children, &unist.Node{
				Type:       "element",
				TagName:    "span",
				Properties: omap.New("style", s.style),
				Children:   []*unist.Node{unist.Text(s.text)},
			})
		}

		codeEl.Children = append(codeEl.Children, lineEl)
	}

	return &unist.Node{
		Type:    "element",
		TagName: "pre",
		Properties: omap.New(
			"class", "shiki shiki-themes "+LightTheme+" "+DarkTheme,
			"style", PreStyle(),
			"tabindex", "0",
		),
		Children: []*unist.Node{codeEl},
	}
}
