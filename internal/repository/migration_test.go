package repo

import (
	"os"
	"path/filepath"
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
	originalTempDir := config.TempDir
	originalDbSrc := config.DbSrc
	t.Cleanup(func() {
		config.AimDir = originalAimDir
		config.DesktopDir = originalDesktopDir
		config.TempDir = originalTempDir
		config.DbSrc = originalDbSrc
	})

	config.AimDir = filepath.Join(newDataHome, "appimage-manager")
	config.DesktopDir = filepath.Join(newDataHome, "applications")
	config.TempDir = filepath.Join(homeDir, "xdg-cache", "appimage-manager", "tmp")
	config.DbSrc = filepath.Join(newStateHome, "appimage-manager", "apps.json")

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
	if err := os.WriteFile(legacyDesktop, []byte("[Desktop Entry]\nName=My App\n"), 0o644); err != nil {
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
	expectedIcon := filepath.Join(config.AimDir, "my-app", "my-app.png")
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
	originalTempDir := config.TempDir
	originalDbSrc := config.DbSrc
	t.Cleanup(func() {
		config.AimDir = originalAimDir
		config.DesktopDir = originalDesktopDir
		config.TempDir = originalTempDir
		config.DbSrc = originalDbSrc
	})

	config.AimDir = filepath.Join(homeDir, "xdg-data", "appimage-manager")
	config.DesktopDir = filepath.Join(homeDir, "xdg-data", "applications")
	config.TempDir = filepath.Join(homeDir, "xdg-cache", "appimage-manager", "tmp")
	config.DbSrc = filepath.Join(homeDir, "xdg-state", "appimage-manager", "apps.json")

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
