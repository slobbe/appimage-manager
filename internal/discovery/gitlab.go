package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/slobbe/appimage-manager/internal/core"
)

type GitLabBackend struct{}

type gitLabProjectResponse struct {
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
	Description       string `json:"description"`
	WebURL            string `json:"web_url"`
	StarCount         int    `json:"star_count"`
}

var gitLabDiscoveryHTTPClient = core.NewHTTPClient(coreHTTPTimeout)
var gitLabDiscoveryAPIBaseURL = "https://gitlab.com/api/v4"
var resolveGitLabReleaseAssetFn = core.ResolveGitLabReleaseAsset

func (GitLabBackend) Name() string {
	return "GitLab"
}

func (GitLabBackend) Resolve(ctx context.Context, ref PackageRef, assetOverride string) (*PackageMetadata, error) {
	if ref.Kind != ProviderGitLab {
		return nil, fmt.Errorf("invalid gitlab package ref")
	}

	project := strings.TrimSpace(ref.ProviderRef)
	assetPattern := normalizeAssetPattern(assetOverride)

	release, err := resolveGitLabReleaseAssetFn(project, assetPattern)
	if err != nil {
		return &PackageMetadata{
			Provider:      "GitLab",
			Ref:           ref,
			RepoURL:       "https://gitlab.com/" + project,
			AssetPattern:  assetPattern,
			Installable:   false,
			InstallReason: err.Error(),
		}, nil
	}

	projectInfo, err := fetchGitLabProject(ctx, project)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(projectInfo.Name)
	if name == "" {
		name = DisplayNameFromRef(project)
	}

	return &PackageMetadata{
		Name:          name,
		Provider:      "GitLab",
		Ref:           ref,
		RepoURL:       firstNonEmpty(strings.TrimSpace(projectInfo.WebURL), "https://gitlab.com/"+project),
		LatestVersion: versionForDisplay(release.NormalizedVersion, release.TagName),
		AssetName:     strings.TrimSpace(release.AssetName),
		AssetPattern:  assetPattern,
		DownloadURL:   strings.TrimSpace(release.DownloadURL),
		Installable:   true,
		ReleaseTag:    strings.TrimSpace(release.TagName),
		Summary:       strings.TrimSpace(projectInfo.Description),
	}, nil
}

func fetchGitLabProject(ctx context.Context, project string) (*gitLabProjectResponse, error) {
	requestURL := fmt.Sprintf("%s/projects/%s", strings.TrimRight(gitLabDiscoveryAPIBaseURL, "/"), url.PathEscape(project))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := gitLabDiscoveryHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("gitlab project api returned status %s", resp.Status)
	}

	var payload gitLabProjectResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	return &payload, nil
}
