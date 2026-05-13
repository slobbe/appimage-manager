package discovery

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/slobbe/appimage-manager/internal/infra/github"
)

type GitHubBackend struct{}

var resolveGitHubReleaseAssetSelectionFn = github.ResolveReleaseAssetSelection
var fetchGitHubRepositoryFn = github.FetchRepository

func SetHTTPClientTimeout(timeout time.Duration) {
	github.SetRepositoryHTTPClientTimeout(timeout)
}

func (GitHubBackend) Name() string {
	return "GitHub"
}

func (GitHubBackend) Resolve(ctx context.Context, ref PackageRef, assetOverride string) (*PackageMetadata, error) {
	if ref.Kind != ProviderGitHub {
		return nil, fmt.Errorf("invalid github package ref")
	}

	repoSlug := strings.TrimSpace(ref.ProviderRef)
	assetPattern := normalizeAssetPattern(assetOverride)
	repoURL := "https://github.com/" + repoSlug

	selection, err := resolveGitHubReleaseAssetSelectionFn(repoSlug, assetPattern, "")
	if err != nil {
		return newUnavailablePackageMetadata("GitHub", ref, repoURL, assetPattern, err.Error()), nil
	}

	repoInfo, err := fetchGitHubRepositoryFn(ctx, repoSlug)
	if err != nil {
		return nil, err
	}

	release := selection.Release
	if release == nil {
		release = &github.ReleaseAsset{}
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

func discoveryAssetCandidates(candidates []github.ReleaseAssetCandidate) []AssetCandidate {
	result := make([]AssetCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, AssetCandidate{
			Name:        candidate.Name,
			DownloadURL: candidate.DownloadURL,
			Arch:        candidate.Arch,
			ArchLabel:   candidate.ArchLabel,
		})
	}
	return result
}
