package util

import (
	"sort"
	"strings"
	"unicode"
)

// NaturalLess reports whether a < b using natural sort order:
// numeric runs within strings are compared numerically so "img2" < "img10".
// Comparison is case-insensitive.
func NaturalLess(a, b string) bool {
	a = strings.ToLower(a)
	b = strings.ToLower(b)
	for {
		if b == "" {
			return false
		}
		if a == "" {
			return true
		}
		aDigit := unicode.IsDigit(rune(a[0]))
		bDigit := unicode.IsDigit(rune(b[0]))

		if aDigit && bDigit {
			// consume and compare numeric run
			an, aRest := leadingInt(a)
			bn, bRest := leadingInt(b)
			if an != bn {
				return an < bn
			}
			a, b = aRest, bRest
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
}

// leadingInt returns the numeric value of the leading digit run and the remainder.
func leadingInt(s string) (n int, rest string) {
	i := 0
	for i < len(s) && unicode.IsDigit(rune(s[i])) {
		n = n*10 + int(s[i]-'0')
		i++
	}
	return n, s[i:]
}

// SortStringsNatural sorts a slice of strings in natural order (case-insensitive).
func SortStringsNatural(ss []string) {
	sort.Slice(ss, func(i, j int) bool {
		return NaturalLess(ss[i], ss[j])
	})
}
