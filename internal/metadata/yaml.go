package metadata

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/nodejs/doc-kit/internal/omap"
	"github.com/nodejs/doc-kit/internal/queries"
	"github.com/nodejs/doc-kit/internal/unist"
	"gopkg.in/yaml.v3"
)

// ExtractYAMLContent extracts the raw YAML of a comment block node.
func ExtractYAMLContent(n *unist.Node) string {
	loc := queries.YAMLInnerContent.FindStringSubmatchIndex(n.Value)

	if loc == nil {
		return n.Value
	}

	// Either a multiline YAML block, or a simple single-line one
	inner := ""

	if loc[2] >= 0 && loc[3] > loc[2] {
		inner = n.Value[loc[2]:loc[3]]
	} else if loc[4] >= 0 {
		inner = n.Value[loc[4]:loc[5]]
	}

	return inner + n.Value[loc[1]:]
}

var edgeLineBreaks = regexp.MustCompile(`^[\r\n]+|[\r\n]+$`)

// NormalizeYAMLSyntax fixes the `key=value` properties of the docs' schema.
func NormalizeYAMLSyntax(content string) string {
	for _, key := range []string{"introduced_in", "source_link", "type", "name", "llm_description"} {
		content = strings.Replace(content, key+"=", key+": ", 1)
	}

	return edgeLineBreaks.ReplaceAllString(content, "")
}

// ParseYAMLIntoMetadata parses a YAML block into the properties it gives an
// entry. A block that is just a string becomes a tag.
func ParseYAMLIntoMetadata(content string) (any, error) {
	normalized := NormalizeYAMLSyntax(content)

	var node yaml.Node

	if err := yaml.Unmarshal([]byte(normalized), &node); err != nil {
		return nil, err
	}

	if len(node.Content) == 0 {
		return nil, nil
	}

	value := convert(node.Content[0])

	if s, ok := value.(string); ok {
		return omap.New("tags", []any{s}), nil
	}

	return value, nil
}

// convert turns a YAML node into the values the JavaScript `yaml` package
// gives: ordered maps, slices, strings, numbers, booleans and nulls.
func convert(n *yaml.Node) any {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil
		}

		return convert(n.Content[0])
	case yaml.AliasNode:
		return convert(n.Alias)
	case yaml.MappingNode:
		m := omap.New()

		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i]

			if key.Tag == "!!merge" {
				if merged, ok := convert(n.Content[i+1]).(*omap.Map); ok {
					for _, k := range merged.Keys() {
						if !m.Has(k) {
							v, _ := merged.Get(k)
							m.Set(k, v)
						}
					}
				}

				continue
			}

			m.Set(fmt.Sprint(convert(key)), convert(n.Content[i+1]))
		}

		return m
	case yaml.SequenceNode:
		items := make([]any, len(n.Content))

		for i, c := range n.Content {
			items[i] = convert(c)
		}

		return items
	}

	switch n.ShortTag() {
	case "!!null":
		return nil
	case "!!bool":
		return n.Value == "true" || n.Value == "True" || n.Value == "TRUE"
	case "!!int":
		v := strings.ReplaceAll(n.Value, "_", "")

		if i, err := strconv.ParseInt(v, 0, 64); err == nil {
			return float64(i)
		}

		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}

		return n.Value
	case "!!float":
		switch strings.ToLower(n.Value) {
		case ".inf", "+.inf":
			return n.Value
		case "-.inf", ".nan":
			return n.Value
		}

		if f, err := strconv.ParseFloat(n.Value, 64); err == nil {
			return f
		}

		return n.Value
	}

	// Timestamps and anything else stay strings, as in the YAML 1.2 core
	// schema
	return n.Value
}
