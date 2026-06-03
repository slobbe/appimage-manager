package discovery

import (
	"strings"

	"github.com/slobbe/appimage-manager/internal/domain"
)

type resolvedReleaseMetadata struct {
	DownloadURL       string
	TagName           string
	NormalizedVersion string
	AssetName         string
	AssetCandidates   []domain.AssetCandidate
	AssetAmbiguous    bool
	AssetReason       string
}

func newUnavailablePackageMetadata(provider string, ref domain.PackageRef, repoURL, assetPattern, reason string) *domain.PackageMetadata {
	return &domain.PackageMetadata{
		Provider:      strings.TrimSpace(provider),
		Ref:           ref,
		RepoURL:       strings.TrimSpace(repoURL),
		AssetPattern:  normalizeAssetPattern(assetPattern),
		Installable:   false,
		InstallReason: strings.TrimSpace(reason),
	}
}

func newInstallablePackageMetadata(provider string, ref domain.PackageRef, repoURL, name, summary, assetPattern string, release resolvedReleaseMetadata) *domain.PackageMetadata {
	return &domain.PackageMetadata{
		Name:            firstNonEmpty(name, domain.DisplayNameFromRef(ref.ProviderRef)),
		Provider:        strings.TrimSpace(provider),
		Ref:             ref,
		RepoURL:         strings.TrimSpace(repoURL),
		LatestVersion:   versionForDisplay(release.NormalizedVersion, release.TagName),
		AssetName:       strings.TrimSpace(release.AssetName),
		AssetPattern:    normalizeAssetPattern(assetPattern),
		DownloadURL:     strings.TrimSpace(release.DownloadURL),
		AssetCandidates: release.AssetCandidates,
		AssetAmbiguous:  release.AssetAmbiguous,
		AssetReason:     strings.TrimSpace(release.AssetReason),
		Installable:     true,
		ReleaseTag:      strings.TrimSpace(release.TagName),
		Summary:         strings.TrimSpace(summary),
	}
}
