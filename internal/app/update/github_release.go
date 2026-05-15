package update

import (
	"fmt"

	models "github.com/slobbe/appimage-manager/internal/domain"
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

type GitHubReleaseAssetCandidate struct {
	Name        string
	DownloadURL string
	Arch        string
	ArchLabel   string
}

type GitHubReleaseAssetSelection struct {
	Release    *GitHubReleaseAsset
	Candidates []GitHubReleaseAssetCandidate
	Ambiguous  bool
	Reason     string
}

type GitHubReleaseResolver interface {
	ResolveReleaseAsset(repoSlug, assetPattern string) (*GitHubReleaseAsset, error)
	ResolveReleaseAssetSelection(repoSlug, assetPattern, arch string) (*GitHubReleaseAssetSelection, error)
	ResolveLatestReleaseTag(owner, repo string) (string, error)
}

var defaultGitHubReleaseResolver GitHubReleaseResolver

func SetGitHubReleaseResolver(resolver GitHubReleaseResolver) {
	defaultGitHubReleaseResolver = resolver
}

func GitHubReleaseUpdateCheck(update *models.UpdateSource, currentVersion, localSHA1 string) (*GitHubReleaseUpdate, error) {
	if update == nil || update.Kind != models.UpdateGitHubRelease || update.GitHubRelease == nil {
		return nil, fmt.Errorf("invalid github release update source")
	}

	release, err := ResolveGitHubReleaseAsset(update.GitHubRelease.Repo, update.GitHubRelease.Asset)
	if err != nil {
		return nil, err
	}

	var transport models.ReleaseTransport
	if models.NewReleaseUpdate(currentVersion, release.TagName, release.DownloadURL, release.AssetName, release.PreRelease, transport).Available {
		resolved := resolveReleaseTransport(release.DownloadURL, localSHA1)
		transport = models.ReleaseTransport{
			Transport:    resolved.Transport,
			ZsyncURL:     resolved.ZsyncURL,
			ExpectedSHA1: resolved.ExpectedSHA1,
		}
	}
	decision := models.NewReleaseUpdate(currentVersion, release.TagName, release.DownloadURL, release.AssetName, release.PreRelease, transport)

	result := &GitHubReleaseUpdate{
		Available:         decision.Available,
		DownloadUrl:       release.DownloadURL,
		TagName:           release.TagName,
		NormalizedVersion: decision.LatestVersion,
		AssetName:         release.AssetName,
		PreRelease:        release.PreRelease,
		Transport:         decision.Transport,
		ZsyncURL:          decision.ZsyncURL,
		ExpectedSHA1:      decision.ExpectedSHA1,
	}

	return result, nil
}

func ResolveGitHubReleaseAsset(repoSlug, assetPattern string) (*GitHubReleaseAsset, error) {
	if defaultGitHubReleaseResolver == nil {
		return nil, fmt.Errorf("github release resolver is not configured")
	}
	return defaultGitHubReleaseResolver.ResolveReleaseAsset(repoSlug, assetPattern)
}

func ResolveGitHubReleaseAssetSelection(repoSlug, assetPattern, arch string) (*GitHubReleaseAssetSelection, error) {
	if defaultGitHubReleaseResolver == nil {
		return nil, fmt.Errorf("github release resolver is not configured")
	}
	return defaultGitHubReleaseResolver.ResolveReleaseAssetSelection(repoSlug, assetPattern, arch)
}

func ResolveLatestGitHubReleaseTag(owner, repo string) (string, error) {
	if defaultGitHubReleaseResolver == nil {
		return "", fmt.Errorf("github release resolver is not configured")
	}
	return defaultGitHubReleaseResolver.ResolveLatestReleaseTag(owner, repo)
}
