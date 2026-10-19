// Package metadata splits parsed API doc files into metadata entries: one
// per heading, with its content, stability index, YAML properties and
// resolved type links.
package metadata

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/nodejs/doc-kit/internal/slug"
	"github.com/nodejs/doc-kit/internal/unist"
)

// DocManBaseURL is the base URL of the Unix manuals.
const DocManBaseURL = "http://man7.org/linux/man-pages/man"

var (
	//go:embed maps/builtin.json
	builtinJSON []byte
	//go:embed maps/mdn.json
	mdnJSON []byte

	builtinTypeMap = mustMap(builtinJSON)
	mdnTypeMap     = mustMap(mdnJSON)
)

func mustMap(data []byte) map[string]string {
	var m map[string]string

	if err := json.Unmarshal(data, &m); err != nil {
		panic(err)
	}

	return m
}

var (
	// DeprecationHeading matches Node.js deprecation headings such as
	// "DEP0001: Description".
	DeprecationHeading = regexp.MustCompile(`^(DEP\d+):`)

	// moduleQualifiedName matches dotted identifier paths, e.g. `vm.Module`.
	moduleQualifiedName = regexp.MustCompile(`^[A-Za-z_]\w*(?:\.[A-Za-z_]\w*)+$`)
)

const (
	camelCase    = `\w+(?:\.\w+)*`
	functionCall = `\([^)]*\)`
	// Group 1: foo[bar]; group 2: foo.bar
	property = camelCase + `(?:(\[[^\]]+\])|\.(\w+))`
)

// headingTypes classify API doc headings, capturing their names.
var headingTypes = []struct {
	typ   string
	regex *regexp.Regexp
}{
	{"method", regexp.MustCompile("(?i)^`(?:" + property + "|(" + camelCase + "))" + functionCall + "`$")},
	{"event", regexp.MustCompile("(?i)^Event: +`'?([^`]*?)'?`$")},
	{"class", regexp.MustCompile("(?i)^Class: +`(" + camelCase + "(?: extends +" + camelCase + ")?)`$")},
	{"ctor", regexp.MustCompile("(?i)^`new +(" + camelCase + ")" + functionCall + "`$")},
	{"classMethod", regexp.MustCompile("(?i)^Static method: +`" + property + functionCall + "`$")},
	{"property", regexp.MustCompile("(?i)^`" + property + "`$")},
}

// TransformUnixManualToLink turns a reference to a Unix manual into a
// Markdown link.
func TransformUnixManualToLink(text, command, section, letter string) string {
	return fmt.Sprintf("[`%s`](%s%s/%s.%s%s.html)", text, DocManBaseURL, section, command, section, letter)
}

// LookupTypeName looks a type name up in the type maps: the given one first
// (by exact name), then the built-in and MDN ones (case-insensitively).
func LookupTypeName(name string, typeMap map[string]string) string {
	if url, ok := typeMap[name]; ok {
		return url
	}

	key := strings.ToLower(name)

	if url, ok := builtinTypeMap[key]; ok {
		return url
	}

	if url, ok := mdnTypeMap[key]; ok {
		return url
	}

	return ""
}

// ResolveTypeReference resolves a type identifier to a URL: map lookups,
// then the dotted-name heuristic for types like `vm.Module` (which link to
// their module's page).
func ResolveTypeReference(name string, typeMap map[string]string) string {
	if url := LookupTypeName(name, typeMap); url != "" {
		return url
	}

	if moduleQualifiedName.MatchString(name) {
		pieces := strings.Split(name, ".")
		last := pieces[len(pieces)-1]

		// A class starts with a capital, and has a lowercase letter after
		prefix := ""
		runes := []rune(last)

		if len(runes) > 0 && runes[0] >= 'A' && runes[0] <= 'Z' && strings.ContainsFunc(string(runes[1:]), func(r rune) bool { return r >= 'a' && r <= 'z' }) {
			prefix = "class-"
		}

		return pieces[0] + ".html#" + prefix + slug.Node(name)
	}

	return ""
}

// TransformNodeToHeading classifies a heading, extracting its name.
func TransformNodeToHeading(n *unist.Node) *unist.Data {
	text := unist.NodesToString(n.Children, false)

	for _, ht := range headingTypes {
		m := ht.regex.FindStringSubmatch(text)

		if len(m) > 1 {
			// The last matching group holds the name
			name := ""

			for _, g := range m[1:] {
				if g != "" {
					name = g
				}
			}

			return &unist.Data{Text: text, HeadType: ht.typ, Name: name}
		}
	}

	return &unist.Data{Text: text, Name: text, Depth: n.Depth}
}
