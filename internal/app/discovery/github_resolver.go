package discovery

import (
	"context"

	githubinfra "github.com/slobbe/appimage-manager/internal/infra/github"
)

type GitHubClientResolver struct {
	Client githubinfra.Client
}

func NewGitHubClientResolver(client githubinfra.Client) GitHubClientResolver {
	return GitHubClientResolver{Client: client}
}

func (resolver GitHubClientResolver) ResolveReleaseAssetSelection(repoSlug, assetPattern, arch string) (*ReleaseAssetSelection, error) {
	selection, err := resolver.Client.ResolveReleaseAssetSelection(repoSlug, assetPattern, arch)
	if err != nil {
		return nil, err
	}

	result := &ReleaseAssetSelection{
		Ambiguous: selection.Ambiguous,
		Reason:    selection.Reason,
	}
	if selection.Release != nil {
		result.Release = &ReleaseAsset{
			DownloadURL:       selection.Release.DownloadURL,
			TagName:           selection.Release.TagName,
			NormalizedVersion: selection.Release.NormalizedVersion,
			AssetName:         selection.Release.AssetName,
			PreRelease:        selection.Release.PreRelease,
		}
	}
	for _, candidate := range selection.Candidates {
		result.Candidates = append(result.Candidates, ReleaseAssetCandidate{
			Name:        candidate.Name,
			DownloadURL: candidate.DownloadURL,
			Arch:        candidate.Arch,
			ArchLabel:   candidate.ArchLabel,
		})
	}
	return result, nil
}

func (resolver GitHubClientResolver) FetchRepository(ctx context.Context, repoSlug string) (*Repository, error) {
	repository, err := resolver.Client.FetchRepository(ctx, repoSlug)
	if err != nil {
		return nil, err
	}
	return &Repository{
		Name:        repository.Name,
		Description: repository.Description,
		HTMLURL:     repository.HTMLURL,
	}, nil
}
