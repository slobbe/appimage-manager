package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/domain"
)

func TestRepositorySaveAndFind(t *testing.T) {
	t.Parallel()

	repo := NewRepository(filepath.Join(t.TempDir(), "aim", "apps.json"))
	stored := testApp(t, "example", "Example", "1.2.3")

	if err := repo.Save(context.Background(), stored); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	found, err := repo.Find(context.Background(), "example")
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	assertApp(t, found, stored)
}

func TestRepositorySaveWritesCurrentSchemaVersion(t *testing.T) {
	t.Parallel()

	repo := NewRepository(filepath.Join(t.TempDir(), "apps.json"))
	if err := repo.Save(context.Background(), testApp(t, "example", "Example", "1.2.3")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	bytes, err := os.ReadFile(repo.Path)
	if err != nil {
		t.Fatalf("read database: %v", err)
	}

	var db databaseFile
	if err := json.Unmarshal(bytes, &db); err != nil {
		t.Fatalf("unmarshal database: %v", err)
	}
	if db.SchemaVersion != 2 {
		t.Fatalf("SchemaVersion = %d, want 2", db.SchemaVersion)
	}
}

func TestRepositorySaveAndFindEmbeddedUpdateSource(t *testing.T) {
	t.Parallel()

	repo := NewRepository(filepath.Join(t.TempDir(), "apps.json"))
	stored := testApp(t, "example", "Example", "1.2.3")
	stored.UpdateSource = domain.NewEmbeddedUpdateSource("gh-releases-zsync|owner|repo|latest|Example-*.AppImage.zsync")

	if err := repo.Save(context.Background(), stored); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	found, err := repo.Find(context.Background(), "example")
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	assertApp(t, found, stored)

	bytes, err := os.ReadFile(repo.Path)
	if err != nil {
		t.Fatalf("read database: %v", err)
	}
	for _, want := range []string{
		`"embedded": true`,
		`"kind": "github"`,
		`"raw": "gh-releases-zsync|owner|repo|latest|Example-*.AppImage.zsync"`,
		`"transport": "gh-releases-zsync"`,
		`"release_tag": "latest"`,
		`"asset_pattern": "Example-*.AppImage"`,
		`"zsync_asset_pattern": "Example-*.AppImage.zsync"`,
	} {
		if !strings.Contains(string(bytes), want) {
			t.Fatalf("database = %s, want field %s", bytes, want)
		}
	}
}

func TestRepositorySaveOmitsEmptyUpdateSource(t *testing.T) {
	t.Parallel()

	repo := NewRepository(filepath.Join(t.TempDir(), "apps.json"))
	stored := testApp(t, "example", "Example", "1.2.3")
	stored.UpdateSource = domain.UpdateSource{}

	if err := repo.Save(context.Background(), stored); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	bytes, err := os.ReadFile(repo.Path)
	if err != nil {
		t.Fatalf("read database: %v", err)
	}
	if json.Valid(bytes) && string(bytes) == "" {
		t.Fatal("database is empty")
	}
	if containsJSONField(bytes, "update_source") {
		t.Fatalf("database contains update_source, want omitted: %s", bytes)
	}
}

func TestRepositorySaveReplacesExistingApp(t *testing.T) {
	t.Parallel()

	repo := NewRepository(filepath.Join(t.TempDir(), "apps.json"))
	original := testApp(t, "example", "Example", "1.0.0")
	updated := testApp(t, "example", "Example Updated", "2.0.0")

	if err := repo.Save(context.Background(), original); err != nil {
		t.Fatalf("Save(original) error = %v", err)
	}
	if err := repo.Save(context.Background(), updated); err != nil {
		t.Fatalf("Save(updated) error = %v", err)
	}

	apps, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("List() length = %d, want 1", len(apps))
	}
	assertApp(t, apps[0], updated)
}

