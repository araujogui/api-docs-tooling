package markdown

import (
	"html"
	"regexp"
	"strings"
)

var characterReference = regexp.MustCompile(`&(?:#[xX][0-9a-fA-F]{1,6}|#[0-9]{1,7}|[A-Za-z][A-Za-z0-9]{1,31});`)

// isASCIIPunct reports whether a byte is ASCII punctuation, which a backslash
// escapes in CommonMark.
func isASCIIPunct(c byte) bool {
	return (c >= '!' && c <= '/') || (c >= ':' && c <= '@') || (c >= '[' && c <= '`') || (c >= '{' && c <= '~')
}

// decodeEntities resolves the character references in a string.
func decodeEntities(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}

	return characterReference.ReplaceAllStringFunc(s, func(ref string) string {
		if ref == "&#0;" || strings.HasPrefix(ref, "&#") && strings.Trim(ref[2:len(ref)-1], "0xX") == "" {
			return "\uFFFD"
		}

		return html.UnescapeString(ref)
	})
}

// decodeString resolves the backslash escapes and character references in a
// string, in a single pass, as CommonMark does for text, link destinations
// and titles.
func decodeString(s string) string {
	if !strings.ContainsAny(s, "\\&") {
		return s
	}

	var b strings.Builder

	start := 0

	flush := func(end int) {
		b.WriteString(decodeEntities(s[start:end]))
	}

	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && isASCIIPunct(s[i+1]) {
			flush(i)
			b.WriteByte(s[i+1])
			i++
			start = i + 1
		}
	}

	flush(len(s))

	return b.String()
}
