package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

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

var githubDiscoveryHTTPClient = core.NewHTTPClient(coreHTTPTimeout)
var resolveGitHubReleaseAssetFn = core.ResolveGitHubReleaseAsset

func SetHTTPClientTimeout(timeout time.Duration) {
	if githubDiscoveryHTTPClient == nil {
		githubDiscoveryHTTPClient = core.NewHTTPClient(timeout)
	} else {
		githubDiscoveryHTTPClient.Timeout = timeout
	}
}

func (GitHubBackend) Name() string {
	return "GitHub"
}

func (GitHubBackend) Resolve(ctx context.Context, ref PackageRef, assetOverride string) (*PackageMetadata, error) {
	if ref.Kind != ProviderGitHub {
		return nil, fmt.Errorf("invalid github package ref")
	}

	repoSlug := strings.TrimSpace(ref.ProviderRef)
	assetPattern := normalizeAssetPattern(assetOverride)
	repoURL := "https://github.com/" + repoSlug

	release, err := resolveGitHubReleaseAssetFn(repoSlug, assetPattern)
	if err != nil {
		return newUnavailablePackageMetadata("GitHub", ref, repoURL, assetPattern, err.Error()), nil
	}

	repoInfo, err := fetchGitHubRepo(ctx, repoSlug)
	if err != nil {
		return nil, err
	}

	return newInstallablePackageMetadata(
		"GitHub",
		ref,
		firstNonEmpty(strings.TrimSpace(repoInfo.HTMLURL), repoURL),
		strings.TrimSpace(repoInfo.Name),
		strings.TrimSpace(repoInfo.Description),
		assetPattern,
		resolvedReleaseMetadata{
			DownloadURL:       release.DownloadURL,
			TagName:           release.TagName,
			NormalizedVersion: release.NormalizedVersion,
			AssetName:         release.AssetName,
		},
	), nil
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
