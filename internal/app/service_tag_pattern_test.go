package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/slobbe/appimage-manager/internal/domain"
)

func TestServiceAddForwardsAndPersistsTagPattern(t *testing.T) {
	t.Parallel()

	for _, prerelease := range []bool{false, true} {
		name := "stable"
		if prerelease {
			name = "include prerelease"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			deps := integrationTestDeps()
			deps.appImages.updateInfo = "gh-releases-zsync|embedded|repo|latest|Other.AppImage.zsync"
			finder := &fakeGitHubReleaseFinder{release: testGitHubRelease("Example.AppImage")}
			deps.ServiceDeps.GitHubReleases = finder
			deps.ServiceDeps.Downloads = &fakeAssetDownloader{}
			service, err := NewService(deps.ServiceDeps)
			if err != nil {
				t.Fatal(err)
			}

			result, err := service.Add(context.Background(), AddRequest{
				GitHubRepo: "owner/repo", TagPattern: " release-v* ", Prerelease: prerelease,
			})
			if err != nil {
				t.Fatalf("Add() error = %v", err)
			}
			assertMatchingTagLookup(t, finder, prerelease, "release-v*")
			want := domain.NewGitHubUpdateSource("owner/repo", prerelease)
			want.TagPattern = "release-v*"
			if !reflect.DeepEqual(result.App.UpdateSource, want) || !reflect.DeepEqual(deps.saved.App.UpdateSource, want) {
				t.Fatalf("UpdateSource result=%#v, saved=%#v, want %#v", result.App.UpdateSource, deps.saved.App.UpdateSource, want)
			}
		})
	}
}

func TestServiceUpdateUsesAndPreservesSavedTagPattern(t *testing.T) {
	t.Parallel()

	for _, checkOnly := range []bool{false, true} {
		for _, prerelease := range []bool{false, true} {
			name := "apply/stable"
			if checkOnly {
				name = "check/stable"
			}
			if prerelease {
				name += "/include prerelease"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				deps := integrationTestDeps()
				installed := testInstalledApp(t)
				installed.UpdateSource = domain.NewGitHubUpdateSource("owner/repo", prerelease)
				installed.UpdateSource.TagPattern = "v2.*"
				deps.apps.listApps = []domain.App{installed}
				deps.desktopEntries.content = []byte("[Desktop Entry]\nName=Example App\nExec=old-exec\nIcon=example-icon\n")
				configureUpdateArtifactPaths(&deps, "example-app", "example-app-2-0-0")
				finder := &fakeGitHubReleaseFinder{release: testGitHubReleaseWithTag("v2.0.0", "Example.AppImage")}
				downloads := &fakeAssetDownloader{}
				confirmation := &fakeUpdateConfirmation{confirmed: true}
				deps.ServiceDeps.GitHubReleases = finder
				deps.ServiceDeps.Downloads = downloads
				service, err := NewService(deps.ServiceDeps)
				if err != nil {
					t.Fatal(err)
				}

				result, err := service.Update(context.Background(), UpdateRequest{CheckOnly: checkOnly, Confirmation: confirmation})
				if err != nil {
					t.Fatalf("Update() error = %v", err)
				}
				assertMatchingTagLookup(t, finder, prerelease, "v2.*")
				assertUpdateCandidates(t, result.Updates, []UpdateCandidate{{ID: installed.ID, CurrentVersion: "1.2.3", NewVersion: "2.0.0"}})
				if result.Checked != 1 || len(result.Failures) != 0 {
					t.Fatalf("Update() = %#v, want one successful check", result)
				}
				if checkOnly {
					if result.Applied || result.AppliedCount != 0 || confirmation.called || downloads.destinationPath != "" || deps.appImageInstaller.called || deps.saved.App.ID != "" {
						t.Fatalf("check-only update caused side effects: result=%#v, confirmation=%v, download=%q, install=%v, saved=%q", result, confirmation.called, downloads.destinationPath, deps.appImageInstaller.called, deps.saved.App.ID)
					}
					return
				}
				if !result.Applied || result.AppliedCount != 1 || downloads.source.FileName != "Example.AppImage" {
					t.Fatalf("Update() = %#v, download=%#v, want one applied update", result, downloads.source)
				}
				if !reflect.DeepEqual(deps.saved.App.UpdateSource, installed.UpdateSource) {
					t.Fatalf("saved UpdateSource = %#v, want preserved %#v", deps.saved.App.UpdateSource, installed.UpdateSource)
				}
			})
		}
	}
}

