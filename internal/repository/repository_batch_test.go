package repo

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/slobbe/appimage-manager/internal/config"
	models "github.com/slobbe/appimage-manager/internal/types"
)

func TestSaveDBCreatesParentDirectory(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "nested", "state", "aim", "apps.json")

	if err := SaveDB(dbPath, &DB{SchemaVersion: 1, Apps: map[string]*models.App{}}); err != nil {
		t.Fatalf("SaveDB returned error: %v", err)
	}

	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected database file to exist: %v", err)
	}

	db, err := LoadDB(dbPath)
	if err != nil {
		t.Fatalf("LoadDB returned error: %v", err)
	}
	if db.SchemaVersion != 1 {
		t.Fatalf("db.SchemaVersion = %d, want 1", db.SchemaVersion)
	}
	if len(db.Apps) != 0 {
		t.Fatalf("len(db.Apps) = %d, want 0", len(db.Apps))
	}
}

func TestSaveDBPreservesExistingPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not portable on Windows")
	}

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "apps.json")

	if err := os.WriteFile(dbPath, []byte(`{"schemaVersion":1,"apps":{}}`), 0o600); err != nil {
		t.Fatalf("failed to seed db file: %v", err)
	}

	if err := SaveDB(dbPath, &DB{SchemaVersion: 1, Apps: map[string]*models.App{}}); err != nil {
		t.Fatalf("SaveDB returned error: %v", err)
	}

	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("failed to stat db file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("db mode = %o, want 600", got)
	}
}

func TestSaveDBUsesUniqueTempFilesAndCleansUp(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "apps.json")

	if err := SaveDB(dbPath, &DB{SchemaVersion: 1, Apps: map[string]*models.App{}}); err != nil {
		t.Fatalf("SaveDB returned error: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(tmp, ".apps.json.*.tmp"))
	if err != nil {
		t.Fatalf("failed to glob temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no temp files after successful save, found %v", matches)
	}
}

func TestUpdateCheckMetadataBatch(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "apps.json")

	originalDbSrc := config.DbSrc
	config.DbSrc = dbPath
	t.Cleanup(func() {
		config.DbSrc = originalDbSrc
	})

	if err := SaveDB(dbPath, &DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"app-a": {
				ID:              "app-a",
				UpdateAvailable: false,
				LatestVersion:   "",
			},
			"app-b": {
				ID:              "app-b",
				UpdateAvailable: true,
				LatestVersion:   "v1.0.0",
			},
		},
	}); err != nil {
		t.Fatalf("failed to seed DB: %v", err)
	}

	updates := []CheckMetadataUpdate{
		{
			ID:            "app-a",
			Checked:       true,
			Available:     true,
			Latest:        "  v2.0.0  ",
			LastCheckedAt: "2026-02-24T11:00:00Z",
		},
		{
			ID:            "app-b",
			Checked:       false,
			Available:     false,
			Latest:        "ignored",
			LastCheckedAt: "2026-02-24T11:01:00Z",
		},
	}

	if err := UpdateCheckMetadataBatch(updates); err != nil {
		t.Fatalf("UpdateCheckMetadataBatch returned error: %v", err)
	}

	appA, err := GetApp("app-a")
	if err != nil {
		t.Fatalf("failed to load app-a: %v", err)
	}
	if !appA.UpdateAvailable {
		t.Fatal("expected app-a update_available to be true")
	}
	if appA.LatestVersion != "v2.0.0" {
		t.Fatalf("app-a latest_version = %q, want %q", appA.LatestVersion, "v2.0.0")
	}
	if appA.LastCheckedAt != "2026-02-24T11:00:00Z" {
		t.Fatalf("app-a last_checked_at = %q, want %q", appA.LastCheckedAt, "2026-02-24T11:00:00Z")
	}

	appB, err := GetApp("app-b")
	if err != nil {
		t.Fatalf("failed to load app-b: %v", err)
	}
	if !appB.UpdateAvailable {
		t.Fatal("expected app-b update_available to remain true")
	}
	if appB.LatestVersion != "v1.0.0" {
		t.Fatalf("app-b latest_version = %q, want %q", appB.LatestVersion, "v1.0.0")
	}
	if appB.LastCheckedAt != "2026-02-24T11:01:00Z" {
		t.Fatalf("app-b last_checked_at = %q, want %q", appB.LastCheckedAt, "2026-02-24T11:01:00Z")
	}
}

func TestUpdateCheckMetadataBatchMissingApp(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "apps.json")

	originalDbSrc := config.DbSrc
	config.DbSrc = dbPath
	t.Cleanup(func() {
		config.DbSrc = originalDbSrc
	})

	if err := SaveDB(dbPath, &DB{SchemaVersion: 1, Apps: map[string]*models.App{}}); err != nil {
		t.Fatalf("failed to seed DB: %v", err)
	}

	err := UpdateCheckMetadataBatch([]CheckMetadataUpdate{{
		ID:            "missing",
		Checked:       true,
		Available:     true,
		Latest:        "v1.0.0",
		LastCheckedAt: "2026-02-24T11:00:00Z",
	}})
	if err == nil {
		t.Fatal("expected error for missing app")
	}
	if !strings.Contains(err.Error(), "does not exists in database") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddAppsBatch(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "apps.json")

	originalDbSrc := config.DbSrc
	config.DbSrc = dbPath
	t.Cleanup(func() {
		config.DbSrc = originalDbSrc
	})

	if err := SaveDB(dbPath, &DB{SchemaVersion: 1, Apps: map[string]*models.App{}}); err != nil {
		t.Fatalf("failed to seed DB: %v", err)
	}

	apps := []*models.App{{ID: "app-a", Name: "A"}, {ID: "app-b", Name: "B"}}
	if err := AddAppsBatch(apps, true); err != nil {
		t.Fatalf("AddAppsBatch returned error: %v", err)
	}

	db, err := LoadDB(dbPath)
	if err != nil {
		t.Fatalf("failed to load db: %v", err)
	}
	if len(db.Apps) != 2 {
		t.Fatalf("len(db.Apps) = %d, want 2", len(db.Apps))
	}
	if db.Apps["app-a"] == nil || db.Apps["app-a"].Name != "A" {
		t.Fatalf("expected app-a to be stored")
	}
	if db.Apps["app-b"] == nil || db.Apps["app-b"].Name != "B" {
		t.Fatalf("expected app-b to be stored")
	}
}

