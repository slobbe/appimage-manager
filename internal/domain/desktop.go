package domain

import (
	"path/filepath"
	"strings"
	"unicode"
)

func DesktopStemFromPath(path string) string {
	base := strings.TrimSpace(filepath.Base(path))
	if base == "" {
		return ""
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func SanitizeDesktopStem(stem string) string {
	stem = strings.TrimSpace(stem)
	if stem == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(stem))

	prevSeparator := false
	for _, r := range stem {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r):
			b.WriteRune(r)
			prevSeparator = false
		case r == '.', r == '-', r == '_':
			if prevSeparator {
				continue
			}
			b.WriteRune(r)
			prevSeparator = true
		case unicode.IsSpace(r):
			if prevSeparator {
				continue
			}
			b.WriteRune('-')
			prevSeparator = true
		}
	}

	return strings.Trim(b.String(), "._-")
}
