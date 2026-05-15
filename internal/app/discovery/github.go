package discovery

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/slobbe/appimage-manager/internal/domain"
)

type GitHubBackend struct{}

var defaultGitHubResolver GitHubResolver

func SetHTTPClientTimeout(timeout time.Duration) {
	_ = timeout
}

func SetGitHubResolver(resolver GitHubResolver) {
	defaultGitHubResolver = resolver
}

func (GitHubBackend) Name() string {
	return "GitHub"
}

func (GitHubBackend) Resolve(ctx context.Context, ref domain.PackageRef, assetOverride string) (*domain.PackageMetadata, error) {
	if ref.Kind != domain.ProviderGitHub {
		return nil, fmt.Errorf("invalid github package ref")
	}

	repoSlug := strings.TrimSpace(ref.ProviderRef)
	assetPattern := normalizeAssetPattern(assetOverride)
	repoURL := "https://github.com/" + repoSlug

	if defaultGitHubResolver == nil {
		return nil, fmt.Errorf("github resolver is not configured")
	}

	selection, err := defaultGitHubResolver.ResolveReleaseAssetSelection(repoSlug, assetPattern, "")
	if err != nil {
		return newUnavailablePackageMetadata("GitHub", ref, repoURL, assetPattern, err.Error()), nil
	}

	repoInfo, err := defaultGitHubResolver.FetchRepository(ctx, repoSlug)
	if err != nil {
		return nil, err
	}

	release := selection.Release
	if release == nil {
		release = &ReleaseAsset{}
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
			AssetCandidates:   discoveryAssetCandidates(selection.Candidates),
			AssetAmbiguous:    selection.Ambiguous,
			AssetReason:       selection.Reason,
		},
	), nil
}

func discoveryAssetCandidates(candidates []ReleaseAssetCandidate) []domain.AssetCandidate {
	result := make([]domain.AssetCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, domain.AssetCandidate{
			Name:        candidate.Name,
			DownloadURL: candidate.DownloadURL,
			Arch:        candidate.Arch,
			ArchLabel:   candidate.ArchLabel,
		})
	}
	return result
}
