package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slobbe/appimage-manager/internal/config"
	"github.com/slobbe/appimage-manager/internal/core"
	util "github.com/slobbe/appimage-manager/internal/helpers"
	repo "github.com/slobbe/appimage-manager/internal/repository"
	models "github.com/slobbe/appimage-manager/internal/types"
	"github.com/urfave/cli/v3"
)

func TestIdentifyInputType(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "apps.json")

	originalDbSrc := config.DbSrc
	config.DbSrc = dbPath
	t.Cleanup(func() {
		config.DbSrc = originalDbSrc
	})

	db := &repo.DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"integrated": {
				ID:               "integrated",
				DesktopEntryLink: "/tmp/integrated.desktop",
			},
			"unlinked": {
				ID:               "unlinked",
				DesktopEntryLink: "",
			},
		},
	}

	if err := repo.SaveDB(dbPath, db); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "local appimage path", input: "/tmp/MyApp.AppImage", expect: InputTypeAppImage},
		{name: "integrated id", input: "integrated", expect: InputTypeIntegrated},
		{name: "unlinked id", input: "unlinked", expect: InputTypeUnlinked},
		{name: "unknown id", input: "missing", expect: InputTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := identifyInputType(tt.input)
			if got != tt.expect {
				t.Fatalf("identifyInputType(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestUpdateDownloadFilename(t *testing.T) {
	tests := []struct {
		name      string
		assetName string
		url       string
		expect    string
	}{
		{name: "uses AppImage asset name", assetName: "MyApp-x86_64.AppImage", url: "https://example.com/file", expect: "MyApp-x86_64.AppImage"},
		{name: "adds extension when missing", assetName: "MyApp", url: "https://example.com/file", expect: "MyApp.AppImage"},
		{name: "falls back to URL basename", assetName: "", url: "https://example.com/download/MyApp.AppImage", expect: "MyApp.AppImage"},
		{name: "falls back to default filename", assetName: "", url: "", expect: "update.AppImage"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := updateDownloadFilename(tt.assetName, tt.url)
			if got != tt.expect {
				t.Fatalf("updateDownloadFilename(%q, %q) = %q, want %q", tt.assetName, tt.url, got, tt.expect)
			}
		})
	}
}

func TestSetPinnedState(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "apps.json")

	originalDbSrc := config.DbSrc
	config.DbSrc = dbPath
	t.Cleanup(func() {
		config.DbSrc = originalDbSrc
	})

	if err := repo.SaveDB(dbPath, &repo.DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {ID: "my-app", Name: "My App", Pinned: false},
		},
	}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	app, changed, err := setPinnedState("my-app", true)
	if err != nil {
		t.Fatalf("setPinnedState returned error: %v", err)
	}
	if !changed {
		t.Fatal("expected change when pinning app")
	}
	if !app.Pinned {
		t.Fatal("expected app to be pinned")
	}

	_, changed, err = setPinnedState("my-app", true)
	if err != nil {
		t.Fatalf("setPinnedState returned error on idempotent pin: %v", err)
	}
	if changed {
		t.Fatal("did not expect change when app already pinned")
	}

	app, changed, err = setPinnedState("my-app", false)
	if err != nil {
		t.Fatalf("setPinnedState returned error: %v", err)
	}
	if !changed {
		t.Fatal("expected change when unpinning app")
	}
	if app.Pinned {
		t.Fatal("expected app to be unpinned")
	}
}

func TestUpdateCheckMetadata(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "apps.json")

	originalDbSrc := config.DbSrc
	config.DbSrc = dbPath
	t.Cleanup(func() {
		config.DbSrc = originalDbSrc
	})

	if err := repo.SaveDB(dbPath, &repo.DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {ID: "my-app", Name: "My App"},
		},
	}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	app, err := repo.GetApp("my-app")
	if err != nil {
		t.Fatalf("failed to load app: %v", err)
	}

	if err := updateCheckMetadata(app, true, true, "v2.0.0"); err != nil {
		t.Fatalf("updateCheckMetadata returned error: %v", err)
	}

	updated, err := repo.GetApp("my-app")
	if err != nil {
		t.Fatalf("failed to load updated app: %v", err)
	}

	if !updated.UpdateAvailable {
		t.Fatal("expected update_available to be true")
	}
	if updated.LatestVersion != "v2.0.0" {
		t.Fatalf("latest_version = %q, want %q", updated.LatestVersion, "v2.0.0")
	}
	if updated.LastCheckedAt == "" {
		t.Fatal("expected last_checked_at to be set")
	}
}

