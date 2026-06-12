package domain

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var (
	versionCandidatePattern = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])v?([0-9][0-9a-z._+\-~]*)`)
	dateLikePattern         = regexp.MustCompile(`^\d{4}[-_.]?\d{2}[-_.]?\d{2}(?:$|[-_.+~])`)
	datePattern             = regexp.MustCompile(`^(\d{4})[-_.]?(\d{2})[-_.]?(\d{2})(.*)$`)
	numericPattern          = regexp.MustCompile(`^(\d+(?:[-_.]\d+){1,5})(.*)$`)
)

// Version is a parsed, normalized version extracted from arbitrary text.
//
// The raw value is the candidate substring that was selected from the input,
// while the string representation is normalized for display and comparison by
// callers. This type intentionally stays format-agnostic: AppImage metadata and
// release assets use semver, calendar versions, build metadata, and many
// project-specific variants.
type Version struct {
	raw        string
	normalized string
}

// ParseVersion extracts the most likely version from arbitrary text and returns
// it in normalized form.
//
// Supported inputs include common release strings such as "v1.2.3",
// "1.2.1-beta.1", "app-2024-06-12-x86_64.AppImage", and desktop metadata
// values containing a plain version. The parser is deliberately permissive, but
// stops suffix parsing at common platform, package, and file-format tokens so
// asset names do not leak architecture or extension data into the version.
func ParseVersion(input string) (Version, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return Version{}, false
	}

	matches := versionCandidatePattern.FindAllStringSubmatch(input, -1)
	if len(matches) == 0 {
		return Version{}, false
	}

	best := Version{}
	bestRank := -1

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		candidate := trimVersionBoundary(match[1])
		normalized, ok := normalizeVersion(candidate)
		if !ok {
			continue
		}

		rank := versionRank(normalized)
		if rank >= bestRank {
			best = Version{
				raw:        candidate,
				normalized: normalized,
			}
			bestRank = rank
		}
	}

	if bestRank == -1 {
		return Version{}, false
	}

	return best, true
}

// NormalizeVersion extracts and normalizes a version from arbitrary text.
func NormalizeVersion(input string) (string, bool) {
	version, ok := ParseVersion(input)
	if !ok {
		return "", false
	}

	return version.String(), true
}

// String returns the normalized version.
func (v Version) String() string {
	return v.normalized
}

// Raw returns the candidate substring selected from the input before
// normalization.
func (v Version) Raw() string {
	return v.raw
}

// IsZero reports whether the version is empty.
func (v Version) IsZero() bool {
	return v.normalized == ""
}

// CompareVersions compares two parsed and normalized versions.
//
// It returns 1 when left is newer, -1 when right is newer, and 0 when both have
// equal precedence. Build metadata is ignored for precedence. Prereleases sort
// before their stable release, so "1.2.3-beta.1" is older than "1.2.3".
func CompareVersions(left, right string) int {
	leftVersion := comparableVersion(left)
	rightVersion := comparableVersion(right)

	maxCoreLen := len(leftVersion.core)
	if len(rightVersion.core) > maxCoreLen {
		maxCoreLen = len(rightVersion.core)
	}

	for i := 0; i < maxCoreLen; i++ {
		leftPart := corePart(leftVersion.core, i)
		rightPart := corePart(rightVersion.core, i)
		if leftPart > rightPart {
			return 1
		}
		if leftPart < rightPart {
			return -1
		}
	}

	return comparePrerelease(leftVersion.prerelease, rightVersion.prerelease)
}

func normalizeVersion(candidate string) (string, bool) {
	candidate = trimVersionBoundary(candidate)
	candidate = strings.TrimPrefix(candidate, "v")
	candidate = strings.TrimPrefix(candidate, "V")
	candidate = trimKnownFileExtension(candidate)

	if candidate == "" || !containsDigit(candidate) {
		return "", false
	}

	if normalized, ok := normalizeDateVersion(candidate); ok {
		return normalized, true
	}
	if dateLikePattern.MatchString(candidate) {
		return "", false
	}

	return normalizeNumericVersion(candidate)
}

func normalizeDateVersion(candidate string) (string, bool) {
	match := datePattern.FindStringSubmatch(candidate)
	if len(match) == 0 {
		return "", false
	}

	year, _ := strconv.Atoi(match[1])
	month, _ := strconv.Atoi(match[2])
	day, _ := strconv.Atoi(match[3])

	if year < 1900 || month < 1 || month > 12 || day < 1 || day > 31 {
		return "", false
	}

	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if date.Year() != year || int(date.Month()) != month || date.Day() != day {
		return "", false
	}

	suffix := normalizeSuffix(match[4])
	if suffix == "" {
		return date.Format("2006.01.02"), true
	}

	return date.Format("2006.01.02") + suffix, true
}

func normalizeNumericVersion(candidate string) (string, bool) {
	match := numericPattern.FindStringSubmatch(candidate)
	if len(match) == 0 {
		return "", false
	}

	core := strings.FieldsFunc(match[1], func(r rune) bool {
		return r == '.' || r == '-' || r == '_'
	})
	if len(core) < 2 {
		return "", false
	}

	for i, part := range core {
		core[i] = trimLeadingZeroes(part)
	}

	suffix := normalizeSuffix(match[2])
	return strings.Join(core, ".") + suffix, true
}

func normalizeSuffix(suffix string) string {
	suffix = trimVersionBoundary(suffix)
	suffix = trimKnownFileExtension(suffix)
	if suffix == "" {
		return ""
	}

	parts := splitSuffixParts(suffix)
	if len(parts) == 0 {
		return ""
	}

	preRelease := make([]string, 0, len(parts))
	build := make([]string, 0)
	inBuild := false

	for _, part := range parts {
		part = trimVersionBoundary(part)
		if part == "" {
			continue
		}

		lower := strings.ToLower(part)
		if isKnownVersionNoise(lower) {
			break
		}

		if lower == "build" || lower == "bld" {
			inBuild = true
			continue
		}

		identifier := normalizeIdentifier(part)
		if inBuild {
			build = append(build, identifier)
			continue
		}

		preRelease = append(preRelease, identifier)
	}

	for len(preRelease) > 0 && isNumeric(preRelease[0]) {
		preRelease = preRelease[1:]
	}

	if len(preRelease) == 0 && len(build) == 0 {
		return ""
	}

	result := ""
	if len(preRelease) > 0 {
		result += "-" + strings.Join(preRelease, ".")
	}
	if len(build) > 0 {
		result += "+" + strings.Join(build, ".")
	}

	return result
}

func splitSuffixParts(suffix string) []string {
	suffix = strings.TrimLeft(suffix, ".-_+~")
	if suffix == "" {
		return nil
	}

	return strings.FieldsFunc(suffix, func(r rune) bool {
		return r == '.' || r == '-' || r == '_' || r == '+' || r == '~'
	})
}

func normalizeIdentifier(identifier string) string {
	identifier = trimVersionBoundary(identifier)
	if isNumeric(identifier) {
		return trimLeadingZeroes(identifier)
	}

	return strings.ToLower(identifier)
}

func trimVersionBoundary(value string) string {
	value = strings.TrimSpace(value)
	return strings.Trim(value, " \t\r\n\"'()[]{}<>")
}

func trimLeadingZeroes(value string) string {
	value = strings.TrimLeft(value, "0")
	if value == "" {
		return "0"
	}

	return value
}

func trimKnownFileExtension(value string) string {
	lower := strings.ToLower(value)

	for _, extension := range []string{
		".appimage.zsync",
		".appimage",
		".desktop",
		".tar.gz",
		".tar.xz",
		".tar.bz2",
		".tgz",
		".zip",
		".gz",
		".xz",
		".bz2",
		".deb",
		".rpm",
		".exe",
		".dmg",
	} {
		if strings.HasSuffix(lower, extension) {
			return value[:len(value)-len(extension)]
		}
	}

	return value
}

func versionRank(version string) int {
	core := version
	if index := strings.IndexAny(core, "-+"); index >= 0 {
		core = core[:index]
	}

	if core == "" {
		return 0
	}

	return strings.Count(core, ".") + 1
}

type comparableParsedVersion struct {
	core       []int
	prerelease []string
}

func comparableVersion(version string) comparableParsedVersion {
	version = strings.TrimSpace(version)
	if buildIndex := strings.Index(version, "+"); buildIndex >= 0 {
		version = version[:buildIndex]
	}

	core := version
	prerelease := ""
	if prereleaseIndex := strings.Index(version, "-"); prereleaseIndex >= 0 {
		core = version[:prereleaseIndex]
		prerelease = version[prereleaseIndex+1:]
	}

	return comparableParsedVersion{
		core:       comparableCore(core),
		prerelease: comparablePrerelease(prerelease),
	}
}

func comparableCore(core string) []int {
	if core == "" {
		return nil
	}

	parts := strings.Split(core, ".")
	result := make([]int, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			result = append(result, 0)
			continue
		}

		value, err := strconv.Atoi(part)
		if err != nil {
			result = append(result, 0)
			continue
		}

		result = append(result, value)
	}

	return result
}

func comparablePrerelease(prerelease string) []string {
	if prerelease == "" {
		return nil
	}

	parts := strings.Split(prerelease, ".")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			result = append(result, part)
		}
	}

	return result
}

func corePart(core []int, index int) int {
	if index >= len(core) {
		return 0
	}

	return core[index]
}

func comparePrerelease(left, right []string) int {
	if len(left) == 0 && len(right) == 0 {
		return 0
	}
	if len(left) == 0 {
		return 1
	}
	if len(right) == 0 {
		return -1
	}

	maxLen := len(left)
	if len(right) > maxLen {
		maxLen = len(right)
	}

	for i := 0; i < maxLen; i++ {
		if i >= len(left) {
			return -1
		}
		if i >= len(right) {
			return 1
		}

		if result := comparePrereleaseIdentifier(left[i], right[i]); result != 0 {
			return result
		}
	}

	return 0
}

func comparePrereleaseIdentifier(left, right string) int {
	leftNumeric := isNumeric(left)
	rightNumeric := isNumeric(right)

	if leftNumeric && rightNumeric {
		leftValue, _ := strconv.Atoi(left)
		rightValue, _ := strconv.Atoi(right)
		return compareInt(leftValue, rightValue)
	}
	if leftNumeric {
		return -1
	}
	if rightNumeric {
		return 1
	}

	leftLabel, leftNumber, leftHasNumber := splitPrereleaseIdentifier(left)
	rightLabel, rightNumber, rightHasNumber := splitPrereleaseIdentifier(right)

	if result := compareInt(prereleaseLabelRank(leftLabel), prereleaseLabelRank(rightLabel)); result != 0 {
		return result
	}
	if leftLabel != rightLabel {
		return strings.Compare(leftLabel, rightLabel)
	}
	if leftHasNumber && rightHasNumber {
		return compareInt(leftNumber, rightNumber)
	}
	if leftHasNumber {
		return 1
	}
	if rightHasNumber {
		return -1
	}

	return strings.Compare(left, right)
}

func splitPrereleaseIdentifier(identifier string) (string, int, bool) {
	index := len(identifier)
	for index > 0 {
		r := rune(identifier[index-1])
		if r < '0' || r > '9' {
			break
		}
		index--
	}

	label := identifier[:index]
	if index == len(identifier) {
		return label, 0, false
	}

	number, err := strconv.Atoi(identifier[index:])
	if err != nil {
		return label, 0, false
	}

	return label, number, true
}

func prereleaseLabelRank(label string) int {
	switch label {
	case "dev", "snapshot", "nightly":
		return 10
	case "a", "alpha":
		return 20
	case "b", "beta":
		return 30
	case "pre", "preview":
		return 40
	case "c", "candidate", "rc":
		return 50
	default:
		return 100
	}
}

func compareInt(left, right int) int {
	if left > right {
		return 1
	}
	if left < right {
		return -1
	}
	return 0
}

func containsDigit(value string) bool {
	for _, r := range value {
		if unicode.IsDigit(r) {
			return true
		}
	}

	return false
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}

	for _, r := range value {
		if !unicode.IsDigit(r) {
			return false
		}
	}

	return true
}

func isKnownVersionNoise(value string) bool {
	switch value {
	case "appimage",
		"linux",
		"unix",
		"windows",
		"win",
		"mac",
		"macos",
		"osx",
		"darwin",
		"freebsd",
		"netbsd",
		"openbsd",
		"amd64",
		"x64",
		"x86",
		"x86_64",
		"i386",
		"i686",
		"arm",
		"arm64",
		"aarch64",
		"riscv64",
		"portable",
		"installer",
		"setup",
		"release",
		"snapshot",
		"stable",
		"latest",
		"desktop",
		"universal",
		"glibc",
		"musl":
		return true
	default:
		return false
	}
}
