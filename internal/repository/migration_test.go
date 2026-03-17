package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/slobbe/appimage-manager/internal/config"
	models "github.com/slobbe/appimage-manager/internal/types"
)

type migrationTestPaths struct {
	currentDataHome   string
	currentConfigHome string
	currentStateHome  string
	currentCacheHome  string
	currentAimDir     string
	currentDesktopDir string
	currentConfigDir  string
	currentTempDir    string
	currentDBPath     string

	legacyHomeDir string

	oldXDGDataDir   string
	oldXDGConfigDir string
	oldXDGStateDir  string
	oldXDGCacheDir  string
}

type testAppFiles struct {
	Exec    string
	Desktop string
	Icon    string
}

func TestMigrateToCurrentPathsFromLegacyHome(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	paths := configureMigrationTestPaths(t, homeDir)
	mustEnsureCurrentDirs(t)

	legacyFiles := createLegacyAppFilesWithContents(t, paths.legacyHomeDir, "my-app", "My App", "legacy-exec", "legacy-icon")
	legacyDesktopDir := filepath.Join(homeDir, ".local", "share", "applications")
	mustMkdirAll(t, legacyDesktopDir)
	legacyLink := filepath.Join(legacyDesktopDir, "aim-my-app.desktop")
	if err := os.Symlink(legacyFiles.Desktop, legacyLink); err != nil {
		t.Fatal(err)
	}

	if err := SaveDB(filepath.Join(paths.legacyHomeDir, "apps.json"), &DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {
				ID:               "my-app",
				ExecPath:         legacyFiles.Exec,
				DesktopEntryPath: legacyFiles.Desktop,
				DesktopEntryLink: legacyLink,
				IconPath:         legacyFiles.Icon,
				Source: models.Source{
					Kind: models.SourceLocalFile,
					LocalFile: &models.LocalFileSource{
						OriginalPath: legacyFiles.Exec,
					},
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if err := MigrateToCurrentPaths(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	db := loadCurrentDBForTest(t)
	app := db.Apps["my-app"]
	if app == nil {
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
	if app.Source.LocalFile == nil || app.Source.LocalFile.OriginalPath != expectedExec {
		t.Fatalf("OriginalPath = %v, want %q", app.Source.LocalFile, expectedExec)
	}

	assertPathExists(t, legacyFiles.Exec)
	assertPathExists(t, legacyFiles.Icon)
	assertPathExists(t, expectedExec)
	assertPathExists(t, expectedIcon)
	if got := string(readTestFile(t, expectedExec)); got != "legacy-exec" {
		t.Fatalf("exec content = %q, want legacy-exec", got)
	}
	if got := string(readTestFile(t, expectedIcon)); got != "legacy-icon" {
		t.Fatalf("icon content = %q, want legacy-icon", got)
	}

	desktopText := string(readTestFile(t, expectedDesktop))
	if !strings.Contains(desktopText, "Name=My App") {
		t.Fatalf("desktop file name was not preserved: %s", desktopText)
	}
	if !strings.Contains(desktopText, "Exec="+expectedExec+" %U") {
		t.Fatalf("desktop file Exec was not rewritten correctly: %s", desktopText)
	}
	if !strings.Contains(desktopText, "Icon="+expectedIcon) {
		t.Fatalf("desktop file Icon was not rewritten correctly: %s", desktopText)
	}
}

func TestMigrateToCurrentPathsFromOldXDGAndCopyConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	paths := configureMigrationTestPaths(t, homeDir)
	mustEnsureCurrentDirs(t)

	oldFiles := createLegacyAppFilesWithContents(t, paths.oldXDGDataDir, "my-app", "My App", "xdg-exec", "xdg-icon")
	mustMkdirAll(t, paths.oldXDGConfigDir)
	mustMkdirAll(t, paths.oldXDGStateDir)
	mustMkdirAll(t, paths.oldXDGCacheDir)

	oldLink := filepath.Join(paths.currentDesktopDir, "aim-my-app.desktop")
	if err := os.Symlink(oldFiles.Desktop, oldLink); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(paths.oldXDGConfigDir, "copied.conf"), []byte("from-source"), 0o644)
	writeTestFile(t, filepath.Join(paths.oldXDGConfigDir, "shared.conf"), []byte("source-value"), 0o644)
	writeTestFile(t, filepath.Join(config.ConfigDir, "shared.conf"), []byte("dest-value"), 0o644)
	writeTestFile(t, filepath.Join(paths.oldXDGCacheDir, "left-behind"), []byte("temp"), 0o644)

	if err := SaveDB(filepath.Join(paths.oldXDGStateDir, "apps.json"), &DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {
				ID:               "my-app",
				ExecPath:         oldFiles.Exec,
				DesktopEntryPath: oldFiles.Desktop,
				DesktopEntryLink: oldLink,
				IconPath:         oldFiles.Icon,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if err := MigrateToCurrentPaths(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	db := loadCurrentDBForTest(t)
	app := db.Apps["my-app"]
	if app == nil {
		t.Fatal("migrated app not found")
	}

	expectedExec := filepath.Join(config.AimDir, "my-app", "my-app.AppImage")
	expectedDesktop := filepath.Join(config.AimDir, "my-app", "my-app.desktop")
	expectedIcon := filepath.Join(config.IconThemeDir, "256x256", "apps", "my-app.png")

	if app.ExecPath != expectedExec || app.DesktopEntryPath != expectedDesktop || app.IconPath != expectedIcon {
		t.Fatalf("unexpected migrated paths: %+v", app)
	}

	assertPathExists(t, oldFiles.Icon)
	assertPathExists(t, filepath.Join(config.ConfigDir, "copied.conf"))
	if got := string(readTestFile(t, filepath.Join(config.ConfigDir, "copied.conf"))); got != "from-source" {
		t.Fatalf("copied config = %q", got)
	}
	if got := string(readTestFile(t, filepath.Join(config.ConfigDir, "shared.conf"))); got != "dest-value" {
		t.Fatalf("destination config should not be overwritten, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(config.TempDir, "left-behind")); !os.IsNotExist(err) {
		t.Fatalf("expected old cache contents to stay behind, stat err = %v", err)
	}
}

func TestMigrateToCurrentPathsChoosesNewestLegacyDBWhenCurrentMissing(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	paths := configureMigrationTestPaths(t, homeDir)
	mustEnsureCurrentDirs(t)
	mustMkdirAll(t, paths.oldXDGStateDir)

	homeFiles := createLegacyAppFilesWithContents(t, paths.legacyHomeDir, "shared-app", "Shared Old", "home-shared", "home-icon")
	xdgFiles := createLegacyAppFilesWithContents(t, paths.oldXDGDataDir, "shared-app", "Shared New", "xdg-shared", "xdg-icon")
	createLegacyAppFilesWithContents(t, paths.legacyHomeDir, "home-only", "Home Only", "home-only", "home-only-icon")
	createLegacyAppFilesWithContents(t, paths.oldXDGDataDir, "xdg-only", "XDG Only", "xdg-only", "xdg-only-icon")

	homeDBPath := filepath.Join(paths.legacyHomeDir, "apps.json")
	xdgDBPath := filepath.Join(paths.oldXDGStateDir, "apps.json")

	if err := SaveDB(homeDBPath, &DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"shared-app": {
				ID:               "shared-app",
				Name:             "Old Shared",
				Version:          "1.0.0",
				UpdatedAt:        "2026-03-01T10:00:00Z",
				ExecPath:         homeFiles.Exec,
				DesktopEntryPath: homeFiles.Desktop,
				IconPath:         homeFiles.Icon,
			},
			"home-only": {
				ID:      "home-only",
				Name:    "Home Only",
				Version: "0.9.0",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := SaveDB(xdgDBPath, &DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"shared-app": {
				ID:               "shared-app",
				Name:             "New Shared",
				Version:          "2.0.0",
				UpdatedAt:        "2026-03-10T10:00:00Z",
				ExecPath:         xdgFiles.Exec,
				DesktopEntryPath: xdgFiles.Desktop,
				IconPath:         xdgFiles.Icon,
			},
			"xdg-only": {
				ID:      "xdg-only",
				Name:    "XDG Only",
				Version: "1.5.0",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	setFileModTime(t, homeDBPath, time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC))
	setFileModTime(t, xdgDBPath, time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC))

	if err := MigrateToCurrentPaths(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	db := loadCurrentDBForTest(t)
	if len(db.Apps) != 2 {
		t.Fatalf("app count = %d, want 2", len(db.Apps))
	}

	shared := db.Apps["shared-app"]
	if shared == nil {
		t.Fatal("shared-app missing")
	}
	if shared.Name != "New Shared" || shared.Version != "2.0.0" || shared.UpdatedAt != "2026-03-10T10:00:00Z" {
		t.Fatalf("shared-app came from wrong DB: %+v", shared)
	}
	if _, ok := db.Apps["xdg-only"]; !ok {
		t.Fatal("xdg-only app missing")
	}
	if _, ok := db.Apps["home-only"]; ok {
		t.Fatal("home-only app should not be imported from losing DB")
	}
}

func TestMigrateToCurrentPathsUsesXDGDBOnEqualMTimeTie(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	paths := configureMigrationTestPaths(t, homeDir)
	mustEnsureCurrentDirs(t)
	mustMkdirAll(t, paths.oldXDGStateDir)

	homeFiles := createLegacyAppFilesWithContents(t, paths.legacyHomeDir, "my-app", "Home Winner?", "home-exec", "home-icon")
	xdgFiles := createLegacyAppFilesWithContents(t, paths.oldXDGDataDir, "my-app", "XDG Winner", "xdg-exec", "xdg-icon")

	homeDBPath := filepath.Join(paths.legacyHomeDir, "apps.json")
	xdgDBPath := filepath.Join(paths.oldXDGStateDir, "apps.json")

	if err := SaveDB(homeDBPath, &DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {
				ID:               "my-app",
				Name:             "Home",
				Version:          "1.0.0",
				ExecPath:         homeFiles.Exec,
				DesktopEntryPath: homeFiles.Desktop,
				IconPath:         homeFiles.Icon,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := SaveDB(xdgDBPath, &DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {
				ID:               "my-app",
				Name:             "XDG",
				Version:          "2.0.0",
				ExecPath:         xdgFiles.Exec,
				DesktopEntryPath: xdgFiles.Desktop,
				IconPath:         xdgFiles.Icon,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	sameTime := time.Date(2026, 3, 12, 10, 0, 0, 0, time.UTC)
	setFileModTime(t, homeDBPath, sameTime)
	setFileModTime(t, xdgDBPath, sameTime)

	if err := MigrateToCurrentPaths(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	db := loadCurrentDBForTest(t)
	app := db.Apps["my-app"]
	if app == nil {
		t.Fatal("my-app missing")
	}
	if app.Name != "XDG" || app.Version != "2.0.0" {
		t.Fatalf("tie-break should prefer xdg DB, got %+v", app)
	}
}

func TestMigrateToCurrentPathsPrefersCanonicalSourceFilesWhenBootstrapping(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	paths := configureMigrationTestPaths(t, homeDir)
	mustEnsureCurrentDirs(t)
	mustMkdirAll(t, paths.oldXDGStateDir)
	mustMkdirAll(t, paths.oldXDGConfigDir)
	mustMkdirAll(t, paths.legacyHomeDir)

	homeFiles := createLegacyAppFilesWithContents(t, paths.legacyHomeDir, "my-app", "Home Desktop", "home-binary", "home-icon")
	xdgFiles := createLegacyAppFilesWithContents(t, paths.oldXDGDataDir, "my-app", "XDG Desktop", "xdg-binary", "xdg-icon")
	homeDBPath := filepath.Join(paths.legacyHomeDir, "apps.json")
	xdgDBPath := filepath.Join(paths.oldXDGStateDir, "apps.json")

	writeTestFile(t, filepath.Join(paths.legacyHomeDir, "my-app", "extra.txt"), []byte("home-extra"), 0o644)
	writeTestFile(t, filepath.Join(paths.oldXDGDataDir, "my-app", "extra.txt"), []byte("xdg-extra"), 0o644)
	writeTestFile(t, filepath.Join(paths.currentConfigHome, "appimage-manager", "shared.conf"), []byte("xdg-config"), 0o644)
	writeTestFile(t, filepath.Join(paths.legacyHomeDir, "settings.conf"), []byte("home-config-unrelated"), 0o644)
	writeTestFile(t, filepath.Join(paths.legacyHomeDir, "conflict.conf"), []byte("home-config"), 0o644)
	writeTestFile(t, filepath.Join(paths.oldXDGConfigDir, "conflict.conf"), []byte("xdg-config"), 0o644)

	if err := SaveDB(homeDBPath, &DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {
				ID:               "my-app",
				Name:             "Home Record",
				Version:          "1.0.0",
				ExecPath:         homeFiles.Exec,
				DesktopEntryPath: homeFiles.Desktop,
				IconPath:         homeFiles.Icon,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := SaveDB(xdgDBPath, &DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {
				ID:               "my-app",
				Name:             "XDG Record",
				Version:          "2.0.0",
				ExecPath:         xdgFiles.Exec,
				DesktopEntryPath: xdgFiles.Desktop,
				IconPath:         xdgFiles.Icon,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	setFileModTime(t, homeDBPath, time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC))
	setFileModTime(t, xdgDBPath, time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC))

	if err := MigrateToCurrentPaths(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	appDir := filepath.Join(config.AimDir, "my-app")
	if got := string(readTestFile(t, filepath.Join(config.ConfigDir, "conflict.conf"))); got != "xdg-config" {
		t.Fatalf("config conflict winner = %q, want xdg-config", got)
	}
	if got := string(readTestFile(t, filepath.Join(appDir, "my-app.AppImage"))); got != "xdg-binary" {
		t.Fatalf("exec content winner = %q, want xdg-binary", got)
	}
	if got := string(readTestFile(t, filepath.Join(appDir, "extra.txt"))); got != "xdg-extra" {
		t.Fatalf("extra file winner = %q, want xdg-extra", got)
	}

	desktopText := string(readTestFile(t, filepath.Join(appDir, "my-app.desktop")))
	if !strings.Contains(desktopText, "Name=XDG Desktop") {
		t.Fatalf("desktop content should come from canonical source: %s", desktopText)
	}
	if got := string(readTestFile(t, filepath.Join(config.IconThemeDir, "256x256", "apps", "my-app.png"))); got != "xdg-icon" {
		t.Fatalf("icon content winner = %q, want xdg-icon", got)
	}
}

func TestMigrateToCurrentPathsUsesNonCanonicalSourceForAssetRepairOnly(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	paths := configureMigrationTestPaths(t, homeDir)
	mustEnsureCurrentDirs(t)
	mustMkdirAll(t, paths.oldXDGStateDir)

	homeFiles := createLegacyAppFilesWithContents(t, paths.legacyHomeDir, "my-app", "Fallback Desktop", "fallback-binary", "fallback-icon")
	xdgDBPath := filepath.Join(paths.oldXDGStateDir, "apps.json")
	homeDBPath := filepath.Join(paths.legacyHomeDir, "apps.json")

	if err := SaveDB(homeDBPath, &DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {
				ID:               "my-app",
				Name:             "Repair Only",
				Version:          "1.0.0",
				ExecPath:         homeFiles.Exec,
				DesktopEntryPath: homeFiles.Desktop,
				IconPath:         homeFiles.Icon,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := SaveDB(xdgDBPath, &DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {
				ID:               "my-app",
				Name:             "Canonical",
				Version:          "2.0.0",
				ExecPath:         filepath.Join(paths.oldXDGDataDir, "my-app", "my-app.AppImage"),
				DesktopEntryPath: filepath.Join(paths.oldXDGDataDir, "my-app", "my-app.desktop"),
				IconPath:         filepath.Join(paths.oldXDGDataDir, "my-app", "my-app.png"),
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	setFileModTime(t, homeDBPath, time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC))
	setFileModTime(t, xdgDBPath, time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC))

	if err := MigrateToCurrentPaths(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	db := loadCurrentDBForTest(t)
	app := db.Apps["my-app"]
	if app == nil {
		t.Fatal("my-app missing")
	}
	if app.Name != "Canonical" || app.Version != "2.0.0" {
		t.Fatalf("metadata should come from canonical DB, got %+v", app)
	}

	if got := string(readTestFile(t, filepath.Join(config.AimDir, "my-app", "my-app.AppImage"))); got != "fallback-binary" {
		t.Fatalf("fallback exec content = %q, want fallback-binary", got)
	}
	desktopText := string(readTestFile(t, filepath.Join(config.AimDir, "my-app", "my-app.desktop")))
	if !strings.Contains(desktopText, "Name=Fallback Desktop") {
		t.Fatalf("fallback desktop content was not used: %s", desktopText)
	}
	if got := string(readTestFile(t, filepath.Join(config.IconThemeDir, "256x256", "apps", "my-app.png"))); got != "fallback-icon" {
		t.Fatalf("fallback icon content = %q, want fallback-icon", got)
	}
}

func TestMigrateToCurrentPathsDoesNotRevertExistingAimDB(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	paths := configureMigrationTestPaths(t, homeDir)
	mustEnsureCurrentDirs(t)
	mustMkdirAll(t, paths.oldXDGStateDir)

	legacyFiles := createLegacyAppFilesWithContents(t, paths.oldXDGDataDir, "my-app", "Legacy Desktop", "legacy-binary", "legacy-icon")
	currentAppDir := filepath.Join(config.AimDir, "my-app")
	currentDesktopPath := filepath.Join(currentAppDir, "my-app.desktop")
	currentIconPath := filepath.Join(config.IconThemeDir, "256x256", "apps", "my-app.png")
	writeTestFile(t, filepath.Join(config.ConfigDir, "shared.conf"), []byte("current-config"), 0o644)
	writeTestFile(t, filepath.Join(currentAppDir, "my-app.AppImage"), []byte("current-binary"), 0o755)
	writeTestFile(t, currentDesktopPath, []byte("[Desktop Entry]\nName=Current Desktop\nExec=/current %U\nIcon=/current.png\n"), 0o644)
	writeTestFile(t, currentIconPath, []byte("current-icon"), 0o644)

	currentDB := &DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {
				ID:               "my-app",
				Name:             "Current",
				Version:          "9.9.9",
				UpdatedAt:        "2026-03-15T12:00:00Z",
				ExecPath:         filepath.Join(currentAppDir, "my-app.AppImage"),
				DesktopEntryPath: currentDesktopPath,
				IconPath:         currentIconPath,
			},
		},
	}
	if err := SaveDB(config.DbSrc, currentDB); err != nil {
		t.Fatal(err)
	}
	if err := SaveDB(filepath.Join(paths.oldXDGStateDir, "apps.json"), &DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {
				ID:               "my-app",
				Name:             "Legacy",
				Version:          "1.0.0",
				UpdatedAt:        "2026-03-01T12:00:00Z",
				ExecPath:         legacyFiles.Exec,
				DesktopEntryPath: legacyFiles.Desktop,
				IconPath:         legacyFiles.Icon,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(paths.oldXDGConfigDir, "shared.conf"), []byte("legacy-config"), 0o644)

	if err := MigrateToCurrentPaths(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	db := loadCurrentDBForTest(t)
	app := db.Apps["my-app"]
	if app == nil {
		t.Fatal("my-app missing")
	}
	if app.Name != "Current" || app.Version != "9.9.9" || app.UpdatedAt != "2026-03-15T12:00:00Z" {
		t.Fatalf("current DB should remain authoritative, got %+v", app)
	}
	if got := string(readTestFile(t, filepath.Join(config.ConfigDir, "shared.conf"))); got != "current-config" {
		t.Fatalf("current config should be preserved, got %q", got)
	}
	if got := string(readTestFile(t, filepath.Join(currentAppDir, "my-app.AppImage"))); got != "current-binary" {
		t.Fatalf("current app binary should be preserved, got %q", got)
	}
	if got := string(readTestFile(t, currentIconPath)); got != "current-icon" {
		t.Fatalf("current icon should be preserved, got %q", got)
	}
}

func TestMigrateToCurrentPathsRestoresMissingFilesFromCanonicalLegacyOrdering(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	paths := configureMigrationTestPaths(t, homeDir)
	mustEnsureCurrentDirs(t)
	mustMkdirAll(t, paths.oldXDGStateDir)

	homeFiles := createLegacyAppFilesWithContents(t, paths.legacyHomeDir, "my-app", "Home Desktop", "home-binary", "home-icon")
	xdgFiles := createLegacyAppFilesWithContents(t, paths.oldXDGDataDir, "my-app", "XDG Desktop", "xdg-binary", "xdg-icon")

	currentAppDir := filepath.Join(config.AimDir, "my-app")
	currentDesktopPath := filepath.Join(currentAppDir, "my-app.desktop")
	currentIconPath := filepath.Join(config.IconThemeDir, "256x256", "apps", "my-app.png")
	if err := SaveDB(config.DbSrc, &DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {
				ID:               "my-app",
				Name:             "Current",
				Version:          "9.9.9",
				ExecPath:         filepath.Join(currentAppDir, "my-app.AppImage"),
				DesktopEntryPath: currentDesktopPath,
				IconPath:         currentIconPath,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := SaveDB(filepath.Join(paths.legacyHomeDir, "apps.json"), &DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {ID: "my-app", ExecPath: homeFiles.Exec, DesktopEntryPath: homeFiles.Desktop, IconPath: homeFiles.Icon},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := SaveDB(filepath.Join(paths.oldXDGStateDir, "apps.json"), &DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {ID: "my-app", ExecPath: xdgFiles.Exec, DesktopEntryPath: xdgFiles.Desktop, IconPath: xdgFiles.Icon},
		},
	}); err != nil {
		t.Fatal(err)
	}

	setFileModTime(t, filepath.Join(paths.legacyHomeDir, "apps.json"), time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC))
	setFileModTime(t, filepath.Join(paths.oldXDGStateDir, "apps.json"), time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC))

	if err := MigrateToCurrentPaths(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	if got := string(readTestFile(t, filepath.Join(currentAppDir, "my-app.AppImage"))); got != "xdg-binary" {
		t.Fatalf("missing app file should be restored from canonical source, got %q", got)
	}
	if got := string(readTestFile(t, currentIconPath)); got != "xdg-icon" {
		t.Fatalf("missing icon should be restored from canonical source, got %q", got)
	}
	desktopText := string(readTestFile(t, currentDesktopPath))
	if !strings.Contains(desktopText, "Name=XDG Desktop") {
		t.Fatalf("missing desktop file should be restored from canonical source: %s", desktopText)
	}
}

func TestMigrateToCurrentPathsDoesNotImportLegacyAppsIntoExistingAimDB(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	paths := configureMigrationTestPaths(t, homeDir)
	mustEnsureCurrentDirs(t)
	mustMkdirAll(t, paths.oldXDGStateDir)

	currentFiles := createLegacyAppFilesWithContents(t, paths.oldXDGDataDir, "app-a", "App A", "app-a", "icon-a")
	legacyBFiles := createLegacyAppFilesWithContents(t, paths.oldXDGDataDir, "app-b", "App B", "app-b", "icon-b")

	if err := SaveDB(config.DbSrc, &DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"app-a": {
				ID:               "app-a",
				Name:             "Current A",
				ExecPath:         currentFiles.Exec,
				DesktopEntryPath: currentFiles.Desktop,
				IconPath:         currentFiles.Icon,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := SaveDB(filepath.Join(paths.oldXDGStateDir, "apps.json"), &DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"app-a": {
				ID:               "app-a",
				Name:             "Legacy A",
				ExecPath:         currentFiles.Exec,
				DesktopEntryPath: currentFiles.Desktop,
				IconPath:         currentFiles.Icon,
			},
			"app-b": {
				ID:               "app-b",
				Name:             "Legacy B",
				ExecPath:         legacyBFiles.Exec,
				DesktopEntryPath: legacyBFiles.Desktop,
				IconPath:         legacyBFiles.Icon,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if err := MigrateToCurrentPaths(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	db := loadCurrentDBForTest(t)
	if len(db.Apps) != 1 {
		t.Fatalf("app count = %d, want 1", len(db.Apps))
	}
	if _, ok := db.Apps["app-b"]; ok {
		t.Fatal("legacy-only app should not be imported into an existing current DB")
	}
}

func TestMigrateToCurrentPathsIsIdempotentAndLeavesNoMarker(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	paths := configureMigrationTestPaths(t, homeDir)
	mustEnsureCurrentDirs(t)
	mustMkdirAll(t, paths.oldXDGStateDir)

	oldFiles := createLegacyAppFilesWithContents(t, paths.oldXDGDataDir, "my-app", "My App", "xdg-exec", "xdg-icon")
	if err := SaveDB(filepath.Join(paths.oldXDGStateDir, "apps.json"), &DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {
				ID:               "my-app",
				ExecPath:         oldFiles.Exec,
				DesktopEntryPath: oldFiles.Desktop,
				IconPath:         oldFiles.Icon,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if err := MigrateToCurrentPaths(); err != nil {
		t.Fatalf("first migration failed: %v", err)
	}
	firstDB := readTestFile(t, config.DbSrc)

	if err := MigrateToCurrentPaths(); err != nil {
		t.Fatalf("second migration failed: %v", err)
	}
	secondDB := readTestFile(t, config.DbSrc)

	if string(firstDB) != string(secondDB) {
		t.Fatalf("database changed across idempotent rerun:\nfirst=%s\nsecond=%s", firstDB, secondDB)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(config.DbSrc), ".migration-repair-state")); !os.IsNotExist(err) {
		t.Fatalf("unexpected migration marker file, stat err = %v", err)
	}
}

func configureMigrationTestPaths(t *testing.T, homeDir string) migrationTestPaths {
	t.Helper()

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

	paths := migrationTestPaths{
		currentDataHome:   filepath.Join(homeDir, "xdg-data"),
		currentConfigHome: filepath.Join(homeDir, "xdg-config"),
		currentStateHome:  filepath.Join(homeDir, "xdg-state"),
		currentCacheHome:  filepath.Join(homeDir, "xdg-cache"),
		legacyHomeDir:     filepath.Join(homeDir, ".appimage-manager"),
	}

	paths.currentAimDir = filepath.Join(paths.currentDataHome, "aim")
	paths.currentDesktopDir = filepath.Join(paths.currentDataHome, "applications")
	paths.currentConfigDir = filepath.Join(paths.currentConfigHome, "aim")
	paths.currentTempDir = filepath.Join(paths.currentCacheHome, "aim", "tmp")
	paths.currentDBPath = filepath.Join(paths.currentStateHome, "aim", "apps.json")
	paths.oldXDGDataDir = filepath.Join(paths.currentDataHome, "appimage-manager")
	paths.oldXDGConfigDir = filepath.Join(paths.currentConfigHome, "appimage-manager")
	paths.oldXDGStateDir = filepath.Join(paths.currentStateHome, "appimage-manager")
	paths.oldXDGCacheDir = filepath.Join(paths.currentCacheHome, "appimage-manager", "tmp")

	config.AimDir = paths.currentAimDir
	config.DesktopDir = paths.currentDesktopDir
	config.ConfigDir = paths.currentConfigDir
	config.TempDir = paths.currentTempDir
	config.DbSrc = paths.currentDBPath
	config.IconThemeDir = filepath.Join(paths.currentDataHome, "icons", "hicolor")
	config.PixmapsDir = filepath.Join(paths.currentDataHome, "pixmaps")

	return paths
}

func mustEnsureCurrentDirs(t *testing.T) {
	t.Helper()
	if err := config.EnsureDirsExist(); err != nil {
		t.Fatal(err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func createLegacyAppFilesWithContents(t *testing.T, rootDir, appID, displayName, execContent, iconContent string) testAppFiles {
	t.Helper()

	appDir := filepath.Join(rootDir, appID)
	files := testAppFiles{
		Exec:    filepath.Join(appDir, appID+".AppImage"),
		Desktop: filepath.Join(appDir, appID+".desktop"),
		Icon:    filepath.Join(appDir, appID+".png"),
	}

	writeTestFile(t, files.Exec, []byte(execContent), 0o755)
	writeTestFile(t, files.Desktop, []byte("[Desktop Entry]\nName="+displayName+"\nExec=/old/AppRun %U\nIcon=/old/icon.png\n"), 0o644)
	writeTestFile(t, files.Icon, []byte(iconContent), 0o644)

	return files
}

func loadCurrentDBForTest(t *testing.T) *DB {
	t.Helper()
	db, err := LoadDB(config.DbSrc)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func setFileModTime(t *testing.T, path string, modTime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
}

func writeTestFile(t *testing.T, path string, contents []byte, perm os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, perm); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertPathExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected path to exist %q: %v", path, err)
	}
}
