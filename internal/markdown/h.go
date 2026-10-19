package markdown

import (
	"fmt"
	"strings"

	"github.com/nodejs/doc-kit/internal/omap"
	"github.com/nodejs/doc-kit/internal/unist"
)

// H creates a hast element like `hastscript`: the selector gives its tag
// name, `#id` and `.class`es; children may be nodes, strings, numbers, slices
// of them, or roots (whose children are added). A leading *omap.Map gives
// its properties.
func H(selector string, children ...any) *unist.Node {
	el := &unist.Node{Type: "element", Properties: omap.New(), Children: []*unist.Node{}}

	tag := selector
	var classes []string
	var id string

	if i := strings.IndexAny(selector, ".#"); i >= 0 {
		tag = selector[:i]
		rest := selector[i:]

		for rest != "" {
			kind := rest[0]
			rest = rest[1:]
			end := strings.IndexAny(rest, ".#")

			if end < 0 {
				end = len(rest)
			}

			value := rest[:end]
			rest = rest[end:]

			if kind == '#' {
				id = value
			} else {
				classes = append(classes, value)
			}
		}
	}

	if tag == "" {
		tag = "div"
	}

	el.TagName = tag

	if len(classes) > 0 {
		el.Properties.Set("className", classes)
	}

	if id != "" {
		el.Properties.Set("id", id)
	}

	if len(children) > 0 {
		if props, ok := children[0].(*omap.Map); ok {
			for _, k := range props.Keys() {
				v, _ := props.Get(k)

				if k == "class" || k == "className" {
					if s, ok := v.(string); ok {
						v = strings.Fields(s)
					}

					k = "className"
				}

				el.Properties.Set(k, v)
			}

			children = children[1:]
		}
	}

	for _, child := range children {
		addChild(el, child)
	}

	return el
}

func addChild(el *unist.Node, child any) {
	switch c := child.(type) {
	case nil:
	case string:
		el.Children = append(el.Children, unist.Text(c))
	case int:
		el.Children = append(el.Children, unist.Text(fmt.Sprint(c)))
	case float64:
		el.Children = append(el.Children, unist.Text(fmt.Sprint(c)))
	case *unist.Node:
		if c == nil {
			return
		}

		if c.Type == "root" {
			for _, n := range c.Children {
				addChild(el, n)
			}

			return
		}

		el.Children = append(el.Children, c)
	case []*unist.Node:
		for _, n := range c {
			addChild(el, n)
		}
	case []any:
		for _, n := range c {
			addChild(el, n)
		}
	default:
		panic(fmt.Sprintf("hastscript: unexpected child %T", child))
	}
}

// Element creates a hast element with properties (in order) and children.
func Element(tag string, props *omap.Map, children ...*unist.Node) *unist.Node {
	if props == nil {
		props = omap.New()
	}

	if children == nil {
		children = []*unist.Node{}
	}

	return &unist.Node{Type: "element", TagName: tag, Properties: props, Children: children}
}
