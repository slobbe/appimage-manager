package domain

import (
	"strings"
	"unicode"
)

// Slugify normalizes arbitrary app names into stable, URL/path-safe IDs.
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
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			prevDash = false
		case r == '-' && !prevDash:
			b.WriteRune('-')
			prevDash = true
		}
	}

	return strings.Trim(b.String(), "-")
}
