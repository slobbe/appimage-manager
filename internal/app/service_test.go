package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aim/internal/domain"
)

func TestServiceAddIntegratesLocalAppImage(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Add(context.Background(), AddRequest{Path: "/downloads/example.AppImage"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if got, want := result.App.ID, "example-app"; got != want {
		t.Fatalf("App.ID = %q, want %q", got, want)
	}
	if got, want := result.App.Name, "Example App"; got != want {
		t.Fatalf("App.Name = %q, want %q", got, want)
	}
	if got, want := result.App.Version.String(), "1.2.3-beta.1"; got != want {
		t.Fatalf("App.Version = %q, want %q", got, want)
	}
	if got, want := result.App.AppImagePath, "/library/example-app.AppImage"; got != want {
		t.Fatalf("App.AppImagePath = %q, want %q", got, want)
	}
	if got, want := result.App.IconPath, "/icons/hicolor/256x256/apps/example-app.png"; got != want {
		t.Fatalf("App.IconPath = %q, want %q", got, want)
	}
	if got, want := result.App.DesktopEntryPath, "/desktop/example-app.desktop"; got != want {
		t.Fatalf("App.DesktopEntryPath = %q, want %q", got, want)
	}
	if got, want := result.App.Source.Kind, domain.SourceKindLocal; got != want {
		t.Fatalf("App.Source.Kind = %q, want %q", got, want)
	}
	if got, want := result.App.Source.LocalFile.Path, "/downloads/example.AppImage"; got != want {
		t.Fatalf("App.Source.LocalFile.Path = %q, want %q", got, want)
	}
	if result.App.Source.LocalFile.IntegratedAt.IsZero() {
		t.Fatal("App.Source.LocalFile.IntegratedAt is zero, want timestamp")
	}
	if got, want := result.App.UpdateSource.Kind, domain.UpdateSourceKindUnknown; got != want {
		t.Fatalf("App.UpdateSource.Kind = %q, want %q", got, want)
	}

	if !deps.workspaces.cleaned {
		t.Fatal("workspace was not cleaned")
	}
	if got, want := deps.appImages.appImagePath, "/downloads/example.AppImage"; got != want {
		t.Fatalf("extract appImagePath = %q, want %q", got, want)
	}
	if !strings.HasSuffix(deps.appImages.destDir, "/workspace/extract") {
		t.Fatalf("extract destDir = %q, want workspace extract dir", deps.appImages.destDir)
	}
	if got, want := deps.icons.iconName, "example-icon"; got != want {
		t.Fatalf("icon discover iconName = %q, want %q", got, want)
	}
	if got, want := deps.appImageInstaller.sourcePath, "/downloads/example.AppImage"; got != want {
		t.Fatalf("appimage installer source = %q, want %q", got, want)
	}
	if got, want := deps.iconInstaller.sourcePath, "/extracted/example.png"; got != want {
		t.Fatalf("icon installer source = %q, want %q", got, want)
	}
	if got, want := deps.saved.App.ID, result.App.ID; got != want {
		t.Fatalf("saved App.ID = %q, want %q", got, want)
	}
	if len(deps.appImageRemover.paths) != 0 || len(deps.iconRemover.paths) != 0 || len(deps.desktopEntryRemover.paths) != 0 {
		t.Fatalf("rollback removers called on success: appimage=%v icon=%v desktop=%v", deps.appImageRemover.paths, deps.iconRemover.paths, deps.desktopEntryRemover.paths)
	}

	desktopContent := string(deps.desktopEntryInstaller.content)
	if !strings.Contains(desktopContent, "Exec=/library/example-app.AppImage") {
		t.Fatalf("desktop content = %q, want updated Exec", desktopContent)
	}
	if !strings.Contains(desktopContent, "Icon=example-app") {
		t.Fatalf("desktop content = %q, want updated Icon", desktopContent)
	}
	if !strings.Contains(desktopContent, "[Desktop Action NewWindow]") {
		t.Fatalf("desktop content = %q, want action group preserved", desktopContent)
	}
	if !strings.Contains(desktopContent, "Exec=old-action") {
		t.Fatalf("desktop content = %q, want action Exec preserved", desktopContent)
	}
}

func TestServiceAddStoresEmbeddedGitHubUpdateSource(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	raw := "gh-releases-zsync|owner|repo|latest|Example-*x86_64.AppImage.zsync"
	deps.appImages.updateInfo = raw
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Add(context.Background(), AddRequest{Path: "/downloads/example.AppImage"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if got, want := result.App.UpdateSource.Kind, domain.UpdateSourceKindGitHub; got != want {
		t.Fatalf("App.UpdateSource.Kind = %q, want %q", got, want)
	}
	if !result.App.UpdateSource.Embedded {
		t.Fatal("App.UpdateSource.Embedded = false, want true")
	}
	if got := result.App.UpdateSource.Raw; got != raw {
		t.Fatalf("App.UpdateSource.Raw = %q, want %q", got, raw)
	}
	if got, want := result.App.UpdateSource.Transport, "gh-releases-zsync"; got != want {
		t.Fatalf("App.UpdateSource.Transport = %q, want %q", got, want)
	}
	if got, want := result.App.UpdateSource.Repo, "owner/repo"; got != want {
		t.Fatalf("App.UpdateSource.Repo = %q, want %q", got, want)
	}
	if got, want := result.App.UpdateSource.ReleaseTag, "latest"; got != want {
		t.Fatalf("App.UpdateSource.ReleaseTag = %q, want %q", got, want)
	}
	if got, want := result.App.UpdateSource.AssetPattern, "Example-*x86_64.AppImage"; got != want {
		t.Fatalf("App.UpdateSource.AssetPattern = %q, want %q", got, want)
	}
	if got, want := deps.saved.App.UpdateSource.Raw, raw; got != want {
		t.Fatalf("saved App.UpdateSource.Raw = %q, want %q", got, want)
	}
}

func TestServiceAddStoresMalformedEmbeddedUpdateSourceAsUnsupported(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	raw := "gh-releases-zsync|owner|repo"
	deps.appImages.updateInfo = raw
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Add(context.Background(), AddRequest{Path: "/downloads/example.AppImage"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if got, want := result.App.UpdateSource.Kind, domain.UpdateSourceKindUnsupported; got != want {
		t.Fatalf("App.UpdateSource.Kind = %q, want %q", got, want)
	}
	if got := result.App.UpdateSource.Raw; got != raw {
		t.Fatalf("App.UpdateSource.Raw = %q, want %q", got, raw)
	}
}

func TestServiceAddFromGitHubIgnoresEmbeddedUpdateSource(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	deps.appImages.updateInfo = "gh-releases-zsync|other|repo|latest|Other-*.AppImage.zsync"
	deps.ServiceDeps.GitHubReleases = &fakeGitHubReleaseFinder{release: testGitHubRelease("Example.AppImage")}
	deps.ServiceDeps.Downloads = &fakeAssetDownloader{}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Add(context.Background(), AddRequest{GitHubRepo: "owner/repo"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if result.App.UpdateSource.Embedded {
		t.Fatal("App.UpdateSource.Embedded = true, want false")
	}
	if got, want := result.App.UpdateSource.Repo, "owner/repo"; got != want {
		t.Fatalf("App.UpdateSource.Repo = %q, want %q", got, want)
	}
	if result.App.UpdateSource.Raw != "" {
		t.Fatalf("App.UpdateSource.Raw = %q, want empty", result.App.UpdateSource.Raw)
	}
}

func TestServiceAddUsesLocalFilenameAsVersionFallback(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	deps.desktopEntries.content = []byte(strings.Join([]string{
		"[Desktop Entry]",
		"Name=Generic Name",
		"X-AppImage-Name=Example App",
		"Exec=old-exec",
		"Icon=example-icon",
		"",
	}, "\n"))
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Add(context.Background(), AddRequest{Path: "/downloads/Example_App-2.4.6-x86_64.AppImage"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if got, want := result.App.Version.String(), "2.4.6"; got != want {
		t.Fatalf("App.Version = %q, want %q", got, want)
	}
	if got, want := deps.saved.App.Version.String(), "2.4.6"; got != want {
		t.Fatalf("saved App.Version = %q, want %q", got, want)
	}
}

func TestServiceAddCleansWorkspaceOnFailure(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	failure := errors.New("discover desktop failed")
	deps.desktopEntries.err = failure
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Add(context.Background(), AddRequest{Path: "/downloads/example.AppImage"})
	if !errors.Is(err, failure) {
		t.Fatalf("Add() error = %v, want %v", err, failure)
	}
	if !deps.workspaces.cleaned {
		t.Fatal("workspace was not cleaned")
	}
	if deps.appImageInstaller.called {
		t.Fatal("appimage installer called after discovery failure")
	}
}

func TestServiceAddStopsOnDesktopParseFailure(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	deps.desktopEntries.content = []byte("[Desktop Entry]\nName\n")
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Add(context.Background(), AddRequest{Path: "/downloads/example.AppImage"})
	if err == nil {
		t.Fatal("Add() error = nil, want parse error")
	}
	if deps.icons.called {
		t.Fatal("icon discoverer called after parse failure")
	}
}

func TestServiceAddStopsOnIconDiscoveryFailure(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	failure := errors.New("icon missing")
	deps.icons.err = failure
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Add(context.Background(), AddRequest{Path: "/downloads/example.AppImage"})
	if !errors.Is(err, failure) {
		t.Fatalf("Add() error = %v, want %v", err, failure)
	}
	if deps.appImageInstaller.called {
		t.Fatal("appimage installer called after icon discovery failure")
	}
}

func TestServiceAddRollsBackInstalledArtifactsOnRepositoryFailure(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	failure := errors.New("save failed")
	deps.apps.err = failure
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Add(context.Background(), AddRequest{Path: "/downloads/example.AppImage"})
	if !errors.Is(err, failure) {
		t.Fatalf("Add() error = %v, want %v", err, failure)
	}
	assertRemovedPaths(t, deps.desktopEntryRemover.paths, []string{"/desktop/example-app.desktop"})
	assertRemovedPaths(t, deps.iconRemover.paths, []string{"/icons/hicolor/256x256/apps/example-app.png"})
	assertRemovedPaths(t, deps.appImageRemover.paths, []string{"/library/example-app.AppImage"})
}

func TestServiceAddRollsBackAppImageWhenIconInstallFails(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	failure := errors.New("icon install failed")
	deps.iconInstaller.err = failure
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Add(context.Background(), AddRequest{Path: "/downloads/example.AppImage"})
	if !errors.Is(err, failure) {
		t.Fatalf("Add() error = %v, want %v", err, failure)
	}
	if got, want := deps.appImageInstaller.sourcePath, "/downloads/example.AppImage"; got != want {
		t.Fatalf("appimage installer source = %q, want %q", got, want)
	}
	assertRemovedPaths(t, deps.appImageRemover.paths, []string{"/library/example-app.AppImage"})
	assertRemovedPaths(t, deps.iconRemover.paths, nil)
	assertRemovedPaths(t, deps.desktopEntryRemover.paths, nil)
}

func TestServiceRemoveRemovesArtifactsAndDeletesApp(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	deps.apps.findApp = installed
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if err := service.Remove(context.Background(), RemoveRequest{Name: installed.ID}); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	assertRemovedPaths(t, deps.desktopEntryRemover.paths, []string{installed.DesktopEntryPath})
	assertRemovedPaths(t, deps.iconRemover.paths, []string{installed.IconPath})
	assertRemovedPaths(t, deps.appImageRemover.paths, []string{installed.AppImagePath})
	if got, want := deps.apps.deletedID, installed.ID; got != want {
		t.Fatalf("deleted app ID = %q, want %q", got, want)
	}
}

func TestServiceRemoveSkipsEmptyArtifactPaths(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	installed.DesktopEntryPath = ""
	installed.IconPath = ""
	deps.apps.findApp = installed
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if err := service.Remove(context.Background(), RemoveRequest{Name: installed.ID}); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	assertRemovedPaths(t, deps.desktopEntryRemover.paths, nil)
	assertRemovedPaths(t, deps.iconRemover.paths, nil)
	assertRemovedPaths(t, deps.appImageRemover.paths, []string{installed.AppImagePath})
}

func TestServiceRemoveReturnsFindFailure(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	deps.apps.findErr = ErrAppNotFound
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	err = service.Remove(context.Background(), RemoveRequest{Name: "missing"})
	if !errors.Is(err, ErrAppNotFound) {
		t.Fatalf("Remove() error = %v, want ErrAppNotFound", err)
	}
	assertRemovedPaths(t, deps.appImageRemover.paths, nil)
}

func TestServiceRemoveStopsBeforeDeletingAppWhenArtifactRemovalFails(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	deps.apps.findApp = installed
	failure := errors.New("remove icon failed")
	deps.iconRemover.err = failure
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	err = service.Remove(context.Background(), RemoveRequest{Name: installed.ID})
	if !errors.Is(err, failure) {
		t.Fatalf("Remove() error = %v, want %v", err, failure)
	}
	assertRemovedPaths(t, deps.desktopEntryRemover.paths, []string{installed.DesktopEntryPath})
	assertRemovedPaths(t, deps.iconRemover.paths, []string{installed.IconPath})
	assertRemovedPaths(t, deps.appImageRemover.paths, nil)
	if deps.apps.deletedID != "" {
		t.Fatalf("deleted app ID = %q, want empty", deps.apps.deletedID)
	}
}

func TestServiceRemoveReturnsDeleteFailure(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	deps.apps.findApp = installed
	failure := errors.New("delete failed")
	deps.apps.deleteErr = failure
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	err = service.Remove(context.Background(), RemoveRequest{Name: installed.ID})
	if !errors.Is(err, failure) {
		t.Fatalf("Remove() error = %v, want %v", err, failure)
	}
}

func TestServiceRemoveValidatesName(t *testing.T) {
	t.Parallel()

	service, err := NewService(integrationTestDeps().ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if err := service.Remove(context.Background(), RemoveRequest{}); err == nil {
		t.Fatal("Remove() error = nil, want error")
	}
}

func TestServiceUpdateAppliesGitHubUpdates(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	installed.UpdateSource = domain.NewGitHubUpdateSource("owner/repo", true)
	deps.apps.listApps = []domain.App{installed}
	deps.desktopEntries.content = []byte(strings.Join([]string{
		"[Desktop Entry]",
		"Name=Example App",
		"Exec=old-exec",
		"Icon=example-icon",
		"",
	}, "\n"))
	releases := &fakeGitHubReleaseFinder{release: testGitHubReleaseWithTag("v2.0.0", "Example.AppImage")}
	downloads := &fakeAssetDownloader{}
	deps.ServiceDeps.GitHubReleases = releases
	deps.ServiceDeps.Downloads = downloads
	confirmation := &fakeUpdateConfirmation{confirmed: true}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Update(context.Background(), UpdateRequest{Confirmation: confirmation})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if !result.Applied {
		t.Fatal("Update().Applied = false, want true")
	}
	wantCandidates := []UpdateCandidate{{ID: installed.ID, CurrentVersion: "1.2.3", NewVersion: "2.0.0"}}
	assertUpdateCandidates(t, result.Updates, wantCandidates)
	assertUpdateCandidates(t, confirmation.updates, wantCandidates)
	if got, want := releases.repo, "owner/repo"; got != want {
		t.Fatalf("LatestRelease() repo = %q, want %q", got, want)
	}
	if !releases.includePrerelease {
		t.Fatal("LatestRelease() includePrerelease = false, want true")
	}
	if got, want := downloads.destinationPath, "/workspace/Example.AppImage"; got != want {
		t.Fatalf("Download() destination = %q, want %q", got, want)
	}
	if got, want := deps.saved.App.ID, installed.ID; got != want {
		t.Fatalf("saved App.ID = %q, want %q", got, want)
	}
	if got, want := deps.saved.App.Version.String(), "2.0.0"; got != want {
		t.Fatalf("saved App.Version = %q, want %q", got, want)
	}
	if got, want := deps.saved.App.UpdateSource.Kind, domain.UpdateSourceKindGitHub; got != want {
		t.Fatalf("saved App.UpdateSource.Kind = %q, want %q", got, want)
	}
	if got, want := deps.saved.App.UpdateSource.Repo, "owner/repo"; got != want {
		t.Fatalf("saved App.UpdateSource.Repo = %q, want %q", got, want)
	}
	if !deps.saved.App.UpdateSource.Prerelease {
		t.Fatal("saved App.UpdateSource.Prerelease = false, want true")
	}
}

func TestServiceUpdateAppliesGitHubUpdateForTargetApp(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	installed.UpdateSource = domain.NewGitHubUpdateSource("owner/repo", false)
	deps.apps.findApp = installed
	deps.desktopEntries.content = []byte(strings.Join([]string{
		"[Desktop Entry]",
		"Name=Example App",
		"Exec=old-exec",
		"Icon=example-icon",
		"",
	}, "\n"))
	deps.ServiceDeps.GitHubReleases = &fakeGitHubReleaseFinder{release: testGitHubReleaseWithTag("v2.0.0", "Example.AppImage")}
	deps.ServiceDeps.Downloads = &fakeAssetDownloader{}
	confirmation := &fakeUpdateConfirmation{confirmed: true}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Update(context.Background(), UpdateRequest{Target: " example-app ", Confirmation: confirmation})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if got, want := deps.apps.findID, "example-app"; got != want {
		t.Fatalf("Find() id = %q, want %q", got, want)
	}
	wantCandidates := []UpdateCandidate{{ID: installed.ID, CurrentVersion: "1.2.3", NewVersion: "2.0.0"}}
	assertUpdateCandidates(t, result.Updates, wantCandidates)
	assertUpdateCandidates(t, confirmation.updates, wantCandidates)
	if got, want := deps.saved.App.ID, installed.ID; got != want {
		t.Fatalf("saved App.ID = %q, want %q", got, want)
	}
	if got, want := deps.saved.App.Version.String(), "2.0.0"; got != want {
		t.Fatalf("saved App.Version = %q, want %q", got, want)
	}
}

func TestServiceUpdateTargetReturnsFindFailure(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	deps.apps.findErr = ErrAppNotFound
	deps.ServiceDeps.GitHubReleases = &fakeGitHubReleaseFinder{release: testGitHubReleaseWithTag("v2.0.0", "Example.AppImage")}
	deps.ServiceDeps.Downloads = &fakeAssetDownloader{}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Update(context.Background(), UpdateRequest{Target: "missing"})
	if !errors.Is(err, ErrAppNotFound) {
		t.Fatalf("Update() error = %v, want ErrAppNotFound", err)
	}
	if got, want := deps.apps.findID, "missing"; got != want {
		t.Fatalf("Find() id = %q, want %q", got, want)
	}
	if deps.saved.App.ID != "" {
		t.Fatalf("saved App.ID = %q, want empty", deps.saved.App.ID)
	}
}

func TestServiceUpdateTargetSkipsAppWithoutUpdateSource(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	deps.apps.findApp = installed
	confirmation := &fakeUpdateConfirmation{confirmed: true}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Update(context.Background(), UpdateRequest{Target: installed.ID, Confirmation: confirmation})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !result.Applied {
		t.Fatal("Update().Applied = false, want true")
	}
	assertUpdateCandidates(t, result.Updates, nil)
	if confirmation.called {
		t.Fatal("confirmation called with no update candidates")
	}
}

func TestServiceUpdateUsesEmbeddedGitHubReleaseTagAndAssetPattern(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	raw := "gh-releases-zsync|owner|repo|latest-pre|Example-*x86_64.AppImage.zsync"
	installed.UpdateSource = domain.NewEmbeddedUpdateSource(raw)
	deps.apps.listApps = []domain.App{installed}
	deps.desktopEntries.content = []byte(strings.Join([]string{
		"[Desktop Entry]",
		"Name=Example App",
		"Exec=old-exec",
		"Icon=example-icon",
		"",
	}, "\n"))
	releases := &fakeGitHubReleaseFinder{release: testGitHubReleaseWithTag("v2.0.0", "Other-x86_64.AppImage", "Example-2.0.0-x86_64.AppImage", "Example-2.0.0-x86_64.AppImage.zsync")}
	deps.ServiceDeps.GitHubReleases = releases
	deps.ServiceDeps.Downloads = &fakeAssetDownloader{}
	confirmation := &fakeUpdateConfirmation{confirmed: true}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Update(context.Background(), UpdateRequest{Confirmation: confirmation})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if got, want := releases.method, "latest-prerelease"; got != want {
		t.Fatalf("release finder method = %q, want %q", got, want)
	}
	if got, want := releases.repo, "owner/repo"; got != want {
		t.Fatalf("release finder repo = %q, want %q", got, want)
	}
	if got, want := deps.appImageInstaller.sourcePath, "/workspace/Example-2.0.0-x86_64.AppImage"; got != want {
		t.Fatalf("appimage installer source = %q, want %q", got, want)
	}
	wantCandidates := []UpdateCandidate{{ID: installed.ID, CurrentVersion: "1.2.3", NewVersion: "2.0.0"}}
	assertUpdateCandidates(t, result.Updates, wantCandidates)
	if got := deps.saved.App.UpdateSource.Raw; got != raw {
		t.Fatalf("saved App.UpdateSource.Raw = %q, want %q", got, raw)
	}
	if got, want := deps.saved.App.UpdateSource.ReleaseTag, "latest-pre"; got != want {
		t.Fatalf("saved App.UpdateSource.ReleaseTag = %q, want %q", got, want)
	}
}

func TestServiceUpdateUsesGitHubAssetPattern(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	installed.UpdateSource = domain.NewGitHubUpdateSource("owner/repo", false)
	installed.UpdateSource.AssetPattern = "Example-arm64.AppImage"
	deps.apps.listApps = []domain.App{installed}
	deps.desktopEntries.content = []byte(strings.Join([]string{
		"[Desktop Entry]",
		"Name=Example App",
		"Exec=old-exec",
		"Icon=example-icon",
		"",
	}, "\n"))
	releases := &fakeGitHubReleaseFinder{release: testGitHubReleaseWithTag("v2.0.0", "Example-arm64.AppImage", "Example-x86_64.AppImage")}
	deps.ServiceDeps.GitHubReleases = releases
	deps.ServiceDeps.Downloads = &fakeAssetDownloader{}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Update(context.Background(), UpdateRequest{Confirmation: &fakeUpdateConfirmation{confirmed: true}})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if got, want := deps.appImageInstaller.sourcePath, "/workspace/Example-arm64.AppImage"; got != want {
		t.Fatalf("appimage installer source = %q, want %q", got, want)
	}
	if got, want := deps.saved.App.UpdateSource.AssetPattern, "Example-arm64.AppImage"; got != want {
		t.Fatalf("saved App.UpdateSource.AssetPattern = %q, want %q", got, want)
	}
}

func TestServiceUpdateUsesEmbeddedGitHubExactReleaseTag(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	installed.UpdateSource = domain.NewEmbeddedUpdateSource("gh-releases-zsync|owner|repo|continuous|Example-*.AppImage.zsync")
	deps.apps.listApps = []domain.App{installed}
	deps.desktopEntries.content = []byte(strings.Join([]string{
		"[Desktop Entry]",
		"Name=Example App",
		"Exec=old-exec",
		"Icon=example-icon",
		"",
	}, "\n"))
	releases := &fakeGitHubReleaseFinder{release: testGitHubReleaseWithTag("v2.0.0", "Example-2.0.0.AppImage")}
	deps.ServiceDeps.GitHubReleases = releases
	deps.ServiceDeps.Downloads = &fakeAssetDownloader{}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Update(context.Background(), UpdateRequest{Confirmation: &fakeUpdateConfirmation{confirmed: true}})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if got, want := releases.method, "tag"; got != want {
		t.Fatalf("release finder method = %q, want %q", got, want)
	}
	if got, want := releases.tag, "continuous"; got != want {
		t.Fatalf("release finder tag = %q, want %q", got, want)
	}
}

func TestServiceUpdateSkipsEmbeddedZsyncSourceForNow(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	installed.UpdateSource = domain.NewEmbeddedUpdateSource("zsync|https://example.test/App.AppImage.zsync")
	deps.apps.listApps = []domain.App{installed}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Update(context.Background(), UpdateRequest{})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !result.Applied {
		t.Fatal("Update().Applied = false, want true")
	}
	assertUpdateCandidates(t, result.Updates, nil)
}

func TestServiceUpdateRollsBackStagedArtifactsWhenIntegrationFails(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	installed.UpdateSource = domain.NewGitHubUpdateSource("owner/repo", false)
	deps.apps.listApps = []domain.App{installed}
	deps.ServiceDeps.GitHubReleases = &fakeGitHubReleaseFinder{release: testGitHubReleaseWithTag("v2.0.0", "Example.AppImage")}
	deps.ServiceDeps.Downloads = &fakeAssetDownloader{}
	deps.appImageInstaller.path = "/library/example-app-2-0-0.AppImage"
	deps.iconInstaller.err = errors.New("icon install failed")
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Update(context.Background(), UpdateRequest{Confirmation: &fakeUpdateConfirmation{confirmed: true}})
	if err == nil || !strings.Contains(err.Error(), "icon install failed") {
		t.Fatalf("Update() error = %v, want icon install failure", err)
	}

	if got, want := deps.appImageInstaller.appID, "example-app-2-0-0"; got != want {
		t.Fatalf("appimage installer appID = %q, want %q", got, want)
	}
	assertRemovedPaths(t, deps.appImageRemover.paths, []string{"/library/example-app-2-0-0.AppImage"})
	assertRemovedPaths(t, deps.iconRemover.paths, nil)
	assertRemovedPaths(t, deps.desktopEntryRemover.paths, nil)
	if deps.saved.App.ID != "" {
		t.Fatalf("saved App.ID = %q, want empty", deps.saved.App.ID)
	}
}

func TestServiceUpdateRollsBackStagedArtifactsWhenSaveFails(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	installed.UpdateSource = domain.NewGitHubUpdateSource("owner/repo", false)
	deps.apps.listApps = []domain.App{installed}
	deps.ServiceDeps.GitHubReleases = &fakeGitHubReleaseFinder{release: testGitHubReleaseWithTag("v2.0.0", "Example.AppImage")}
	deps.ServiceDeps.Downloads = &fakeAssetDownloader{}
	deps.appImageInstaller.path = "/library/example-app-2-0-0.AppImage"
	deps.iconInstaller.path = "/icons/hicolor/256x256/apps/example-app-2-0-0.png"
	deps.desktopEntryInstaller.path = "/desktop/example-app-2-0-0.desktop"
	failure := errors.New("save failed")
	deps.apps.err = failure
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Update(context.Background(), UpdateRequest{Confirmation: &fakeUpdateConfirmation{confirmed: true}})
	if !errors.Is(err, failure) {
		t.Fatalf("Update() error = %v, want %v", err, failure)
	}

	assertRemovedPaths(t, deps.desktopEntryRemover.paths, []string{"/desktop/example-app-2-0-0.desktop"})
	assertRemovedPaths(t, deps.iconRemover.paths, []string{"/icons/hicolor/256x256/apps/example-app-2-0-0.png"})
	assertRemovedPaths(t, deps.appImageRemover.paths, []string{"/library/example-app-2-0-0.AppImage"})
}

func TestServiceUpdateKeepsSavedUpdateWhenOldArtifactCleanupFails(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	installed.UpdateSource = domain.NewGitHubUpdateSource("owner/repo", false)
	deps.apps.listApps = []domain.App{installed}
	deps.ServiceDeps.GitHubReleases = &fakeGitHubReleaseFinder{release: testGitHubReleaseWithTag("v2.0.0", "Example.AppImage")}
	deps.ServiceDeps.Downloads = &fakeAssetDownloader{}
	deps.appImageInstaller.path = "/library/example-app-2-0-0.AppImage"
	deps.iconInstaller.path = "/icons/hicolor/256x256/apps/example-app-2-0-0.png"
	deps.desktopEntryInstaller.path = "/desktop/example-app-2-0-0.desktop"
	failure := errors.New("remove old icon failed")
	deps.iconRemover.err = failure
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Update(context.Background(), UpdateRequest{Confirmation: &fakeUpdateConfirmation{confirmed: true}})
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "failed to remove replaced artifacts") {
		t.Fatalf("Update() error = %v, want cleanup failure", err)
	}

	if got, want := deps.saved.App.ID, installed.ID; got != want {
		t.Fatalf("saved App.ID = %q, want %q", got, want)
	}
	if got, want := deps.saved.App.AppImagePath, "/library/example-app-2-0-0.AppImage"; got != want {
		t.Fatalf("saved App.AppImagePath = %q, want %q", got, want)
	}
	assertRemovedPaths(t, deps.desktopEntryRemover.paths, []string{installed.DesktopEntryPath})
	assertRemovedPaths(t, deps.iconRemover.paths, []string{installed.IconPath})
	assertRemovedPaths(t, deps.appImageRemover.paths, nil)
}

func TestServiceUpdateSkipsAppsWithoutUpdates(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	installed.UpdateSource = domain.NewGitHubUpdateSource("owner/repo", false)
	deps.apps.listApps = []domain.App{installed}
	deps.ServiceDeps.GitHubReleases = &fakeGitHubReleaseFinder{release: testGitHubReleaseWithTag("v1.2.3", "Example.AppImage")}
	deps.ServiceDeps.Downloads = &fakeAssetDownloader{}
	confirmation := &fakeUpdateConfirmation{confirmed: true}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Update(context.Background(), UpdateRequest{Confirmation: confirmation})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !result.Applied {
		t.Fatal("Update().Applied = false, want true")
	}
	assertUpdateCandidates(t, result.Updates, nil)
	if confirmation.called {
		t.Fatal("confirmation called with no update candidates")
	}
	if deps.saved.App.ID != "" {
		t.Fatalf("saved App.ID = %q, want empty", deps.saved.App.ID)
	}
}

func TestServiceUpdateDoesNotApplyWhenConfirmationRejects(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	installed.UpdateSource = domain.NewGitHubUpdateSource("owner/repo", false)
	deps.apps.listApps = []domain.App{installed}
	deps.ServiceDeps.GitHubReleases = &fakeGitHubReleaseFinder{release: testGitHubReleaseWithTag("v2.0.0", "Example.AppImage")}
	deps.ServiceDeps.Downloads = &fakeAssetDownloader{}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Update(context.Background(), UpdateRequest{Confirmation: &fakeUpdateConfirmation{confirmed: false}})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if result.Applied {
		t.Fatal("Update().Applied = true, want false")
	}
	if deps.saved.App.ID != "" {
		t.Fatalf("saved App.ID = %q, want empty", deps.saved.App.ID)
	}
}

func TestServiceUpdateSkipsAppsWithoutUpdateSource(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	deps.apps.listApps = []domain.App{testInstalledApp(t)}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Update(context.Background(), UpdateRequest{})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !result.Applied {
		t.Fatal("Update().Applied = false, want true")
	}
	assertUpdateCandidates(t, result.Updates, nil)
}

func TestServiceSelfUpdateInstallsLatestStableRelease(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	deps.ServiceDeps.CurrentVersion = "0.17.0"
	releases := &fakeGitHubReleaseFinder{release: testGitHubReleaseWithTag("v0.18.0", "aim-v0.18.0-linux-amd64.tar.gz")}
	installer := &fakeSelfUpdater{}
	deps.ServiceDeps.GitHubReleases = releases
	deps.ServiceDeps.SelfUpdater = installer
	confirmation := &fakeSelfUpdateConfirmation{confirmed: true}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.SelfUpdate(context.Background(), SelfUpdateRequest{Confirmation: confirmation})
	if err != nil {
		t.Fatalf("SelfUpdate() error = %v", err)
	}

	if !result.Applied {
		t.Fatal("SelfUpdate().Applied = false, want true")
	}
	if got, want := releases.repo, "slobbe/appimage-manager"; got != want {
		t.Fatalf("LatestRelease() repo = %q, want %q", got, want)
	}
	if releases.includePrerelease {
		t.Fatal("LatestRelease() includePrerelease = true, want false")
	}
	if got, want := confirmation.update.CurrentVersion, "0.17.0"; got != want {
		t.Fatalf("confirmation current version = %q, want %q", got, want)
	}
	if got, want := confirmation.update.NewVersion, "0.18.0"; got != want {
		t.Fatalf("confirmation new version = %q, want %q", got, want)
	}
	if got, want := installer.version, "v0.18.0"; got != want {
		t.Fatalf("Install() version = %q, want %q", got, want)
	}
}

func TestServiceSelfUpdateAllowsPrerelease(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	deps.ServiceDeps.CurrentVersion = "0.17.0"
	releases := &fakeGitHubReleaseFinder{release: testGitHubReleaseWithTag("v0.18.0-beta.1", "aim-v0.18.0-beta.1-linux-amd64.tar.gz")}
	installer := &fakeSelfUpdater{}
	deps.ServiceDeps.GitHubReleases = releases
	deps.ServiceDeps.SelfUpdater = installer
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.SelfUpdate(context.Background(), SelfUpdateRequest{Prerelease: true, Confirmation: &fakeSelfUpdateConfirmation{confirmed: true}})
	if err != nil {
		t.Fatalf("SelfUpdate() error = %v", err)
	}
	if !releases.includePrerelease {
		t.Fatal("LatestRelease() includePrerelease = false, want true")
	}
	if got, want := installer.version, "v0.18.0-beta.1"; got != want {
		t.Fatalf("Install() version = %q, want %q", got, want)
	}
}

func TestServiceSelfUpdateDoesNotInstallWhenUpToDate(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	deps.ServiceDeps.CurrentVersion = "v0.18.0"
	deps.ServiceDeps.GitHubReleases = &fakeGitHubReleaseFinder{release: testGitHubReleaseWithTag("v0.18.0", "aim-v0.18.0-linux-amd64.tar.gz")}
	installer := &fakeSelfUpdater{}
	deps.ServiceDeps.SelfUpdater = installer
	confirmation := &fakeSelfUpdateConfirmation{confirmed: true}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.SelfUpdate(context.Background(), SelfUpdateRequest{Confirmation: confirmation})
	if err != nil {
		t.Fatalf("SelfUpdate() error = %v", err)
	}
	if !result.Applied {
		t.Fatal("SelfUpdate().Applied = false, want true")
	}
	if got, want := result.Update.CurrentVersion, "0.18.0"; got != want {
		t.Fatalf("CurrentVersion = %q, want %q", got, want)
	}
	if got, want := result.Update.NewVersion, "0.18.0"; got != want {
		t.Fatalf("NewVersion = %q, want %q", got, want)
	}
	if confirmation.called {
		t.Fatal("confirmation called for up-to-date self-update")
	}
	if installer.called {
		t.Fatal("installer called for up-to-date self-update")
	}
}

func TestServiceSelfUpdateDoesNotInstallWhenConfirmationRejects(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	deps.ServiceDeps.CurrentVersion = "0.17.0"
	deps.ServiceDeps.GitHubReleases = &fakeGitHubReleaseFinder{release: testGitHubReleaseWithTag("v0.18.0", "aim-v0.18.0-linux-amd64.tar.gz")}
	installer := &fakeSelfUpdater{}
	deps.ServiceDeps.SelfUpdater = installer
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.SelfUpdate(context.Background(), SelfUpdateRequest{Confirmation: &fakeSelfUpdateConfirmation{confirmed: false}})
	if err != nil {
		t.Fatalf("SelfUpdate() error = %v", err)
	}
	if result.Applied {
		t.Fatal("SelfUpdate().Applied = true, want false")
	}
	if installer.called {
		t.Fatal("installer called after confirmation rejected")
	}
}

func TestServiceSetUpdateSourceSetsGitHubSource(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	deps.apps.findApp = installed
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.SetUpdateSource(context.Background(), SetUpdateSourceRequest{ID: " example-app ", GitHubRepo: " owner/repo ", AssetPattern: "Example-arm64.AppImage", Prerelease: true})
	if err != nil {
		t.Fatalf("SetUpdateSource() error = %v", err)
	}

	if got, want := deps.apps.findID, installed.ID; got != want {
		t.Fatalf("Find() id = %q, want %q", got, want)
	}
	if got, want := result.ID, installed.ID; got != want {
		t.Fatalf("result.ID = %q, want %q", got, want)
	}
	if got, want := result.UpdateSource.Kind, domain.UpdateSourceKindGitHub; got != want {
		t.Fatalf("UpdateSource.Kind = %q, want %q", got, want)
	}
	if got, want := result.UpdateSource.Repo, "owner/repo"; got != want {
		t.Fatalf("UpdateSource.Repo = %q, want %q", got, want)
	}
	if got, want := result.UpdateSource.AssetPattern, "Example-arm64.AppImage"; got != want {
		t.Fatalf("UpdateSource.AssetPattern = %q, want %q", got, want)
	}
	if !result.UpdateSource.Prerelease {
		t.Fatal("UpdateSource.Prerelease = false, want true")
	}
	if got, want := deps.saved.App.UpdateSource, result.UpdateSource; got != want {
		t.Fatalf("saved UpdateSource = %#v, want %#v", got, want)
	}
}

func TestServiceSetUpdateSourceSetsEmbeddedSource(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	deps.apps.findApp = installed
	raw := "gh-releases-zsync|owner|repo|latest|Example-*.AppImage.zsync"
	deps.appImages.updateInfo = raw
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.SetUpdateSource(context.Background(), SetUpdateSourceRequest{ID: installed.ID, Embedded: true})
	if err != nil {
		t.Fatalf("SetUpdateSource() error = %v", err)
	}

	if got, want := deps.appImages.appImagePath, installed.AppImagePath; got != want {
		t.Fatalf("Extract() appImagePath = %q, want %q", got, want)
	}
	if !strings.HasSuffix(deps.appImages.destDir, "/workspace/extract") {
		t.Fatalf("Extract() destDir = %q, want workspace extract dir", deps.appImages.destDir)
	}
	if !deps.workspaces.cleaned {
		t.Fatal("workspace was not cleaned")
	}
	if got, want := result.UpdateSource.Kind, domain.UpdateSourceKindGitHub; got != want {
		t.Fatalf("UpdateSource.Kind = %q, want %q", got, want)
	}
	if got := result.UpdateSource.Raw; got != raw {
		t.Fatalf("UpdateSource.Raw = %q, want %q", got, raw)
	}
	if got, want := deps.saved.App.UpdateSource, result.UpdateSource; got != want {
		t.Fatalf("saved UpdateSource = %#v, want %#v", got, want)
	}
}

func TestServiceSetUpdateSourceReturnsErrorWhenEmbeddedInfoMissing(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	deps.apps.findApp = installed
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.SetUpdateSource(context.Background(), SetUpdateSourceRequest{ID: installed.ID, Embedded: true})
	if err == nil || !strings.Contains(err.Error(), "embedded update information not found") {
		t.Fatalf("SetUpdateSource() error = %v, want missing embedded update info error", err)
	}
	if deps.saved.App.ID != "" {
		t.Fatalf("saved App.ID = %q, want empty", deps.saved.App.ID)
	}
}

func TestServiceUnsetUpdateSourceClearsUpdateSource(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	installed.UpdateSource = domain.NewGitHubUpdateSource("owner/repo", true)
	deps.apps.findApp = installed
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if err := service.UnsetUpdateSource(context.Background(), UnsetUpdateSourceRequest{ID: installed.ID}); err != nil {
		t.Fatalf("UnsetUpdateSource() error = %v", err)
	}

	if got, want := deps.saved.App.ID, installed.ID; got != want {
		t.Fatalf("saved App.ID = %q, want %q", got, want)
	}
	if got, want := deps.saved.App.UpdateSource.Kind, domain.UpdateSourceKindUnknown; got != want {
		t.Fatalf("saved UpdateSource.Kind = %q, want %q", got, want)
	}
}

func TestServiceListReturnsInstalledApps(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	first := testInstalledApp(t)
	secondVersion, ok := domain.ParseVersion("2.0.0")
	if !ok {
		t.Fatal("ParseVersion() ok = false, want true")
	}
	second := domain.App{
		ID:      "another-app",
		Name:    "Another App",
		Version: secondVersion,
	}
	deps.apps.listApps = []domain.App{first, second}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.List(context.Background(), ListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	want := []ListItem{
		{ID: "example-app", Name: "Example App", Version: "1.2.3"},
		{ID: "another-app", Name: "Another App", Version: "2.0.0"},
	}
	if len(result.Items) != len(want) {
		t.Fatalf("List() items = %#v, want %#v", result.Items, want)
	}
	for i := range want {
		if result.Items[i] != want[i] {
			t.Fatalf("List() items = %#v, want %#v", result.Items, want)
		}
	}
}

func TestServiceListReturnsRepositoryFailure(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	failure := errors.New("list failed")
	deps.apps.listErr = failure
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.List(context.Background(), ListRequest{})
	if !errors.Is(err, failure) {
		t.Fatalf("List() error = %v, want %v", err, failure)
	}
}

func TestServiceInfoReturnsInstalledApp(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	deps.apps.findApp = installed
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Info(context.Background(), InfoRequest{Target: "  example-app  "})
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}

	if got, want := deps.apps.findID, installed.ID; got != want {
		t.Fatalf("Find() id = %q, want %q", got, want)
	}
	if got, want := result.ID, installed.ID; got != want {
		t.Fatalf("Info().ID = %q, want %q", got, want)
	}
	if got, want := result.Name, installed.Name; got != want {
		t.Fatalf("Info().Name = %q, want %q", got, want)
	}
	if got, want := result.Version, installed.Version.String(); got != want {
		t.Fatalf("Info().Version = %q, want %q", got, want)
	}
	if got, want := result.ExecPath, installed.AppImagePath; got != want {
		t.Fatalf("Info().ExecPath = %q, want %q", got, want)
	}
	if got, want := result.Source, installed.Source; got != want {
		t.Fatalf("Info().Source = %#v, want %#v", got, want)
	}
	if got, want := result.UpdateSource, installed.UpdateSource; got != want {
		t.Fatalf("Info().UpdateSource = %#v, want %#v", got, want)
	}
}

func TestServiceInfoValidatesTarget(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Info(context.Background(), InfoRequest{Target: "  "})
	if err == nil {
		t.Fatal("Info() error = nil, want error")
	}
	if deps.apps.findID != "" {
		t.Fatalf("Find() id = %q, want empty", deps.apps.findID)
	}
}

func TestServiceInfoReturnsFindFailure(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	deps.apps.findErr = ErrAppNotFound
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Info(context.Background(), InfoRequest{Target: "missing"})
	if !errors.Is(err, ErrAppNotFound) {
		t.Fatalf("Info() error = %v, want ErrAppNotFound", err)
	}
}

func TestServicePathsReturnsConfiguredPaths(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	deps.ServiceDeps.Config = Config{
		ConfigFile:  "/config/aim/config.toml",
		AppImageDir: "/data/aim/appimages",
		CacheDir:    "/cache/aim",
		DesktopDir:  "/data/applications",
		IconDir:     "/data/icons",
	}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Paths(context.Background(), PathsRequest{})
	if err != nil {
		t.Fatalf("Paths() error = %v", err)
	}

	if got, want := result.ConfigFile, deps.ServiceDeps.Config.ConfigFile; got != want {
		t.Fatalf("Paths().ConfigFile = %q, want %q", got, want)
	}
	if got, want := result.AppImageDir, deps.ServiceDeps.Config.AppImageDir; got != want {
		t.Fatalf("Paths().AppImageDir = %q, want %q", got, want)
	}
	if got, want := result.CacheDir, deps.ServiceDeps.Config.CacheDir; got != want {
		t.Fatalf("Paths().CacheDir = %q, want %q", got, want)
	}
	if got, want := result.DesktopDir, deps.ServiceDeps.Config.DesktopDir; got != want {
		t.Fatalf("Paths().DesktopDir = %q, want %q", got, want)
	}
	if got, want := result.IconDir, deps.ServiceDeps.Config.IconDir; got != want {
		t.Fatalf("Paths().IconDir = %q, want %q", got, want)
	}
}

func TestServiceAddFromGitHubIntegratesDownloadedAppImage(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	releases := &fakeGitHubReleaseFinder{
		release: testGitHubRelease("Example.AppImage"),
	}
	downloads := &fakeAssetDownloader{}
	deps.ServiceDeps.GitHubReleases = releases
	deps.ServiceDeps.Downloads = downloads
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Add(context.Background(), AddRequest{GitHubRepo: " owner/repo "})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if got, want := releases.repo, "owner/repo"; got != want {
		t.Fatalf("LatestRelease() repo = %q, want %q", got, want)
	}
	if releases.includePrerelease {
		t.Fatal("LatestRelease() includePrerelease = true, want false")
	}
	if got, want := downloads.source.URL, "https://example.test/Example.AppImage"; got != want {
		t.Fatalf("Download() source URL = %q, want %q", got, want)
	}
	if got, want := downloads.source.FileName, "Example.AppImage"; got != want {
		t.Fatalf("Download() source FileName = %q, want %q", got, want)
	}
	if got, want := downloads.destinationPath, "/workspace/Example.AppImage"; got != want {
		t.Fatalf("Download() destination = %q, want %q", got, want)
	}
	if downloads.progress == nil {
		t.Fatal("Download() progress = nil, want progress task")
	}
	if got, want := deps.appImages.appImagePath, "/workspace/Example.AppImage"; got != want {
		t.Fatalf("Extract() appImagePath = %q, want %q", got, want)
	}
	if got, want := deps.appImageInstaller.sourcePath, "/workspace/Example.AppImage"; got != want {
		t.Fatalf("appimage installer source = %q, want %q", got, want)
	}
	if got, want := result.App.Source.Kind, domain.SourceKindGitHub; got != want {
		t.Fatalf("App.Source.Kind = %q, want %q", got, want)
	}
	if got, want := result.App.Source.GitHubRelease.Repo, "owner/repo"; got != want {
		t.Fatalf("App.Source.GitHubRelease.Repo = %q, want %q", got, want)
	}
	if got, want := result.App.Source.GitHubRelease.Tag, "v1.2.3"; got != want {
		t.Fatalf("App.Source.GitHubRelease.Tag = %q, want %q", got, want)
	}
	if got, want := result.App.Source.GitHubRelease.Asset, "Example.AppImage"; got != want {
		t.Fatalf("App.Source.GitHubRelease.Asset = %q, want %q", got, want)
	}
	if result.App.Source.GitHubRelease.DownloadedAt.IsZero() {
		t.Fatal("App.Source.GitHubRelease.DownloadedAt is zero, want timestamp")
	}
	if got, want := result.App.UpdateSource.Kind, domain.UpdateSourceKindGitHub; got != want {
		t.Fatalf("App.UpdateSource.Kind = %q, want %q", got, want)
	}
	if got, want := result.App.UpdateSource.Repo, "owner/repo"; got != want {
		t.Fatalf("App.UpdateSource.Repo = %q, want %q", got, want)
	}
	if result.App.UpdateSource.Embedded {
		t.Fatal("App.UpdateSource.Embedded = true, want false")
	}
	if got, want := deps.saved.App.Source.Kind, domain.SourceKindGitHub; got != want {
		t.Fatalf("saved App.Source.Kind = %q, want %q", got, want)
	}
	if got, want := deps.saved.App.UpdateSource.Kind, domain.UpdateSourceKindGitHub; got != want {
		t.Fatalf("saved App.UpdateSource.Kind = %q, want %q", got, want)
	}
	if got, want := deps.saved.App.UpdateSource.Repo, "owner/repo"; got != want {
		t.Fatalf("saved App.UpdateSource.Repo = %q, want %q", got, want)
	}
	if !deps.workspaces.cleaned {
		t.Fatal("workspace was not cleaned")
	}
}

func TestServiceAddFromGitHubUsesAssetPattern(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	releases := &fakeGitHubReleaseFinder{release: testGitHubRelease("Example-arm64.AppImage", "Example-x86_64.AppImage")}
	downloads := &fakeAssetDownloader{}
	deps.ServiceDeps.GitHubReleases = releases
	deps.ServiceDeps.Downloads = downloads
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Add(context.Background(), AddRequest{GitHubRepo: "owner/repo", AssetPattern: "Example-arm64.AppImage"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if got, want := downloads.source.FileName, "Example-arm64.AppImage"; got != want {
		t.Fatalf("Download() source FileName = %q, want %q", got, want)
	}
	if got, want := result.App.Source.GitHubRelease.Asset, "Example-arm64.AppImage"; got != want {
		t.Fatalf("App.Source.GitHubRelease.Asset = %q, want %q", got, want)
	}
	if got, want := result.App.UpdateSource.AssetPattern, "Example-arm64.AppImage"; got != want {
		t.Fatalf("App.UpdateSource.AssetPattern = %q, want %q", got, want)
	}
	if got, want := deps.saved.App.UpdateSource.AssetPattern, "Example-arm64.AppImage"; got != want {
		t.Fatalf("saved App.UpdateSource.AssetPattern = %q, want %q", got, want)
	}
	if got, want := deps.appImageInstaller.sourcePath, "/workspace/Example-arm64.AppImage"; got != want {
		t.Fatalf("appimage installer source = %q, want %q", got, want)
	}
	if got, want := releases.repo, "owner/repo"; got != want {
		t.Fatalf("LatestRelease() repo = %q, want %q", got, want)
	}
}

func TestServiceAddFromGitHubStoresPrereleaseUpdateSource(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	releases := &fakeGitHubReleaseFinder{release: testGitHubRelease("Example.AppImage")}
	deps.ServiceDeps.GitHubReleases = releases
	deps.ServiceDeps.Downloads = &fakeAssetDownloader{}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Add(context.Background(), AddRequest{GitHubRepo: "owner/repo", Prerelease: true})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if !releases.includePrerelease {
		t.Fatal("LatestRelease() includePrerelease = false, want true")
	}
	if !result.App.UpdateSource.Prerelease {
		t.Fatal("App.UpdateSource.Prerelease = false, want true")
	}
	if !deps.saved.App.UpdateSource.Prerelease {
		t.Fatal("saved App.UpdateSource.Prerelease = false, want true")
	}
}

func TestServiceAddFromGitHubUsesReleaseTagAsVersionFallback(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	deps.desktopEntries.content = []byte(strings.Join([]string{
		"[Desktop Entry]",
		"Name=Generic Name",
		"X-AppImage-Name=Example App",
		"Exec=old-exec",
		"Icon=example-icon",
		"",
	}, "\n"))
	deps.ServiceDeps.GitHubReleases = &fakeGitHubReleaseFinder{
		release: testGitHubRelease("Example.AppImage"),
	}
	deps.ServiceDeps.Downloads = &fakeAssetDownloader{}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Add(context.Background(), AddRequest{GitHubRepo: "owner/repo"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if got, want := result.App.Version.String(), "1.2.3"; got != want {
		t.Fatalf("App.Version = %q, want %q", got, want)
	}
	if got, want := deps.saved.App.Version.String(), "1.2.3"; got != want {
		t.Fatalf("saved App.Version = %q, want %q", got, want)
	}
}

func TestServiceAddFromGitHubRequiresReleaseFinder(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	deps.ServiceDeps.Downloads = &fakeAssetDownloader{}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Add(context.Background(), AddRequest{GitHubRepo: "owner/repo"})
	if err == nil || !strings.Contains(err.Error(), "github release finder is required") {
		t.Fatalf("Add() error = %v, want missing release finder error", err)
	}
}

func TestServiceAddFromGitHubRequiresDownloader(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	deps.ServiceDeps.GitHubReleases = &fakeGitHubReleaseFinder{}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Add(context.Background(), AddRequest{GitHubRepo: "owner/repo"})
	if err == nil || !strings.Contains(err.Error(), "asset downloader is required") {
		t.Fatalf("Add() error = %v, want missing downloader error", err)
	}
}

func TestServiceAddFromGitHubReturnsReleaseFailure(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	failure := errors.New("release failed")
	deps.ServiceDeps.GitHubReleases = &fakeGitHubReleaseFinder{err: failure}
	deps.ServiceDeps.Downloads = &fakeAssetDownloader{}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Add(context.Background(), AddRequest{GitHubRepo: "owner/repo"})
	if !errors.Is(err, failure) {
		t.Fatalf("Add() error = %v, want %v", err, failure)
	}
	if deps.appImages.appImagePath != "" {
		t.Fatalf("Extract() appImagePath = %q, want empty", deps.appImages.appImagePath)
	}
}

func TestServiceAddFromGitHubReturnsNoAssetFailure(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	deps.ServiceDeps.GitHubReleases = &fakeGitHubReleaseFinder{release: testGitHubRelease("Example.deb")}
	deps.ServiceDeps.Downloads = &fakeAssetDownloader{}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Add(context.Background(), AddRequest{GitHubRepo: "owner/repo"})
	if err == nil || !strings.Contains(err.Error(), "no AppImage assets") {
		t.Fatalf("Add() error = %v, want no AppImage assets error", err)
	}
	if deps.appImages.appImagePath != "" {
		t.Fatalf("Extract() appImagePath = %q, want empty", deps.appImages.appImagePath)
	}
}

func TestServiceAddFromGitHubReturnsDownloadFailure(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	failure := errors.New("download failed")
	downloads := &fakeAssetDownloader{err: failure}
	deps.ServiceDeps.GitHubReleases = &fakeGitHubReleaseFinder{release: testGitHubRelease("Example.AppImage")}
	deps.ServiceDeps.Downloads = downloads
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Add(context.Background(), AddRequest{GitHubRepo: "owner/repo"})
	if !errors.Is(err, failure) {
		t.Fatalf("Add() error = %v, want %v", err, failure)
	}
	if deps.appImages.appImagePath != "" {
		t.Fatalf("Extract() appImagePath = %q, want empty", deps.appImages.appImagePath)
	}
	if !deps.workspaces.cleaned {
		t.Fatal("workspace was not cleaned")
	}
}

func TestServiceAddFromGitHubValidatesRepo(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	deps.ServiceDeps.GitHubReleases = &fakeGitHubReleaseFinder{}
	deps.ServiceDeps.Downloads = &fakeAssetDownloader{}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Add(context.Background(), AddRequest{GitHubRepo: "owner/repo/extra"})
	if err == nil || !strings.Contains(err.Error(), "owner/repo") {
		t.Fatalf("Add() error = %v, want repo format error", err)
	}
}

func TestNewServiceValidatesDependencies(t *testing.T) {
	t.Parallel()

	_, err := NewService(ServiceDeps{})
	if err == nil {
		t.Fatal("NewService() error = nil, want error")
	}
}

type integrationFakes struct {
	ServiceDeps
	workspaces            *fakeWorkspaceProvider
	appImages             *fakeAppImageExtractor
	desktopEntries        *fakeDesktopEntryDiscoverer
	icons                 *fakeIconDiscoverer
	appImageInstaller     *fakeAppImageInstaller
	appImageRemover       *fakeArtifactRemover
	iconInstaller         *fakeIconInstaller
	iconRemover           *fakeArtifactRemover
	desktopEntryInstaller *fakeDesktopEntryInstaller
	desktopEntryRemover   *fakeArtifactRemover
	apps                  *fakeAppRepository
	saved                 *fakeAppRepository
}

func integrationTestDeps() integrationFakes {
	workspaces := &fakeWorkspaceProvider{path: "/workspace"}
	appImages := &fakeAppImageExtractor{rootDir: "/extracted"}
	desktopEntries := &fakeDesktopEntryDiscoverer{
		path: "/extracted/example.desktop",
		content: []byte(strings.Join([]string{
			"[Desktop Entry]",
			"Name=Generic Name",
			"X-AppImage-Name=Example App",
			"X-AppImage-Version=1.2.3-beta.1",
			"Exec=old-exec",
			"Icon=example-icon",
			"",
			"[Desktop Action NewWindow]",
			"Exec=old-action",
			"",
		}, "\n")),
	}
	icons := &fakeIconDiscoverer{path: "/extracted/example.png"}
	appImageInstaller := &fakeAppImageInstaller{path: "/library/example-app.AppImage"}
	appImageRemover := &fakeArtifactRemover{}
	iconInstaller := &fakeIconInstaller{path: "/icons/hicolor/256x256/apps/example-app.png"}
	iconRemover := &fakeArtifactRemover{}
	desktopEntryInstaller := &fakeDesktopEntryInstaller{path: "/desktop/example-app.desktop"}
	desktopEntryRemover := &fakeArtifactRemover{}
	apps := &fakeAppRepository{}

	return integrationFakes{
		ServiceDeps: ServiceDeps{
			Workspaces:            workspaces,
			AppImages:             appImages,
			DesktopEntries:        desktopEntries,
			Icons:                 icons,
			AppImageInstaller:     appImageInstaller,
			AppImageRemover:       appImageRemover,
			IconInstaller:         iconInstaller,
			IconRemover:           iconRemover,
			DesktopEntryInstaller: desktopEntryInstaller,
			DesktopEntryRemover:   desktopEntryRemover,
			Apps:                  apps,
		},
		workspaces:            workspaces,
		appImages:             appImages,
		desktopEntries:        desktopEntries,
		icons:                 icons,
		appImageInstaller:     appImageInstaller,
		appImageRemover:       appImageRemover,
		iconInstaller:         iconInstaller,
		iconRemover:           iconRemover,
		desktopEntryInstaller: desktopEntryInstaller,
		desktopEntryRemover:   desktopEntryRemover,
		apps:                  apps,
		saved:                 apps,
	}
}

type fakeWorkspaceProvider struct {
	path    string
	cleaned bool
	err     error
}

func (f *fakeWorkspaceProvider) Create(ctx context.Context) (Workspace, error) {
	if f.err != nil {
		return Workspace{}, f.err
	}
	return Workspace{Path: f.path, Cleanup: func() error { f.cleaned = true; return nil }}, nil
}

type fakeAppImageExtractor struct {
	appImagePath string
	destDir      string
	rootDir      string
	updateInfo   string
	err          error
}

func (f *fakeAppImageExtractor) Extract(ctx context.Context, appImagePath string, destDir string) (AppImageExtraction, error) {
	f.appImagePath = appImagePath
	f.destDir = destDir
	if f.err != nil {
		return AppImageExtraction{}, f.err
	}
	return AppImageExtraction{RootDir: f.rootDir, UpdateInfo: f.updateInfo}, nil
}

type fakeDesktopEntryDiscoverer struct {
	rootDir string
	path    string
	content []byte
	err     error
}

func (f *fakeDesktopEntryDiscoverer) Discover(ctx context.Context, rootDir string) (DesktopEntryFile, error) {
	f.rootDir = rootDir
	if f.err != nil {
		return DesktopEntryFile{}, f.err
	}
	return DesktopEntryFile{Path: f.path, Content: f.content}, nil
}

type fakeIconDiscoverer struct {
	called   bool
	rootDir  string
	iconName string
	path     string
	err      error
}

func (f *fakeIconDiscoverer) Discover(ctx context.Context, rootDir string, iconName string) (IconFile, error) {
	f.called = true
	f.rootDir = rootDir
	f.iconName = iconName
	if f.err != nil {
		return IconFile{}, f.err
	}
	return IconFile{Path: f.path}, nil
}

type fakeAppImageInstaller struct {
	called     bool
	sourcePath string
	appID      string
	path       string
	err        error
}

func (f *fakeAppImageInstaller) Install(ctx context.Context, sourcePath string, appID string) (string, error) {
	f.called = true
	f.sourcePath = sourcePath
	f.appID = appID
	if f.err != nil {
		return "", f.err
	}
	return f.path, nil
}

type fakeIconInstaller struct {
	sourcePath string
	appID      string
	path       string
	err        error
}

func (f *fakeIconInstaller) Install(ctx context.Context, sourcePath string, appID string) (string, error) {
	f.sourcePath = sourcePath
	f.appID = appID
	if f.err != nil {
		return "", f.err
	}
	return f.path, nil
}

type fakeDesktopEntryInstaller struct {
	appID   string
	content []byte
	path    string
	err     error
}

func (f *fakeDesktopEntryInstaller) Install(ctx context.Context, appID string, content []byte) (string, error) {
	f.appID = appID
	f.content = content
	if f.err != nil {
		return "", f.err
	}
	return f.path, nil
}

type fakeArtifactRemover struct {
	paths []string
	err   error
}

func (f *fakeArtifactRemover) Remove(ctx context.Context, path string) error {
	f.paths = append(f.paths, path)
	return f.err
}

func assertUpdateCandidates(t *testing.T, got []UpdateCandidate, want []UpdateCandidate) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("updates = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("updates = %#v, want %#v", got, want)
		}
	}
}

func testGitHubReleaseWithTag(tag string, assetNames ...string) GitHubRelease {
	release := testGitHubRelease(assetNames...)
	release.TagName = tag
	return release
}

type fakeSelfUpdater struct {
	called  bool
	version string
	err     error
}

func (f *fakeSelfUpdater) Install(ctx context.Context, version string) error {
	f.called = true
	f.version = version
	return f.err
}

type fakeSelfUpdateConfirmation struct {
	called    bool
	update    SelfUpdateCandidate
	confirmed bool
	err       error
}

func (f *fakeSelfUpdateConfirmation) ConfirmSelfUpdate(ctx context.Context, update SelfUpdateCandidate) (bool, error) {
	f.called = true
	f.update = update
	if f.err != nil {
		return false, f.err
	}
	return f.confirmed, nil
}

type fakeUpdateConfirmation struct {
	called    bool
	updates   []UpdateCandidate
	confirmed bool
	err       error
}

func (f *fakeUpdateConfirmation) ConfirmUpdates(ctx context.Context, updates []UpdateCandidate) (bool, error) {
	f.called = true
	f.updates = append([]UpdateCandidate(nil), updates...)
	if f.err != nil {
		return false, f.err
	}
	return f.confirmed, nil
}

func assertRemovedPaths(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("removed paths = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("removed paths = %v, want %v", got, want)
		}
	}
}

type fakeGitHubReleaseFinder struct {
	repo              string
	includePrerelease bool
	tag               string
	method            string
	release           GitHubRelease
	err               error
}

func (f *fakeGitHubReleaseFinder) LatestRelease(ctx context.Context, repo string, includePrerelease bool) (GitHubRelease, error) {
	f.repo = repo
	f.includePrerelease = includePrerelease
	f.method = "latest"
	if f.err != nil {
		return GitHubRelease{}, f.err
	}

	return f.release, nil
}

func (f *fakeGitHubReleaseFinder) LatestPrerelease(ctx context.Context, repo string) (GitHubRelease, error) {
	f.repo = repo
	f.method = "latest-prerelease"
	if f.err != nil {
		return GitHubRelease{}, f.err
	}

	return f.release, nil
}

func (f *fakeGitHubReleaseFinder) ReleaseByTag(ctx context.Context, repo string, tag string) (GitHubRelease, error) {
	f.repo = repo
	f.tag = tag
	f.method = "tag"
	if f.err != nil {
		return GitHubRelease{}, f.err
	}

	return f.release, nil
}

type fakeAssetDownloader struct {
	source          DownloadSource
	destinationPath string
	progress        DownloadProgress
	downloaded      DownloadedFile
	err             error
}

func (f *fakeAssetDownloader) Download(ctx context.Context, source DownloadSource, destinationPath string, progress DownloadProgress) (DownloadedFile, error) {
	f.source = source
	f.destinationPath = destinationPath
	f.progress = progress
	if f.err != nil {
		return DownloadedFile{}, f.err
	}
	if f.downloaded.Path == "" {
		f.downloaded.Path = destinationPath
	}

	return f.downloaded, nil
}

type fakeAppRepository struct {
	App       domain.App
	err       error
	findID    string
	findApp   domain.App
	findErr   error
	listApps  []domain.App
	listErr   error
	deletedID string
	deleteErr error
}

func (f *fakeAppRepository) Save(ctx context.Context, app domain.App) error {
	f.App = app
	return f.err
}

func (f *fakeAppRepository) Find(ctx context.Context, id string) (domain.App, error) {
	f.findID = id
	if f.findErr != nil {
		return domain.App{}, f.findErr
	}
	if f.findApp.ID == "" {
		return domain.App{}, ErrAppNotFound
	}
	return f.findApp, nil
}

func (f *fakeAppRepository) List(ctx context.Context) ([]domain.App, error) {
	return f.listApps, f.listErr
}

func (f *fakeAppRepository) Delete(ctx context.Context, id string) error {
	f.deletedID = id
	return f.deleteErr
}

func testSourceTime() time.Time {
	return time.Date(2026, 6, 3, 14, 6, 7, 0, time.UTC)
}

func testInstalledApp(t *testing.T) domain.App {
	t.Helper()

	version, ok := domain.ParseVersion("1.2.3")
	if !ok {
		t.Fatal("ParseVersion() ok = false, want true")
	}

	return domain.App{
		ID:               "example-app",
		Name:             "Example App",
		Version:          version,
		AppImagePath:     "/library/example-app.AppImage",
		DesktopEntryPath: "/desktop/example-app.desktop",
		IconPath:         "/icons/hicolor/256x256/apps/example-app.png",
		Source:           domain.NewLocalSource("/downloads/example.AppImage", testSourceTime()),
		UpdateSource:     domain.UpdateSource{},
	}
}
