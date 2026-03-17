package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/slobbe/appimage-manager/internal/core"
)

type GitHubBackend struct{}

type gitHubRepoResponse struct {
	Name            string `json:"name"`
	FullName        string `json:"full_name"`
	Description     string `json:"description"`
	HTMLURL         string `json:"html_url"`
	StargazersCount int    `json:"stargazers_count"`
}

var githubDiscoveryHTTPClient = &http.Client{Timeout: coreHTTPTimeout}
var resolveGitHubReleaseAssetFn = core.ResolveGitHubReleaseAsset

func (GitHubBackend) Name() string {
	return "GitHub"
}

func (GitHubBackend) Resolve(ctx context.Context, ref PackageRef, assetOverride string) (*PackageMetadata, error) {
	if ref.Kind != ProviderGitHub {
		return nil, fmt.Errorf("invalid github package ref")
	}

	repoSlug := strings.TrimSpace(ref.ProviderRef)
	assetPattern := normalizeAssetPattern(assetOverride)

	release, err := resolveGitHubReleaseAssetFn(repoSlug, assetPattern)
	if err != nil {
		return &PackageMetadata{
			Provider:      "GitHub",
			Ref:           ref,
			RepoURL:       "https://github.com/" + repoSlug,
			AssetPattern:  assetPattern,
			Installable:   false,
			InstallReason: err.Error(),
		}, nil
	}

	repoInfo, err := fetchGitHubRepo(ctx, repoSlug)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(repoInfo.Name)
	if name == "" {
		name = DisplayNameFromRef(repoSlug)
	}

	return &PackageMetadata{
		Name:          name,
		Provider:      "GitHub",
		Ref:           ref,
		RepoURL:       firstNonEmpty(strings.TrimSpace(repoInfo.HTMLURL), "https://github.com/"+repoSlug),
		LatestVersion: versionForDisplay(release.NormalizedVersion, release.TagName),
		AssetName:     strings.TrimSpace(release.AssetName),
		AssetPattern:  assetPattern,
		DownloadURL:   strings.TrimSpace(release.DownloadURL),
		Installable:   true,
		ReleaseTag:    strings.TrimSpace(release.TagName),
		Summary:       strings.TrimSpace(repoInfo.Description),
	}, nil
}

func fetchGitHubRepo(ctx context.Context, repoSlug string) (*gitHubRepoResponse, error) {
	requestURL := fmt.Sprintf("https://api.github.com/repos/%s", repoSlug)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := githubDiscoveryHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("github repo api returned status %s", resp.Status)
	}

	var payload gitHubRepoResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	return &payload, nil
}
