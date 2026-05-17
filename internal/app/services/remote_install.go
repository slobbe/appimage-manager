package services

import (
	"context"
	"fmt"
	"strings"

	appintegrate "github.com/slobbe/appimage-manager/internal/app/integrate"
	"github.com/slobbe/appimage-manager/internal/domain"
)

type RemoteInstallService struct {
	Store             AppStore
	Filename          func(assetName, downloadURL string) string
	StableDestination func(assetURL, nameHint string) (string, error)
	Download          func(context.Context, string, string) error
	VerifySHA256      func(path, expectedSHA256 string) error
	IntegrateLocalApp IntegrateFunc
	PersistApp        func(*domain.App, bool) error
	RemoveApp         func(context.Context, string, bool) (*domain.App, error)
	RemoveStaged      func(string)
}

func NewRemoteInstallService(service RemoteInstallService) RemoteInstallService {
	return service
}

func (service RemoteInstallService) InstallDirectURL(ctx context.Context, req InstallDirectURLRequest) (*domain.App, error) {
	return service.install(ctx, remoteInstallRequest{
		DownloadURL:    req.URL,
		ExpectedSHA256: req.SHA256,
		BuildSource: func(app *domain.App) domain.Source {
			return domain.Source{Kind: domain.SourceDirectURL, DirectURL: &domain.DirectURLSource{URL: req.URL, SHA256: req.SHA256, DownloadedAt: app.UpdatedAt}}
		},
		BuildUpdate: func(*domain.App) *domain.UpdateSource { return &domain.UpdateSource{Kind: domain.UpdateNone} },
	})
}

func (service RemoteInstallService) InstallPackageMetadata(ctx context.Context, metadata *domain.PackageMetadata) (*domain.App, error) {
	if metadata == nil {
		return nil, fmt.Errorf("package metadata cannot be empty")
	}
	if metadata.Ref.Kind != domain.ProviderGitHub {
		return nil, fmt.Errorf("unsupported add provider %q", metadata.Ref.Kind)
	}
	repo := strings.TrimSpace(metadata.Ref.ProviderRef)
	assetPattern := strings.TrimSpace(metadata.AssetPattern)
	return service.install(ctx, remoteInstallRequest{
		DownloadURL: strings.TrimSpace(metadata.DownloadURL),
		AssetName:   strings.TrimSpace(metadata.AssetName),
		BuildSource: func(app *domain.App) domain.Source {
			return domain.Source{Kind: domain.SourceGitHubRelease, GitHubRelease: &domain.GitHubReleaseSource{
				Repo:         repo,
				Asset:        assetPattern,
				Tag:          strings.TrimSpace(metadata.ReleaseTag),
				AssetName:    strings.TrimSpace(metadata.AssetName),
				DownloadedAt: app.UpdatedAt,
			}}
		},
		BuildUpdate: func(*domain.App) *domain.UpdateSource {
			return &domain.UpdateSource{Kind: domain.UpdateGitHubRelease, GitHubRelease: &domain.GitHubReleaseUpdateSource{Repo: repo, Asset: assetPattern}}
		},
	})
}

type remoteInstallRequest struct {
	DownloadURL    string
	AssetName      string
	ExpectedSHA256 string
	BuildSource    func(*domain.App) domain.Source
	BuildUpdate    func(*domain.App) *domain.UpdateSource
}

func (service RemoteInstallService) install(ctx context.Context, req remoteInstallRequest) (*domain.App, error) {
	if service.Filename == nil {
		return nil, fmt.Errorf("remote install filename service is not configured")
	}
	if service.StableDestination == nil {
		return nil, fmt.Errorf("remote install destination service is not configured")
	}
	if service.Download == nil {
		return nil, fmt.Errorf("remote install downloader is not configured")
	}
	if service.IntegrateLocalApp == nil {
		return nil, fmt.Errorf("remote install integrator is not configured")
	}
	if service.PersistApp == nil {
		return nil, fmt.Errorf("remote install store is not configured")
	}

	fileName := service.Filename(req.AssetName, req.DownloadURL)
	downloadPath, err := service.StableDestination(req.DownloadURL, fileName)
	if err != nil {
		return nil, err
	}
	if service.RemoveStaged != nil {
		defer service.RemoveStaged(downloadPath)
	}

	if err := service.Download(ctx, req.DownloadURL, downloadPath); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.ExpectedSHA256) != "" {
		if service.VerifySHA256 == nil {
			return nil, fmt.Errorf("remote install hash verifier is not configured")
		}
		if err := service.VerifySHA256(downloadPath, req.ExpectedSHA256); err != nil {
			return nil, err
		}
	}

	app, err := service.IntegrateLocalApp(ctx, downloadPath, appintegrate.UpdateOverwritePrompt(func(existing, incoming *domain.UpdateSource) (bool, error) {
		_ = existing
		_ = incoming
		return false, nil
	}))
	if err != nil {
		return nil, err
	}

	incomingUpdate := req.BuildUpdate(app)
	if incomingUpdate == nil {
		incomingUpdate = &domain.UpdateSource{Kind: domain.UpdateNone}
	}
	finalUpdate := incomingUpdate
	if service.Store != nil {
		if existing, err := service.Store.GetApp(app.ID); err == nil && existing != nil && existing.Update != nil && existing.Update.Kind != domain.UpdateNone && !domain.UpdateSourcesEqual(existing.Update, incomingUpdate) {
			finalUpdate = existing.Update
		}
	}

	app.Source = req.BuildSource(app)
	app.Update = finalUpdate

	if err := service.PersistApp(app, true); err != nil {
		return nil, err
	}
	if strings.TrimSpace(app.ReplacesID) != "" && service.RemoveApp != nil {
		if _, err := service.RemoveApp(ctx, app.ReplacesID, false); err != nil {
			return nil, fmt.Errorf("failed to remove superseded app %s: %w", app.ReplacesID, err)
		}
		app.ReplacesID = ""
	}

	return app, nil
}
