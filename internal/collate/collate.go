// Package collate compares strings like JavaScript's localeCompare does
// with the default (ICU root) collation, closely enough for the names and
// ids the generators sort: whitespace, then punctuation and symbols, then
// digits, then letters, case-insensitively, with lowercase first on ties.
package collate

import (
	"strings"
	"unicode"
)

// punctuationOrder is the ICU root order of the ASCII punctuation and
// symbols.
const punctuationOrder = "_-,;:!?.'\"()[]{}@*/\\&#%`^+<=>|~$"

func primary(r rune) (int, rune) {
	switch {
	case unicode.IsSpace(r):
		return 0, r
	case strings.ContainsRune(punctuationOrder, r):
		return 1, rune(strings.IndexRune(punctuationOrder, r))
	case unicode.IsPunct(r) || unicode.IsSymbol(r):
		return 2, r
	case unicode.IsDigit(r):
		return 3, r
	default:
		return 4, unicode.ToLower(r)
	}
}

// Compare returns -1, 0 or 1.
func Compare(a, b string) int {
	ra, rb := []rune(a), []rune(b)

	for i := 0; i < len(ra) && i < len(rb); i++ {
		ca, ka := primary(ra[i])
		cb, kb := primary(rb[i])

		if ca != cb {
			return sign(ca - cb)
		}

		if ka != kb {
			return sign(int(ka - kb))
		}
	}

	if len(ra) != len(rb) {
		return sign(len(ra) - len(rb))
	}

	// Ties: lowercase before uppercase
	for i := range ra {
		if ra[i] != rb[i] {
			if unicode.IsLower(ra[i]) {
				return -1
			}

			return 1
		}
	}

	return 0
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}

	return 0
}
