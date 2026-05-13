package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"

	models "github.com/slobbe/appimage-manager/internal/domain"
)

type ReleaseAsset struct {
	DownloadURL       string
	TagName           string
	NormalizedVersion string
	AssetName         string
	PreRelease        bool
}

type ReleaseAssetCandidate struct {
	Name        string
	DownloadURL string
	Arch        string
	ArchLabel   string
}

type ReleaseAssetSelection struct {
	Release    *ReleaseAsset
	Candidates []ReleaseAssetCandidate
	Ambiguous  bool
	Reason     string
}

type releaseResponse struct {
	TagName    string     `json:"tag_name"`
	Prerelease bool       `json:"prerelease"`
	Draft      bool       `json:"draft"`
	Assets     []apiAsset `json:"assets"`
}

func (c Client) ResolveReleaseAsset(repoSlug, assetPattern string) (*ReleaseAsset, error) {
	selection, err := c.ResolveReleaseAssetSelection(repoSlug, assetPattern, runtime.GOARCH)
	if err != nil {
		return nil, err
	}
	if selection.Ambiguous {
		return nil, fmt.Errorf("%s: %s", selection.Reason, FormatAssetCandidateNames(selection.Candidates))
	}
	return selection.Release, nil
}

func (c Client) ResolveReleaseAssetSelection(repoSlug, assetPattern, arch string) (*ReleaseAssetSelection, error) {
	repoSlug = strings.TrimSpace(repoSlug)
	if repoSlug == "" || strings.Count(repoSlug, "/") != 1 {
		return nil, fmt.Errorf("invalid github repo %q", repoSlug)
	}

	assetPattern = strings.TrimSpace(assetPattern)
	if assetPattern == "" {
		return nil, fmt.Errorf("missing github asset pattern")
	}

	payload, err := c.fetchReleases(repoSlug)
	if err != nil {
		return nil, err
	}

	release, ok := selectRelease(payload, false)
	if !ok {
		return nil, fmt.Errorf("no matching github releases found")
	}

	if strings.TrimSpace(arch) == "" {
		arch = runtime.GOARCH
	}
	assetSelection := matchAssetSelection(release.Assets, assetPattern, arch)
	if len(assetSelection.candidates) == 0 {
		return nil, fmt.Errorf("no assets match pattern %q", assetPattern)
	}
	candidates := releaseAssetCandidates(assetSelection.candidates)

	if assetSelection.ambiguous {
		return &ReleaseAssetSelection{
			Candidates: candidates,
			Ambiguous:  true,
			Reason:     assetSelection.reason,
		}, nil
	}

	selected := assetSelection.selected
	return &ReleaseAssetSelection{
		Release: &ReleaseAsset{
			DownloadURL:       selected.url,
			TagName:           release.TagName,
			NormalizedVersion: models.NormalizeComparableVersion(release.TagName),
			AssetName:         selected.name,
			PreRelease:        release.Prerelease,
		},
		Candidates: candidates,
	}, nil
}

func (c Client) fetchReleases(repoSlug string) ([]releaseResponse, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases", repoSlug)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("github api returned status %s", resp.Status)
	}

	var payload []releaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	return payload, nil
}
