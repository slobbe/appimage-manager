package util

import (
	"strings"
	"unicode"
)

func Slugify(s string) string {
	if s == "" {
		return ""
	}

	s = strings.ToLower(s)

	s = strings.Map(func(r rune) rune {
		if r == ' ' || r == '_' {
			return '-'
		}
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return r
	}, s)

	var b strings.Builder
	b.Grow(len(s))

	prevDash := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
			prevDash = false
		} else if r == '-' && !prevDash {
			b.WriteRune('-')
			prevDash = true
		}
	}

	result := b.String()
	return strings.Trim(result, "-")
}
