// Package slug creates anchor IDs like `github-slugger`, with the Node.js
// specific replacements on top.
package slug

import (
	"sort"
	"strconv"
	"strings"

	"github.com/dlclark/regexp2/v2"
)

func isRemoved(r rune) bool {
	i := sort.Search(len(removed), func(i int) bool { return removed[i][1] >= r })

	return i < len(removed) && removed[i][0] <= r
}

// GitHub slugs a string like `github-slugger`'s `slug`: lowercased, without
// punctuation and symbols, with spaces turned into dashes.
func GitHub(value string) string {
	var b strings.Builder

	for _, r := range strings.ToLower(value) {
		if isRemoved(r) {
			continue
		}

		if r == ' ' {
			b.WriteByte('-')
		} else {
			b.WriteRune(r)
		}
	}

	return b.String()
}

type replacement struct {
	from *regexp2.Regexp
	to   string
	all  bool
}

func mustCompile(pattern string, opts regexp2.RegexOptions) *regexp2.Regexp {
	return regexp2.MustCompile(pattern, opts)
}

// The string replacements specific to Node.js anchor IDs.
var replacements = []replacement{
	{from: mustCompile(`node.js`, regexp2.IgnoreCase), to: "nodejs"},
	{from: mustCompile(`&`, 0), to: "-and-"},
	{from: mustCompile(`[/,:;\\ ]`, 0), to: "-", all: true},
	{from: mustCompile(`^-(?!-|$)`, 0), to: "", all: true},
	{from: mustCompile(`(?<!^-*)-+$`, 0), to: "", all: true},
	{from: mustCompile(`^(?!-)(?!-+$).*?(--+)`, 0), to: "-", all: true},
}

func replace(re *regexp2.Regexp, s, to string, all bool) string {
	count := 1

	if all {
		count = -1
	}

	out, err := re.Replace(s, to, 0, count)
	if err != nil {
		return s
	}

	return out
}

// Node slugs a title the way the Node.js API docs build their anchors.
func Node(title string) string {
	return ApplyReplacements(GitHub(title))
}

// ApplyReplacements applies the Node.js replacements to a slug.
func ApplyReplacements(s string) string {
	for _, r := range replacements {
		s = replace(r.from, s, r.to, r.all)
	}

	return s
}

// Slugger creates unique slugs, numbering repeated ones like
// `github-slugger`'s class does.
type Slugger struct {
	occurrences map[string]int
}

// NewSlugger creates a slugger.
func NewSlugger() *Slugger {
	return &Slugger{occurrences: map[string]int{}}
}

// GitHub returns a unique GitHub slug.
func (s *Slugger) GitHub(value string) string {
	result := GitHub(value)
	original := result

	for {
		if _, ok := s.occurrences[result]; !ok {
			break
		}

		s.occurrences[original]++
		result = original + "-" + strconv.Itoa(s.occurrences[original])
	}

	s.occurrences[result] = 0

	return result
}

// Node returns a unique slug with the Node.js replacements applied.
func (s *Slugger) Node(title string) string {
	return ApplyReplacements(s.GitHub(title))
}
