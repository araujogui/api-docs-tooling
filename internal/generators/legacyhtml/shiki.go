package legacyhtml

import (
	"regexp"
	"strings"

	"github.com/nodejs/doc-kit/internal/highlight"
	"github.com/nodejs/doc-kit/internal/omap"
	"github.com/nodejs/doc-kit/internal/unist"
)

// languagePrefix prefixes a code block's language in its class names.
const languagePrefix = "language-"

// toolbar creates a code block's toolbar: its language, and a copy button.
func toolbar(language string) *unist.Node {
	return h("div", omap.New("class", "code-toolbar"),
		h("span", omap.New("class", "code-language"), language),
		h("button", omap.New("class", "copy-button"), "copy"),
	)
}

func isCodeBlock(n *unist.Node) bool {
	return n != nil && n.TagName == "pre" && len(n.Children) > 0 && n.Children[0].TagName == "code"
}

var languageClass = regexp.MustCompile(`language-(.*)`)

func classString(n *unist.Node) string {
	switch v := n.Properties.Value("class").(type) {
	case string:
		return v
	case []string:
		return strings.Join(v, " ")
	}

	return ""
}

// highlightCode highlights the code blocks of a hast tree, adding their
// toolbars, and turns adjacent CommonJS and ES module blocks into one block
// switching between them.
func highlightCode(tree *unist.Node) {
	unist.Visit(tree, unist.IsType("element"), func(n *unist.Node, index int, parent *unist.Node) unist.Step {
		if parent == nil || index < 0 || n.TagName != "pre" || len(n.Children) == 0 {
			return unist.Next
		}

		code := n.Children[0]

		if code.Properties == nil || code.Type != "element" || code.TagName != "code" {
			return unist.Next
		}

		classes, _ := code.Properties.Value("className").([]string)

		if len(classes) == 0 {
			return unist.Next
		}

		var language string

		for _, c := range classes {
			if strings.HasPrefix(c, languagePrefix) {
				language = c
				break
			}
		}

		if language == "" {
			return unist.Next
		}

		id := language[len(languagePrefix):]
		value := ""

		if len(code.Children) > 0 {
			value = code.Children[0].Value
		}

		pre := highlight.Highlight(value, id)
		pre.Properties.Set("class", classString(pre)+" "+language)
		pre.Children = append(pre.Children, toolbar(id))

		parent.Splice(index, 1, pre)

		return unist.Next
	})

	unist.Visit(tree, unist.IsType("element"), func(_ *unist.Node, index int, parent *unist.Node) unist.Step {
		if parent == nil {
			return unist.Next
		}

		var codes []*unist.Node

		current := index

		for current < len(parent.Children) && isCodeBlock(parent.Children[current]) {
			pre := parent.Children[current]
			code := pre.Children[0]

			if className := classString(pre); className != "" {
				language := "text"

				if m := languageClass.FindStringSubmatch(className); m != nil {
					language = m[1]
				}

				// Only CommonJS and ES module blocks switch
				if language == "cjs" || language == "mjs" {
					code.Properties.Set("class", className+" "+language)
					code.Properties.Set("style", pre.Properties.Value("style"))
					code.Properties.Set("language", language)
					codes = append(codes, code)
				}
			}

			current++

			if len(codes) == 2 && codes[0].Properties.Value("language") != codes[1].Properties.Value("language") {
				children := []any{
					h("input", omap.New("class", "js-flavor-toggle", "type", "checkbox", "checked", codes[0].Properties.Value("language") == "cjs")),
				}

				for _, c := range codes {
					children = append(children, c)
				}

				children = append(children, toolbar("javascript"))

				switchable := h("pre", append([]any{omap.New("style", codes[0].Properties.Value("style"), "class", "shiki")}, children...)...)

				parent.Splice(index, current-index, switchable)

				return unist.SkipStep
			}
		}

		return unist.Next
	})
}
