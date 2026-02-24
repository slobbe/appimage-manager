package repo

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/slobbe/appimage-manager/internal/config"
	models "github.com/slobbe/appimage-manager/internal/types"
)

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
