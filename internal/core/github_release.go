package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"runtime"
	"strings"

	models "github.com/slobbe/appimage-manager/internal/types"
)

type GitHubReleaseUpdate struct {
	Available   bool
	DownloadUrl string
	TagName     string
	AssetName   string
	PreRelease  bool
}

type gitHubReleaseResponse struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
	Assets     []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func GitHubReleaseUpdateCheck(update *models.UpdateSource, currentVersion string) (*GitHubReleaseUpdate, error) {
	if update == nil || update.Kind != models.UpdateGitHubRelease || update.GitHubRelease == nil {
		return nil, fmt.Errorf("invalid github release update source")
	}

	repoSlug := strings.TrimSpace(update.GitHubRelease.Repo)
	if repoSlug == "" || strings.Count(repoSlug, "/") != 1 {
		return nil, fmt.Errorf("invalid github repo %q", repoSlug)
	}

	assetPattern := strings.TrimSpace(update.GitHubRelease.Asset)
	if assetPattern == "" {
		return nil, fmt.Errorf("missing github asset pattern")
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/releases", repoSlug)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("github api returned status %s", resp.Status)
	}

	var payload []gitHubReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	release, ok := selectRelease(payload, releaseAllowsPrerelease(update.GitHubRelease.ReleaseKind))
	if !ok {
		return nil, fmt.Errorf("no matching github releases found")
	}

	assetName, downloadURL := matchAsset(release.Assets, assetPattern, runtime.GOARCH)
	if downloadURL == "" {
		return nil, fmt.Errorf("no assets match pattern %q", assetPattern)
	}

	latest := normalizeVersion(release.TagName)
	current := normalizeVersion(currentVersion)
	available := latest != "" && latest != current

	return &GitHubReleaseUpdate{
		Available:   available,
		DownloadUrl: downloadURL,
		TagName:     release.TagName,
		AssetName:   assetName,
		PreRelease:  release.Prerelease,
	}, nil
}

type assetMatch struct {
	name string
	url  string
}

func matchAsset(assets []struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}, pattern, arch string) (string, string) {
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

func normalizeVersion(version string) string {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "v")
	return version
}

func selectRelease(releases []gitHubReleaseResponse, allowPrerelease bool) (gitHubReleaseResponse, bool) {
	for _, release := range releases {
		if release.Draft {
			continue
		}
		if !allowPrerelease && release.Prerelease {
			continue
		}
		return release, true
	}
	return gitHubReleaseResponse{}, false
}

func releaseAllowsPrerelease(kind string) bool {
	return strings.EqualFold(strings.TrimSpace(kind), "pre")
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

	if arch == "arm64" {
		if len(archHits) > 0 {
			return archHits[0]
		}
		if len(noArch) > 0 {
			return noArch[0]
		}
		return matches[0]
	}

	if arch == "amd64" {
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
