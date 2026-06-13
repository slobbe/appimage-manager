package migration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateV1MovesAppImagesUpdatesDesktopEntriesAndWritesV2Database(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	stateDir := filepath.Join(root, "state", "aim")
	dataDir := filepath.Join(root, "share", "aim")
	appImageDir := filepath.Join(dataDir, "appimages")
	desktopDir := filepath.Join(root, "share", "applications")
	oldAppDir := filepath.Join(dataDir, "example")
	oldAppImagePath := filepath.Join(oldAppDir, "example.AppImage")
	desktopEntryPath := filepath.Join(desktopDir, "example.desktop")
	iconPath := filepath.Join(root, "share", "icons", "hicolor", "256x256", "apps", "example.png")

	mkdirAll(t, stateDir)
	mkdirAll(t, oldAppDir)
	mkdirAll(t, desktopDir)
	mkdirAll(t, filepath.Dir(iconPath))
	writeFile(t, oldAppImagePath, "appimage")
	writeFile(t, iconPath, "icon")
	writeFile(t, desktopEntryPath, "[Desktop Entry]\nName=Example\nExec="+oldAppImagePath+"\nIcon="+iconPath+"\n")

	sourcePath := filepath.Join(stateDir, "apps.json")
	destPath := filepath.Join(dataDir, "apps.json")
	writeFile(t, sourcePath, `{
  "schemaVersion": 1,
  "apps": {
    "example": {
      "name": "Example",
      "id": "example",
      "version": "1.2.3",
      "exec_path": "`+oldAppImagePath+`",
      "icon_path": "`+iconPath+`",
      "desktop_entry_path": "`+filepath.Join(oldAppDir, "example.desktop")+`",
      "desktop_entry_link": "`+desktopEntryPath+`",
      "source": {
        "kind": "github_release",
        "github_release": {
          "repo": "owner/example",
          "asset": "*.AppImage",
          "tag": "v1.2.3",
          "asset_name": "Example-1.2.3.AppImage",
          "downloaded_at": "2026-06-03T14:06:07Z"
        }
      },
      "update": {
        "kind": "github_release",
        "github_release": {
          "repo": "owner/example",
          "asset": "*.AppImage"
        }
      }
    }
  }
}`)

	migrated, err := MigrateV1(context.Background(), V1Options{
		SourcePath:  sourcePath,
		DestPath:    destPath,
		AppImageDir: appImageDir,
		DesktopDir:  desktopDir,
	})
	if err != nil {
		t.Fatalf("MigrateV1() error = %v", err)
	}
	if !migrated {
		t.Fatal("MigrateV1() migrated = false, want true")
	}

	newAppImagePath := filepath.Join(appImageDir, "example.AppImage")
	if _, err := os.Stat(oldAppImagePath); !os.IsNotExist(err) {
		t.Fatalf("old AppImage stat error = %v, want not exist", err)
	}
	if got := readFile(t, newAppImagePath); got != "appimage" {
		t.Fatalf("new AppImage content = %q, want appimage", got)
	}

	desktopEntry := readFile(t, desktopEntryPath)
	if !strings.Contains(desktopEntry, "Exec="+newAppImagePath) {
		t.Fatalf("desktop entry = %q, want updated Exec", desktopEntry)
	}
	if !strings.Contains(desktopEntry, "Icon=example") {
		t.Fatalf("desktop entry = %q, want updated Icon", desktopEntry)
	}

	var db databaseFile
	if err := json.Unmarshal([]byte(readFile(t, destPath)), &db); err != nil {
		t.Fatalf("unmarshal v2 database: %v", err)
	}
	if db.SchemaVersion != 2 {
		t.Fatalf("SchemaVersion = %d, want 2", db.SchemaVersion)
	}
	if len(db.Apps) != 1 {
		t.Fatalf("Apps length = %d, want 1", len(db.Apps))
	}
	app := db.Apps[0]
	if app.AppImagePath != newAppImagePath {
		t.Fatalf("AppImagePath = %q, want %q", app.AppImagePath, newAppImagePath)
	}
	if app.DesktopEntryPath != desktopEntryPath {
		t.Fatalf("DesktopEntryPath = %q, want %q", app.DesktopEntryPath, desktopEntryPath)
	}
	if app.IconPath != iconPath {
		t.Fatalf("IconPath = %q, want %q", app.IconPath, iconPath)
	}
	if app.Source == nil || app.Source.Kind != "github" || app.Source.GitHubRelease.Asset != "Example-1.2.3.AppImage" {
		t.Fatalf("Source = %#v, want migrated GitHub source", app.Source)
	}
	if app.UpdateSource == nil || app.UpdateSource.Kind != "github" || app.UpdateSource.Repo != "owner/example" || app.UpdateSource.AssetPattern != "*.AppImage" {
		t.Fatalf("UpdateSource = %#v, want migrated GitHub update source", app.UpdateSource)
	}
}

func TestMigrateV1SkipsWhenDestinationExists(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourcePath := filepath.Join(root, "state", "aim", "apps.json")
	destPath := filepath.Join(root, "share", "aim", "apps.json")
	mkdirAll(t, filepath.Dir(sourcePath))
	mkdirAll(t, filepath.Dir(destPath))
	writeFile(t, sourcePath, `{"schemaVersion":1,"apps":{}}`)
	writeFile(t, destPath, `{"schema_version":2,"apps":[]}`)

	migrated, err := MigrateV1(context.Background(), V1Options{
		SourcePath:  sourcePath,
		DestPath:    destPath,
		AppImageDir: filepath.Join(root, "share", "aim", "appimages"),
		DesktopDir:  filepath.Join(root, "share", "applications"),
	})
	if err != nil {
		t.Fatalf("MigrateV1() error = %v", err)
	}
	if migrated {
		t.Fatal("MigrateV1() migrated = true, want false")
	}
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(bytes)
}