func TestServiceTagPatternLookupFailureDoesNotFallback(t *testing.T) {
	t.Parallel()

	for _, operation := range []string{"add", "check", "apply"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			deps := integrationTestDeps()
			installed := testInstalledApp(t)
			installed.UpdateSource = domain.NewGitHubUpdateSource("owner/repo", true)
			installed.UpdateSource.TagPattern = "v2.*"
			deps.apps.listApps = []domain.App{installed}
			lookupErr := errors.New("no release matches tag pattern")
			finder := &fakeGitHubReleaseFinder{err: lookupErr, release: testGitHubReleaseWithTag("v3.0.0", "Example.AppImage")}
			downloads := &fakeAssetDownloader{}
			confirmation := &fakeUpdateConfirmation{confirmed: true}
			deps.ServiceDeps.GitHubReleases = finder
			deps.ServiceDeps.Downloads = downloads
			service, err := NewService(deps.ServiceDeps)
			if err != nil {
				t.Fatal(err)
			}

			if operation == "add" {
				_, err = service.Add(context.Background(), AddRequest{GitHubRepo: "owner/repo", TagPattern: "v2.*", Prerelease: true})
				if !errors.Is(err, lookupErr) {
					t.Fatalf("Add() error = %v, want %v", err, lookupErr)
				}
			} else {
				result, err := service.Update(context.Background(), UpdateRequest{CheckOnly: operation == "check", Confirmation: confirmation})
				if err != nil {
					t.Fatalf("Update() error = %v", err)
				}
				if len(result.Failures) != 1 || result.Failures[0].AppID != installed.ID || !strings.Contains(result.Failures[0].Error, lookupErr.Error()) || len(result.Updates) != 0 || result.Applied || result.AppliedCount != 0 {
					t.Fatalf("Update() = %#v, want only lookup failure", result)
				}
			}
			assertMatchingTagLookup(t, finder, true, "v2.*")
			if confirmation.called || downloads.destinationPath != "" || deps.appImages.appImagePath != "" || deps.appImageInstaller.called || deps.saved.App.ID != "" {
				t.Fatal("failed lookup caused confirmation, download, extraction, installation, or persistence")
			}
		})
	}
}

func TestServiceSetUpdateSourcePersistsAndClearsTagPattern(t *testing.T) {
	t.Parallel()

	deps := integrationTestDeps()
	installed := testInstalledApp(t)
	deps.apps.findApp = installed
	service, err := NewService(deps.ServiceDeps)
	if err != nil {
		t.Fatal(err)
	}
	for _, pattern := range []string{" v2.* ", "v3.*", ""} {
		t.Run("pattern="+pattern, func(t *testing.T) {
			result, err := service.SetUpdateSource(context.Background(), SetUpdateSourceRequest{
				ID: installed.ID, GitHubRepo: "owner/repo", TagPattern: pattern, Prerelease: true,
			})
			if err != nil {
				t.Fatalf("SetUpdateSource() error = %v", err)
			}
			want := domain.NewGitHubUpdateSource("owner/repo", true)
			want.TagPattern = strings.TrimSpace(pattern)
			if !reflect.DeepEqual(result.UpdateSource, want) || !reflect.DeepEqual(deps.saved.App.UpdateSource, want) {
				t.Fatalf("UpdateSource result=%#v, saved=%#v, want %#v", result.UpdateSource, deps.saved.App.UpdateSource, want)
			}
			deps.apps.findApp = deps.saved.App
		})
	}
}

