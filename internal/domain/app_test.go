package domain

import (
	"testing"
	"time"
)

func TestNewApp(t *testing.T) {
	t.Parallel()

	version, ok := ParseVersion("v1.2.3")
	if !ok {
		t.Fatal("ParseVersion() ok = false, want true")
	}

	app := NewApp(AppInput{
		Name:             "  Standard Notes Desktop  ",
		Version:          version,
		AppImagePath:     "  /apps/standard-notes.AppImage  ",
		DesktopEntryPath: "  /desktop/standard-notes.desktop  ",
		IconPath:         "  /icons/standard-notes.png  ",
		Source:           NewGitHubReleaseSource(" standardnotes/app ", " v1.2.3 ", " StandardNotes.AppImage ", " https://example.test/StandardNotes.AppImage ", 123, time.Date(2026, 6, 3, 14, 6, 7, 0, time.FixedZone("CEST", 2*60*60))),
		UpdateSource:     NewGitHubUpdateSource(" github:standardnotes/app ", true),
	})

	if got, want := app.ID, "standard-notes-desktop"; got != want {
		t.Fatalf("App.ID = %q, want %q", got, want)
	}
	if got, want := app.Name, "Standard Notes Desktop"; got != want {
		t.Fatalf("App.Name = %q, want %q", got, want)
	}
	if got, want := app.Version.String(), "1.2.3"; got != want {
		t.Fatalf("App.Version = %q, want %q", got, want)
	}
	if got, want := app.AppImagePath, "/apps/standard-notes.AppImage"; got != want {
		t.Fatalf("App.AppImagePath = %q, want %q", got, want)
	}
	if got, want := app.DesktopEntryPath, "/desktop/standard-notes.desktop"; got != want {
		t.Fatalf("App.DesktopEntryPath = %q, want %q", got, want)
	}
	if got, want := app.IconPath, "/icons/standard-notes.png"; got != want {
		t.Fatalf("App.IconPath = %q, want %q", got, want)
	}
	if got, want := app.Source.Kind, SourceKindGitHub; got != want {
		t.Fatalf("App.Source.Kind = %q, want %q", got, want)
	}
	if got, want := app.Source.GitHubRelease.Repo, "standardnotes/app"; got != want {
		t.Fatalf("App.Source.GitHubRelease.Repo = %q, want %q", got, want)
	}
	if got, want := app.Source.GitHubRelease.Tag, "v1.2.3"; got != want {
		t.Fatalf("App.Source.GitHubRelease.Tag = %q, want %q", got, want)
	}
	if got, want := app.Source.GitHubRelease.Asset, "StandardNotes.AppImage"; got != want {
		t.Fatalf("App.Source.GitHubRelease.Asset = %q, want %q", got, want)
	}
	if got, want := app.Source.GitHubRelease.DownloadURL, "https://example.test/StandardNotes.AppImage"; got != want {
		t.Fatalf("App.Source.GitHubRelease.DownloadURL = %q, want %q", got, want)
	}
	if got, want := app.Source.GitHubRelease.SizeBytes, int64(123); got != want {
		t.Fatalf("App.Source.GitHubRelease.SizeBytes = %d, want %d", got, want)
	}
	if got, want := app.Source.GitHubRelease.DownloadedAt, time.Date(2026, 6, 3, 12, 6, 7, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("App.Source.GitHubRelease.DownloadedAt = %s, want %s", got, want)
	}
	if got, want := app.UpdateSource.Kind, UpdateSourceKindGitHub; got != want {
		t.Fatalf("App.UpdateSource.Kind = %q, want %q", got, want)
	}
	if got, want := app.UpdateSource.Repo, "github:standardnotes/app"; got != want {
		t.Fatalf("App.UpdateSource.Repo = %q, want %q", got, want)
	}
	if !app.UpdateSource.Prerelease {
		t.Fatal("App.UpdateSource.Prerelease = false, want true")
	}
}

func TestNewAppFromDesktopEntry(t *testing.T) {
	t.Parallel()

	version, ok := ParseVersion("v2.3.4")
	if !ok {
		t.Fatal("ParseVersion() ok = false, want true")
	}

	app := NewAppFromDesktopEntry(
		DesktopEntry{
			Name:    "Example App",
			Version: version,
		},
		AppInput{
			AppImagePath:     " /apps/example.AppImage ",
			DesktopEntryPath: " /desktop/example.desktop ",
			IconPath:         " /icons/example.png ",
			Source:           NewLocalSource(" /downloads/example.AppImage ", time.Date(2026, 6, 3, 14, 6, 7, 0, time.UTC)),
			UpdateSource:     NewGitHubUpdateSource(" github:owner/example ", false),
		},
	)

	if got, want := app.ID, "example-app"; got != want {
		t.Fatalf("App.ID = %q, want %q", got, want)
	}
	if got, want := app.Name, "Example App"; got != want {
		t.Fatalf("App.Name = %q, want %q", got, want)
	}
	if got, want := app.Version.String(), "2.3.4"; got != want {
		t.Fatalf("App.Version = %q, want %q", got, want)
	}
	if got, want := app.AppImagePath, "/apps/example.AppImage"; got != want {
		t.Fatalf("App.AppImagePath = %q, want %q", got, want)
	}
	if got, want := app.DesktopEntryPath, "/desktop/example.desktop"; got != want {
		t.Fatalf("App.DesktopEntryPath = %q, want %q", got, want)
	}
	if got, want := app.IconPath, "/icons/example.png"; got != want {
		t.Fatalf("App.IconPath = %q, want %q", got, want)
	}
	if got, want := app.Source.Kind, SourceKindLocal; got != want {
		t.Fatalf("App.Source.Kind = %q, want %q", got, want)
	}
	if got, want := app.Source.LocalFile.Path, "/downloads/example.AppImage"; got != want {
		t.Fatalf("App.Source.LocalFile.Path = %q, want %q", got, want)
	}
	if got, want := app.Source.LocalFile.IntegratedAt, time.Date(2026, 6, 3, 14, 6, 7, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("App.Source.LocalFile.IntegratedAt = %s, want %s", got, want)
	}
	if got, want := app.UpdateSource.Kind, UpdateSourceKindGitHub; got != want {
		t.Fatalf("App.UpdateSource.Kind = %q, want %q", got, want)
	}
	if got, want := app.UpdateSource.Repo, "github:owner/example"; got != want {
		t.Fatalf("App.UpdateSource.Repo = %q, want %q", got, want)
	}
}

