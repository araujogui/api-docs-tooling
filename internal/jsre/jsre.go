// Package jsre compiles JavaScript regular expressions into Go ones, giving
// `\s` and `\S` JavaScript's meaning (Unicode whitespace, which Go leaves
// out).
package jsre

import (
	"regexp"
	"strings"
)

// spaces are the characters JavaScript's `\s` matches.
const spaces = `\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}`

// Translate rewrites `\s` and `\S` in a pattern.
func Translate(pattern string) string {
	var b strings.Builder

	inClass := false

	for i := 0; i < len(pattern); i++ {
		c := pattern[i]

		if c == '\\' && i+1 < len(pattern) {
			next := pattern[i+1]

			switch {
			case next == 's' && inClass:
				b.WriteString(spaces)
			case next == 's':
				b.WriteString("[" + spaces + "]")
			case next == 'S' && !inClass:
				b.WriteString("[^" + spaces + "]")
			default:
				b.WriteByte(c)
				b.WriteByte(next)
			}

			i++

			continue
		}

		if c == '[' && !inClass {
			inClass = true
		} else if c == ']' && inClass {
			inClass = false
		}

		b.WriteByte(c)
	}

	return b.String()
}

// MustCompile compiles a JavaScript pattern.
func MustCompile(pattern string) *regexp.Regexp {
	return regexp.MustCompile(Translate(pattern))
}

// IsSpace reports whether JavaScript's `\s` matches a rune.
func IsSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}

	return r >= 0x2000 && r <= 0x200a
}

// Trim trims JavaScript whitespace, like String.prototype.trim.
func Trim(s string) string {
	return strings.TrimFunc(s, IsSpace)
}

// expand expands a JavaScript replacement string's `$$`, `$&`, “$`” and `$'`
// for a match of s[start:end].
func expand(repl, s string, start, end int) string {
	if !strings.Contains(repl, "$") {
		return repl
	}

	var b strings.Builder

	for i := 0; i < len(repl); i++ {
		if repl[i] == '$' && i+1 < len(repl) {
			switch repl[i+1] {
			case '$':
				b.WriteByte('$')
				i++

				continue
			case '&':
				b.WriteString(s[start:end])
				i++

				continue
			case '`':
				b.WriteString(s[:start])
				i++

				continue
			case '\'':
				b.WriteString(s[end:])
				i++

				continue
			}
		}

		b.WriteByte(repl[i])
	}

	return b.String()
}

// Replace replaces the first occurrence of old, like
// String.prototype.replace with a string pattern.
func Replace(s, old, repl string) string {
	i := strings.Index(s, old)

	if i < 0 {
		return s
	}

	return s[:i] + expand(repl, s, i, i+len(old)) + s[i+len(old):]
}

// ReplaceAll replaces every occurrence of old, like String.prototype.replace
// with a global regular expression matching it literally.
func ReplaceAll(s, old, repl string) string {
	if old == "" || !strings.Contains(s, old) {
		return s
	}

	var b strings.Builder

	last := 0

	for {
		i := strings.Index(s[last:], old)

		if i < 0 {
			break
		}

		i += last
		b.WriteString(s[last:i])
		b.WriteString(expand(repl, s, i, i+len(old)))
		last = i + len(old)
	}

	b.WriteString(s[last:])

	return b.String()
}
