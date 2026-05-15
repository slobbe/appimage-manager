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

	latest, available := models.ReleaseAvailability(currentVersion, release.TagName)

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
