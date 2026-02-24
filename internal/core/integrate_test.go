package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/slobbe/appimage-manager/internal/config"
	repo "github.com/slobbe/appimage-manager/internal/repository"
)

func TestIntegrateFromLocalFileWithSymlinkedDesktopEntry(t *testing.T) {
	tmp := t.TempDir()
	setupIntegrationConfigForTest(t, tmp)
	stubDesktopValidationForTest(t)
	stubIntegrationCacheRefreshForTest(t)

	appImagePath := filepath.Join(tmp, "0ad-0.28.0-x86_64.AppImage")
	writeFakeAppImageExtractor(t, appImagePath)

	app, err := IntegrateFromLocalFile(context.Background(), appImagePath, nil)
	if err != nil {
		t.Fatalf("IntegrateFromLocalFile returned error: %v", err)
	}

	if app == nil {
		t.Fatal("expected integrated app")
	}
	if app.Name != "0 A.D." {
		t.Fatalf("app.Name = %q, want %q", app.Name, "0 A.D.")
	}
	if app.ID != "0-ad" {
		t.Fatalf("app.ID = %q, want %q", app.ID, "0-ad")
	}

	desktopInfo, err := os.Lstat(app.DesktopEntryPath)
	if err != nil {
		t.Fatalf("expected desktop entry at %q: %v", app.DesktopEntryPath, err)
	}
	if desktopInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("expected integrated desktop entry to be a regular file: %s", app.DesktopEntryPath)
	}

	linkTarget, err := os.Readlink(app.DesktopEntryLink)
	if err != nil {
		t.Fatalf("expected desktop integration symlink at %q: %v", app.DesktopEntryLink, err)
	}
	if linkTarget != app.DesktopEntryPath {
		t.Fatalf("desktop symlink target = %q, want %q", linkTarget, app.DesktopEntryPath)
	}

	persisted, err := repo.GetApp(app.ID)
	if err != nil {
		t.Fatalf("expected persisted app %q: %v", app.ID, err)
	}
	if persisted.DesktopEntryPath != app.DesktopEntryPath {
		t.Fatalf("persisted desktop_entry_path = %q, want %q", persisted.DesktopEntryPath, app.DesktopEntryPath)
	}
}

func setupIntegrationConfigForTest(t *testing.T, tmp string) {
	t.Helper()

	originalAimDir := config.AimDir
	originalTempDir := config.TempDir
	originalDesktopDir := config.DesktopDir
	originalIconThemeDir := config.IconThemeDir
	originalPixmapsDir := config.PixmapsDir
	originalDbSrc := config.DbSrc
	t.Cleanup(func() {
		config.AimDir = originalAimDir
		config.TempDir = originalTempDir
		config.DesktopDir = originalDesktopDir
		config.IconThemeDir = originalIconThemeDir
		config.PixmapsDir = originalPixmapsDir
		config.DbSrc = originalDbSrc
	})

	config.AimDir = filepath.Join(tmp, "aim")
	config.TempDir = filepath.Join(tmp, "cache", "tmp")
	config.DesktopDir = filepath.Join(tmp, "applications")
	config.IconThemeDir = filepath.Join(tmp, "icons", "hicolor")
	config.PixmapsDir = filepath.Join(tmp, "pixmaps")
	config.DbSrc = filepath.Join(tmp, "state", "appimage-manager", "apps.json")

	dirs := []string{
		config.AimDir,
		config.DesktopDir,
		config.IconThemeDir,
		config.PixmapsDir,
		filepath.Dir(config.DbSrc),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create test dir %q: %v", dir, err)
		}
	}
}

func stubDesktopValidationForTest(t *testing.T) {
	t.Helper()

	originalLookPath := desktopValidateLookPath
	originalWarn := desktopValidateWarn
	t.Cleanup(func() {
		desktopValidateLookPath = originalLookPath
		desktopValidateWarn = originalWarn
	})

	desktopValidateLookPath = func(string) (string, error) {
		return "", exec.ErrNotFound
	}
	desktopValidateWarn = func(string) {}
}

func stubIntegrationCacheRefreshForTest(t *testing.T) {
	t.Helper()

	originalLookPath := integrationCacheLookPath
	originalWarn := integrationCacheWarn
	t.Cleanup(func() {
		integrationCacheLookPath = originalLookPath
		integrationCacheWarn = originalWarn
	})

	integrationCacheLookPath = func(string) (string, error) {
		return "", exec.ErrNotFound
	}
	integrationCacheWarn = func(string) {}
}
