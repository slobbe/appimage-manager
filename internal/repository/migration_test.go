package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slobbe/appimage-manager/internal/config"
	models "github.com/slobbe/appimage-manager/internal/types"
)

func TestMigrateLegacyToXDG(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	newDataHome := filepath.Join(homeDir, "xdg-data")
	newStateHome := filepath.Join(homeDir, "xdg-state")

	originalAimDir := config.AimDir
	originalDesktopDir := config.DesktopDir
	originalConfigDir := config.ConfigDir
	originalTempDir := config.TempDir
	originalDbSrc := config.DbSrc
	originalIconThemeDir := config.IconThemeDir
	originalPixmapsDir := config.PixmapsDir
	t.Cleanup(func() {
		config.AimDir = originalAimDir
		config.DesktopDir = originalDesktopDir
		config.ConfigDir = originalConfigDir
		config.TempDir = originalTempDir
		config.DbSrc = originalDbSrc
		config.IconThemeDir = originalIconThemeDir
		config.PixmapsDir = originalPixmapsDir
	})

	config.AimDir = filepath.Join(newDataHome, "appimage-manager")
	config.DesktopDir = filepath.Join(newDataHome, "applications")
	config.ConfigDir = filepath.Join(homeDir, "xdg-config", "appimage-manager")
	config.TempDir = filepath.Join(homeDir, "xdg-cache", "appimage-manager", "tmp")
	config.DbSrc = filepath.Join(newStateHome, "appimage-manager", "apps.json")
	config.IconThemeDir = filepath.Join(newDataHome, "icons", "hicolor")
	config.PixmapsDir = filepath.Join(newDataHome, "pixmaps")

	legacyAimDir := filepath.Join(homeDir, ".appimage-manager")
	legacyAppDir := filepath.Join(legacyAimDir, "my-app")
	legacyDesktopDir := filepath.Join(homeDir, ".local", "share", "applications")
	legacyDBPath := filepath.Join(legacyAimDir, "apps.json")

	if err := os.MkdirAll(legacyAppDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(legacyDesktopDir, 0o755); err != nil {
		t.Fatal(err)
	}

	legacyExec := filepath.Join(legacyAppDir, "my-app.AppImage")
	legacyDesktop := filepath.Join(legacyAppDir, "my-app.desktop")
	legacyIcon := filepath.Join(legacyAppDir, "my-app.png")
	legacyLink := filepath.Join(legacyDesktopDir, "aim-my-app.desktop")

	if err := os.WriteFile(legacyExec, []byte("exec"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyDesktop, []byte("[Desktop Entry]\nName=My App\nExec=/old/AppRun %U\nIcon=/old/icon.png\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyIcon, []byte("icon"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(legacyDesktop, legacyLink); err != nil {
		t.Fatal(err)
	}

	legacyDB := &DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {
				ID:               "my-app",
				ExecPath:         legacyExec,
				DesktopEntryPath: legacyDesktop,
				DesktopEntryLink: legacyLink,
				IconPath:         legacyIcon,
				Source: models.Source{
					Kind: models.SourceLocalFile,
					LocalFile: &models.LocalFileSource{
						OriginalPath: legacyExec,
					},
				},
			},
		},
	}

	if err := SaveDB(legacyDBPath, legacyDB); err != nil {
		t.Fatal(err)
	}

	if err := MigrateLegacyToXDG(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	migratedDB, err := LoadDB(config.DbSrc)
	if err != nil {
		t.Fatalf("failed to load migrated DB: %v", err)
	}

	app, ok := migratedDB.Apps["my-app"]
	if !ok {
		t.Fatal("migrated app not found")
	}

	expectedExec := filepath.Join(config.AimDir, "my-app", "my-app.AppImage")
	expectedDesktop := filepath.Join(config.AimDir, "my-app", "my-app.desktop")
	expectedIcon := filepath.Join(config.IconThemeDir, "256x256", "apps", "my-app.png")
	expectedLink := filepath.Join(config.DesktopDir, "aim-my-app.desktop")

	if app.ExecPath != expectedExec {
		t.Fatalf("ExecPath = %q, want %q", app.ExecPath, expectedExec)
	}
	if app.DesktopEntryPath != expectedDesktop {
		t.Fatalf("DesktopEntryPath = %q, want %q", app.DesktopEntryPath, expectedDesktop)
	}
	if app.IconPath != expectedIcon {
		t.Fatalf("IconPath = %q, want %q", app.IconPath, expectedIcon)
	}
	if app.DesktopEntryLink != expectedLink {
		t.Fatalf("DesktopEntryLink = %q, want %q", app.DesktopEntryLink, expectedLink)
	}
	if app.Source.LocalFile == nil {
		t.Fatal("expected LocalFile source after migration")
	}
	if app.Source.LocalFile.OriginalPath != expectedExec {
		t.Fatalf("OriginalPath = %q, want %q", app.Source.LocalFile.OriginalPath, expectedExec)
	}

	if _, err := os.Stat(legacyExec); err != nil {
		t.Fatalf("legacy exec should still exist: %v", err)
	}
	if _, err := os.Stat(expectedExec); err != nil {
		t.Fatalf("migrated exec missing: %v", err)
	}
	if _, err := os.Stat(expectedIcon); err != nil {
		t.Fatalf("migrated icon missing: %v", err)
	}

	desktopContents, err := os.ReadFile(expectedDesktop)
	if err != nil {
		t.Fatalf("failed to read migrated desktop file: %v", err)
	}
	desktopText := string(desktopContents)
	if !strings.Contains(desktopText, "Exec="+expectedExec+" %U") {
		t.Fatalf("desktop file Exec was not rewritten correctly: %s", desktopText)
	}
	if !strings.Contains(desktopText, "Icon=my-app") {
		t.Fatalf("desktop file Icon was not rewritten to icon name: %s", desktopText)
	}

	linkTarget, err := os.Readlink(expectedLink)
	if err != nil {
		t.Fatalf("expected desktop symlink missing: %v", err)
	}
	if linkTarget != expectedDesktop {
		t.Fatalf("desktop link target = %q, want %q", linkTarget, expectedDesktop)
	}
}

func TestMigrateLegacyToXDGIdempotent(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	originalAimDir := config.AimDir
	originalDesktopDir := config.DesktopDir
	originalConfigDir := config.ConfigDir
	originalTempDir := config.TempDir
	originalDbSrc := config.DbSrc
	originalIconThemeDir := config.IconThemeDir
	originalPixmapsDir := config.PixmapsDir
	t.Cleanup(func() {
		config.AimDir = originalAimDir
		config.DesktopDir = originalDesktopDir
		config.ConfigDir = originalConfigDir
		config.TempDir = originalTempDir
		config.DbSrc = originalDbSrc
		config.IconThemeDir = originalIconThemeDir
		config.PixmapsDir = originalPixmapsDir
	})

	config.AimDir = filepath.Join(homeDir, "xdg-data", "appimage-manager")
	config.DesktopDir = filepath.Join(homeDir, "xdg-data", "applications")
	config.ConfigDir = filepath.Join(homeDir, "xdg-config", "appimage-manager")
	config.TempDir = filepath.Join(homeDir, "xdg-cache", "appimage-manager", "tmp")
	config.DbSrc = filepath.Join(homeDir, "xdg-state", "appimage-manager", "apps.json")
	config.IconThemeDir = filepath.Join(homeDir, "xdg-data", "icons", "hicolor")
	config.PixmapsDir = filepath.Join(homeDir, "xdg-data", "pixmaps")

	if err := os.MkdirAll(filepath.Dir(config.DbSrc), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SaveDB(config.DbSrc, &DB{SchemaVersion: 1, Apps: map[string]*models.App{}}); err != nil {
		t.Fatal(err)
	}

	if err := MigrateLegacyToXDG(); err != nil {
		t.Fatalf("migration should no-op when new DB exists: %v", err)
	}
}

func TestMigrateLegacyToXDGRepairsExistingXDGDB(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	originalAimDir := config.AimDir
	originalDesktopDir := config.DesktopDir
	originalConfigDir := config.ConfigDir
	originalTempDir := config.TempDir
	originalDbSrc := config.DbSrc
	originalIconThemeDir := config.IconThemeDir
	originalPixmapsDir := config.PixmapsDir
	t.Cleanup(func() {
		config.AimDir = originalAimDir
		config.DesktopDir = originalDesktopDir
		config.ConfigDir = originalConfigDir
		config.TempDir = originalTempDir
		config.DbSrc = originalDbSrc
		config.IconThemeDir = originalIconThemeDir
		config.PixmapsDir = originalPixmapsDir
	})

	config.AimDir = filepath.Join(homeDir, "xdg-data", "appimage-manager")
	config.DesktopDir = filepath.Join(homeDir, "xdg-data", "applications")
	config.ConfigDir = filepath.Join(homeDir, "xdg-config", "appimage-manager")
	config.TempDir = filepath.Join(homeDir, "xdg-cache", "appimage-manager", "tmp")
	config.DbSrc = filepath.Join(homeDir, "xdg-state", "appimage-manager", "apps.json")
	config.IconThemeDir = filepath.Join(homeDir, "xdg-data", "icons", "hicolor")
	config.PixmapsDir = filepath.Join(homeDir, "xdg-data", "pixmaps")

	legacyAimDir := filepath.Join(homeDir, ".appimage-manager")
	legacyAppDir := filepath.Join(legacyAimDir, "my-app")
	legacyDesktopDir := filepath.Join(homeDir, ".local", "share", "applications")
	if err := os.MkdirAll(legacyAppDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(legacyDesktopDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(config.DbSrc), 0o755); err != nil {
		t.Fatal(err)
	}

	legacyExec := filepath.Join(legacyAppDir, "my-app.AppImage")
	legacyDesktop := filepath.Join(legacyAppDir, "my-app.desktop")
	legacyIcon := filepath.Join(legacyAppDir, "my-app.png")
	legacyLink := filepath.Join(legacyDesktopDir, "aim-my-app.desktop")

	if err := os.WriteFile(legacyExec, []byte("exec"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyDesktop, []byte("[Desktop Entry]\nName=My App\nExec=/legacy/run %U\nIcon=/legacy/icon.png\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyIcon, []byte("icon"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(legacyDesktop, legacyLink); err != nil {
		t.Fatal(err)
	}

	if err := SaveDB(config.DbSrc, &DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {
				ID:               "my-app",
				ExecPath:         legacyExec,
				DesktopEntryPath: legacyDesktop,
				DesktopEntryLink: legacyLink,
				IconPath:         legacyIcon,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if err := MigrateLegacyToXDG(); err != nil {
		t.Fatalf("repair migration failed: %v", err)
	}

	db, err := LoadDB(config.DbSrc)
	if err != nil {
		t.Fatal(err)
	}
	app := db.Apps["my-app"]
	if app == nil {
		t.Fatal("app missing from repaired DB")
	}

	expectedExec := filepath.Join(config.AimDir, "my-app", "my-app.AppImage")
	expectedDesktop := filepath.Join(config.AimDir, "my-app", "my-app.desktop")
	expectedIcon := filepath.Join(config.IconThemeDir, "256x256", "apps", "my-app.png")

	if app.ExecPath != expectedExec {
		t.Fatalf("ExecPath = %q, want %q", app.ExecPath, expectedExec)
	}
	if app.DesktopEntryPath != expectedDesktop {
		t.Fatalf("DesktopEntryPath = %q, want %q", app.DesktopEntryPath, expectedDesktop)
	}
	if app.IconPath != expectedIcon {
		t.Fatalf("IconPath = %q, want %q", app.IconPath, expectedIcon)
	}

	desktopContents, err := os.ReadFile(expectedDesktop)
	if err != nil {
		t.Fatal(err)
	}
	desktopText := string(desktopContents)
	if !strings.Contains(desktopText, "Exec="+expectedExec+" %U") {
		t.Fatalf("desktop file Exec not rewritten correctly: %s", desktopText)
	}
	if !strings.Contains(desktopText, "Icon=my-app") {
		t.Fatalf("desktop file Icon not rewritten to icon name: %s", desktopText)
	}
}
