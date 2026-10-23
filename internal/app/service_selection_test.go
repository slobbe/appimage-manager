package app

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/slobbe/appimage-manager/internal/domain"
)

func TestServiceAddFromGitHubUsesAndPersistsSelectionRules(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		exclude []string
		prefer  []string
		assets  []string
	}{
		{name: "exclude", exclude: []string{"*debug*", "*legacy*"}, assets: []string{"Example-debug.AppImage", "Example-legacy.AppImage", "Example.AppImage"}},
		{name: "prefer", prefer: []string{"*missing*", "Example.AppImage"}, assets: []string{"Example-portable.AppImage", "Example.AppImage"}},
		{name: "combined", exclude: []string{"*debug*"}, prefer: []string{"*debug*", "Example.AppImage"}, assets: []string{"Example-debug.AppImage", "Example-portable.AppImage", "Example.AppImage"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			deps := integrationTestDeps()
			deps.appImages.updateInfo = "gh-releases-zsync|embedded|repo|latest|Other.AppImage.zsync"
			deps.ServiceDeps.GitHubReleases = &fakeGitHubReleaseFinder{release: testGitHubRelease(tc.assets...)}
			downloads := &fakeAssetDownloader{}
			deps.ServiceDeps.Downloads = downloads
			service, err := NewService(deps.ServiceDeps)
			if err != nil {
				t.Fatalf("NewService() error = %v", err)
			}

			result, err := service.Add(context.Background(), AddRequest{
				GitHubRepo: "owner/repo", AssetPattern: "Example*.AppImage",
				AssetExcludePatterns: tc.exclude, AssetPreferPatterns: tc.prefer,
			})
			if err != nil {
				t.Fatalf("Add() error = %v", err)
			}
			if got, want := downloads.source.FileName, "Example.AppImage"; got != want {
				t.Fatalf("Download() filename = %q, want %q", got, want)
			}
			if got, want := downloads.source.URL, "https://example.test/Example.AppImage"; got != want {
				t.Fatalf("Download() URL = %q, want %q", got, want)
			}
			if got, want := filepath.Base(deps.appImages.appImagePath), "Example.AppImage"; got != want {
				t.Fatalf("Extract() path base = %q, want %q", got, want)
			}
			if got, want := filepath.Base(deps.appImageInstaller.sourcePath), "Example.AppImage"; got != want {
				t.Fatalf("Install() source base = %q, want %q", got, want)
			}
			if got, want := deps.saved.App.Source.GitHubRelease.Asset, "Example.AppImage"; got != want {
				t.Fatalf("saved source asset = %q, want %q", got, want)
			}
			wantSource := domain.NewGitHubUpdateSource("owner/repo", false)
			wantSource.AssetPattern = "Example*.AppImage"
			wantSource.AssetExcludePatterns = tc.exclude
			wantSource.AssetPreferPatterns = tc.prefer
			if !reflect.DeepEqual(result.App.UpdateSource, wantSource) {
				t.Fatalf("result UpdateSource = %#v, want %#v", result.App.UpdateSource, wantSource)
			}
			if !reflect.DeepEqual(deps.saved.App.UpdateSource, wantSource) {
				t.Fatalf("saved UpdateSource = %#v, want %#v", deps.saved.App.UpdateSource, wantSource)
			}
		})
	}
}

