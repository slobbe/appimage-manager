package github

import (
	"context"

	"github.com/slobbe/appimage-manager/internal/app/discovery"
)

type DiscoveryResolver struct {
	Client Client
}

func (resolver DiscoveryResolver) ResolveReleaseAssetSelection(repoSlug, assetPattern, arch string) (*discovery.ReleaseAssetSelection, error) {
	selection, err := resolver.Client.ResolveReleaseAssetSelection(repoSlug, assetPattern, arch)
	if err != nil {
		return nil, err
	}

	result := &discovery.ReleaseAssetSelection{
		Ambiguous: selection.Ambiguous,
		Reason:    selection.Reason,
	}
	if selection.Release != nil {
		result.Release = &discovery.ReleaseAsset{
			DownloadURL:       selection.Release.DownloadURL,
			TagName:           selection.Release.TagName,
			NormalizedVersion: selection.Release.NormalizedVersion,
			AssetName:         selection.Release.AssetName,
			PreRelease:        selection.Release.PreRelease,
		}
	}
	for _, candidate := range selection.Candidates {
		result.Candidates = append(result.Candidates, discovery.ReleaseAssetCandidate{
			Name:        candidate.Name,
			DownloadURL: candidate.DownloadURL,
			Arch:        candidate.Arch,
			ArchLabel:   candidate.ArchLabel,
		})
	}
	return result, nil
}

func (resolver DiscoveryResolver) FetchRepository(ctx context.Context, repoSlug string) (*discovery.Repository, error) {
	repository, err := resolver.Client.FetchRepository(ctx, repoSlug)
	if err != nil {
		return nil, err
	}
	return &discovery.Repository{
		Name:        repository.Name,
		Description: repository.Description,
		HTMLURL:     repository.HTMLURL,
	}, nil
}
