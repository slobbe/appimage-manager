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

func TestIntegrateFromLocalFileReplacesEquivalentManagedAppWhenDesktopIDChanges(t *testing.T) {
	tmp := t.TempDir()
	setupIntegrationConfigForTest(t, tmp)
	stubDesktopValidationForTest(t)
	stubIntegrationCacheRefreshForTest(t)

	originalGetEmbeddedUpdateInfo := getEmbeddedUpdateInfo
	t.Cleanup(func() {
		getEmbeddedUpdateInfo = originalGetEmbeddedUpdateInfo
	})
	getEmbeddedUpdateInfo = func(string) (*UpdateInfo, error) {
		return &UpdateInfo{
			Kind:       models.UpdateZsync,
			UpdateInfo: "zsync|https://example.com/t3-code.AppImage.zsync",
			Transport:  "zsync",
		}, nil
	}

	oldAppDir := filepath.Join(config.AimDir, "t3-code-desktop")
	oldExecPath := filepath.Join(oldAppDir, "t3-code-desktop.AppImage")
	oldDesktopPath := filepath.Join(oldAppDir, "t3-code-desktop.desktop")
	oldDesktopLink := filepath.Join(config.DesktopDir, "t3-code-desktop.desktop")
	oldIconPath := filepath.Join(config.IconThemeDir, "scalable", "apps", "t3-code-desktop.svg")

	if err := os.MkdirAll(oldAppDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(oldIconPath), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{oldExecPath, oldDesktopPath, oldIconPath} {
		if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(oldDesktopPath, oldDesktopLink); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddApp(&models.App{
		ID:               "t3-code-desktop",
		Name:             "T3 Code",
		Version:          "0.0.14",
		ExecPath:         oldExecPath,
		DesktopEntryPath: oldDesktopPath,
		DesktopEntryLink: oldDesktopLink,
		IconPath:         oldIconPath,
		AddedAt:          "2026-04-01T12:00:00Z",
		LastCheckedAt:    "2026-04-02T12:00:00Z",
		Update: &models.UpdateSource{
			Kind: models.UpdateZsync,
			Zsync: &models.ZsyncUpdateSource{
				UpdateInfo: "zsync|https://example.com/t3-code.AppImage.zsync",
				Transport:  "zsync",
			},
		},
	}, true); err != nil {
		t.Fatal(err)
	}

	appImagePath := filepath.Join(tmp, "t3-code.AppImage")
	writeFakeAppImageExtractorWithDesktop(t, appImagePath, "t3-code.desktop", "T3 Code", "0.0.15", "t3-code", "t3-code.svg")

	app, err := IntegrateFromLocalFile(context.Background(), appImagePath, nil)
	if err != nil {
		t.Fatalf("IntegrateFromLocalFile returned error: %v", err)
	}

	if app.ID != "t3-code" {
		t.Fatalf("app.ID = %q, want %q", app.ID, "t3-code")
	}
	if app.ReplacesID != "" {
		t.Fatalf("app.ReplacesID = %q, want empty after persisted replacement", app.ReplacesID)
	}
	if app.AddedAt != "2026-04-01T12:00:00Z" {
		t.Fatalf("app.AddedAt = %q", app.AddedAt)
	}
	if app.LastCheckedAt != "2026-04-02T12:00:00Z" {
		t.Fatalf("app.LastCheckedAt = %q", app.LastCheckedAt)
	}
	if _, err := repo.GetApp("t3-code-desktop"); err == nil {
		t.Fatal("expected old app id to be removed from database")
	}
	persisted, err := repo.GetApp("t3-code")
	if err != nil {
		t.Fatalf("expected new app id to be persisted: %v", err)
	}
	if persisted.Update == nil || persisted.Update.Kind != models.UpdateZsync {
		t.Fatalf("persisted.Update = %#v", persisted.Update)
	}
	if _, err := os.Stat(oldAppDir); !os.IsNotExist(err) {
		t.Fatalf("expected old app dir to be removed, got err=%v", err)
	}
	if _, err := os.Lstat(oldDesktopLink); !os.IsNotExist(err) {
		t.Fatalf("expected old desktop link to be removed, got err=%v", err)
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
	originalDbSrc := config.DbSrc
	t.Cleanup(func() {
		config.AimDir = originalAimDir
		config.TempDir = originalTempDir
		config.DesktopDir = originalDesktopDir
		config.IconThemeDir = originalIconThemeDir
		config.DbSrc = originalDbSrc
	})

	config.AimDir = filepath.Join(tmp, "aim")
	config.TempDir = filepath.Join(tmp, "cache", "tmp")
	config.DesktopDir = filepath.Join(tmp, "applications")
	config.IconThemeDir = filepath.Join(tmp, "icons", "hicolor")
	config.DbSrc = filepath.Join(tmp, "state", "aim", "apps.json")

	dirs := []string{
		config.AimDir,
		config.DesktopDir,
		config.IconThemeDir,
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
