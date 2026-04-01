package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"

	models "github.com/slobbe/appimage-manager/internal/types"
)

type GitHubReleaseUpdate struct {
	Available         bool
	DownloadUrl       string
	TagName           string
	NormalizedVersion string
	AssetName         string
	PreRelease        bool
	Transport         string
	ZsyncURL          string
	ExpectedSHA1      string
}

type GitHubReleaseAsset struct {
	DownloadURL       string
	TagName           string
	NormalizedVersion string
	AssetName         string
	PreRelease        bool
}

type gitHubReleaseResponse struct {
	TagName    string         `json:"tag_name"`
	Prerelease bool           `json:"prerelease"`
	Draft      bool           `json:"draft"`
	Assets     []releaseAsset `json:"assets"`
}

var githubReleaseHTTPClient = sharedHTTPClient

func GitHubReleaseUpdateCheck(update *models.UpdateSource, currentVersion, localSHA1 string) (*GitHubReleaseUpdate, error) {
	if update == nil || update.Kind != models.UpdateGitHubRelease || update.GitHubRelease == nil {
		return nil, fmt.Errorf("invalid github release update source")
	}

	release, err := ResolveGitHubReleaseAsset(update.GitHubRelease.Repo, update.GitHubRelease.Asset)
	if err != nil {
		return nil, err
	}

	latest, available := releaseAvailability(currentVersion, release.TagName)

	result := &GitHubReleaseUpdate{
		Available:         available,
		DownloadUrl:       release.DownloadURL,
		TagName:           release.TagName,
		NormalizedVersion: latest,
		AssetName:         release.AssetName,
		PreRelease:        release.PreRelease,
	}

	if !available {
		return result, nil
	}

	transport := resolveReleaseTransport(release.DownloadURL, localSHA1)
	result.Transport = transport.Transport
	result.ZsyncURL = transport.ZsyncURL
	result.ExpectedSHA1 = transport.ExpectedSHA1

	return result, nil
}

func ResolveGitHubReleaseAsset(repoSlug, assetPattern string) (*GitHubReleaseAsset, error) {
	repoSlug = strings.TrimSpace(repoSlug)
	if repoSlug == "" || strings.Count(repoSlug, "/") != 1 {
		return nil, fmt.Errorf("invalid github repo %q", repoSlug)
	}

	assetPattern = strings.TrimSpace(assetPattern)
	if assetPattern == "" {
		return nil, fmt.Errorf("missing github asset pattern")
	}

	payload, err := fetchGitHubReleases(repoSlug)
	if err != nil {
		return nil, err
	}

	release, ok := selectRelease(payload, false)
	if !ok {
		return nil, fmt.Errorf("no matching github releases found")
	}

	assetName, downloadURL := matchAsset(release.Assets, assetPattern, runtime.GOARCH)
	if downloadURL == "" {
		return nil, fmt.Errorf("no assets match pattern %q", assetPattern)
	}

	return &GitHubReleaseAsset{
		DownloadURL:       downloadURL,
		TagName:           release.TagName,
		NormalizedVersion: normalizeVersion(release.TagName),
		AssetName:         assetName,
		PreRelease:        release.Prerelease,
	}, nil
}

func fetchGitHubReleases(repoSlug string) ([]gitHubReleaseResponse, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases", repoSlug)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := githubReleaseHTTPClient.Do(req)
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

	return payload, nil
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