func TestRepositoryConcurrentSavesPreserveAllApps(t *testing.T) {
	t.Parallel()

	for iteration := 0; iteration < 100; iteration++ {
		repo := NewRepository(filepath.Join(t.TempDir(), "apps.json"))
		apps := []domain.App{
			testApp(t, "alpha", "Alpha", "1.0.0"),
			testApp(t, "bravo", "Bravo", "1.0.0"),
		}

		var wg sync.WaitGroup
		start := make(chan struct{})
		errors := make(chan error, len(apps))
		for _, stored := range apps {
			stored := stored
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				errors <- repo.Save(context.Background(), stored)
			}()
		}

		close(start)
		wg.Wait()
		close(errors)
		for err := range errors {
			if err != nil {
				t.Fatalf("iteration %d: Save() error = %v", iteration, err)
			}
		}

		stored, err := repo.List(context.Background())
		if err != nil {
			t.Fatalf("iteration %d: List() error = %v", iteration, err)
		}
		got := map[string]bool{}
		for _, app := range stored {
			got[app.ID] = true
		}
		for _, app := range apps {
			if !got[app.ID] {
				t.Fatalf("iteration %d: stored app IDs = %v, want %s present", iteration, got, app.ID)
			}
		}
	}
}

func TestRepositoryListReturnsAppsSortedByID(t *testing.T) {
	t.Parallel()

	repo := NewRepository(filepath.Join(t.TempDir(), "apps.json"))
	for _, app := range []domain.App{
		testApp(t, "zulu", "Zulu", "1.0.0"),
		testApp(t, "alpha", "Alpha", "1.0.0"),
		testApp(t, "middle", "Middle", "1.0.0"),
	} {
		if err := repo.Save(context.Background(), app); err != nil {
			t.Fatalf("Save(%s) error = %v", app.ID, err)
		}
	}

	apps, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	got := []string{apps[0].ID, apps[1].ID, apps[2].ID}
	want := []string{"alpha", "middle", "zulu"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List IDs = %v, want %v", got, want)
		}
	}
}

func TestRepositoryDelete(t *testing.T) {
	t.Parallel()

	repo := NewRepository(filepath.Join(t.TempDir(), "apps.json"))
	if err := repo.Save(context.Background(), testApp(t, "example", "Example", "1.0.0")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if err := repo.Delete(context.Background(), "example"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err := repo.Find(context.Background(), "example")
	if !errors.Is(err, app.ErrAppNotFound) {
		t.Fatalf("Find() error = %v, want ErrAppNotFound", err)
	}
}

func TestRepositoryMissingFileBehavesAsEmptyDatabase(t *testing.T) {
	t.Parallel()

	repo := NewRepository(filepath.Join(t.TempDir(), "missing.json"))

	apps, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(apps) != 0 {
		t.Fatalf("List() length = %d, want 0", len(apps))
	}

	_, err = repo.Find(context.Background(), "missing")
	if !errors.Is(err, app.ErrAppNotFound) {
		t.Fatalf("Find() error = %v, want ErrAppNotFound", err)
	}
}

func TestRepositoryDeleteMissingAppReturnsNotFound(t *testing.T) {
	t.Parallel()

	repo := NewRepository(filepath.Join(t.TempDir(), "apps.json"))

	err := repo.Delete(context.Background(), "missing")
	if !errors.Is(err, app.ErrAppNotFound) {
		t.Fatalf("Delete() error = %v, want ErrAppNotFound", err)
	}
}

func TestRepositoryRejectsMalformedDatabase(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "apps.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatalf("write database: %v", err)
	}

	_, err := NewRepository(path).List(context.Background())
	if err == nil {
		t.Fatal("List() error = nil, want error")
	}
}

func TestRepositoryReadsLegacyFlatSource(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "apps.json")
	if err := os.WriteFile(path, []byte(`{
		"apps": [
			{
				"id": "example",
				"name": "Example",
				"version": "1.2.3",
				"app_image_path": "/apps/example.AppImage",
				"source": "github",
				"update_source": "owner/repo"
			}
		]
	}`), 0o644); err != nil {
		t.Fatalf("write database: %v", err)
	}

	stored, err := NewRepository(path).Find(context.Background(), "example")
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if got, want := stored.Source.Kind, domain.SourceKindGitHub; got != want {
		t.Fatalf("Source.Kind = %q, want %q", got, want)
	}
	if got, want := stored.UpdateSource.Kind, domain.UpdateSourceKindGitHub; got != want {
		t.Fatalf("UpdateSource.Kind = %q, want %q", got, want)
	}
	if got, want := stored.UpdateSource.Repo, "owner/repo"; got != want {
		t.Fatalf("UpdateSource.Repo = %q, want %q", got, want)
	}
}