func TestServiceValidatesTagPatternBeforeSideEffects(t *testing.T) {
	t.Parallel()

	for _, operation := range []string{"add", "set update source"} {
		for _, tc := range []struct {
			name, repo, pattern, wantError string
			embedded                       bool
		}{
			{name: "unclosed class", repo: "owner/repo", pattern: "[", wantError: "tag pattern"},
			{name: "malformed suffix", repo: "owner/repo", pattern: "v*[", wantError: "tag pattern"},
			{name: "trailing escape", repo: "owner/repo", pattern: "v*\\", wantError: "tag pattern"},
			{name: "invalid repo", repo: "owner", pattern: "v*", wantError: "owner/repo"},
			{name: "requires github", pattern: "v*", embedded: true, wantError: "tag pattern requires github repo"},
		} {
			t.Run(operation+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				deps := integrationTestDeps()
				deps.apps.findApp = testInstalledApp(t)
				deps.appImages.updateInfo = "gh-releases-zsync|owner|repo|latest|Example.AppImage.zsync"
				finder := &fakeGitHubReleaseFinder{release: testGitHubRelease("Example.AppImage")}
				downloads := &fakeAssetDownloader{}
				deps.ServiceDeps.GitHubReleases = finder
				deps.ServiceDeps.Downloads = downloads
				service, err := NewService(deps.ServiceDeps)
				if err != nil {
					t.Fatal(err)
				}
				if operation == "add" {
					req := AddRequest{GitHubRepo: tc.repo, TagPattern: tc.pattern}
					if tc.repo == "" {
						req.Path = testAppImagePath(t, "Example.AppImage")
					}
					_, err = service.Add(context.Background(), req)
				} else {
					_, err = service.SetUpdateSource(context.Background(), SetUpdateSourceRequest{ID: "example-app", GitHubRepo: tc.repo, TagPattern: tc.pattern, Embedded: tc.embedded})
				}
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v, want %q", err, tc.wantError)
				}
				if len(finder.methods) != 0 || downloads.destinationPath != "" || deps.saved.App.ID != "" || deps.appImages.appImagePath != "" {
					t.Fatal("invalid tag pattern or repo caused lookup, download, persistence, or extraction")
				}
				if tc.name != "invalid repo" && deps.apps.findID != "" {
					t.Fatal("invalid tag pattern caused repository access")
				}
			})
		}
	}
}

func TestServiceEmptyTagPatternKeepsExistingReleaseLookups(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, releaseTag, method string
		prerelease               bool
	}{
		{name: "stable", method: "latest"},
		{name: "include prerelease", method: "latest", prerelease: true},
		{name: "embedded latest", releaseTag: "latest", method: "latest"},
		{name: "embedded prerelease", releaseTag: "latest-pre", method: "latest-prerelease"},
		{name: "embedded all", releaseTag: "latest-all", method: "latest", prerelease: true},
		{name: "embedded exact tag", releaseTag: "continuous", method: "tag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			deps := integrationTestDeps()
			installed := testInstalledApp(t)
			installed.UpdateSource = domain.NewGitHubUpdateSource("owner/repo", tc.prerelease)
			if tc.releaseTag != "" {
				installed.UpdateSource = domain.NewEmbeddedUpdateSource("gh-releases-zsync|owner|repo|" + tc.releaseTag + "|Example.AppImage.zsync")
			}
			deps.apps.listApps = []domain.App{installed}
			finder := &fakeGitHubReleaseFinder{release: testGitHubReleaseWithTag("v2.0.0", "Example.AppImage")}
			deps.ServiceDeps.GitHubReleases = finder
			service, err := NewService(deps.ServiceDeps)
			if err != nil {
				t.Fatal(err)
			}

			result, err := service.Update(context.Background(), UpdateRequest{CheckOnly: true})
			if err != nil {
				t.Fatalf("Update() error = %v", err)
			}
			assertUpdateCandidates(t, result.Updates, []UpdateCandidate{{ID: installed.ID, CurrentVersion: "1.2.3", NewVersion: "2.0.0"}})
			if !reflect.DeepEqual(finder.methods, []string{tc.method}) || finder.repo != "owner/repo" || finder.includePrerelease != tc.prerelease || finder.tagPattern != "" {
				t.Fatalf("lookup methods=%v, repo=%q, prerelease=%v, pattern=%q; want %s for owner/repo, prerelease=%v, no pattern", finder.methods, finder.repo, finder.includePrerelease, finder.tagPattern, tc.method, tc.prerelease)
			}
			if tc.method == "tag" && finder.tag != tc.releaseTag {
				t.Fatalf("lookup tag = %q, want %q", finder.tag, tc.releaseTag)
			}
		})
	}
}

func assertMatchingTagLookup(t *testing.T, finder *fakeGitHubReleaseFinder, prerelease bool, pattern string) {
	t.Helper()
	if !reflect.DeepEqual(finder.methods, []string{"latest-matching-tag"}) || finder.repo != "owner/repo" || finder.includePrerelease != prerelease || finder.tagPattern != pattern {
		t.Fatalf("lookup methods=%v, repo=%q, prerelease=%v, pattern=%q; want only matching lookup for owner/repo, prerelease=%v, pattern=%q", finder.methods, finder.repo, finder.includePrerelease, finder.tagPattern, prerelease, pattern)
	}
}