func TestResolveUpdateSourceFromSetFlags(t *testing.T) {
	tests := []struct {
		name      string
		flags     map[string]string
		expect    models.UpdateKind
		wantError bool
	}{
		{
			name:   "github source",
			flags:  map[string]string{"github": "owner/repo", "asset": "*.AppImage"},
			expect: models.UpdateGitHubRelease,
		},
		{
			name:   "gitlab source",
			flags:  map[string]string{"gitlab": "group/project", "asset": "*.AppImage"},
			expect: models.UpdateGitLabRelease,
		},
		{
			name:      "direct url missing sha",
			flags:     map[string]string{"url": "https://example.com/MyApp.AppImage"},
			wantError: true,
		},
		{
			name:   "direct url source",
			flags:  map[string]string{"url": "https://example.com/MyApp.AppImage", "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			expect: models.UpdateDirectURL,
		},
		{
			name:      "mutually exclusive selectors",
			flags:     map[string]string{"github": "owner/repo", "gitlab": "group/project", "asset": "*.AppImage"},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newUpdateSetTestCommand(t, tt.flags)

			source, err := resolveUpdateSourceFromSetFlags(cmd)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("resolveUpdateSourceFromSetFlags returned error: %v", err)
			}
			if source == nil {
				t.Fatal("expected source")
			}
			if source.Kind != tt.expect {
				t.Fatalf("source.Kind = %q, want %q", source.Kind, tt.expect)
			}
		})
	}
}

func newUpdateSetTestCommand(t *testing.T, values map[string]string) *cli.Command {
	t.Helper()

	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "github"},
			&cli.StringFlag{Name: "gitlab"},
			&cli.StringFlag{Name: "asset"},
			&cli.StringFlag{Name: "zsync-url"},
			&cli.StringFlag{Name: "manifest-url"},
			&cli.StringFlag{Name: "url"},
			&cli.StringFlag{Name: "sha256"},
		},
	}

	for key, value := range values {
		if err := cmd.Set(key, value); err != nil {
			t.Fatalf("failed to set %s: %v", key, err)
		}
	}

	return cmd
}

