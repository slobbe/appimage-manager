package core

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slobbe/appimage-manager/internal/config"
	repo "github.com/slobbe/appimage-manager/internal/repository"
	models "github.com/slobbe/appimage-manager/internal/types"
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
	if app.ID != "0ad" {
		t.Fatalf("app.ID = %q, want %q", app.ID, "0ad")
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

func TestIntegrateFromLocalFileWithoutCacheRefreshSkipsRefresh(t *testing.T) {
	tmp := t.TempDir()
	setupIntegrationConfigForTest(t, tmp)
	stubDesktopValidationForTest(t)

	var lookupCalls int32
	originalLookPath := integrationCacheLookPath
	originalWarn := integrationCacheWarn
	t.Cleanup(func() {
		integrationCacheLookPath = originalLookPath
		integrationCacheWarn = originalWarn
	})

	integrationCacheLookPath = func(string) (string, error) {
		atomic.AddInt32(&lookupCalls, 1)
		return "", exec.ErrNotFound
	}
	integrationCacheWarn = func(string) {}

	appImagePath := filepath.Join(tmp, "0ad-0.28.0-x86_64.AppImage")
	writeFakeAppImageExtractor(t, appImagePath)

	app, err := IntegrateFromLocalFileWithoutCacheRefresh(context.Background(), appImagePath, nil)
	if err != nil {
		t.Fatalf("IntegrateFromLocalFileWithoutCacheRefresh returned error: %v", err)
	}
	if app == nil {
		t.Fatal("expected integrated app")
	}

	if atomic.LoadInt32(&lookupCalls) != 0 {
		t.Fatalf("expected no cache refresh calls, got %d", lookupCalls)
	}
}

func TestIntegrateFromLocalFileWithoutCacheRefreshOrPersistSkipsDatabaseSave(t *testing.T) {
	tmp := t.TempDir()
	setupIntegrationConfigForTest(t, tmp)
	stubDesktopValidationForTest(t)
	stubIntegrationCacheRefreshForTest(t)

	appImagePath := filepath.Join(tmp, "0ad-0.28.0-x86_64.AppImage")
	writeFakeAppImageExtractor(t, appImagePath)

	app, err := IntegrateFromLocalFileWithoutCacheRefreshOrPersist(context.Background(), appImagePath, nil)
	if err != nil {
		t.Fatalf("IntegrateFromLocalFileWithoutCacheRefreshOrPersist returned error: %v", err)
	}
	if app == nil {
		t.Fatal("expected integrated app")
	}

	if _, err := repo.GetApp(app.ID); err == nil {
		t.Fatalf("expected app %q not to be persisted", app.ID)
	}
}

func TestIntegrateFromLocalFileReturnsPromptlyWhenContextCanceled(t *testing.T) {
	tmp := t.TempDir()
	setupIntegrationConfigForTest(t, tmp)
	stubDesktopValidationForTest(t)
	stubIntegrationCacheRefreshForTest(t)

	appImagePath := filepath.Join(tmp, "slow.AppImage")
	writeSlowFakeAppImageExtractor(t, appImagePath)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := IntegrateFromLocalFile(ctx, appImagePath, nil)
		done <- err
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want %v", err, context.Canceled)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("IntegrateFromLocalFile did not return after cancellation")
	}
}

func TestIntegrateFromLocalFilePreservesUpstreamDesktopStemForManagedIdentity(t *testing.T) {
	tmp := t.TempDir()
	setupIntegrationConfigForTest(t, tmp)
	stubDesktopValidationForTest(t)
	stubIntegrationCacheRefreshForTest(t)

	appImagePath := filepath.Join(tmp, "t3-code-alpha.AppImage")
	writeFakeAppImageExtractorWithDesktop(t, appImagePath, "t3-code-desktop.desktop", "T3 Code (Alpha)", "0.0.14", "t3-code-desktop", "t3-code-desktop.svg")

	app, err := IntegrateFromLocalFile(context.Background(), appImagePath, nil)
	if err != nil {
		t.Fatalf("IntegrateFromLocalFile returned error: %v", err)
	}

	if app.ID != "t3-code-desktop" {
		t.Fatalf("app.ID = %q, want %q", app.ID, "t3-code-desktop")
	}

	expectedAppDir := filepath.Join(config.AimDir, "t3-code-desktop")
	if app.ExecPath != filepath.Join(expectedAppDir, "t3-code-desktop.AppImage") {
		t.Fatalf("ExecPath = %q", app.ExecPath)
	}
	if app.DesktopEntryPath != filepath.Join(expectedAppDir, "t3-code-desktop.desktop") {
		t.Fatalf("DesktopEntryPath = %q", app.DesktopEntryPath)
	}
	if app.IconPath != filepath.Join(config.IconThemeDir, "scalable", "apps", "t3-code-desktop.svg") {
		t.Fatalf("IconPath = %q", app.IconPath)
	}
	if _, err := os.Stat(app.IconPath); err != nil {
		t.Fatalf("expected installed icon at %q: %v", app.IconPath, err)
	}
	if app.DesktopEntryLink != filepath.Join(config.DesktopDir, "t3-code-desktop.desktop") {
		t.Fatalf("DesktopEntryLink = %q", app.DesktopEntryLink)
	}
}

func TestIntegrateExistingPrefersUnprefixedDesktopLink(t *testing.T) {
	tmp := t.TempDir()
	setupIntegrationConfigForTest(t, tmp)
	stubDesktopValidationForTest(t)
	stubIntegrationCacheRefreshForTest(t)

	appDir := filepath.Join(config.AimDir, "my-app")
	execPath := filepath.Join(appDir, "my-app.AppImage")
	desktopPath := filepath.Join(appDir, "my-app.desktop")
	iconPath := filepath.Join(config.IconThemeDir, "256x256", "apps", "my-app.png")

	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(desktopPath, []byte("[Desktop Entry]\nName=My App\nExec="+execPath+"\nIcon="+iconPath+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(iconPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(iconPath, []byte("icon"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := repo.AddApp(&models.App{
		ID:               "my-app",
		Name:             "My App",
		ExecPath:         execPath,
		DesktopEntryPath: desktopPath,
		IconPath:         iconPath,
	}, true); err != nil {
		t.Fatal(err)
	}

	integrated, err := IntegrateExisting(context.Background(), "my-app")
	if err != nil {
		t.Fatalf("IntegrateExisting returned error: %v", err)
	}
	if integrated.DesktopEntryLink != filepath.Join(config.DesktopDir, "my-app.desktop") {
		t.Fatalf("DesktopEntryLink = %q", integrated.DesktopEntryLink)
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
	config.DbSrc = filepath.Join(tmp, "state", "aim", "apps.json")

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