func TestRepositoryRejectsInvalidStoredVersion(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "apps.json")
	writeRawDatabase(t, path, databaseFile{
		Apps: []appRecord{{
			ID:      "example",
			Name:    "Example",
			Version: "unknown",
		}},
	})

	_, err := NewRepository(path).Find(context.Background(), "example")
	if err == nil {
		t.Fatal("Find() error = nil, want error")
	}
}

func TestRepositoryValidatesInputs(t *testing.T) {
	t.Parallel()

	repo := NewRepository("")
	if err := repo.Save(context.Background(), testApp(t, "example", "Example", "1.0.0")); err == nil {
		t.Fatal("Save() error = nil, want missing storage path error")
	}

	repo = NewRepository(filepath.Join(t.TempDir(), "apps.json"))
	if err := repo.Save(context.Background(), domain.App{}); err == nil {
		t.Fatal("Save() error = nil, want missing app id error")
	}
	if _, err := repo.Find(context.Background(), ""); err == nil {
		t.Fatal("Find() error = nil, want missing app id error")
	}
	if err := repo.Delete(context.Background(), ""); err == nil {
		t.Fatal("Delete() error = nil, want missing app id error")
	}
}

func TestRepositoryRespectsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := NewRepository(filepath.Join(t.TempDir(), "apps.json"))
	if err := repo.Save(ctx, testApp(t, "example", "Example", "1.0.0")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save() error = %v, want context.Canceled", err)
	}
	if _, err := repo.Find(ctx, "example"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Find() error = %v, want context.Canceled", err)
	}
	if _, err := repo.List(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("List() error = %v, want context.Canceled", err)
	}
	if err := repo.Delete(ctx, "example"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Delete() error = %v, want context.Canceled", err)
	}
}

func testApp(t *testing.T, id string, name string, versionString string) domain.App {
	t.Helper()

	version, ok := domain.ParseVersion(versionString)
	if !ok {
		t.Fatalf("ParseVersion(%q) ok = false, want true", versionString)
	}

	return domain.App{
		ID:               id,
		Name:             name,
		Version:          version,
		AppImagePath:     "/apps/" + id + ".AppImage",
		DesktopEntryPath: "/desktop/" + id + ".desktop",
		IconPath:         "/icons/" + id + ".png",
		Source:           domain.NewGitHubReleaseSource("owner/"+id, "v"+versionString, id+".AppImage", "https://example.test/"+id+".AppImage", 123, testSourceTime()),
		UpdateSource:     domain.NewGitHubUpdateSource("owner/"+id, true),
	}
}

func testSourceTime() time.Time {
	return time.Date(2026, 6, 3, 14, 6, 7, 0, time.UTC)
}

func assertApp(t *testing.T, got domain.App, want domain.App) {
	t.Helper()

	if got.ID != want.ID ||
		got.Name != want.Name ||
		got.Version.String() != want.Version.String() ||
		got.AppImagePath != want.AppImagePath ||
		got.DesktopEntryPath != want.DesktopEntryPath ||
		got.IconPath != want.IconPath ||
		got.Source != want.Source ||
		got.UpdateSource != want.UpdateSource {
		t.Fatalf("app = %#v, want %#v", got, want)
	}
}

func containsJSONField(bytes []byte, field string) bool {
	var decoded map[string]any
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		return false
	}
	apps, _ := decoded["apps"].([]any)
	for _, item := range apps {
		appRecord, _ := item.(map[string]any)
		if _, ok := appRecord[field]; ok {
			return true
		}
	}
	return false
}

func writeRawDatabase(t *testing.T, path string, db databaseFile) {
	t.Helper()

	bytes, err := json.Marshal(db)
	if err != nil {
		t.Fatalf("marshal database: %v", err)
	}
	if err := os.WriteFile(path, bytes, 0o644); err != nil {
		t.Fatalf("write database: %v", err)
	}
}
