package discovery

import "strings"

type resolvedReleaseMetadata struct {
	DownloadURL       string
	TagName           string
	NormalizedVersion string
	AssetName         string
}

func newUnavailablePackageMetadata(provider string, ref PackageRef, repoURL, assetPattern, reason string) *PackageMetadata {
	return &PackageMetadata{
		Provider:      strings.TrimSpace(provider),
		Ref:           ref,
		RepoURL:       strings.TrimSpace(repoURL),
		AssetPattern:  normalizeAssetPattern(assetPattern),
		Installable:   false,
		InstallReason: strings.TrimSpace(reason),
	}
}

func newInstallablePackageMetadata(provider string, ref PackageRef, repoURL, name, summary, assetPattern string, release resolvedReleaseMetadata) *PackageMetadata {
	return &PackageMetadata{
		Name:          firstNonEmpty(name, DisplayNameFromRef(ref.ProviderRef)),
		Provider:      strings.TrimSpace(provider),
		Ref:           ref,
		RepoURL:       strings.TrimSpace(repoURL),
		LatestVersion: versionForDisplay(release.NormalizedVersion, release.TagName),
		AssetName:     strings.TrimSpace(release.AssetName),
		AssetPattern:  normalizeAssetPattern(assetPattern),
		DownloadURL:   strings.TrimSpace(release.DownloadURL),
		Installable:   true,
		ReleaseTag:    strings.TrimSpace(release.TagName),
		Summary:       strings.TrimSpace(summary),
	}
}
