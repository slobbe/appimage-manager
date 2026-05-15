package discovery

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/slobbe/appimage-manager/internal/domain"
	"github.com/slobbe/appimage-manager/internal/infra/github"
	"github.com/slobbe/appimage-manager/internal/infra/httpclient"
)

type GitHubBackend struct{}

var discoveryHTTPClient = httpclient.New(coreHTTPTimeout)
var resolveGitHubReleaseAssetSelectionFn = func(repoSlug, assetPattern, arch string) (*github.ReleaseAssetSelection, error) {
	return (github.Client{HTTPClient: discoveryHTTPClient}).ResolveReleaseAssetSelection(repoSlug, assetPattern, arch)
}
var fetchGitHubRepositoryFn = func(ctx context.Context, repoSlug string) (*github.Repository, error) {
	return (github.Client{HTTPClient: discoveryHTTPClient}).FetchRepository(ctx, repoSlug)
}

func SetHTTPClientTimeout(timeout time.Duration) {
	if discoveryHTTPClient == nil {
		discoveryHTTPClient = httpclient.New(timeout)
		return
	}
	discoveryHTTPClient.Timeout = timeout
}

func SetHTTPClient(client *http.Client) *http.Client {
	previous := discoveryHTTPClient
	discoveryHTTPClient = client
	return previous
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

func discoveryAssetCandidates(candidates []github.ReleaseAssetCandidate) []domain.AssetCandidate {
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
