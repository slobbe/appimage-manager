package services

import (
	"testing"

	appupdate "github.com/slobbe/appimage-manager/internal/app/update"
	"github.com/slobbe/appimage-manager/internal/domain"
)

func TestAppDetailsFromDomainMapsDomainFreeView(t *testing.T) {
	app := &domain.App{
		ID:               " app ",
		Name:             " My App ",
		Version:          " 1.0.0 ",
		ExecPath:         " /apps/app/app.AppImage ",
		IconPath:         " /icons/app.svg ",
		DesktopEntryPath: " /apps/app/app.desktop ",
		DesktopEntryLink: " /applications/app.desktop ",
		LatestVersion:    " 1.1.0 ",
		UpdateAvailable:  true,
		Source: domain.Source{
			Kind: domain.SourceGitHubRelease,
			GitHubRelease: &domain.GitHubReleaseSource{
				Repo:      " owner/repo ",
				Asset:     " *.AppImage ",
				Tag:       " v1.0.0 ",
				AssetName: " MyApp.AppImage ",
			},
		},
		Update: &domain.UpdateSource{
			Kind: domain.UpdateGitHubRelease,
			GitHubRelease: &domain.GitHubReleaseUpdateSource{
				Repo:  " owner/repo ",
				Asset: " *.AppImage ",
			},
		},
	}

	view := appDetailsFromDomain(app)
	if view == nil {
		t.Fatal("appDetailsFromDomain returned nil")
	}
	if view.ID != "app" || view.Name != "My App" || view.Version != "1.0.0" {
		t.Fatalf("unexpected app summary: %+v", view.AppSummary)
	}
	if !view.Integrated || !view.UpdateAvailable || view.LatestVersion != "1.1.0" {
		t.Fatalf("unexpected app status: %+v", view.AppSummary)
	}
	if view.Source == nil || view.Source.GitHubRelease == nil || view.Source.GitHubRelease.Repo != "owner/repo" {
		t.Fatalf("unexpected source view: %+v", view.Source)
	}
	if view.UpdateSource == nil || view.UpdateSource.GitHubRelease == nil || view.UpdateSource.GitHubRelease.Asset != "*.AppImage" {
		t.Fatalf("unexpected update source view: %+v", view.UpdateSource)
	}
}

func TestPackageViewFromDomainMapsAssetCandidates(t *testing.T) {
	metadata := &domain.PackageMetadata{
		Name:          " My App ",
		Provider:      " GitHub ",
		Ref:           domain.PackageRef{Kind: domain.ProviderGitHub, ProviderRef: " owner/repo "},
		LatestVersion: " v1.2.3 ",
		AssetName:     " MyApp.AppImage ",
		AssetPattern:  " *.AppImage ",
		DownloadURL:   " https://example.com/MyApp.AppImage ",
		Installable:   true,
		AssetCandidates: []domain.AssetCandidate{
			{Name: " MyApp.AppImage ", DownloadURL: " https://example.com/MyApp.AppImage ", Arch: " x86_64 ", ArchLabel: " amd64 "},
		},
	}

	view := packageViewFromDomain(metadata)
	if view == nil {
		t.Fatal("packageViewFromDomain returned nil")
	}
	if view.Name != "My App" || view.Provider != "GitHub" || view.Ref.Provider != "github" || view.Ref.Ref != "owner/repo" {
		t.Fatalf("unexpected package view: %+v", view)
	}
	if len(view.AssetCandidates) != 1 || view.AssetCandidates[0].ArchLabel != "amd64" {
		t.Fatalf("unexpected asset candidates: %+v", view.AssetCandidates)
	}
}

func TestUpdateSourceInputConvertsToDomain(t *testing.T) {
	input := UpdateSourceInput{
		Kind: string(domain.UpdateGitHubRelease),
		GitHubRelease: &GitHubReleaseUpdateSourceInput{
			Repo:        " owner/repo ",
			Asset:       " *.AppImage ",
			ReleaseKind: " latest ",
		},
	}

	source := input.domainUpdateSource()
	if source.Kind != domain.UpdateGitHubRelease || source.GitHubRelease == nil {
		t.Fatalf("unexpected source: %+v", source)
	}
	if source.GitHubRelease.Repo != "owner/repo" || source.GitHubRelease.Asset != "*.AppImage" || source.GitHubRelease.ReleaseKind != "latest" {
		t.Fatalf("unexpected github source: %+v", source.GitHubRelease)
	}
}

func TestManagedUpdateAndEventViews(t *testing.T) {
	update := &appupdate.ManagedUpdate{
		App:       &domain.App{ID: "app", Name: "App"},
		URL:       " https://example.com/app.AppImage ",
		Asset:     " app.AppImage ",
		Available: true,
		Latest:    " 2.0.0 ",
		FromKind:  domain.UpdateZsync,
	}

	view := managedUpdateViewFromAppUpdate(update)
	if view == nil || view.App == nil || view.App.ID != "app" || view.FromKind != "zsync" || view.URL != "https://example.com/app.AppImage" {
		t.Fatalf("unexpected managed update view: %+v", view)
	}

	applyResult := managedApplyResultViewFromAppUpdate(appupdate.ManagedApplyResult{
		Index:      1,
		App:        &domain.App{ID: "app", Name: "App"},
		UpdatedApp: &domain.App{ID: "app", Name: "App", Version: "2.0.0"},
	})
	if applyResult.Index != 1 || applyResult.App == nil || applyResult.UpdatedApp == nil || applyResult.UpdatedApp.Version != "2.0.0" {
		t.Fatalf("unexpected managed apply result view: %+v", applyResult)
	}

	event := managedApplyEventViewFromAppUpdate(appupdate.ManagedApplyEvent{
		AppID:         " app ",
		Stage:         appupdate.ManagedApplyStageDownload,
		Downloaded:    10,
		DownloadTotal: 20,
		DownloadName:  " app.AppImage ",
	})
	if event.AppID != "app" || event.Stage != "download" || event.DownloadName != "app.AppImage" {
		t.Fatalf("unexpected managed apply event view: %+v", event)
	}
}
