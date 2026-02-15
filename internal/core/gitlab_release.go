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
	Available   bool
	DownloadURL string
	TagName     string
	AssetName   string
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

func GitLabReleaseUpdateCheck(update *models.UpdateSource, currentVersion string) (*GitLabReleaseUpdate, error) {
	if update == nil || update.Kind != models.UpdateGitLabRelease || update.GitLabRelease == nil {
		return nil, fmt.Errorf("invalid gitlab release update source")
	}

	project := strings.TrimSpace(update.GitLabRelease.Project)
	if project == "" {
		return nil, fmt.Errorf("invalid gitlab project")
	}

	assetPattern := strings.TrimSpace(update.GitLabRelease.Asset)
	if assetPattern == "" {
		return nil, fmt.Errorf("missing gitlab asset pattern")
	}

	projectEscaped := url.PathEscape(project)
	requestURL := fmt.Sprintf("%s/projects/%s/releases", strings.TrimRight(gitLabReleaseAPIBaseURL, "/"), projectEscaped)

	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
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

	release, ok := selectGitLabRelease(payload)
	if !ok {
		return nil, fmt.Errorf("no matching gitlab releases found")
	}

	assets := gitLabAssetsAsReleaseAssets(release.Assets.Links)
	assetName, downloadURL := matchAsset(assets, assetPattern, runtime.GOARCH)
	if downloadURL == "" {
		return nil, fmt.Errorf("no assets match pattern %q", assetPattern)
	}

	latest := normalizeVersion(release.TagName)
	current := normalizeVersion(currentVersion)
	available := latest != "" && latest != current

	return &GitLabReleaseUpdate{
		Available:   available,
		DownloadURL: downloadURL,
		TagName:     release.TagName,
		AssetName:   assetName,
	}, nil
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
