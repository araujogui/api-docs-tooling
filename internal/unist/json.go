package unist

import "github.com/nodejs/doc-kit/internal/omap"

// ToJSON turns a node into the object mdast would serialize it as.
func ToJSON(n *Node) *omap.Map {
	m := omap.New("type", n.Type)

	switch n.Type {
	case "heading":
		m.Set("depth", n.Depth)
	case "list":
		m.Set("ordered", n.Ordered)

		if n.Start != nil {
			m.Set("start", *n.Start)
		} else {
			m.Set("start", nil)
		}

		m.Set("spread", n.Spread)
	case "listItem":
		m.Set("spread", n.Spread)

		if n.Checked != nil {
			m.Set("checked", *n.Checked)
		} else {
			m.Set("checked", nil)
		}
	case "code":
		m.Set("lang", strOrNil(n.Lang))
		m.Set("meta", strOrNil(n.Meta))
	case "link", "image", "definition":
		m.Set("title", strOrNil(n.Title))
		m.Set("url", n.URL)

		if n.Type == "image" {
			m.Set("alt", n.Alt)
		}
	case "table":
		align := make([]any, len(n.Align))

		for i, a := range n.Align {
			if a != "" {
				align[i] = a
			}
		}

		m.Set("align", align)
	case "element":
		m.Set("tagName", n.TagName)
		m.Set("properties", n.Properties)
	}

	if n.Identifier != "" {
		m.Set("identifier", n.Identifier)
		m.Set("label", n.Label)
	}

	if n.ReferenceType != "" {
		m.Set("referenceType", n.ReferenceType)
	}

	if n.HasValue() {
		m.Set("value", n.Value)
	}

	if n.Children != nil {
		children := make([]any, len(n.Children))

		for i, c := range n.Children {
			children[i] = ToJSON(c)
		}

		m.Set("children", children)
	}

	if n.Position != nil {
		m.Set("position", n.Position)
	}

	if d := n.Data; d != nil {
		m.Set("data", dataJSON(n, d))
	}

	return m
}

func strOrNil(s *string) any {
	if s == nil {
		return nil
	}

	return *s
}

func dataJSON(n *Node, d *Data) *omap.Map {
	switch {
	case n.Type == "heading":
		data := omap.New("text", d.Text)

		if d.HeadType != "" {
			data.Set("type", d.HeadType)
		}

		data.Set("name", d.Name)

		if d.HeadType == "" {
			data.Set("depth", d.Depth)
		}

		data.Set("slug", d.Slug)

		if d.IsOverload {
			data.Set("isOverload", true)
			data.Set("overloadOf", d.OverloadOf)
		}

		return data
	case d.IsStability:
		return omap.New("index", d.Index, "description", d.Description)
	case n.Type == "typeAnnotation":
		links := make([]any, len(d.Links))

		for i, l := range d.Links {
			links[i] = omap.New("start", l.Start, "end", l.End, "text", l.Text, "href", l.Href)
		}

		data := omap.New("links", links, "typescript", d.TypeScript)

		if d.ParseError {
			data.Set("parseError", true)
		}

		return data
	}

	return omap.New()
}
