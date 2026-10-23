package app

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode"
)

func selectGitHubAppImageAssetMatchingPattern(release GitHubRelease, pattern string) (GitHubReleaseAsset, error) {
	return selectGitHubAppImageAssetWithSelection(release, runtime.GOARCH, githubAssetSelection{pattern: pattern})
}

func selectGitHubAppImageAssetForArch(release GitHubRelease, goarch string) (GitHubReleaseAsset, error) {
	return selectGitHubAppImageAssetWithSelection(release, goarch, githubAssetSelection{})
}

type githubAssetSelection struct {
	pattern         string
	excludePatterns []string
	preferPatterns  []string
}

func validateGitHubAssetSelection(selection githubAssetSelection) error {
	if pattern := strings.TrimSpace(selection.pattern); pattern != "" {
		if err := validateGitHubAssetPattern(pattern); err != nil {
			return fmt.Errorf("invalid GitHub asset pattern %q: %w", pattern, err)
		}
	}
	for _, group := range []struct {
		name     string
		patterns []string
	}{
		{name: "exclude", patterns: selection.excludePatterns},
		{name: "prefer", patterns: selection.preferPatterns},
	} {
		for _, pattern := range group.patterns {
			pattern = strings.TrimSpace(pattern)
			if pattern == "" {
				return fmt.Errorf("GitHub asset %s pattern must not be blank", group.name)
			}
			if err := validateGitHubAssetPattern(pattern); err != nil {
				return fmt.Errorf("invalid GitHub asset %s pattern %q: %w", group.name, pattern, err)
			}
		}
	}
	return nil
}

func validateGitHubAssetPattern(pattern string) error {
	// Match(pattern, "") can return before checking later chunks (e.g. "x*[").
	// Mirror filepath.scanChunk's escape/class boundaries and let Match validate
	// each chunk; matchChunk checks its entire chunk even after a mismatch.
	start := 0
	inClass := false
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			if filepath.Separator != '\\' {
				i++
			}
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '*':
			if !inClass {
				if _, err := filepath.Match(pattern[start:i], ""); err != nil {
					return err
				}
				start = i + 1
			}
		}
	}
	_, err := filepath.Match(pattern[start:], "")
	return err
}

func selectGitHubAppImageAssetWithSelection(release GitHubRelease, goarch string, selection githubAssetSelection) (GitHubReleaseAsset, error) {
	if err := validateGitHubAssetSelection(selection); err != nil {
		return GitHubReleaseAsset{}, err
	}

	pattern := strings.TrimSpace(selection.pattern)
	candidates := make([]GitHubReleaseAsset, 0, len(release.Assets))
	for _, asset := range release.Assets {
		if !isAppImageAssetName(asset.Name) {
			continue
		}
		if pattern != "" && !matchesGitHubAssetPatterns(asset.Name, []string{pattern}) {
			continue
		}
		candidates = append(candidates, asset)
	}
	if len(candidates) == 0 {
		if pattern != "" {
			return GitHubReleaseAsset{}, fmt.Errorf("release %s has no AppImage assets matching pattern %q", releaseLabel(release), pattern)
		}
		return GitHubReleaseAsset{}, fmt.Errorf("release %s has no AppImage assets", releaseLabel(release))
	}

	remaining := make([]GitHubReleaseAsset, 0, len(candidates))
	for _, asset := range candidates {
		if !matchesGitHubAssetPatterns(asset.Name, selection.excludePatterns) {
			remaining = append(remaining, asset)
		}
	}
	candidates = remaining
	if len(candidates) == 0 {
		return GitHubReleaseAsset{}, fmt.Errorf("release %s has no AppImage assets remaining after exclusions", releaseLabel(release))
	}

	if pattern == "" {
		bestScore := scoreAppImageAsset(candidates[0].Name, goarch)
		remaining = nil
		for _, asset := range candidates {
			score := scoreAppImageAsset(asset.Name, goarch)
			if score > bestScore {
				bestScore = score
				remaining = nil
			}
			if score == bestScore {
				remaining = append(remaining, asset)
			}
		}
		candidates = remaining
	}

	preferred := make([]GitHubReleaseAsset, 0, len(candidates))
	for _, asset := range candidates {
		if matchesGitHubAssetPatterns(asset.Name, selection.preferPatterns) {
			preferred = append(preferred, asset)
		}
	}
	if len(preferred) > 0 {
		candidates = preferred
	}
	if len(candidates) > 1 {
		if pattern != "" {
			return GitHubReleaseAsset{}, fmt.Errorf("release %s has multiple AppImage assets matching pattern %q: %s", releaseLabel(release), pattern, assetNames(candidates))
		}
		return GitHubReleaseAsset{}, fmt.Errorf("release %s has multiple matching AppImage assets for architecture %q: %s", releaseLabel(release), goarch, assetNames(candidates))
	}
	return candidates[0], nil
}

