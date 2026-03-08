package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/slobbe/appimage-manager/internal/config"
	repo "github.com/slobbe/appimage-manager/internal/repository"
	models "github.com/slobbe/appimage-manager/internal/types"
)

func TestRemoveUnlinkPreservesManagedFiles(t *testing.T) {
	tmp := t.TempDir()
	setupIntegrationConfigForTest(t, tmp)
	stubIntegrationCacheRefreshForTest(t)

	appDir := filepath.Join(config.AimDir, "my-app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}

	execPath := filepath.Join(appDir, "my-app.AppImage")
	desktopPath := filepath.Join(appDir, "my-app.desktop")
	iconPath := filepath.Join(appDir, "my-app.png")
	desktopLink := filepath.Join(config.DesktopDir, "aim-my-app.desktop")

	for _, path := range []string{execPath, desktopPath, iconPath} {
		if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(desktopPath, desktopLink); err != nil {
		t.Fatal(err)
	}

	if err := repo.SaveDB(config.DbSrc, &repo.DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {
				ID:               "my-app",
				Name:             "My App",
				ExecPath:         execPath,
				DesktopEntryPath: desktopPath,
				DesktopEntryLink: desktopLink,
				IconPath:         iconPath,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	app, err := Remove(context.Background(), "my-app", true)
	if err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if app.DesktopEntryLink != "" {
		t.Fatalf("returned app DesktopEntryLink = %q, want empty", app.DesktopEntryLink)
	}

	persisted, err := repo.GetApp("my-app")
	if err != nil {
		t.Fatalf("expected app to remain persisted: %v", err)
	}
	if persisted.DesktopEntryLink != "" {
		t.Fatalf("persisted DesktopEntryLink = %q, want empty", persisted.DesktopEntryLink)
	}

	if _, err := os.Lstat(desktopLink); !os.IsNotExist(err) {
		t.Fatalf("expected desktop link to be removed, got err=%v", err)
	}
	for _, path := range []string{appDir, execPath, desktopPath, iconPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %q to remain after unlink removal: %v", path, err)
		}
	}
}

func TestRemoveDeletesManagedFilesWhenNotUnlinking(t *testing.T) {
	tmp := t.TempDir()
	setupIntegrationConfigForTest(t, tmp)
	stubIntegrationCacheRefreshForTest(t)

	appDir := filepath.Join(config.AimDir, "my-app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}

	execPath := filepath.Join(appDir, "my-app.AppImage")
	desktopPath := filepath.Join(appDir, "my-app.desktop")
	desktopLink := filepath.Join(config.DesktopDir, "aim-my-app.desktop")
	externalIcon := filepath.Join(tmp, "external-icon.png")

	for _, path := range []string{execPath, desktopPath, externalIcon} {
		if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(desktopPath, desktopLink); err != nil {
		t.Fatal(err)
	}

	if err := repo.SaveDB(config.DbSrc, &repo.DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {
				ID:               "my-app",
				Name:             "My App",
				ExecPath:         execPath,
				DesktopEntryPath: desktopPath,
				DesktopEntryLink: desktopLink,
				IconPath:         externalIcon,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := Remove(context.Background(), "my-app", false); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}

	if _, err := repo.GetApp("my-app"); err == nil {
		t.Fatal("expected app to be removed from database")
	}
	if _, err := os.Stat(appDir); !os.IsNotExist(err) {
		t.Fatalf("expected app dir to be removed, got err=%v", err)
	}
	if _, err := os.Lstat(desktopLink); !os.IsNotExist(err) {
		t.Fatalf("expected desktop link to be removed, got err=%v", err)
	}
	if _, err := os.Stat(externalIcon); !os.IsNotExist(err) {
		t.Fatalf("expected external icon to be removed, got err=%v", err)
	}
}
