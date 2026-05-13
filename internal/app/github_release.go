package app

import (
	"fmt"

	models "github.com/slobbe/appimage-manager/internal/domain"
	"github.com/slobbe/appimage-manager/internal/infra/github"
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

type GitHubReleaseAsset = github.ReleaseAsset
type GitHubReleaseAssetCandidate = github.ReleaseAssetCandidate
type GitHubReleaseAssetSelection = github.ReleaseAssetSelection

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
	return github.ResolveReleaseAsset(repoSlug, assetPattern)
}

func ResolveGitHubReleaseAssetSelection(repoSlug, assetPattern, arch string) (*GitHubReleaseAssetSelection, error) {
	return github.ResolveReleaseAssetSelection(repoSlug, assetPattern, arch)
}