func matchesGitHubAssetPatterns(name string, patterns []string) bool {
	for _, pattern := range patterns {
		// Patterns are validated before inspecting any assets.
		matched, _ := filepath.Match(strings.TrimSpace(pattern), name)
		if matched {
			return true
		}
	}
	return false
}

func isAppImageAssetName(name string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(name)), ".appimage")
}

func scoreAppImageAsset(name string, goarch string) int {
	score := 1000
	arch := architectureFromGOARCH(goarch)
	labels := architectureLabelsInName(name)

	if labels[arch] {
		score += 500
	}
	for label := range labels {
		if label != arch {
			score -= 1000
		}
	}

	if containsNameToken(name, "linux") {
		score += 100
	}

	return score
}

type architecture string

const (
	architectureUnknown architecture = "unknown"
	architectureAMD64   architecture = "amd64"
	architectureARM64   architecture = "arm64"
	architectureARM     architecture = "arm"
	architecture386     architecture = "386"
	architectureRISCV64 architecture = "riscv64"
	architecturePPC64LE architecture = "ppc64le"
	architecturePPC64   architecture = "ppc64"
	architectureS390X   architecture = "s390x"
)

func architectureFromGOARCH(goarch string) architecture {
	switch strings.ToLower(strings.TrimSpace(goarch)) {
	case "amd64":
		return architectureAMD64
	case "arm64":
		return architectureARM64
	case "arm":
		return architectureARM
	case "386":
		return architecture386
	case "riscv64":
		return architectureRISCV64
	case "ppc64le":
		return architecturePPC64LE
	case "ppc64":
		return architecturePPC64
	case "s390x":
		return architectureS390X
	default:
		return architectureUnknown
	}
}

func architectureLabelsInName(name string) map[architecture]bool {
	labels := make(map[architecture]bool)
	for arch, aliases := range architectureAliases() {
		for _, alias := range aliases {
			if nameContainsArchitectureAlias(name, alias) {
				labels[arch] = true
				break
			}
		}
	}

	return labels
}

func architectureAliases() map[architecture][]string {
	return map[architecture][]string{
		architectureAMD64: {
			"amd64",
			"x86_64",
			"x86-64",
			"x64",
			"64bit",
			"64-bit",
		},
		architectureARM64: {
			"arm64",
			"aarch64",
			"armv8",
			"arm-v8",
		},
		architectureARM: {
			"arm",
			"arm32",
			"armhf",
			"armv6",
			"arm-v6",
			"armv7",
			"arm-v7",
			"armv7l",
		},
		architecture386: {
			"386",
			"i386",
			"i486",
			"i586",
			"i686",
			"x86",
			"ia32",
			"32bit",
			"32-bit",
		},
		architectureRISCV64: {
			"riscv64",
			"risc-v64",
			"riscv-64",
		},
		architecturePPC64LE: {
			"ppc64le",
			"powerpc64le",
			"powerpc-64le",
		},
		architecturePPC64: {
			"ppc64",
			"powerpc64",
			"powerpc-64",
		},
		architectureS390X: {
			"s390x",
		},
	}
}

func nameContainsArchitectureAlias(name string, alias string) bool {
	nameTokens := tokenizeName(name)
	aliasTokens := tokenizeName(alias)
	if len(aliasTokens) == 0 {
		return false
	}
	if len(aliasTokens) == 1 {
		for i, token := range nameTokens {
			if token != aliasTokens[0] {
				continue
			}
			if aliasTokens[0] == "x86" && adjacentTokenIs64BitMarker(nameTokens, i) {
				continue
			}

			return true
		}

		return false
	}

	for i := 0; i+len(aliasTokens) <= len(nameTokens); i++ {
		matched := true
		for j := range aliasTokens {
			if nameTokens[i+j] != aliasTokens[j] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}

	return strings.Contains(compactName(name), compactName(alias))
}

func adjacentTokenIs64BitMarker(tokens []string, index int) bool {
	return (index > 0 && tokens[index-1] == "64") || (index+1 < len(tokens) && tokens[index+1] == "64")
}

func containsNameToken(name string, token string) bool {
	for _, nameToken := range tokenizeName(name) {
		if nameToken == token {
			return true
		}
	}

	return false
}

func tokenizeName(name string) []string {
	var tokens []string
	var current strings.Builder
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
			continue
		}
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

func compactName(name string) string {
	var compact strings.Builder
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			compact.WriteRune(r)
		}
	}

	return compact.String()
}

func releaseLabel(release GitHubRelease) string {
	if release.Repo != "" && release.TagName != "" {
		return fmt.Sprintf("%s@%s", release.Repo, release.TagName)
	}
	if release.TagName != "" {
		return release.TagName
	}
	if release.Repo != "" {
		return release.Repo
	}

	return "<unknown>"
}

func assetNames(assets []GitHubReleaseAsset) string {
	names := make([]string, 0, len(assets))
	for _, asset := range assets {
		names = append(names, asset.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
