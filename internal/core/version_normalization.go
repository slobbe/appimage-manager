package core

import (
	"regexp"
	"strings"
)

var comparableVersionPattern = regexp.MustCompile(`(?i)v?\d+(?:\.\d+)+(?:-[0-9a-z.-]+)?(?:\+[0-9a-z.-]+)?`)

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

func isVersionContinuationByte(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_'
}
