package core

import (
	"regexp"
	"strings"
)

var comparableVersionPattern = regexp.MustCompile(`(?i)v?\d+(?:\.\d+)+(?:-[0-9a-z.-]+)?(?:\+[0-9a-z.-]+)?`)
var numericVersionCorePattern = regexp.MustCompile(`^\d+(?:\.\d+)+`)

var packagingVersionSegments = map[string]struct{}{
	"aarch64":   {},
	"amd64":     {},
	"appimage":  {},
	"arm64":     {},
	"darwin":    {},
	"glibc":     {},
	"installer": {},
	"linux":     {},
	"macos":     {},
	"musl":      {},
	"portable":  {},
	"setup":     {},
	"universal": {},
	"windows":   {},
	"x64":       {},
	"x86":       {},
	"x86_64":    {},
}

// NormalizeComparableVersion extracts the comparable version token from decorated
// AppImage metadata and release tags.
func NormalizeComparableVersion(raw string) string {
	return normalizeComparableVersion(raw)
}

func normalizeComparableVersion(raw string) string {
	value := strings.TrimSpace(strings.Trim(raw, `"'`))
	if value == "" {
		return ""
	}

	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "version") {
		value = strings.TrimSpace(value[len("version"):])
		value = strings.TrimLeft(value, " :=-")
		lower = strings.ToLower(strings.TrimSpace(value))
	}

	switch lower {
	case "", "n/a", "na", "none", "unknown", "-":
		return ""
	}

	matchRanges := comparableVersionPattern.FindAllStringIndex(value, -1)
	for i := len(matchRanges) - 1; i >= 0; i-- {
		start, end := matchRanges[i][0], matchRanges[i][1]
		candidate := value[start:end]
		if end < len(value) && isVersionContinuationByte(value[end]) {
			if cut := strings.LastIndexAny(candidate, "-+"); cut >= 0 {
				candidate = candidate[:cut]
			} else {
				continue
			}
		}
		normalized := trimLeadingVIfNumeric(candidate)
		normalized = stripPackagingVersionSuffix(normalized)
		if normalized != "" {
			return normalized
		}
	}

	if !strings.ContainsAny(value, "0123456789") {
		return ""
	}

	return trimLeadingVIfNumeric(value)
}

func trimLeadingVIfNumeric(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) > 1 && value[0] == 'v' {
		next := value[1]
		if next >= '0' && next <= '9' {
			value = strings.TrimSpace(value[1:])
		}
	}
	return value
}

func stripPackagingVersionSuffix(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	core := numericVersionCorePattern.FindString(value)
	if core == "" {
		return value
	}

	remainder := value[len(core):]
	if remainder == "" {
		return core
	}

	build := ""
	if plus := strings.IndexByte(remainder, '+'); plus >= 0 {
		build = remainder[plus:]
		remainder = remainder[:plus]
	}

	if remainder == "" {
		return core + build
	}
	if remainder[0] != '-' {
		return core + remainder + build
	}

	segments := strings.Split(remainder[1:], "-")
	if len(segments) == 0 {
		return core + build
	}

	packagingStart := -1
	for i, segment := range segments {
		if isPackagingVersionSegment(segment) {
			packagingStart = i
			break
		}
	}

	if packagingStart == -1 {
		return core + remainder + build
	}
	if packagingStart == 0 {
		return core
	}

	return core + "-" + strings.Join(segments[:packagingStart], "-")
}

func isPackagingVersionSegment(segment string) bool {
	segment = strings.ToLower(strings.TrimSpace(segment))
	if segment == "" {
		return false
	}

	_, ok := packagingVersionSegments[segment]
	return ok
}

func isVersionContinuationByte(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_'
}
