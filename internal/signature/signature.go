// Package signature takes method signatures and typed parameter lists apart.
// Parameters are ordered maps, as the legacy JSON output serializes them with
// their keys in the order they were set.
package signature

import (
	"github.com/nodejs/doc-kit/internal/jsre"
	"regexp"
	"strings"

	"github.com/nodejs/doc-kit/internal/omap"
)

var (
	// NameExpression grabs a parameter's name.
	NameExpression = jsre.MustCompile("^['`\"]?([^'`\": {]+)['`\"]?\\s*:?\\s*")
	// LeadingHyphen matches a leading hyphen.
	LeadingHyphen = jsre.MustCompile(`^-\s*`)
	// TrimmablePadding matches the separators between a parameter's type and
	// description.
	TrimmablePadding = jsre.MustCompile(`^[\s:-]+`)
	// DefaultExpression grabs a default value.
	DefaultExpression = jsre.MustCompile(`(?is)\s*\*\*Default:\*\*\s*(.+)$`)
	// paramExpression grabs the parameters of a signature.
	paramExpression = regexp.MustCompile(`\(([^)]+)\);?$`)
	surroundingTick = regexp.MustCompile("^`|`$")
)

func isBracket(r rune) bool {
	return r == '[' || r == ']'
}

func depthChange(r rune) int {
	switch r {
	case '[':
		return 1
	case ']':
		return -1
	}

	return 0
}

// ParseNameAndOptionalStatus strips a parameter's optional brackets, tracking
// the optional depth: the brackets before the name apply to it, those after
// it to the next parameter.
func ParseNameAndOptionalStatus(name string, depth int) (string, int, bool) {
	runes := []rune(name)

	start := len(runes)

	for i, r := range runes {
		if !isBracket(r) {
			start = i
			break
		}
	}

	if start == len(runes) {
		start = -1
	}

	end := -1

	for i := len(runes) - 1; i >= 0; i-- {
		if !isBracket(runes[i]) {
			end = i
			break
		}
	}

	// JavaScript's slice(0, -1) keeps all but the last character
	leading := sliceRunes(runes, 0, start)

	for _, r := range leading {
		depth += depthChange(r)
	}

	actual := sliceRunes(runes, start, end+1)
	optional := depth > 0

	for _, r := range sliceRunes(runes, end+1, len(runes)) {
		depth += depthChange(r)
	}

	return string(actual), depth, optional
}

// sliceRunes slices like String.prototype.slice, negative indexes counting
// from the end.
func sliceRunes(r []rune, start, end int) []rune {
	n := len(r)

	if start < 0 {
		start = max(n+start, 0)
	}

	if end < 0 {
		end = max(n+end, 0)
	}

	if start > n {
		start = n
	}

	if end > n {
		end = n
	}

	if start >= end {
		return nil
	}

	return r[start:end]
}

// ParseDefaultValue splits a `name=value` parameter.
func ParseDefaultValue(name string) (string, string) {
	if i := strings.Index(name, "="); i >= 0 {
		return name[:i], jsre.Trim(name[i:])
	}

	return name, ""
}

func nameOf(p *omap.Map) (string, bool) {
	s, ok := p.Value("name").(string)

	return s, ok
}

// FindParameter finds the documented parameter for a declared one: the one
// at its position when the names agree, or any other (or option of one)
// with its name.
func FindParameter(name string, index int, params []*omap.Map) *omap.Map {
	if index < len(params) {
		if n, ok := nameOf(params[index]); ok && n == name {
			return params[index]
		}
	}

	for _, p := range params {
		if n, ok := nameOf(p); ok && n == name {
			return p
		}

		if options, ok := p.Value("options").([]*omap.Map); ok {
			for _, option := range options {
				if n, ok := nameOf(option); ok && n == name {
					return option.Clone()
				}
			}
		}
	}

	return omap.New("name", name)
}

// ParseParameters matches the parameters a signature declares to the
// documented ones.
func ParseParameters(declared []string, params []*omap.Map) []*omap.Map {
	result := []*omap.Map{}
	depth := 0

	for i, name := range declared {
		name = jsre.Trim(name)

		var optional bool
		name, depth, optional = ParseNameAndOptionalStatus(name, depth)

		var def string
		name, def = ParseDefaultValue(name)

		p := FindParameter(name, i, params)

		if optional {
			p.Set("optional", true)
		}

		if def != "" {
			p.Set("default", def)
		}

		result = append(result, p)
	}

	return result
}

// ParseSignature builds a signature from a heading's text (like
// `new buffer.Blob([sources[, options]])`) and its documented parameters.
func ParseSignature(textRaw string, params []*omap.Map) *omap.Map {
	sig := omap.New("params", []*omap.Map{})

	var rest []*omap.Map

	for _, p := range params {
		if n, _ := nameOf(p); n == "return" || n == "extends" {
			sig.Set(n, p)
			continue
		}

		rest = append(rest, p)
	}

	m := paramExpression.FindStringSubmatch(surroundingTick.ReplaceAllString(textRaw, ""))

	if m == nil {
		return sig
	}

	sig.Set("params", ParseParameters(strings.Split(m[1], ","), rest))

	return sig
}
