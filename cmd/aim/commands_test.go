package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slobbe/appimage-manager/internal/config"
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