func TestServiceUpdateUsesPersistedSelectionRules(t *testing.T) {
	t.Parallel()

	for _, checkOnly := range []bool{false, true} {
		name := "apply"
		if checkOnly {
			name = "check only"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			deps := integrationTestDeps()
			installed := testInstalledApp(t)
			installed.UpdateSource = domain.NewGitHubUpdateSource("owner/repo", false)
			installed.UpdateSource.AssetPattern = "Example*.AppImage"
			installed.UpdateSource.AssetExcludePatterns = []string{"*debug*"}
			installed.UpdateSource.AssetPreferPatterns = []string{"*debug*", "Example.AppImage"}
			deps.apps.listApps = []domain.App{installed}
			deps.desktopEntries.content = []byte("[Desktop Entry]\nName=Example App\nExec=old-exec\nIcon=example-icon\n")
			configureUpdateArtifactPaths(&deps, "example-app", "example-app-2-0-0")
			deps.ServiceDeps.GitHubReleases = &fakeGitHubReleaseFinder{release: testGitHubReleaseWithTag("v2.0.0",
				"Example-debug.AppImage", "Example-portable.AppImage", "Example.AppImage")}
			downloads := &fakeAssetDownloader{}
			deps.ServiceDeps.Downloads = downloads
			confirmation := &fakeUpdateConfirmation{confirmed: true}
			service, err := NewService(deps.ServiceDeps)
			if err != nil {
				t.Fatalf("NewService() error = %v", err)
			}

			result, err := service.Update(context.Background(), UpdateRequest{CheckOnly: checkOnly, Confirmation: confirmation})
			if err != nil {
				t.Fatalf("Update() error = %v", err)
			}
			assertUpdateCandidates(t, result.Updates, []UpdateCandidate{{ID: installed.ID, CurrentVersion: "1.2.3", NewVersion: "2.0.0"}})
			if len(result.Failures) != 0 || result.Checked != 1 {
				t.Fatalf("Update() = %#v, want one successful check", result)
			}
			if checkOnly {
				if result.Applied || result.AppliedCount != 0 || confirmation.called || downloads.destinationPath != "" || deps.appImageInstaller.called || deps.saved.App.ID != "" {
					t.Fatalf("CheckOnly applied side effects: result=%#v, confirmation=%v, download=%q, install=%v, saved=%q", result, confirmation.called, downloads.destinationPath, deps.appImageInstaller.called, deps.saved.App.ID)
				}
				return
			}
			if !result.Applied || result.AppliedCount != 1 {
				t.Fatalf("Update() = %#v, want one applied update", result)
			}
			if got, want := downloads.source.FileName, "Example.AppImage"; got != want {
				t.Fatalf("Download() filename = %q, want %q", got, want)
			}
			if got, want := deps.saved.App.Source.GitHubRelease.Asset, "Example.AppImage"; got != want {
				t.Fatalf("saved source asset = %q, want %q", got, want)
			}
			if !reflect.DeepEqual(deps.saved.App.UpdateSource, installed.UpdateSource) {
				t.Fatalf("saved UpdateSource = %#v, want preserved %#v", deps.saved.App.UpdateSource, installed.UpdateSource)
			}
		})
	}
}

func TestServiceSetUpdateSourcePersistsReplacesAndClearsSelectionRules(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	deps.apps.findApp = installed
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	for _, tc := range []struct {
		name    string
		exclude []string
		prefer  []string
	}{
		{name: "persist", exclude: []string{"*debug*", "*legacy*"}, prefer: []string{"*portable*", "*modern*"}},
		{name: "replace", exclude: []string{"*minimal*"}, prefer: []string{"*full*"}},
		{name: "clear"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := service.SetUpdateSource(context.Background(), SetUpdateSourceRequest{
				ID: installed.ID, GitHubRepo: "owner/repo", AssetPattern: "Example*.AppImage",
				AssetExcludePatterns: tc.exclude, AssetPreferPatterns: tc.prefer,
			})
			if err != nil {
				t.Fatalf("SetUpdateSource() error = %v", err)
			}
			want := domain.NewGitHubUpdateSource("owner/repo", false)
			want.AssetPattern = "Example*.AppImage"
			want.AssetExcludePatterns = tc.exclude
			want.AssetPreferPatterns = tc.prefer
			if !reflect.DeepEqual(result.UpdateSource, want) || !reflect.DeepEqual(deps.saved.App.UpdateSource, want) {
				t.Fatalf("UpdateSource result=%#v, saved=%#v, want %#v", result.UpdateSource, deps.saved.App.UpdateSource, want)
			}
			deps.apps.findApp = deps.saved.App
		})
	}
}