func TestAddAppsBatchOverwriteBehavior(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "apps.json")

	originalDbSrc := config.DbSrc
	config.DbSrc = dbPath
	t.Cleanup(func() {
		config.DbSrc = originalDbSrc
	})

	if err := SaveDB(dbPath, &DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"app-a": {ID: "app-a", Name: "Old"},
		},
	}); err != nil {
		t.Fatalf("failed to seed DB: %v", err)
	}

	err := AddAppsBatch([]*models.App{{ID: "app-a", Name: "New"}}, false)
	if err == nil {
		t.Fatal("expected overwrite protection error")
	}

	if err := AddAppsBatch([]*models.App{{ID: "app-a", Name: "New"}}, true); err != nil {
		t.Fatalf("AddAppsBatch with overwrite returned error: %v", err)
	}

	app, err := GetApp("app-a")
	if err != nil {
		t.Fatalf("failed to load app-a: %v", err)
	}
	if app.Name != "New" {
		t.Fatalf("app.Name = %q, want %q", app.Name, "New")
	}
}

func TestLoadDBRejectsUnsupportedUpdateKind(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "apps.json")

	raw := `{
  "schemaVersion": 1,
  "apps": {
    "manifest-app": {
      "id": "manifest-app",
      "name": "Manifest App",
      "update": {
        "kind": "manifest"
      }
    }
  }
}`
	if err := os.WriteFile(dbPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("failed to write legacy DB: %v", err)
	}

	_, err := LoadDB(dbPath)
	if err == nil {
		t.Fatal("expected unsupported update kind error")
	}
	if !strings.Contains(err.Error(), `unsupported update kind for manifest-app: "manifest"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadDBRejectsMissingSchemaVersion(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "apps.json")

	raw := `{
  "apps": {}
}`
	if err := os.WriteFile(dbPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("failed to write db: %v", err)
	}

	_, err := LoadDB(dbPath)
	if err == nil {
		t.Fatal("expected schema version error")
	}
	if !strings.Contains(err.Error(), `unsupported schema version: 0`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadDBRejectsZeroSchemaVersion(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "apps.json")

	raw := `{
  "schemaVersion": 0,
  "apps": {}
}`
	if err := os.WriteFile(dbPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("failed to write db: %v", err)
	}

	_, err := LoadDB(dbPath)
	if err == nil {
		t.Fatal("expected schema version error")
	}
	if !strings.Contains(err.Error(), `unsupported schema version: 0`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadDBRejectsUnsupportedSchemaVersion(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "apps.json")

	raw := `{
  "schemaVersion": 2,
  "apps": {}
}`
	if err := os.WriteFile(dbPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("failed to write db: %v", err)
	}

	_, err := LoadDB(dbPath)
	if err == nil {
		t.Fatal("expected schema version error")
	}
	if !strings.Contains(err.Error(), `unsupported schema version: 2`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadDBRejectsMissingAppsCollection(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "apps.json")

	raw := `{
  "schemaVersion": 1
}`
	if err := os.WriteFile(dbPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("failed to write db: %v", err)
	}

	_, err := LoadDB(dbPath)
	if err == nil {
		t.Fatal("expected apps collection error")
	}
	if !strings.Contains(err.Error(), `apps collection cannot be empty`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadDBRejectsNullAppsCollection(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "apps.json")

	raw := `{
  "schemaVersion": 1,
  "apps": null
}`
	if err := os.WriteFile(dbPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("failed to write db: %v", err)
	}

	_, err := LoadDB(dbPath)
	if err == nil {
		t.Fatal("expected apps collection error")
	}
	if !strings.Contains(err.Error(), `apps collection cannot be empty`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadDBRejectsUnsupportedDirectURLUpdateKind(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "apps.json")

	raw := `{
  "schemaVersion": 1,
  "apps": {
    "direct-app": {
      "id": "direct-app",
      "name": "Direct App",
      "update": {
        "kind": "direct_url"
      }
    }
  }
}`
	if err := os.WriteFile(dbPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("failed to write legacy DB: %v", err)
	}

	_, err := LoadDB(dbPath)
	if err == nil {
		t.Fatal("expected unsupported update kind error")
	}
	if !strings.Contains(err.Error(), `unsupported update kind for direct-app: "direct_url"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadDBRejectsUnsupportedSourceKind(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "apps.json")

	raw := `{
  "schemaVersion": 1,
  "apps": {
    "legacy-app": {
      "id": "legacy-app",
      "name": "Legacy App",
      "source": {
        "kind": "manifest"
      }
    }
  }
}`
	if err := os.WriteFile(dbPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("failed to write legacy DB: %v", err)
	}

	_, err := LoadDB(dbPath)
	if err == nil {
		t.Fatal("expected unsupported source kind error")
	}
	if !strings.Contains(err.Error(), `unsupported source kind for legacy-app: "manifest"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}
