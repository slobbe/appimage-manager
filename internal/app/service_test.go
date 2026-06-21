package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/slobbe/appimage-manager/internal/domain"
)

func TestServiceAddIntegratesLocalAppImage(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	sourcePath := testAppImagePath(t, "example.AppImage")

	result, err := service.Add(context.Background(), AddRequest{Path: sourcePath})
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
	if got, want := result.App.Source.LocalFile.Path, sourcePath; got != want {
		t.Fatalf("App.Source.LocalFile.Path = %q, want %q", got, want)
	}
	if result.App.Source.LocalFile.IntegratedAt.IsZero() {
		t.Fatal("App.Source.LocalFile.IntegratedAt is zero, want timestamp")
	}
	if got, want := result.App.UpdateSource.Kind, domain.UpdateSourceKindUnknown; got != want {
		t.Fatalf("App.UpdateSource.Kind = %q, want %q", got, want)
	}

	workspacePath := workspaceFromExtractDir(t, deps.appImages.destDir)
	assertWorkspaceCleaned(t, workspacePath)
	if got := deps.appImages.appImagePath; got == sourcePath {
		t.Fatalf("extract appImagePath = %q, want staged path", got)
	}
	if got, want := deps.appImages.appImagePath, filepath.Join(workspacePath, "example.AppImage"); got != want {
		t.Fatalf("extract appImagePath = %q, want %q", got, want)
	}
	if !strings.HasSuffix(deps.appImages.destDir, string(filepath.Separator)+"extract") {
		t.Fatalf("extract destDir = %q, want workspace extract dir", deps.appImages.destDir)
	}
	if got, want := deps.icons.iconName, "example-icon"; got != want {
		t.Fatalf("icon discover iconName = %q, want %q", got, want)
	}
	if got, want := deps.appImageInstaller.sourcePath, sourcePath; got != want {
		t.Fatalf("appimage installer source = %q, want %q", got, want)
	}
	if got, want := deps.iconInstaller.sourcePath, "/extracted/example.png"; got != want {
		t.Fatalf("icon installer source = %q, want %q", got, want)
	}

	if !deps.desktopIntegrationRefresher.called {
		t.Fatal("desktop integration refresher was not called")
	}
	if got, want := deps.saved.App.ID, result.App.ID; got != want {
		t.Fatalf("saved App.ID = %q, want %q", got, want)
	}
	assertRemovedPaths(t, deps.artifactRemover.paths, nil)

	desktopContent := string(deps.desktopEntryInstaller.content)
	if !strings.Contains(desktopContent, "Exec=/library/example-app.AppImage %U") {
		t.Fatalf("desktop content = %q, want updated Exec with arguments preserved", desktopContent)
	}
	if !strings.Contains(desktopContent, "Icon=/icons/hicolor/256x256/apps/example-app.png") {
		t.Fatalf("desktop content = %q, want absolute updated Icon", desktopContent)
	}
	if !strings.Contains(desktopContent, "[Desktop Action NewWindow]") {
		t.Fatalf("desktop content = %q, want action group preserved", desktopContent)
	}
	if !strings.Contains(desktopContent, "Exec=/library/example-app.AppImage --new-window %U") {
		t.Fatalf("desktop content = %q, want rewritten action Exec with arguments preserved", desktopContent)
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

	result, err := service.Add(context.Background(), AddRequest{Path: testAppImagePath(t, "example.AppImage")})
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

	result, err := service.Add(context.Background(), AddRequest{Path: testAppImagePath(t, "example.AppImage")})
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

	result, err := service.Add(context.Background(), AddRequest{Path: testAppImagePath(t, "Example_App-2.4.6-x86_64.AppImage")})
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

func TestServiceAddIgnoresDesktopIntegrationRefreshFailure(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	deps.desktopIntegrationRefresher.err = errors.New("refresh failed")
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Add(context.Background(), AddRequest{Path: testAppImagePath(t, "example.AppImage")})
	if err != nil {
		t.Fatalf("Add() error = %v, want refresh failure to be non-fatal", err)
	}
	if got, want := result.App.ID, "example-app"; got != want {
		t.Fatalf("App.ID = %q, want %q", got, want)
	}
	if !deps.desktopIntegrationRefresher.called {
		t.Fatal("desktop integration refresher was not called")
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

	_, err = service.Add(context.Background(), AddRequest{Path: testAppImagePath(t, "example.AppImage")})
	if !errors.Is(err, failure) {
		t.Fatalf("Add() error = %v, want %v", err, failure)
	}
	assertWorkspaceCleaned(t, workspaceFromExtractDir(t, deps.appImages.destDir))
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

	_, err = service.Add(context.Background(), AddRequest{Path: testAppImagePath(t, "example.AppImage")})
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

	_, err = service.Add(context.Background(), AddRequest{Path: testAppImagePath(t, "example.AppImage")})
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

	_, err = service.Add(context.Background(), AddRequest{Path: testAppImagePath(t, "example.AppImage")})
	if !errors.Is(err, failure) {
		t.Fatalf("Add() error = %v, want %v", err, failure)
	}
	assertRemovedPaths(t, deps.artifactRemover.paths, []string{
		"/desktop/example-app.desktop",
		"/icons/hicolor/256x256/apps/example-app.png",
		"/library/example-app.AppImage",
	})
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

	sourcePath := testAppImagePath(t, "example.AppImage")

	_, err = service.Add(context.Background(), AddRequest{Path: sourcePath})
	if !errors.Is(err, failure) {
		t.Fatalf("Add() error = %v, want %v", err, failure)
	}
	if got, want := deps.appImageInstaller.sourcePath, sourcePath; got != want {
		t.Fatalf("appimage installer source = %q, want %q", got, want)
	}
	assertRemovedPaths(t, deps.artifactRemover.paths, []string{"/library/example-app.AppImage"})
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

	assertRemovedPaths(t, deps.artifactRemover.paths, []string{installed.DesktopEntryPath, installed.IconPath, installed.AppImagePath})
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

	assertRemovedPaths(t, deps.artifactRemover.paths, []string{installed.AppImagePath})
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
	assertRemovedPaths(t, deps.artifactRemover.paths, nil)
}

func TestServiceRemoveStopsBeforeDeletingAppWhenArtifactRemovalFails(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	deps.apps.findApp = installed
	failure := errors.New("remove icon failed")
	deps.artifactRemover.err = failure
	deps.artifactRemover.failPath = installed.IconPath
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	err = service.Remove(context.Background(), RemoveRequest{Name: installed.ID})
	if !errors.Is(err, failure) {
		t.Fatalf("Remove() error = %v, want %v", err, failure)
	}
	assertRemovedPaths(t, deps.artifactRemover.paths, []string{installed.DesktopEntryPath, installed.IconPath})
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
	configureUpdateArtifactPaths(&deps, "example-app", "example-app-2-0-0")
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
	wantDownloadPath := filepath.Join(filepath.Dir(downloads.destinationPath), "Example.AppImage")
	if got, want := downloads.destinationPath, wantDownloadPath; got != want {
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
	if got, want := deps.saved.App.AppImagePath, "/library/example-app.AppImage"; got != want {
		t.Fatalf("saved App.AppImagePath = %q, want %q", got, want)
	}
	if got, want := deps.saved.App.IconPath, "/icons/hicolor/256x256/apps/example-app.png"; got != want {
		t.Fatalf("saved App.IconPath = %q, want %q", got, want)
	}
	if got, want := deps.saved.App.DesktopEntryPath, "/desktop/example-app.desktop"; got != want {
		t.Fatalf("saved App.DesktopEntryPath = %q, want %q", got, want)
	}
	assertInstallCallsByBase(t, deps.appImageInstaller.calls, []fakeInstallCall{
		{sourcePath: "Example.AppImage", appID: "example-app-2-0-0"},
		{sourcePath: "example-app-2-0-0.AppImage", appID: "example-app"},
	})
	assertInstallCalls(t, deps.iconInstaller.calls, []fakeInstallCall{
		{sourcePath: "/extracted/example.png", appID: "example-app-2-0-0"},
		{sourcePath: "/icons/hicolor/256x256/apps/example-app-2-0-0.png", appID: "example-app"},
	})
	finalDesktopContent := string(deps.desktopEntryInstaller.content)
	if strings.Contains(finalDesktopContent, "example-app-2-0-0") {
		t.Fatalf("desktop content = %q, want no staged ID", finalDesktopContent)
	}
	if !strings.Contains(finalDesktopContent, "Exec=/library/example-app.AppImage") {
		t.Fatalf("desktop content = %q, want stable Exec", finalDesktopContent)
	}
	if !strings.Contains(finalDesktopContent, "Icon=/icons/hicolor/256x256/apps/example-app.png") {
		t.Fatalf("desktop content = %q, want absolute stable Icon", finalDesktopContent)
	}
	assertRemovedPaths(t, deps.artifactRemover.paths, []string{
		"/desktop/example-app-2-0-0.desktop",
		"/icons/hicolor/256x256/apps/example-app-2-0-0.png",
		"/library/example-app-2-0-0.AppImage",
	})
}

func TestServiceUpdateCheckOnlyReturnsCandidatesWithoutApplying(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	installed.UpdateSource = domain.NewGitHubUpdateSource("owner/repo", false)
	deps.apps.listApps = []domain.App{installed}
	downloads := &fakeAssetDownloader{}
	deps.ServiceDeps.GitHubReleases = &fakeGitHubReleaseFinder{release: testGitHubReleaseWithTag("v2.0.0", "Example.AppImage")}
	deps.ServiceDeps.Downloads = downloads
	confirmation := &fakeUpdateConfirmation{confirmed: true}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Update(context.Background(), UpdateRequest{CheckOnly: true, Confirmation: confirmation})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if result.Applied {
		t.Fatal("Update().Applied = true, want false")
	}
	assertUpdateCandidates(t, result.Updates, []UpdateCandidate{{ID: installed.ID, CurrentVersion: "1.2.3", NewVersion: "2.0.0"}})
	if confirmation.called {
		t.Fatal("confirmation called for check-only update")
	}
	if downloads.destinationPath != "" {
		t.Fatalf("Download() destination = %q, want empty", downloads.destinationPath)
	}
	if deps.appImageInstaller.called {
		t.Fatal("appimage installer called for check-only update")
	}
	if deps.saved.App.ID != "" {
		t.Fatalf("saved App.ID = %q, want empty", deps.saved.App.ID)
	}
}

func TestServiceUpdateCheckOnlyWithNoCandidatesDoesNotApply(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	installed.UpdateSource = domain.NewGitHubUpdateSource("owner/repo", false)
	deps.apps.listApps = []domain.App{installed}
	downloads := &fakeAssetDownloader{}
	deps.ServiceDeps.GitHubReleases = &fakeGitHubReleaseFinder{release: testGitHubReleaseWithTag("v1.2.3", "Example.AppImage")}
	deps.ServiceDeps.Downloads = downloads
	confirmation := &fakeUpdateConfirmation{confirmed: true}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Update(context.Background(), UpdateRequest{CheckOnly: true, Confirmation: confirmation})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if result.Applied {
		t.Fatal("Update().Applied = true, want false")
	}
	assertUpdateCandidates(t, result.Updates, nil)
	if confirmation.called {
		t.Fatal("confirmation called for check-only update with no candidates")
	}
	if downloads.destinationPath != "" {
		t.Fatalf("Download() destination = %q, want empty", downloads.destinationPath)
	}
	if deps.appImageInstaller.called {
		t.Fatal("appimage installer called for check-only update with no candidates")
	}
	if deps.saved.App.ID != "" {
		t.Fatalf("saved App.ID = %q, want empty", deps.saved.App.ID)
	}
}

func TestServiceSetIDChangesInstalledAppID(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	deps.apps.findApps = map[string]domain.App{installed.ID: installed}
	deps.appImageInstaller.path = "/library/custom-id.AppImage"
	deps.iconInstaller.path = "/icons/hicolor/256x256/apps/custom-id.png"
	deps.desktopEntryInstaller.path = "/desktop/custom-id.desktop"
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.SetID(context.Background(), SetIDRequest{CurrentID: installed.ID, NewID: " Custom ID "})
	if err != nil {
		t.Fatalf("SetID() error = %v", err)
	}

	if !result.Changed {
		t.Fatal("SetID().Changed = false, want true")
	}
	if got, want := result.PreviousID, installed.ID; got != want {
		t.Fatalf("PreviousID = %q, want %q", got, want)
	}
	if got, want := result.App.ID, "custom-id"; got != want {
		t.Fatalf("App.ID = %q, want %q", got, want)
	}
	if got, want := deps.saved.App.ID, "custom-id"; got != want {
		t.Fatalf("saved App.ID = %q, want %q", got, want)
	}
	if got, want := deps.appImageInstaller.sourcePath, installed.AppImagePath; got != want {
		t.Fatalf("appimage installer source = %q, want %q", got, want)
	}
	if got, want := deps.appImageInstaller.appID, "custom-id"; got != want {
		t.Fatalf("appimage installer appID = %q, want %q", got, want)
	}
	if got, want := deps.iconInstaller.sourcePath, installed.IconPath; got != want {
		t.Fatalf("icon installer source = %q, want %q", got, want)
	}
	if got, want := deps.iconInstaller.appID, "custom-id"; got != want {
		t.Fatalf("icon installer appID = %q, want %q", got, want)
	}
	if got, want := deps.desktopEntryInstaller.appID, "custom-id"; got != want {
		t.Fatalf("desktop installer appID = %q, want %q", got, want)
	}
	content := string(deps.desktopEntryInstaller.content)
	if !strings.Contains(content, "Exec=/library/custom-id.AppImage") {
		t.Fatalf("desktop content = %q, want updated Exec", content)
	}
	if !strings.Contains(content, "Icon=/icons/hicolor/256x256/apps/custom-id.png") {
		t.Fatalf("desktop content = %q, want absolute updated Icon", content)
	}
	if got, want := deps.apps.deletedID, installed.ID; got != want {
		t.Fatalf("deleted app ID = %q, want %q", got, want)
	}
	assertRemovedPaths(t, deps.artifactRemover.paths, []string{installed.DesktopEntryPath, installed.IconPath, installed.AppImagePath})
	if !deps.desktopIntegrationRefresher.called {
		t.Fatal("desktop integration refresher was not called")
	}
}

func TestServiceSetIDAutoDerivesIDFromInstalledAppImageMetadata(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	installed.ID = "legacy-id"
	deps.apps.findApps = map[string]domain.App{installed.ID: installed}
	deps.appImageInstaller.path = "/library/example-app.AppImage"
	deps.iconInstaller.path = "/icons/hicolor/256x256/apps/example-app.png"
	deps.desktopEntryInstaller.path = "/desktop/example-app.desktop"
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.SetID(context.Background(), SetIDRequest{CurrentID: "legacy-id", Auto: true})
	if err != nil {
		t.Fatalf("SetID() error = %v", err)
	}
	if got, want := result.App.ID, "example-app"; got != want {
		t.Fatalf("App.ID = %q, want %q", got, want)
	}
	if got, want := deps.saved.App.ID, "example-app"; got != want {
		t.Fatalf("saved App.ID = %q, want %q", got, want)
	}
}

func TestServiceSetIDRejectsExistingTargetID(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	deps.apps.findApps = map[string]domain.App{
		installed.ID: installed,
		"taken-id":   {ID: "taken-id", Name: "Taken"},
	}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.SetID(context.Background(), SetIDRequest{CurrentID: installed.ID, NewID: "taken-id"})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("SetID() error = %v, want already exists", err)
	}
	if deps.appImageInstaller.called {
		t.Fatal("appimage installer called after target collision")
	}
}

func TestServiceSetIDAutoNoopsWhenDerivedIDMatchesCurrentID(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	deps.apps.findApps = map[string]domain.App{installed.ID: installed}
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.SetID(context.Background(), SetIDRequest{CurrentID: installed.ID, Auto: true})
	if err != nil {
		t.Fatalf("SetID() error = %v", err)
	}
	if result.Changed {
		t.Fatal("SetID().Changed = true, want false")
	}
	if deps.appImageInstaller.called {
		t.Fatal("appimage installer called for no-op ID change")
	}
	if deps.saved.App.ID != "" {
		t.Fatalf("saved App.ID = %q, want empty", deps.saved.App.ID)
	}
	if deps.apps.deletedID != "" {
		t.Fatalf("deleted app ID = %q, want empty", deps.apps.deletedID)
	}
}

func TestServiceSetIDRollsBackNewArtifactsWhenSaveFails(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	deps.apps.findApps = map[string]domain.App{installed.ID: installed}
	deps.appImageInstaller.path = "/library/custom-id.AppImage"
	deps.iconInstaller.path = "/icons/hicolor/256x256/apps/custom-id.png"
	deps.desktopEntryInstaller.path = "/desktop/custom-id.desktop"
	failure := errors.New("save failed")
	deps.apps.err = failure
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.SetID(context.Background(), SetIDRequest{CurrentID: installed.ID, NewID: "custom-id"})
	if !errors.Is(err, failure) {
		t.Fatalf("SetID() error = %v, want %v", err, failure)
	}
	assertRemovedPaths(t, deps.artifactRemover.paths, []string{
		"/desktop/custom-id.desktop",
		"/icons/hicolor/256x256/apps/custom-id.png",
		"/library/custom-id.AppImage",
	})
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
	configureUpdateArtifactPaths(&deps, "example-app", "example-app-2-0-0")
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
	configureUpdateArtifactPaths(&deps, "example-app", "example-app-2-0-0")
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
	assertInstallCallsByBase(t, deps.appImageInstaller.calls, []fakeInstallCall{
		{sourcePath: "Example-2.0.0-x86_64.AppImage", appID: "example-app-2-0-0"},
		{sourcePath: "example-app-2-0-0.AppImage", appID: "example-app"},
	})
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
	configureUpdateArtifactPaths(&deps, "example-app", "example-app-2-0-0")
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

	assertInstallCallsByBase(t, deps.appImageInstaller.calls, []fakeInstallCall{
		{sourcePath: "Example-arm64.AppImage", appID: "example-app-2-0-0"},
		{sourcePath: "example-app-2-0-0.AppImage", appID: "example-app"},
	})
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
	configureUpdateArtifactPaths(&deps, "example-app", "example-app-2-0-0")
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

func TestServiceUpdateSkipsNonGitHubUpdateSourcesForNow(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		source domain.UpdateSource
	}{
		{name: "local file", source: domain.NewLocalFileUpdateSource("/downloads/Example.AppImage")},
		{name: "zsync", source: domain.NewEmbeddedUpdateSource("zsync|https://example.test/App.AppImage.zsync")},
		{name: "unsupported", source: domain.NewEmbeddedUpdateSource("gh-releases-zsync|owner|repo")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			deps := integrationTestDeps()
			installed := testInstalledApp(t)
			installed.UpdateSource = tc.source
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
		})
	}
}

func TestServiceUpdateRollsBackStagedArtifactsWhenIntegrationFails(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	installed.UpdateSource = domain.NewGitHubUpdateSource("owner/repo", false)
	deps.apps.listApps = []domain.App{installed}
	deps.ServiceDeps.GitHubReleases = &fakeGitHubReleaseFinder{release: testGitHubReleaseWithTag("v2.0.0", "Example.AppImage")}
	deps.ServiceDeps.Downloads = &fakeAssetDownloader{}
	configureUpdateArtifactPaths(&deps, "example-app", "example-app-2-0-0")
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
	assertRemovedPaths(t, deps.artifactRemover.paths, []string{"/library/example-app-2-0-0.AppImage"})
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
	configureUpdateArtifactPaths(&deps, "example-app", "example-app-2-0-0")
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

	assertRemovedPaths(t, deps.artifactRemover.paths, []string{
		"/desktop/example-app-2-0-0.desktop",
		"/icons/hicolor/256x256/apps/example-app-2-0-0.png",
		"/library/example-app-2-0-0.AppImage",
	})
}

func TestServiceUpdateKeepsSavedUpdateWhenStagedArtifactCleanupFails(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	installed.UpdateSource = domain.NewGitHubUpdateSource("owner/repo", false)
	deps.apps.listApps = []domain.App{installed}
	deps.ServiceDeps.GitHubReleases = &fakeGitHubReleaseFinder{release: testGitHubReleaseWithTag("v2.0.0", "Example.AppImage")}
	deps.ServiceDeps.Downloads = &fakeAssetDownloader{}
	configureUpdateArtifactPaths(&deps, "example-app", "example-app-2-0-0")
	failure := errors.New("remove staged icon failed")
	deps.artifactRemover.err = failure
	deps.artifactRemover.failPath = "/icons/hicolor/256x256/apps/example-app-2-0-0.png"
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Update(context.Background(), UpdateRequest{Confirmation: &fakeUpdateConfirmation{confirmed: true}})
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "failed to remove staged artifacts") {
		t.Fatalf("Update() error = %v, want staged cleanup failure", err)
	}

	if got, want := deps.saved.App.ID, installed.ID; got != want {
		t.Fatalf("saved App.ID = %q, want %q", got, want)
	}
	if got, want := deps.saved.App.AppImagePath, "/library/example-app.AppImage"; got != want {
		t.Fatalf("saved App.AppImagePath = %q, want %q", got, want)
	}
	assertRemovedPaths(t, deps.artifactRemover.paths, []string{
		"/desktop/example-app-2-0-0.desktop",
		"/icons/hicolor/256x256/apps/example-app-2-0-0.png",
	})
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
	if !strings.HasSuffix(deps.appImages.destDir, string(filepath.Separator)+"extract") {
		t.Fatalf("Extract() destDir = %q, want workspace extract dir", deps.appImages.destDir)
	}
	assertWorkspaceCleaned(t, workspaceFromExtractDir(t, deps.appImages.destDir))
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
	if !result.Installed {
		t.Fatal("Info().Installed = false, want true")
	}
	if got, want := result.TargetKind, "installed"; got != want {
		t.Fatalf("Info().TargetKind = %q, want %q", got, want)
	}
	if got, want := result.Source, installed.Source; got != want {
		t.Fatalf("Info().Source = %#v, want %#v", got, want)
	}
	if got, want := result.UpdateSource, installed.UpdateSource; got != want {
		t.Fatalf("Info().UpdateSource = %#v, want %#v", got, want)
	}
}

func TestServiceInfoInspectsLocalAppImageWithoutInstalling(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	raw := "gh-releases-zsync|owner|repo|latest|Example-*x86_64.AppImage.zsync"
	deps.appImages.updateInfo = raw
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	sourcePath := testAppImagePath(t, "Example_App-2.4.6-x86_64.AppImage")

	result, err := service.Info(context.Background(), InfoRequest{Target: sourcePath})
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}

	if deps.apps.findID != "" {
		t.Fatalf("Find() id = %q, want empty for local inspection", deps.apps.findID)
	}
	workspacePath := workspaceFromExtractDir(t, deps.appImages.destDir)
	if got := deps.appImages.appImagePath; got == sourcePath {
		t.Fatalf("Extract() appImagePath = %q, want staged path", got)
	}
	if got, want := deps.appImages.appImagePath, filepath.Join(workspacePath, "Example_App-2.4.6-x86_64.AppImage"); got != want {
		t.Fatalf("Extract() appImagePath = %q, want %q", got, want)
	}
	if got, want := deps.desktopEntries.rootDir, "/extracted"; got != want {
		t.Fatalf("DesktopEntryDiscoverer root = %q, want %q", got, want)
	}
	if !deps.icons.called {
		t.Fatal("IconDiscoverer was not called")
	}
	if deps.appImageInstaller.called {
		t.Fatal("AppImageInstaller was called for local inspection")
	}
	if deps.iconInstaller.sourcePath != "" {
		t.Fatalf("IconInstaller source = %q, want empty", deps.iconInstaller.sourcePath)
	}
	if deps.desktopEntryInstaller.content != nil {
		t.Fatalf("DesktopEntryInstaller content = %q, want nil", deps.desktopEntryInstaller.content)
	}
	if deps.saved.App.ID != "" {
		t.Fatalf("repository Save app ID = %q, want empty", deps.saved.App.ID)
	}
	assertWorkspaceCleaned(t, workspacePath)

	if result.Installed {
		t.Fatal("Info().Installed = true, want false")
	}
	if got, want := result.TargetKind, "local_path"; got != want {
		t.Fatalf("Info().TargetKind = %q, want %q", got, want)
	}
	if got, want := result.ID, "example-app"; got != want {
		t.Fatalf("Info().ID = %q, want %q", got, want)
	}
	if got, want := result.Name, "Example App"; got != want {
		t.Fatalf("Info().Name = %q, want %q", got, want)
	}
	if got, want := result.Version, "1.2.3-beta.1"; got != want {
		t.Fatalf("Info().Version = %q, want %q", got, want)
	}
	if got, want := result.ExecPath, sourcePath; got != want {
		t.Fatalf("Info().ExecPath = %q, want %q", got, want)
	}
	if got, want := result.Source.Kind, domain.SourceKindLocal; got != want {
		t.Fatalf("Info().Source.Kind = %q, want %q", got, want)
	}
	if got, want := result.Source.LocalFile.Path, sourcePath; got != want {
		t.Fatalf("Info().Source.LocalFile.Path = %q, want %q", got, want)
	}
	if !result.Source.LocalFile.IntegratedAt.IsZero() {
		t.Fatalf("Info().Source.LocalFile.IntegratedAt = %v, want zero", result.Source.LocalFile.IntegratedAt)
	}
	if got, want := result.UpdateSource.Kind, domain.UpdateSourceKindGitHub; got != want {
		t.Fatalf("Info().UpdateSource.Kind = %q, want %q", got, want)
	}
	if got := result.UpdateSource.Raw; got != raw {
		t.Fatalf("Info().UpdateSource.Raw = %q, want %q", got, raw)
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
	wantDownloadPath := filepath.Join(filepath.Dir(downloads.destinationPath), "Example.AppImage")
	if got, want := downloads.destinationPath, wantDownloadPath; got != want {
		t.Fatalf("Download() destination = %q, want %q", got, want)
	}
	if downloads.progress == nil {
		t.Fatal("Download() progress = nil, want progress task")
	}
	if got, want := filepath.Base(deps.appImages.appImagePath), "Example.AppImage"; got != want {
		t.Fatalf("Extract() appImagePath base = %q, want %q", got, want)
	}
	if got, want := filepath.Base(deps.appImageInstaller.sourcePath), "Example.AppImage"; got != want {
		t.Fatalf("appimage installer source base = %q, want %q", got, want)
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
	assertWorkspaceCleaned(t, filepath.Dir(downloads.destinationPath))
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
	if got, want := filepath.Base(deps.appImageInstaller.sourcePath), "Example-arm64.AppImage"; got != want {
		t.Fatalf("appimage installer source base = %q, want %q", got, want)
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
	assertWorkspaceCleaned(t, filepath.Dir(downloads.destinationPath))
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
	appImages                   *fakeAppImageExtractor
	desktopEntries              *fakeDesktopEntryDiscoverer
	icons                       *fakeIconDiscoverer
	appImageInstaller           *fakeAppImageInstaller
	iconInstaller               *fakeIconInstaller
	desktopEntryInstaller       *fakeDesktopEntryInstaller
	artifactRemover             *fakeArtifactRemover
	desktopIntegrationRefresher *fakeDesktopIntegrationRefresher
	apps                        *fakeAppRepository
	saved                       *fakeAppRepository
}

func integrationTestDeps() integrationFakes {
	appImages := &fakeAppImageExtractor{rootDir: "/extracted"}
	desktopEntries := &fakeDesktopEntryDiscoverer{
		path: "/extracted/example.desktop",
		content: []byte(strings.Join([]string{
			"[Desktop Entry]",
			"Name=Generic Name",
			"X-AppImage-Name=Example App",
			"X-AppImage-Version=1.2.3-beta.1",
			"Exec=old-exec %U",
			"Icon=example-icon",
			"",
			"[Desktop Action NewWindow]",
			"Exec=old-action --new-window %U",
			"",
		}, "\n")),
	}
	icons := &fakeIconDiscoverer{path: "/extracted/example.png"}
	appImageInstaller := &fakeAppImageInstaller{path: "/library/example-app.AppImage"}
	iconInstaller := &fakeIconInstaller{path: "/icons/hicolor/256x256/apps/example-app.png"}
	desktopEntryInstaller := &fakeDesktopEntryInstaller{path: "/desktop/example-app.desktop"}
	artifactRemover := &fakeArtifactRemover{}
	desktopIntegrationRefresher := &fakeDesktopIntegrationRefresher{}
	apps := &fakeAppRepository{}

	return integrationFakes{
		ServiceDeps: ServiceDeps{
			AppImages:                   appImages,
			DesktopEntries:              desktopEntries,
			Icons:                       icons,
			AppImageInstaller:           appImageInstaller,
			IconInstaller:               iconInstaller,
			DesktopEntryInstaller:       desktopEntryInstaller,
			ArtifactRemover:             artifactRemover,
			DesktopIntegrationRefresher: desktopIntegrationRefresher,
			Apps:                        apps,
		},
		appImages:                   appImages,
		desktopEntries:              desktopEntries,
		icons:                       icons,
		appImageInstaller:           appImageInstaller,
		iconInstaller:               iconInstaller,
		desktopEntryInstaller:       desktopEntryInstaller,
		artifactRemover:             artifactRemover,
		desktopIntegrationRefresher: desktopIntegrationRefresher,
		apps:                        apps,
		saved:                       apps,
	}
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

type fakeInstallCall struct {
	sourcePath string
	appID      string
}

type fakeDesktopInstallCall struct {
	appID   string
	content []byte
}

type fakeAppImageInstaller struct {
	called     bool
	sourcePath string
	appID      string
	path       string
	paths      map[string]string
	calls      []fakeInstallCall
	err        error
}

func (f *fakeAppImageInstaller) Install(ctx context.Context, sourcePath string, appID string) (string, error) {
	f.called = true
	f.sourcePath = sourcePath
	f.appID = appID
	f.calls = append(f.calls, fakeInstallCall{sourcePath: sourcePath, appID: appID})
	if f.err != nil {
		return "", f.err
	}
	if path := f.paths[appID]; path != "" {
		return path, nil
	}
	return f.path, nil
}

type fakeIconInstaller struct {
	sourcePath string
	appID      string
	path       string
	paths      map[string]string
	calls      []fakeInstallCall
	err        error
}

func (f *fakeIconInstaller) Install(ctx context.Context, sourcePath string, appID string) (string, error) {
	f.sourcePath = sourcePath
	f.appID = appID
	f.calls = append(f.calls, fakeInstallCall{sourcePath: sourcePath, appID: appID})
	if f.err != nil {
		return "", f.err
	}
	if path := f.paths[appID]; path != "" {
		return path, nil
	}
	return f.path, nil
}

type fakeDesktopEntryInstaller struct {
	appID   string
	content []byte
	path    string
	paths   map[string]string
	calls   []fakeDesktopInstallCall
	err     error
}

func (f *fakeDesktopEntryInstaller) Install(ctx context.Context, appID string, content []byte) (string, error) {
	f.appID = appID
	f.content = content
	f.calls = append(f.calls, fakeDesktopInstallCall{appID: appID, content: content})
	if f.err != nil {
		return "", f.err
	}
	if path := f.paths[appID]; path != "" {
		return path, nil
	}
	return f.path, nil
}

type fakeDesktopIntegrationRefresher struct {
	called bool
	err    error
}

func (f *fakeDesktopIntegrationRefresher) Refresh(ctx context.Context) error {
	f.called = true
	return f.err
}

type fakeArtifactRemover struct {
	paths    []string
	failPath string
	err      error
}

func (f *fakeArtifactRemover) Remove(ctx context.Context, path string) error {
	f.paths = append(f.paths, path)
	if f.err != nil && (f.failPath == "" || f.failPath == path) {
		return f.err
	}
	return nil
}

func assertWorkspaceCleaned(t *testing.T, path string) {
	t.Helper()
	if path == "" {
		t.Fatal("workspace path is empty")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace stat error = %v, want not exist", err)
	}
}

func testAppImagePath(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("appimage"), 0o755); err != nil {
		t.Fatalf("write test AppImage: %v", err)
	}
	return path
}

func workspaceFromExtractDir(t *testing.T, destDir string) string {
	t.Helper()
	if filepath.Base(destDir) != "extract" {
		t.Fatalf("extract dir = %q, want path ending in extract", destDir)
	}
	return filepath.Dir(destDir)
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
	if err := os.WriteFile(f.downloaded.Path, []byte("appimage"), 0o755); err != nil {
		return DownloadedFile{}, err
	}

	return f.downloaded, nil
}

type fakeAppRepository struct {
	App        domain.App
	err        error
	findID     string
	findApp    domain.App
	findApps   map[string]domain.App
	findErr    error
	listApps   []domain.App
	listErr    error
	deletedID  string
	deletedIDs []string
	deleteErr  error
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
	if f.findApps != nil {
		app, ok := f.findApps[id]
		if !ok || app.ID == "" {
			return domain.App{}, ErrAppNotFound
		}
		return app, nil
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
	f.deletedIDs = append(f.deletedIDs, id)
	return f.deleteErr
}

func testSourceTime() time.Time {
	return time.Date(2026, 6, 3, 14, 6, 7, 0, time.UTC)
}

func configureUpdateArtifactPaths(deps *integrationFakes, appID string, stageID string) {
	deps.appImageInstaller.paths = map[string]string{
		appID:   "/library/" + appID + ".AppImage",
		stageID: "/library/" + stageID + ".AppImage",
	}
	deps.iconInstaller.paths = map[string]string{
		appID:   "/icons/hicolor/256x256/apps/" + appID + ".png",
		stageID: "/icons/hicolor/256x256/apps/" + stageID + ".png",
	}
	deps.desktopEntryInstaller.paths = map[string]string{
		appID:   "/desktop/" + appID + ".desktop",
		stageID: "/desktop/" + stageID + ".desktop",
	}
}

func assertInstallCalls(t *testing.T, got []fakeInstallCall, want []fakeInstallCall) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("install calls = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("install calls = %#v, want %#v", got, want)
		}
	}
}

func assertInstallCallsByBase(t *testing.T, got []fakeInstallCall, want []fakeInstallCall) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("install calls = %#v, want %#v", got, want)
	}
	for i := range want {
		if filepath.Base(got[i].sourcePath) != want[i].sourcePath || got[i].appID != want[i].appID {
			t.Fatalf("install calls = %#v, want base/appID %#v", got, want)
		}
	}
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
