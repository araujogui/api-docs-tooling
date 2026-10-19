package metadata

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/nodejs/doc-kit/internal/logger"
	"github.com/nodejs/doc-kit/internal/tstype"
	"github.com/nodejs/doc-kit/internal/unist"
)

var (
	displayNamePart = regexp.MustCompile(`(?:[^|]|\|\|)+`)
	arraySuffix     = regexp.MustCompile(`(\[\])+$`)
)

// resolveDisplayNames resolves a value as a `|`-separated union of display
// names (e.g. `HTTP/2 Headers Object | HTTP/2 Raw Headers`), which may not
// parse as TypeScript, though each part may resolve on its own.
func resolveDisplayNames(value string, typeMap map[string]string) []unist.TypeLink {
	links := []unist.TypeLink{}

	for _, loc := range displayNamePart.FindAllStringIndex(value, -1) {
		part := value[loc[0]:loc[1]]

		// An array of a type links to the type itself
		name := arraySuffix.ReplaceAllString(strings.TrimSpace(part), "")

		if name == "" {
			continue
		}

		if href := ResolveTypeReference(name, typeMap); href != "" {
			start := utf16Len(value[:loc[0]]) + utf16Len(part[:strings.Index(part, name)])

			links = append(links, unist.TypeLink{Start: start, End: start + utf16Len(name), Text: name, Href: href})
		}
	}

	return links
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

// resolveLinks resolves a type's links: the whole value when it is a map key
// in its own right, then its parsed identifiers, then its display names.
func resolveLinks(value string, typeMap map[string]string, ids []tstype.Identifier, parsed bool) []unist.TypeLink {
	if url := LookupTypeName(value, typeMap); url != "" {
		return []unist.TypeLink{{Start: 0, End: utf16Len(value), Text: value, Href: url}}
	}

	if !parsed {
		return resolveDisplayNames(value, typeMap)
	}

	links := []unist.TypeLink{}

	for _, id := range ids {
		if href := ResolveTypeReference(id.Text, typeMap); href != "" {
			links = append(links, unist.TypeLink{Start: id.Start, End: id.End, Text: id.Text, Href: href})
		}
	}

	// The HTML handlers slice the value by these ranges in order
	sort.SliceStable(links, func(i, j int) bool { return links[i].Start < links[j].Start })

	return links
}

// ResolveTypeAnnotations resolves the links of every `typeAnnotation` node in
// a tree.
func ResolveTypeAnnotations(tree *unist.Node, typeMap map[string]string, path string) {
	unist.Visit(tree, unist.IsType("typeAnnotation"), func(n *unist.Node, _ int, _ *unist.Node) unist.Step {
		ids, ok := tstype.Parse(n.Value)
		links := resolveLinks(n.Value, typeMap, ids, ok)

		// Unparseable display names aren't highlighted as TypeScript
		n.Data = &unist.Data{Links: links, TypeScript: ok, Resolved: true}

		if !ok && len(links) == 0 {
			n.Data.ParseError = true

			fields := logger.Fields{File: &logger.File{Path: path}}

			if n.Position != nil {
				fields.File.Position = &logger.Position{Start: n.Position.Start.Line, End: n.Position.End.Line}
			}

			logger.Default.Warn(fmt.Sprintf("Invalid type annotation: {%s}", n.Value), fields)
		}

		return unist.Next
	})
}