func TestNewAppUsesExplicitSlugifiedID(t *testing.T) {
	t.Parallel()

	app := NewApp(AppInput{
		ID:   "  T3 Code Alpha  ",
		Name: "Ignored For ID",
	})

	if got, want := app.ID, "t3-code-alpha"; got != want {
		t.Fatalf("App.ID = %q, want %q", got, want)
	}
}

func TestNewAppDefaultsToUnknownSource(t *testing.T) {
	t.Parallel()

	app := NewApp(AppInput{Name: "Example"})

	if got, want := app.Source.Kind, SourceKindUnknown; got != want {
		t.Fatalf("App.Source.Kind = %q, want %q", got, want)
	}
}

func TestNewEmbeddedUpdateSourceParsesGitHubReleasesZsync(t *testing.T) {
	raw := "gh-releases-zsync|probono|AppImages|latest|Subsurface-*x86_64.AppImage.zsync"

	source := NewEmbeddedUpdateSource(raw)

	if !source.Embedded {
		t.Fatal("Embedded = false, want true")
	}
	if got, want := source.Kind, UpdateSourceKindGitHub; got != want {
		t.Fatalf("Kind = %q, want %q", got, want)
	}
	if got := source.Raw; got != raw {
		t.Fatalf("Raw = %q, want %q", got, raw)
	}
	if got, want := source.Transport, "gh-releases-zsync"; got != want {
		t.Fatalf("Transport = %q, want %q", got, want)
	}
	if got, want := source.Repo, "probono/AppImages"; got != want {
		t.Fatalf("Repo = %q, want %q", got, want)
	}
	if got, want := source.ReleaseTag, "latest"; got != want {
		t.Fatalf("ReleaseTag = %q, want %q", got, want)
	}
	if got, want := source.AssetPattern, "Subsurface-*x86_64.AppImage"; got != want {
		t.Fatalf("AssetPattern = %q, want %q", got, want)
	}
	if got, want := source.ZsyncAssetPattern, "Subsurface-*x86_64.AppImage.zsync"; got != want {
		t.Fatalf("ZsyncAssetPattern = %q, want %q", got, want)
	}
}

func TestNewEmbeddedUpdateSourceParsesZsync(t *testing.T) {
	raw := "zsync|https://example.test/App-latest-x86_64.AppImage.zsync"

	source := NewEmbeddedUpdateSource(raw)

	if got, want := source.Kind, UpdateSourceKindZsync; got != want {
		t.Fatalf("Kind = %q, want %q", got, want)
	}
	if got := source.Raw; got != raw {
		t.Fatalf("Raw = %q, want %q", got, raw)
	}
	if got, want := source.Transport, "zsync"; got != want {
		t.Fatalf("Transport = %q, want %q", got, want)
	}
	if got, want := source.URL, "https://example.test/App-latest-x86_64.AppImage.zsync"; got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
}

func TestNewEmbeddedUpdateSourceStoresMalformedAsUnsupported(t *testing.T) {
	raw := "gh-releases-zsync|owner|repo"

	source := NewEmbeddedUpdateSource(raw)

	if got, want := source.Kind, UpdateSourceKindUnsupported; got != want {
		t.Fatalf("Kind = %q, want %q", got, want)
	}
	if got := source.Raw; got != raw {
		t.Fatalf("Raw = %q, want %q", got, raw)
	}
	if got, want := source.Transport, "gh-releases-zsync"; got != want {
		t.Fatalf("Transport = %q, want %q", got, want)
	}
}

func TestAppHasUpdate(t *testing.T) {
	t.Parallel()

	current, ok := ParseVersion("1.2.2")
	if !ok {
		t.Fatal("ParseVersion(current) ok = false, want true")
	}
	newerPrerelease, ok := ParseVersion("1.2.3-beta.1")
	if !ok {
		t.Fatal("ParseVersion(newerPrerelease) ok = false, want true")
	}
	older, ok := ParseVersion("1.2.1")
	if !ok {
		t.Fatal("ParseVersion(older) ok = false, want true")
	}

	app := NewApp(AppInput{Name: "Example", Version: current})

	if !app.HasUpdate(newerPrerelease) {
		t.Fatal("App.HasUpdate(newer prerelease) = false, want true")
	}
	if app.HasUpdate(older) {
		t.Fatal("App.HasUpdate(older version) = true, want false")
	}
	if app.HasUpdate(Version{}) {
		t.Fatal("App.HasUpdate(zero version) = true, want false")
	}
}

func TestAppWithoutCurrentVersionHasNoUpdate(t *testing.T) {
	t.Parallel()

	candidate, ok := ParseVersion("1.2.3")
	if !ok {
		t.Fatal("ParseVersion(candidate) ok = false, want true")
	}

	app := NewApp(AppInput{Name: "Example"})
	if app.HasUpdate(candidate) {
		t.Fatal("App.HasUpdate(candidate) = true, want false")
	}
}