func TestServiceRejectsInvalidSelectionRulesBeforeSideEffects(t *testing.T) {
	t.Parallel()

	for _, operation := range []string{"add", "set update source"} {
		for _, field := range []string{"exclude", "prefer"} {
			for _, pattern := range []string{"[", "Example*[", "", " \t "} {
				t.Run(operation+"/"+field+"/"+pattern, func(t *testing.T) {
					t.Parallel()

					deps := integrationTestDeps()
					deps.apps.findApp = testInstalledApp(t)
					releases := &fakeGitHubReleaseFinder{release: testGitHubRelease("Example.AppImage")}
					downloads := &fakeAssetDownloader{}
					deps.ServiceDeps.GitHubReleases = releases
					deps.ServiceDeps.Downloads = downloads
					service, err := NewService(deps.ServiceDeps)
					if err != nil {
						t.Fatalf("NewService() error = %v", err)
					}
					var exclude, prefer []string
					if field == "exclude" {
						exclude = []string{"*debug*", pattern}
					} else {
						prefer = []string{"*portable*", pattern}
					}
					if operation == "add" {
						_, err = service.Add(context.Background(), AddRequest{GitHubRepo: "owner/repo", AssetExcludePatterns: exclude, AssetPreferPatterns: prefer})
					} else {
						_, err = service.SetUpdateSource(context.Background(), SetUpdateSourceRequest{ID: "example-app", GitHubRepo: "owner/repo", AssetExcludePatterns: exclude, AssetPreferPatterns: prefer})
					}
					if err == nil || !strings.Contains(err.Error(), field+" pattern") {
						t.Fatalf("error = %v, want invalid %s pattern error", err, field)
					}
					if releases.method != "" || downloads.destinationPath != "" || deps.saved.App.ID != "" || deps.appImages.appImagePath != "" || deps.apps.findID != "" {
						t.Fatalf("invalid rules caused side effects: release=%q, download=%q, saved=%q, extract=%q, find=%q", releases.method, downloads.destinationPath, deps.saved.App.ID, deps.appImages.appImagePath, deps.apps.findID)
					}
				})
			}
		}
	}
}

func TestServiceSelectionRulesRequireGitHubSource(t *testing.T) {
	t.Parallel()

	for _, operation := range []string{"local add", "embedded source"} {
		for _, field := range []string{"exclude", "prefer"} {
			t.Run(operation+"/"+field, func(t *testing.T) {
				t.Parallel()

				deps := integrationTestDeps()
				deps.apps.findApp = testInstalledApp(t)
				deps.appImages.updateInfo = "gh-releases-zsync|owner|repo|latest|Example.AppImage.zsync"
				service, err := NewService(deps.ServiceDeps)
				if err != nil {
					t.Fatalf("NewService() error = %v", err)
				}
				var exclude, prefer []string
				if field == "exclude" {
					exclude = []string{"*debug*"}
				} else {
					prefer = []string{"*portable*"}
				}
				if operation == "local add" {
					_, err = service.Add(context.Background(), AddRequest{Path: testAppImagePath(t, "Example.AppImage"), AssetExcludePatterns: exclude, AssetPreferPatterns: prefer})
				} else {
					_, err = service.SetUpdateSource(context.Background(), SetUpdateSourceRequest{ID: "example-app", Embedded: true, AssetExcludePatterns: exclude, AssetPreferPatterns: prefer})
				}
				if err == nil || !strings.Contains(err.Error(), "require github repo") {
					t.Fatalf("error = %v, want rules require github repo", err)
				}
				if deps.saved.App.ID != "" || deps.appImages.appImagePath != "" || deps.apps.findID != "" {
					t.Fatalf("non-GitHub rules caused side effects: saved=%q, extract=%q, find=%q", deps.saved.App.ID, deps.appImages.appImagePath, deps.apps.findID)
				}
			})
		}
	}
}
