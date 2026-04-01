package core

import (
	"path"
	"strings"
)

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type assetMatch struct {
	name string
	url  string
}

type releaseTransportDetails struct {
	Transport    string
	ZsyncURL     string
	ExpectedSHA1 string
}

func normalizeVersion(version string) string {
	return normalizeComparableVersion(version)
}

func releaseAvailability(currentVersion, tagName string) (string, bool) {
	latest := normalizeVersion(tagName)
	current := normalizeVersion(currentVersion)
	return latest, latest != "" && latest != current
}

func resolveReleaseTransport(downloadURL, localSHA1 string) releaseTransportDetails {
	transport, err := probeReleaseZsyncTransport(downloadURL, localSHA1)
	if err != nil || transport == nil {
		return releaseTransportDetails{}
	}
	return releaseTransportDetails{
		Transport:    transport.Transport,
		ZsyncURL:     transport.ZsyncURL,
		ExpectedSHA1: transport.ExpectedSHA1,
	}
}

func matchAsset(assets []releaseAsset, pattern, arch string) (string, string) {
	var matches []assetMatch
	for _, asset := range assets {
		ok, err := path.Match(pattern, asset.Name)
		if err == nil && ok {
			matches = append(matches, assetMatch{name: asset.Name, url: asset.BrowserDownloadURL})
		}
	}

	if len(matches) == 0 {
		return "", ""
	}

	best := selectBestAsset(matches, arch)
	return best.name, best.url
}

func selectBestAsset(matches []assetMatch, arch string) assetMatch {
	arch = strings.ToLower(strings.TrimSpace(arch))
	archTokens := archAliases(arch)
	allTokens := allArchTokens()

	var archHits []assetMatch
	var noArch []assetMatch

	for _, match := range matches {
		nameLower := strings.ToLower(match.name)
		hasAnyArch := containsAny(nameLower, allTokens)
		if containsAny(nameLower, archTokens) {
			archHits = append(archHits, match)
			continue
		}
		if !hasAnyArch {
			noArch = append(noArch, match)
		}
	}

	if arch == "arm64" || arch == "amd64" {
		if len(archHits) > 0 {
			return archHits[0]
		}
		if len(noArch) > 0 {
			return noArch[0]
		}
		return matches[0]
	}

	if len(archHits) > 0 {
		return archHits[0]
	}
	if len(noArch) > 0 {
		return noArch[0]
	}
	return matches[0]
}

func archAliases(arch string) []string {
	switch arch {
	case "amd64":
		return []string{"amd64", "x86_64", "x64"}
	case "arm64":
		return []string{"arm64", "aarch64"}
	default:
		return []string{arch}
	}
}

func allArchTokens() []string {
	return []string{"amd64", "x86_64", "x64", "arm64", "aarch64"}
}

func containsAny(s string, tokens []string) bool {
	for _, token := range tokens {
		if strings.Contains(s, token) {
			return true
		}
	}
	return false
}
