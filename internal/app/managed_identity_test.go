package app

import (
	"path/filepath"
	"testing"

	"github.com/slobbe/appimage-manager/internal/cli/config"
	models "github.com/slobbe/appimage-manager/internal/domain"
	repo "github.com/slobbe/appimage-manager/internal/infra/repository"
)

func TestResolveManagedAppIDUsesSlugifiedName(t *testing.T) {
	tmp := t.TempDir()
	setupIntegrationConfigForTest(t, tmp)

	id, replacement, err := ResolveManagedAppID("ClickUp", "desktop", "/tmp/desktop.AppImage", incomingIdentity("desktop", "ClickUp", "/tmp/desktop.AppImage"))
	if err != nil {
		t.Fatalf("ResolveManagedAppID returned error: %v", err)
	}
	if id != "clickup" {
		t.Fatalf("id = %q, want clickup", id)
	}
	if replacement != nil {
		t.Fatalf("replacement = %#v, want nil", replacement)
	}
}

func TestResolveManagedAppIDFallsBackToUpstreamID(t *testing.T) {
	tmp := t.TempDir()
	setupIntegrationConfigForTest(t, tmp)

	id, _, err := ResolveManagedAppID("---", "desktop", "/tmp/desktop.AppImage", incomingIdentity("desktop", "---", "/tmp/desktop.AppImage"))
	if err != nil {
		t.Fatalf("ResolveManagedAppID returned error: %v", err)
	}
	if id != "desktop" {
		t.Fatalf("id = %q, want desktop", id)
	}
}

func TestResolveManagedAppIDDisambiguatesWithUpstreamID(t *testing.T) {
	tmp := t.TempDir()
	setupIntegrationConfigForTest(t, tmp)

	if err := repo.NewStore(config.DbSrc).AddApp(existingIdentity("notes", "Notes", "/tmp/vendor1.AppImage"), true); err != nil {
		t.Fatal(err)
	}

	id, _, err := ResolveManagedAppID("Notes", "com.vendor2.Notes", "/tmp/vendor2.AppImage", incomingIdentity("com.vendor2.Notes", "Notes", "/tmp/vendor2.AppImage"))
	if err != nil {
		t.Fatalf("ResolveManagedAppID returned error: %v", err)
	}
	if id != "notes-com.vendor2.Notes" {
		t.Fatalf("id = %q, want notes-com.vendor2.Notes", id)
	}
}

func TestResolveManagedAppIDDisambiguatesWithHash(t *testing.T) {
	tmp := t.TempDir()
	setupIntegrationConfigForTest(t, tmp)

	if err := repo.NewStore(config.DbSrc).AddApp(existingIdentity("notes", "Notes", "/tmp/vendor1.AppImage"), true); err != nil {
		t.Fatal(err)
	}
	if err := repo.NewStore(config.DbSrc).AddApp(existingIdentity("notes-com.vendor2.Notes", "Notes", "/tmp/other-vendor2.AppImage"), true); err != nil {
		t.Fatal(err)
	}

	seed := "/tmp/vendor2.AppImage"
	id, _, err := ResolveManagedAppID("Notes", "com.vendor2.Notes", seed, incomingIdentity("com.vendor2.Notes", "Notes", seed))
	if err != nil {
		t.Fatalf("ResolveManagedAppID returned error: %v", err)
	}

	want := "notes-com.vendor2.Notes-" + shortIdentityHash("com.vendor2.Notes", seed)
	if id != want {
		t.Fatalf("id = %q, want %q", id, want)
	}
}

func TestResolveManagedAppIDReusesSameEquivalentID(t *testing.T) {
	tmp := t.TempDir()
	setupIntegrationConfigForTest(t, tmp)

	src := filepath.Join(tmp, "desktop.AppImage")
	if err := repo.NewStore(config.DbSrc).AddApp(existingIdentity("clickup", "ClickUp", src), true); err != nil {
		t.Fatal(err)
	}

	id, replacement, err := ResolveManagedAppID("ClickUp", "desktop", src, incomingIdentity("desktop", "ClickUp", src))
	if err != nil {
		t.Fatalf("ResolveManagedAppID returned error: %v", err)
	}
	if id != "clickup" {
		t.Fatalf("id = %q, want clickup", id)
	}
	if replacement != nil {
		t.Fatalf("replacement = %#v, want nil", replacement)
	}
}

func TestResolveManagedAppIDReturnsEquivalentReplacementForOldID(t *testing.T) {
	tmp := t.TempDir()
	setupIntegrationConfigForTest(t, tmp)

	src := filepath.Join(tmp, "desktop.AppImage")
	if err := repo.NewStore(config.DbSrc).AddApp(existingIdentity("desktop", "ClickUp", src), true); err != nil {
		t.Fatal(err)
	}

	id, replacement, err := ResolveManagedAppID("ClickUp", "desktop", src, incomingIdentity("desktop", "ClickUp", src))
	if err != nil {
		t.Fatalf("ResolveManagedAppID returned error: %v", err)
	}
	if id != "clickup" {
		t.Fatalf("id = %q, want clickup", id)
	}
	if replacement == nil || replacement.ID != "desktop" {
		t.Fatalf("replacement = %#v, want desktop app", replacement)
	}
}

func incomingIdentity(id, name, src string) *models.App {
	return &models.App{
		ID:     id,
		Name:   name,
		Source: localFileSource(src),
	}
}

func existingIdentity(id, name, src string) *models.App {
	return &models.App{
		ID:     id,
		Name:   name,
		Source: localFileSource(src),
	}
}

func localFileSource(src string) models.Source {
	return models.Source{
		Kind: models.SourceLocalFile,
		LocalFile: &models.LocalFileSource{
			OriginalPath: src,
		},
	}
}
