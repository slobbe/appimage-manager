package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strings"

	models "github.com/slobbe/appimage-manager/internal/types"
)

type GitLabReleaseUpdate struct {
	Available         bool
	DownloadURL       string
	TagName           string
	NormalizedVersion string
	AssetName         string
}

type GitLabReleaseAsset struct {
	DownloadURL       string
	TagName           string
	NormalizedVersion string
	AssetName         string
}

type gitLabReleaseResponse struct {
	TagName         string `json:"tag_name"`
	UpcomingRelease bool   `json:"upcoming_release"`
	Assets          struct {
		Links []struct {
			Name           string `json:"name"`
			URL            string `json:"url"`
			DirectAssetURL string `json:"direct_asset_url"`
		} `json:"links"`
	} `json:"assets"`
}

var gitLabReleaseAPIBaseURL = "https://gitlab.com/api/v4"
var gitLabReleaseHTTPClient = sharedHTTPClient

func GitLabReleaseUpdateCheck(update *models.UpdateSource, currentVersion string) (*GitLabReleaseUpdate, error) {
	if update == nil || update.Kind != models.UpdateGitLabRelease || update.GitLabRelease == nil {
		return nil, fmt.Errorf("invalid gitlab release update source")
	}

	release, err := ResolveGitLabReleaseAsset(update.GitLabRelease.Project, update.GitLabRelease.Asset)
	if err != nil {
		return nil, err
	}

	latest := normalizeVersion(release.TagName)
	current := normalizeVersion(currentVersion)
	available := latest != "" && latest != current

	return &GitLabReleaseUpdate{
		Available:         available,
		DownloadURL:       release.DownloadURL,
		TagName:           release.TagName,
		NormalizedVersion: latest,
		AssetName:         release.AssetName,
	}, nil
}

func ResolveGitLabReleaseAsset(project, assetPattern string) (*GitLabReleaseAsset, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		return nil, fmt.Errorf("invalid gitlab project")
	}

	assetPattern = strings.TrimSpace(assetPattern)
	if assetPattern == "" {
		return nil, fmt.Errorf("missing gitlab asset pattern")
	}

	payload, err := fetchGitLabReleases(project)
	if err != nil {
		return nil, err
	}

	release, ok := selectGitLabRelease(payload)
	if !ok {
		return nil, fmt.Errorf("no matching gitlab releases found")
	}

	assets := gitLabAssetsAsReleaseAssets(release.Assets.Links)
	assetName, downloadURL := matchAsset(assets, assetPattern, runtime.GOARCH)
	if downloadURL == "" {
		return nil, fmt.Errorf("no assets match pattern %q", assetPattern)
	}

	return &GitLabReleaseAsset{
		DownloadURL:       downloadURL,
		TagName:           release.TagName,
		NormalizedVersion: normalizeVersion(release.TagName),
		AssetName:         assetName,
	}, nil
}

func fetchGitLabReleases(project string) ([]gitLabReleaseResponse, error) {
	projectEscaped := url.PathEscape(project)
	requestURL := fmt.Sprintf("%s/projects/%s/releases", strings.TrimRight(gitLabReleaseAPIBaseURL, "/"), projectEscaped)

	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := gitLabReleaseHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("gitlab api returned status %s", resp.Status)
	}

	var payload []gitLabReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	return payload, nil
}

func selectGitLabRelease(releases []gitLabReleaseResponse) (gitLabReleaseResponse, bool) {
	for _, release := range releases {
		if release.UpcomingRelease {
			continue
		}
		if strings.TrimSpace(release.TagName) == "" {
			continue
		}
		return release, true
	}

	return gitLabReleaseResponse{}, false
}

func gitLabAssetsAsReleaseAssets(links []struct {
	Name           string `json:"name"`
	URL            string `json:"url"`
	DirectAssetURL string `json:"direct_asset_url"`
}) []releaseAsset {
	assets := make([]releaseAsset, 0, len(links))
	for _, link := range links {
		downloadURL := strings.TrimSpace(link.DirectAssetURL)
		if downloadURL == "" {
			downloadURL = strings.TrimSpace(link.URL)
		}
		if downloadURL == "" {
			continue
		}
		assets = append(assets, releaseAsset{
			Name:               strings.TrimSpace(link.Name),
			BrowserDownloadURL: downloadURL,
		})
	}

	return assets
}