func TestRunManagedUpdateSingleUpToDatePrintedOnce(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "apps.json")

	originalDbSrc := config.DbSrc
	config.DbSrc = dbPath
	t.Cleanup(func() {
		config.DbSrc = originalDbSrc
	})

	if err := repo.SaveDB(dbPath, &repo.DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {
				ID:     "my-app",
				Name:   "My App",
				SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Update: &models.UpdateSource{
					Kind: models.UpdateDirectURL,
					DirectURL: &models.DirectURLUpdateSource{
						URL:    "https://example.com/MyApp.AppImage",
						SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	cmd := &cli.Command{Flags: []cli.Flag{&cli.BoolFlag{Name: "yes"}, &cli.BoolFlag{Name: "check-only"}, &cli.BoolFlag{Name: "no-color"}}}

	output := captureStdout(t, func() {
		if err := runManagedUpdate(context.Background(), cmd, "my-app"); err != nil {
			t.Fatalf("runManagedUpdate returned error: %v", err)
		}
	})

	if strings.Count(output, "You are up-to-date!") != 1 {
		t.Fatalf("expected exactly one up-to-date message, got output:\n%s", output)
	}
}

func TestAddCmdSkipsPostCheckByDefault(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "apps.json")

	originalDbSrc := config.DbSrc
	config.DbSrc = dbPath
	t.Cleanup(func() {
		config.DbSrc = originalDbSrc
	})

	if err := repo.SaveDB(dbPath, &repo.DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {
				ID:               "my-app",
				Name:             "My App",
				Version:          "1.0.0",
				SHA1:             strings.Repeat("a", 40),
				DesktopEntryLink: "/tmp/aim-my-app.desktop",
				Update: &models.UpdateSource{
					Kind: models.UpdateZsync,
					Zsync: &models.ZsyncUpdateSource{
						UpdateInfo: "https://example.com/my-app.zsync",
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	originalZsyncCheck := runZsyncUpdateCheck
	t.Cleanup(func() {
		runZsyncUpdateCheck = originalZsyncCheck
	})

	var calls int32
	runZsyncUpdateCheck = func(*models.UpdateSource, string) (*core.UpdateData, error) {
		atomic.AddInt32(&calls, 1)
		return nil, nil
	}

	if err := runAddCommand(context.Background(), []string{"my-app", "--no-color"}); err != nil {
		t.Fatalf("runAddCommand returned error: %v", err)
	}

	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("runZsyncUpdateCheck calls = %d, want 0", calls)
	}
}

func TestAddCmdPostCheckRunsWhenEnabled(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "apps.json")

	originalDbSrc := config.DbSrc
	config.DbSrc = dbPath
	t.Cleanup(func() {
		config.DbSrc = originalDbSrc
	})

	if err := repo.SaveDB(dbPath, &repo.DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {
				ID:               "my-app",
				Name:             "My App",
				Version:          "1.0.0",
				SHA1:             strings.Repeat("a", 40),
				DesktopEntryLink: "/tmp/aim-my-app.desktop",
				Update: &models.UpdateSource{
					Kind: models.UpdateZsync,
					Zsync: &models.ZsyncUpdateSource{
						UpdateInfo: "https://example.com/my-app.zsync",
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	originalZsyncCheck := runZsyncUpdateCheck
	t.Cleanup(func() {
		runZsyncUpdateCheck = originalZsyncCheck
	})

	var calls int32
	runZsyncUpdateCheck = func(*models.UpdateSource, string) (*core.UpdateData, error) {
		atomic.AddInt32(&calls, 1)
		return &core.UpdateData{Available: false}, nil
	}

	if err := runAddCommand(context.Background(), []string{"my-app", "--no-color", "--post-check"}); err != nil {
		t.Fatalf("runAddCommand returned error: %v", err)
	}

	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("runZsyncUpdateCheck calls = %d, want 1", calls)
	}
}

func TestRunManagedChecksPreservesInputOrder(t *testing.T) {
	originalCheck := runAppUpdateCheck
	t.Cleanup(func() {
		runAppUpdateCheck = originalCheck
	})

	runAppUpdateCheck = func(app *models.App) (*pendingManagedUpdate, error) {
		switch app.ID {
		case "a":
			time.Sleep(40 * time.Millisecond)
		case "b":
			time.Sleep(10 * time.Millisecond)
		case "c":
			time.Sleep(20 * time.Millisecond)
		}

		return &pendingManagedUpdate{
			App:       app,
			Available: false,
			FromKind:  models.UpdateNone,
		}, nil
	}

	apps := []*models.App{
		{ID: "a"},
		{ID: "b"},
		{ID: "c"},
	}

	results := runManagedChecks(apps)
	if len(results) != len(apps) {
		t.Fatalf("len(results) = %d, want %d", len(results), len(apps))
	}

	for i, app := range apps {
		if results[i].app == nil || results[i].app.ID != app.ID {
			t.Fatalf("results[%d].app.ID = %q, want %q", i, results[i].app.ID, app.ID)
		}
		if results[i].update == nil || results[i].update.App == nil || results[i].update.App.ID != app.ID {
			t.Fatalf("results[%d].update app mismatch", i)
		}
	}
}

func TestRunManagedChecksDeduplicatesEquivalentInputs(t *testing.T) {
	originalCheck := runAppUpdateCheck
	t.Cleanup(func() {
		runAppUpdateCheck = originalCheck
	})

	var calls int32
	runAppUpdateCheck = func(app *models.App) (*pendingManagedUpdate, error) {
		atomic.AddInt32(&calls, 1)
		return &pendingManagedUpdate{
			App:       app,
			Available: false,
			FromKind:  models.UpdateGitHubRelease,
		}, nil
	}

	apps := []*models.App{
		{
			ID:      "app-one",
			Version: "1.0.0",
			Update: &models.UpdateSource{
				Kind: models.UpdateGitHubRelease,
				GitHubRelease: &models.GitHubReleaseUpdateSource{
					Repo:  "owner/repo",
					Asset: "*.AppImage",
				},
			},
		},
		{
			ID:      "app-two",
			Version: "1.0.0",
			Update: &models.UpdateSource{
				Kind: models.UpdateGitHubRelease,
				GitHubRelease: &models.GitHubReleaseUpdateSource{
					Repo:  "owner/repo",
					Asset: "*.AppImage",
				},
			},
		},
	}

	results := runManagedChecks(apps)
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("runAppUpdateCheck calls = %d, want 1", calls)
	}
	if results[0].update == nil || results[1].update == nil {
		t.Fatal("expected updates in both results")
	}
	if results[0].update.App == results[1].update.App {
		t.Fatal("expected distinct app pointers per deduplicated result")
	}
}

func TestRunManagedChecksDoesNotDeduplicateDifferentLocalVersion(t *testing.T) {
	originalCheck := runAppUpdateCheck
	t.Cleanup(func() {
		runAppUpdateCheck = originalCheck
	})

	var calls int32
	runAppUpdateCheck = func(app *models.App) (*pendingManagedUpdate, error) {
		atomic.AddInt32(&calls, 1)
		return &pendingManagedUpdate{
			App:       app,
			Available: false,
			FromKind:  models.UpdateGitHubRelease,
		}, nil
	}

	apps := []*models.App{
		{
			ID:      "app-one",
			Version: "1.0.0",
			Update: &models.UpdateSource{
				Kind:          models.UpdateGitHubRelease,
				GitHubRelease: &models.GitHubReleaseUpdateSource{Repo: "owner/repo", Asset: "*.AppImage"},
			},
		},
		{
			ID:      "app-two",
			Version: "2.0.0",
			Update: &models.UpdateSource{
				Kind:          models.UpdateGitHubRelease,
				GitHubRelease: &models.GitHubReleaseUpdateSource{Repo: "owner/repo", Asset: "*.AppImage"},
			},
		},
	}

	_ = runManagedChecks(apps)
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("runAppUpdateCheck calls = %d, want 2", calls)
	}
}

func TestManagedCheckWorkerCount(t *testing.T) {
	tests := []struct {
		input  int
		expect int
	}{
		{input: 0, expect: 0},
		{input: 1, expect: 1},
		{input: 3, expect: 3},
		{input: 4, expect: 4},
		{input: 12, expect: 4},
	}

	for _, tt := range tests {
		got := managedCheckWorkerCount(tt.input)
		if got != tt.expect {
			t.Fatalf("managedCheckWorkerCount(%d) = %d, want %d", tt.input, got, tt.expect)
		}
	}
}

func TestPersistManagedAppliedAppsUsesBatch(t *testing.T) {
	originalBatch := addAppsBatch
	originalSingle := addSingleApp
	t.Cleanup(func() {
		addAppsBatch = originalBatch
		addSingleApp = originalSingle
	})

	var batchCalls int32
	var singleCalls int32
	addAppsBatch = func(apps []*models.App, overwrite bool) error {
		atomic.AddInt32(&batchCalls, 1)
		if !overwrite {
			t.Fatalf("expected overwrite true")
		}
		if len(apps) != 2 {
			t.Fatalf("len(apps) = %d, want 2", len(apps))
		}
		return nil
	}
	addSingleApp = func(*models.App, bool) error {
		atomic.AddInt32(&singleCalls, 1)
		return nil
	}

	err := persistManagedAppliedApps([]*models.App{{ID: "a"}, {ID: "b"}})
	if err != nil {
		t.Fatalf("persistManagedAppliedApps returned error: %v", err)
	}
	if atomic.LoadInt32(&batchCalls) != 1 {
		t.Fatalf("batch calls = %d, want 1", batchCalls)
	}
	if atomic.LoadInt32(&singleCalls) != 0 {
		t.Fatalf("single calls = %d, want 0", singleCalls)
	}
}

func TestPersistManagedAppliedAppsFallsBackToSingleWrites(t *testing.T) {
	originalBatch := addAppsBatch
	originalSingle := addSingleApp
	t.Cleanup(func() {
		addAppsBatch = originalBatch
		addSingleApp = originalSingle
	})

	var singleCalls int32
	addAppsBatch = func([]*models.App, bool) error {
		return fmt.Errorf("batch failed")
	}
	addSingleApp = func(*models.App, bool) error {
		atomic.AddInt32(&singleCalls, 1)
		return nil
	}

	err := persistManagedAppliedApps([]*models.App{{ID: "a"}, {ID: "b"}})
	if err != nil {
		t.Fatalf("persistManagedAppliedApps returned error: %v", err)
	}
	if atomic.LoadInt32(&singleCalls) != 2 {
		t.Fatalf("single calls = %d, want 2", singleCalls)
	}
}

func TestBuildManagedUpdateMessage(t *testing.T) {
	update := pendingManagedUpdate{
		App: &models.App{
			ID:      "obsidian",
			Version: "1.11.6",
		},
		Label:  "Newer version found!",
		Latest: "1.11.7",
		URL:    "https://example.com/Obsidian-1.11.7.AppImage",
		Asset:  "Obsidian-1.11.7.AppImage",
	}

	msgManaged := buildManagedUpdateMessage(update, false)
	if strings.Contains(msgManaged, "Download from") {
		t.Fatalf("managed update message should not include manual download hint: %s", msgManaged)
	}
	if !strings.Contains(msgManaged, "(v1.11.6 -> v1.11.7)") {
		t.Fatalf("managed update message should include version transition: %s", msgManaged)
	}

	msgCheckOnly := buildManagedUpdateMessage(update, true)
	if !strings.Contains(msgCheckOnly, "Download from https://example.com/Obsidian-1.11.7.AppImage") {
		t.Fatalf("check-only message should include download URL: %s", msgCheckOnly)
	}
	if !strings.Contains(msgCheckOnly, "Then integrate it with") {
		t.Fatalf("check-only message should include integration hint: %s", msgCheckOnly)
	}
}

func TestUpdateVersionTransitionUnknownLatest(t *testing.T) {
	update := pendingManagedUpdate{
		App:    &models.App{Version: "2.0.0"},
		Latest: "",
	}

	transition := updateVersionTransition(update)
	if transition != "(current: v2.0.0, latest: unknown)" {
		t.Fatalf("updateVersionTransition = %q", transition)
	}
}

func TestBuildDownloadProgressLine(t *testing.T) {
	known := buildDownloadProgressLine(512, 1024, 0)
	if !strings.Contains(known, "50.00%") {
		t.Fatalf("expected known-length progress to include percent, got: %s", known)
	}
	if !strings.Contains(known, "1.0KB") {
		t.Fatalf("expected known-length progress to include total bytes, got: %s", known)
	}

	unknown := buildDownloadProgressLine(1536, -1, 1)
	if !strings.Contains(unknown, "Downloading") {
		t.Fatalf("expected unknown-length progress to include label, got: %s", unknown)
	}
	if !strings.Contains(unknown, "1.5KB") {
		t.Fatalf("expected unknown-length progress to include byte count, got: %s", unknown)
	}
}

func TestFormatByteSize(t *testing.T) {
	tests := []struct {
		input  int64
		expect string
	}{
		{input: 512, expect: "512B"},
		{input: 1024, expect: "1.0KB"},
		{input: 1536, expect: "1.5KB"},
		{input: 1048576, expect: "1.0MB"},
	}

	for _, tt := range tests {
		got := formatByteSize(tt.input)
		if got != tt.expect {
			t.Fatalf("formatByteSize(%d) = %q, want %q", tt.input, got, tt.expect)
		}
	}
}

func TestDisplayVersion(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "numeric", input: "1.2.3", expect: "v1.2.3"},
		{name: "already prefixed", input: "v1.2.3", expect: "v1.2.3"},
		{name: "unknown literal", input: "unknown", expect: "unknown"},
		{name: "na placeholder", input: "n/a", expect: "unknown"},
		{name: "empty", input: "", expect: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := displayVersion(tt.input)
			if got != tt.expect {
				t.Fatalf("displayVersion(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestVerifyDownloadedUpdateWithBothHashes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.AppImage")
	if err := os.WriteFile(path, []byte("payload"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	sha256sum, sha1sum, err := util.Sha256AndSha1(path)
	if err != nil {
		t.Fatalf("failed to compute hashes: %v", err)
	}

	err = verifyDownloadedUpdate(path, pendingManagedUpdate{ExpectedSHA256: sha256sum, ExpectedSHA1: sha1sum})
	if err != nil {
		t.Fatalf("verifyDownloadedUpdate returned error: %v", err)
	}
}

func TestVerifyDownloadedUpdateWithBothHashesMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.AppImage")
	if err := os.WriteFile(path, []byte("payload"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	err := verifyDownloadedUpdate(path, pendingManagedUpdate{
		ExpectedSHA256: strings.Repeat("a", 64),
		ExpectedSHA1:   strings.Repeat("b", 40),
	})
	if err == nil {
		t.Fatal("expected hash mismatch error")
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed creating stdout pipe: %v", err)
	}

	os.Stdout = w
	defer func() {
		os.Stdout = originalStdout
	}()

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()
	_ = w.Close()
	return <-done
}

func runAddCommand(ctx context.Context, args []string) error {
	cmd := &cli.Command{
		Name: "add",
		Arguments: []cli.Argument{
			&cli.StringArg{Name: "app"},
		},
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "no-color"},
			&cli.BoolFlag{Name: "post-check"},
		},
		Action: AddCmd,
	}

	argv := append([]string{"add"}, args...)
	return cmd.Run(ctx, argv)
}
