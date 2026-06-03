package domain

import (
	"fmt"
	"regexp"
	"strconv"
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
			if value[end] == '_' && isPackagingSuffixStart(value[end+1:]) {
				return normalizeComparableVersionCandidate(candidate)
			}
			if cut := strings.LastIndexAny(candidate, "-+"); cut >= 0 {
				candidate = candidate[:cut]
			} else {
				continue
			}
		}
		if normalized := normalizeComparableVersionCandidate(candidate); normalized != "" {
			return normalized
		}
	}

	if !strings.ContainsAny(value, "0123456789") {
		return ""
	}

	return trimLeadingVIfNumeric(value)
}

func NormalizeSelfUpdateVersion(raw string) string {
	value := strings.TrimSpace(strings.Trim(raw, `"'`))
	if value == "" || strings.EqualFold(value, "dev") {
		return ""
	}
	return NormalizeComparableVersion(value)
}

func ReleaseAvailability(currentVersion, tagName string) (string, bool) {
	latest := NormalizeComparableVersion(tagName)
	current := NormalizeComparableVersion(currentVersion)
	return latest, latest != "" && latest != current
}

func CompareComparableVersions(left, right string) (int, error) {
	leftVersion, err := parseComparableSemver(left)
	if err != nil {
		return 0, err
	}
	rightVersion, err := parseComparableSemver(right)
	if err != nil {
		return 0, err
	}

	return compareVersionCores(leftVersion, rightVersion), nil
}

func CompareSelfUpdateVersions(left, right string) (int, error) {
	leftVersion, err := parseSelfUpdateSemver(left)
	if err != nil {
		return 0, err
	}
	rightVersion, err := parseSelfUpdateSemver(right)
	if err != nil {
		return 0, err
	}

	if coreComparison := compareVersionCores(leftVersion.core, rightVersion.core); coreComparison != 0 {
		return coreComparison, nil
	}
	return comparePrerelease(leftVersion.prerelease, rightVersion.prerelease), nil
}

type selfUpdateSemver struct {
	core       [3]int
	prerelease string
}

func parseComparableSemver(version string) ([3]int, error) {
	parsed, err := parseSelfUpdateSemver(version)
	if err != nil {
		return [3]int{}, err
	}
	return parsed.core, nil
}

func parseSelfUpdateSemver(version string) (selfUpdateSemver, error) {
	var parsed selfUpdateSemver

	normalized := strings.TrimSpace(strings.Trim(version, `"'`))
	if normalized == "" {
		return parsed, fmt.Errorf("invalid version %q", version)
	}

	if idx := strings.Index(normalized, "+"); idx >= 0 {
		normalized = normalized[:idx]
	}
	if idx := strings.Index(normalized, "-"); idx >= 0 {
		parsed.prerelease = normalized[idx+1:]
		normalized = normalized[:idx]
	}

	parts := strings.Split(normalized, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return parsed, fmt.Errorf("invalid version %q", version)
	}

	for i := range parts {
		value, err := strconv.Atoi(strings.TrimSpace(parts[i]))
		if err != nil || value < 0 {
			return parsed, fmt.Errorf("invalid version %q", version)
		}
		parsed.core[i] = value
	}

	return parsed, nil
}

func compareVersionCores(left, right [3]int) int {
	for i := range left {
		if left[i] > right[i] {
			return 1
		}
		if left[i] < right[i] {
			return -1
		}
	}
	return 0
}

func comparePrerelease(left, right string) int {
	if left == "" && right == "" {
		return 0
	}
	if left == "" {
		return 1
	}
	if right == "" {
		return -1
	}

	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	maxLen := len(leftParts)
	if len(rightParts) > maxLen {
		maxLen = len(rightParts)
	}

	for i := 0; i < maxLen; i++ {
		if i >= len(leftParts) {
			return -1
		}
		if i >= len(rightParts) {
			return 1
		}
		if comparison := comparePrereleaseIdentifier(leftParts[i], rightParts[i]); comparison != 0 {
			return comparison
		}
	}
	return 0
}

func comparePrereleaseIdentifier(left, right string) int {
	leftNumber, leftNumeric := parsePrereleaseNumber(left)
	rightNumber, rightNumeric := parsePrereleaseNumber(right)
	if leftNumeric && rightNumeric {
		if leftNumber > rightNumber {
			return 1
		}
		if leftNumber < rightNumber {
			return -1
		}
		return 0
	}
	if leftNumeric {
		return -1
	}
	if rightNumeric {
		return 1
	}
	return strings.Compare(left, right)
}

func parsePrereleaseNumber(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	if len(value) > 1 && value[0] == '0' {
		return 0, false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	number, err := strconv.Atoi(value)
	return number, err == nil
}

func normalizeComparableVersionCandidate(candidate string) string {
	normalized := trimLeadingVIfNumeric(candidate)
	return stripPackagingVersionSuffix(normalized)
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

func isPackagingSuffixStart(suffix string) bool {
	segment := leadingPackagingSuffixSegment(suffix)
	return isPackagingVersionSegment(segment)
}

func leadingPackagingSuffixSegment(suffix string) string {
	suffix = strings.TrimLeft(strings.TrimSpace(suffix), "-_ .")
	if suffix == "" {
		return ""
	}
	end := len(suffix)
	for i, r := range suffix {
		if r == '-' || r == '_' || r == '.' || r == '+' {
			end = i
			break
		}
	}
	return suffix[:end]
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
