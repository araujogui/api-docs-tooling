package markdown

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/nodejs/doc-kit/internal/unist"
)

// HTMLOptions configure the serialization of hast, like `rehype-stringify`'s.
type HTMLOptions struct {
	// AllowDangerousHTML writes raw nodes as they are.
	AllowDangerousHTML bool
}

var voidElements = map[string]bool{
	"area": true, "base": true, "basefont": true, "bgsound": true, "br": true, "col": true,
	"command": true, "embed": true, "frame": true, "hr": true, "image": true, "img": true,
	"input": true, "keygen": true, "link": true, "meta": true, "param": true, "source": true,
	"track": true, "wbr": true,
}

// booleanProperties are the HTML attributes that take no value.
var booleanProperties = map[string]bool{
	"allowFullScreen": true, "async": true, "autoFocus": true, "autoPlay": true, "checked": true,
	"controls": true, "default": true, "defer": true, "disabled": true, "formNoValidate": true,
	"hidden": true, "inert": true, "isMap": true, "itemScope": true, "loop": true, "multiple": true,
	"muted": true, "noModule": true, "noValidate": true, "open": true, "playsInline": true,
	"readOnly": true, "required": true, "reversed": true, "selected": true,
}

// attributeNames maps hast property names to the attributes they stand for,
// where those differ by more than case.
var attributeNames = map[string]string{
	"className":     "class",
	"htmlFor":       "for",
	"httpEquiv":     "http-equiv",
	"acceptCharset": "accept-charset",
}

// spaceSeparated are the properties whose list values join with spaces.
var spaceSeparated = map[string]bool{
	"className": true, "class": true, "rel": true, "ariaDescribedBy": true, "ariaLabelledBy": true,
	"ariaControls": true, "ariaOwns": true, "accessKey": true, "headers": true, "itemProp": true,
	"itemRef": true, "itemType": true, "ping": true, "sandbox": true, "for": true, "htmlFor": true,
}

// attributeName returns the attribute a hast property stands for.
func attributeName(key string) string {
	if name, ok := attributeNames[key]; ok {
		return name
	}

	// `aria*` properties are lowercased, `data*` ones dash-cased
	if strings.HasPrefix(key, "aria") && len(key) > 4 && unicode.IsUpper(rune(key[4])) {
		return "aria-" + strings.ToLower(key[4:])
	}

	if strings.HasPrefix(key, "data") && len(key) > 4 && unicode.IsUpper(rune(key[4])) {
		var b strings.Builder

		for _, r := range key {
			if unicode.IsUpper(r) {
				b.WriteByte('-')
				b.WriteRune(unicode.ToLower(r))
			} else {
				b.WriteRune(r)
			}
		}

		return b.String()
	}

	return strings.ToLower(key)
}

var (
	textEscaper      = strings.NewReplacer("&", "&#x26;", "<", "&#x3C;")
	attributeEscaper = strings.NewReplacer("&", "&#x26;", "\"", "&#x22;", "'", "&#x27;", "`", "&#x60;", "\x00", "&#x0;")
)

// ToHTML serializes a hast tree, like `hast-util-to-html`.
func ToHTML(tree *unist.Node, opts HTMLOptions) string {
	var b strings.Builder

	writeHTML(&b, tree, nil, opts)

	return b.String()
}

// AllToHTML serializes hast nodes.
func AllToHTML(nodes []*unist.Node, opts HTMLOptions) string {
	var b strings.Builder

	for _, n := range nodes {
		writeHTML(&b, n, nil, opts)
	}

	return b.String()
}

func writeHTML(b *strings.Builder, n, parent *unist.Node, opts HTMLOptions) {
	switch n.Type {
	case "root":
		for _, c := range n.Children {
			writeHTML(b, c, n, opts)
		}
	case "text":
		if parent != nil && parent.Type == "element" && (parent.TagName == "script" || parent.TagName == "style") {
			b.WriteString(n.Value)
		} else {
			b.WriteString(textEscaper.Replace(n.Value))
		}
	case "raw":
		if opts.AllowDangerousHTML {
			b.WriteString(n.Value)
		} else {
			b.WriteString(textEscaper.Replace(n.Value))
		}
	case "comment":
		b.WriteString("<!--")
		b.WriteString(n.Value)
		b.WriteString("-->")
	case "doctype":
		b.WriteString("<!doctype html>")
	case "element":
		writeElement(b, n, opts)
	}
}

func writeElement(b *strings.Builder, n *unist.Node, opts HTMLOptions) {
	attrs := serializeAttributes(n)

	b.WriteByte('<')
	b.WriteString(n.TagName)

	if attrs != "" {
		b.WriteByte(' ')
		b.WriteString(attrs)
	}

	b.WriteByte('>')

	var content strings.Builder

	for _, c := range n.Children {
		writeHTML(&content, c, n, opts)
	}

	b.WriteString(content.String())

	if !voidElements[strings.ToLower(n.TagName)] || content.Len() > 0 {
		b.WriteString("</")
		b.WriteString(n.TagName)
		b.WriteByte('>')
	}
}

func serializeAttributes(n *unist.Node) string {
	var values []string

	for _, key := range n.Properties.Keys() {
		value := n.Properties.Value(key)

		if value == nil {
			continue
		}

		if s := serializeAttribute(key, value); s != "" {
			values = append(values, s)
		}
	}

	return strings.Join(values, " ")
}

func serializeAttribute(key string, value any) string {
	if booleanProperties[key] {
		if s, ok := value.(string); !ok || s == "" || s == attributeName(key) {
			value = truthy(value)
		}
	}

	if value == false {
		return ""
	}

	name := attributeName(key)

	if value == true {
		return name
	}

	var s string

	switch v := value.(type) {
	case []string:
		s = strings.Join(v, " ")
	case string:
		s = v
	default:
		s = fmt.Sprint(v)
	}

	return name + "=\"" + attributeEscaper.Replace(s) + "\""
}

func truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x != ""
	case nil:
		return false
	case int:
		return x != 0
	default:
		return true
	}
}
