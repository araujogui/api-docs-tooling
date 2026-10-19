// Package config loads and holds the configuration of a run.
package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/nodejs/doc-kit/internal/parsers"
	"github.com/nodejs/doc-kit/internal/semver"
)

// Templates for the URLs generators link to.
const (
	// ChangelogURL is the CHANGELOG listing every major version.
	ChangelogURL = "https://raw.githubusercontent.com/{repository}/{ref}/CHANGELOG.md"
	// GitHubBlobURL is the base URL for viewing a file on GitHub.
	GitHubBlobURL = "https://github.com/{repository}/blob/{ref}/"
	// GitHubEditURL is the base URL for editing a file on GitHub.
	GitHubEditURL = "https://github.com/{repository}/edit/{ref}"
)

// Options are a generator's settings (its own and the global ones it
// inherits), as loosely typed as the configuration files they come from.
type Options map[string]any

// Clone returns a shallow copy.
func (o Options) Clone() Options {
	c := make(Options, len(o))

	for k, v := range o {
		c[k] = v
	}

	return c
}

// With returns a copy with more values.
func (o Options) With(kv ...any) Options {
	c := o.Clone()

	for i := 0; i+1 < len(kv); i += 2 {
		c[kv[i].(string)] = kv[i+1]
	}

	return c
}

// String returns a string option ("" when unset).
func (o Options) String(key string) string {
	if v, ok := o[key]; ok && v != nil {
		return JSString(v)
	}

	return ""
}

// Bool returns a boolean option.
func (o Options) Bool(key string) bool {
	switch v := o[key].(type) {
	case bool:
		return v
	case string:
		return v != ""
	case nil:
		return false
	case int:
		return v != 0
	case float64:
		return v != 0
	default:
		return true
	}
}

// Strings returns an option holding one or more strings.
func (o Options) Strings(key string) []string {
	switch v := o[key].(type) {
	case nil:
		return nil
	case string:
		return []string{v}
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))

		for _, item := range v {
			out = append(out, JSString(item))
		}

		return out
	default:
		return []string{JSString(v)}
	}
}

// Version returns the target version.
func (o Options) Version() semver.Version {
	switch v := o["version"].(type) {
	case semver.Version:
		return v
	case *semver.Version:
		if v != nil {
			return *v
		}
	case string:
		return semver.MustCoerce(v)
	}

	return semver.Version{}
}

// Changelog returns the release history.
func (o Options) Changelog() []parsers.Release {
	if v, ok := o["changelog"].([]parsers.Release); ok {
		return v
	}

	return nil
}

// Index returns the documentation index.
func (o Options) Index() []parsers.IndexEntry {
	if v, ok := o["index"].([]parsers.IndexEntry); ok {
		return v
	}

	return nil
}

// HasIndex reports whether an index was given.
func (o Options) HasIndex() bool {
	_, ok := o["index"].([]parsers.IndexEntry)

	return ok
}

// JSString converts a value to a string the way JavaScript's String() does.
func JSString(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.Itoa(x)
	case float64:
		if x == float64(int64(x)) && x < 1e21 && x > -1e21 {
			return strconv.FormatInt(int64(x), 10)
		}

		return strconv.FormatFloat(x, 'g', -1, 64)
	case []any:
		parts := make([]string, len(x))

		for i, item := range x {
			if item != nil {
				parts[i] = JSString(item)
			}
		}

		return strings.Join(parts, ",")
	case []string:
		return strings.Join(x, ",")
	case fmt.Stringer:
		return x.String()
	case map[string]any:
		return "[object Object]"
	default:
		return fmt.Sprint(x)
	}
}

var templatePattern = regexp.MustCompile(`\{(\w+)(?:\|(\w+))?\}`)

// Populate fills a template's `{key}` (or `{key|fallback}`) placeholders from
// options, leaving unknown ones as they are.
func Populate(template string, o Options) string {
	return templatePattern.ReplaceAllStringFunc(template, func(match string) string {
		m := templatePattern.FindStringSubmatch(match)

		if v, ok := o[m[1]]; ok && v != nil {
			return JSString(v)
		}

		if m[2] != "" {
			return m[2]
		}

		return match
	})
}
