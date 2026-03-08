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

func TestUpgradeCmdUsesSelfUpgradeFlow(t *testing.T) {
	original := runSelfUpgrade
	t.Cleanup(func() {
		runSelfUpgrade = original
	})
	runSelfUpgrade = func(context.Context, string) (*core.SelfUpgradeResult, error) {
		return nil, fmt.Errorf("self-upgrade is unavailable for development builds")
	}

	cmd := &cli.Command{
		Name:   "upgrade",
		Action: UpgradeCmd,
	}

	err := cmd.Run(context.Background(), []string{"upgrade"})
	if err == nil {
		t.Fatal("expected dev-build self-upgrade error")
	}
	if !strings.Contains(err.Error(), "self-upgrade is unavailable for development builds") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpgradeCmdOutputsUpdatedMessage(t *testing.T) {
	original := runSelfUpgrade
	t.Cleanup(func() {
		runSelfUpgrade = original
	})
	runSelfUpgrade = func(context.Context, string) (*core.SelfUpgradeResult, error) {
		return &core.SelfUpgradeResult{
			Updated:        true,
			CurrentVersion: "0.8.0",
			LatestVersion:  "0.8.1",
		}, nil
	}

	cmd := &cli.Command{Name: "upgrade", Action: UpgradeCmd}
	output := captureStdout(t, func() {
		if err := cmd.Run(context.Background(), []string{"upgrade"}); err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Updated aim v0.8.0 -> v0.8.1") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestUpgradeCmdOutputsUpToDateMessage(t *testing.T) {
	original := runSelfUpgrade
	t.Cleanup(func() {
		runSelfUpgrade = original
	})
	runSelfUpgrade = func(context.Context, string) (*core.SelfUpgradeResult, error) {
		return &core.SelfUpgradeResult{
			Updated:        false,
			CurrentVersion: "0.8.1",
			LatestVersion:  "0.8.1",
		}, nil
	}

	cmd := &cli.Command{Name: "upgrade", Action: UpgradeCmd}
	output := captureStdout(t, func() {
		if err := cmd.Run(context.Background(), []string{"upgrade"}); err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "aim is up to date (v0.8.1)") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestRootCommandDoesNotAcceptUpgradeFlag(t *testing.T) {
	cmd := newRootTestCommand()

	err := cmd.Run(context.Background(), []string{"aim", "--upgrade"})
	if err == nil {
		t.Fatal("expected unknown flag error")
	}
	if !strings.Contains(err.Error(), "flag provided but not defined: -upgrade") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRootVersionOutputIsCompact(t *testing.T) {
	original := cli.VersionPrinter
	t.Cleanup(func() {
		cli.VersionPrinter = original
	})
	cli.VersionPrinter = func(cmd *cli.Command) {
		root := cmd.Root()
		_, _ = fmt.Fprintf(root.Writer, "%s %s\n", root.Name, root.Version)
	}

	cmd := newRootTestCommand()
	cmd.Version = "v0.8.0"

	output := captureStdout(t, func() {
		if err := cmd.Run(context.Background(), []string{"aim", "--version"}); err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "aim v0.8.0") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if strings.Contains(output, "aim version v0.8.0") {
		t.Fatalf("did not expect duplicated version label:\n%s", output)
	}
}

func TestRootHelpDoesNotAdvertiseRemovedCommands(t *testing.T) {
	cmd := newRootTestCommand()

	output := captureStdout(t, func() {
		if err := cmd.Run(context.Background(), []string{"aim"}); err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	})

	for _, unwanted := range []string{"--upgrade", " pin", " unpin"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("root help unexpectedly contains %q:\n%s", unwanted, output)
		}
	}
}

func TestRemovedCommandsAreUnavailable(t *testing.T) {
	cmd := newRootTestCommand()

	for _, unwanted := range []string{"pin", "unpin"} {
		for _, subcommand := range cmd.Commands {
			if subcommand.Name == unwanted {
				t.Fatalf("unexpected command registration for %q", unwanted)
			}
		}
	}
}

func TestRemoveCommandUsesUnlinkFlag(t *testing.T) {
	cmd := newRootTestCommand()

	var removeCmd *cli.Command
	for _, subcommand := range cmd.Commands {
		if subcommand.Name == "remove" {
			removeCmd = subcommand
			break
		}
	}
	if removeCmd == nil {
		t.Fatal("expected remove command")
	}

	var (
		foundUnlink bool
		foundKeep   bool
	)
	for _, flag := range removeCmd.Flags {
		boolFlag, ok := flag.(*cli.BoolFlag)
		if !ok {
			continue
		}
		if boolFlag.Name == "unlink" {
			foundUnlink = true
			if len(boolFlag.Aliases) != 0 {
				t.Fatalf("expected no aliases for --unlink, got %v", boolFlag.Aliases)
			}
			if boolFlag.Usage != "remove only desktop integration; keep managed AppImage files" {
				t.Fatalf("unexpected --unlink usage: %q", boolFlag.Usage)
			}
		}
		if boolFlag.Name == "keep" {
			foundKeep = true
		}
	}

	if !foundUnlink {
		t.Fatal("expected --unlink flag on remove command")
	}
	if foundKeep {
		t.Fatal("did not expect --keep flag on remove command")
	}
}

func TestRemoveCmdOutputsRemovedMessage(t *testing.T) {
	original := removeManagedApp
	t.Cleanup(func() {
		removeManagedApp = original
	})
	removeManagedApp = func(context.Context, string, bool) (*models.App, error) {
		return &models.App{ID: "my-app", Name: "My App"}, nil
	}

	cmd := &cli.Command{
		Name: "remove",
		Arguments: []cli.Argument{
			&cli.StringArg{Name: "id"},
		},
		Action: RemoveCmd,
	}

	output := captureStdout(t, func() {
		if err := cmd.Run(context.Background(), []string{"remove", "my-app"}); err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Removed: My App [my-app]") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestRemoveCmdOutputsUnlinkedMessage(t *testing.T) {
	original := removeManagedApp
	t.Cleanup(func() {
		removeManagedApp = original
	})
	removeManagedApp = func(context.Context, string, bool) (*models.App, error) {
		return &models.App{ID: "my-app", Name: "My App"}, nil
	}

	cmd := &cli.Command{
		Name: "remove",
		Arguments: []cli.Argument{
			&cli.StringArg{Name: "id"},
		},
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "unlink"},
		},
		Action: RemoveCmd,
	}

	output := captureStdout(t, func() {
		if err := cmd.Run(context.Background(), []string{"remove", "--unlink", "my-app"}); err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Unlinked: My App [my-app]") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestUpdateCheckSubcommandRemoved(t *testing.T) {
	cmd := &cli.Command{
		Name: "update",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "yes"},
			&cli.BoolFlag{Name: "check-only"},
			&cli.StringFlag{Name: "github"},
			&cli.StringFlag{Name: "asset"},
			&cli.StringFlag{Name: "gitlab"},
			&cli.StringFlag{Name: "zsync-url"},
		},
		Action: UpdateCmd,
	}

	err := cmd.Run(context.Background(), []string{"update", "check", "./MyApp.AppImage"})
	if err == nil {
		t.Fatal("expected removed-subcommand error")
	}
	if !strings.Contains(err.Error(), "`aim update check` has been removed") {
		t.Fatalf("unexpected error: %v", err)
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

func TestAddCmdAlreadyIntegratedMessage(t *testing.T) {
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
				DesktopEntryLink: "/tmp/my-app.desktop",
			},
		},
	}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	output := captureStdout(t, func() {
		if err := runAddCommand(context.Background(), []string{"my-app"}); err != nil {
			t.Fatalf("runAddCommand returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Already integrated: My App v1.0.0 [my-app]") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestAddCmdReintegratedMessage(t *testing.T) {
	original := integrateExistingApp
	t.Cleanup(func() {
		integrateExistingApp = original
	})
	integrateExistingApp = func(context.Context, string) (*models.App, error) {
		return &models.App{ID: "my-app", Name: "My App", Version: "1.0.0"}, nil
	}

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
				ID:      "my-app",
				Name:    "My App",
				Version: "1.0.0",
			},
		},
	}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	output := captureStdout(t, func() {
		if err := runAddCommand(context.Background(), []string{"my-app"}); err != nil {
			t.Fatalf("runAddCommand returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Reintegrated: My App v1.0.0 [my-app]") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestAddCmdIntegratedMessage(t *testing.T) {
	original := integrateLocalApp
	t.Cleanup(func() {
		integrateLocalApp = original
	})
	integrateLocalApp = func(context.Context, string, core.UpdateOverwritePrompt) (*models.App, error) {
		return &models.App{
			ID:      "my-app",
			Name:    "My App",
			Version: "1.0.0",
			Update:  &models.UpdateSource{Kind: models.UpdateNone},
		}, nil
	}

	output := captureStdout(t, func() {
		if err := runAddCommand(context.Background(), []string{"/tmp/MyApp.AppImage"}); err != nil {
			t.Fatalf("runAddCommand returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Integrating MyApp.AppImage") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if !strings.Contains(output, "Integrated: My App v1.0.0 [my-app]") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestListCmdEmptyStates(t *testing.T) {
	t.Run("no managed apps", func(t *testing.T) {
		tempDir := t.TempDir()
		dbPath := filepath.Join(tempDir, "apps.json")

		originalDbSrc := config.DbSrc
		config.DbSrc = dbPath
		t.Cleanup(func() {
			config.DbSrc = originalDbSrc
		})

		if err := repo.SaveDB(dbPath, &repo.DB{SchemaVersion: 1, Apps: map[string]*models.App{}}); err != nil {
			t.Fatalf("failed to write test DB: %v", err)
		}

		output := captureStdout(t, func() {
			if err := runListCommand(context.Background(), nil); err != nil {
				t.Fatalf("runListCommand returned error: %v", err)
			}
		})

		if !strings.Contains(output, "No managed AppImages") {
			t.Fatalf("unexpected output:\n%s", output)
		}
	})

	t.Run("no integrated apps", func(t *testing.T) {
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

		output := captureStdout(t, func() {
			if err := runListCommand(context.Background(), []string{"--integrated"}); err != nil {
				t.Fatalf("runListCommand returned error: %v", err)
			}
		})

		if !strings.Contains(output, "No integrated AppImages") {
			t.Fatalf("unexpected output:\n%s", output)
		}
	})

	t.Run("no unlinked apps", func(t *testing.T) {
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
				"my-app": {ID: "my-app", Name: "My App", DesktopEntryLink: "/tmp/my-app.desktop"},
			},
		}); err != nil {
			t.Fatalf("failed to write test DB: %v", err)
		}

		output := captureStdout(t, func() {
			if err := runListCommand(context.Background(), []string{"--unlinked"}); err != nil {
				t.Fatalf("runListCommand returned error: %v", err)
			}
		})

		if !strings.Contains(output, "No unlinked AppImages") {
			t.Fatalf("unexpected output:\n%s", output)
		}
	})
}

func TestListCmdUsesDynamicIDWidthAndTruncatesLongNames(t *testing.T) {
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
			"obsidian": {
				ID:               "obsidian",
				Name:             "Obsidian",
				Version:          "1.12.4",
				DesktopEntryLink: "/tmp/obsidian.desktop",
			},
			"raspberry-pi-imager": {
				ID:               "raspberry-pi-imager",
				Name:             "Raspberry Pi Imager With An Extremely Long Name",
				Version:          "2.0.6",
				DesktopEntryLink: "/tmp/rpi.desktop",
			},
		},
	}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	output := captureStdout(t, func() {
		if err := runListCommand(context.Background(), nil); err != nil {
			t.Fatalf("runListCommand returned error: %v", err)
		}
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines, got:\n%s", output)
	}

	if !strings.Contains(lines[0], "ID") || !strings.Contains(lines[0], "App Name") || !strings.Contains(lines[0], "Version") {
		t.Fatalf("unexpected header:\n%s", lines[0])
	}
	if !strings.Contains(output, "raspberry-pi-imager Raspberry Pi Imager With ... 2.0.6") {
		t.Fatalf("expected dynamic ID width and truncated name, got:\n%s", output)
	}
	if !strings.Contains(output, "obsidian            Obsidian") {
		t.Fatalf("expected shorter ID to align within dynamic ID column, got:\n%s", output)
	}
}

func TestResolveUpdateSourceFromSetFlags(t *testing.T) {
	tests := []struct {
		name      string
		flags     map[string]string
		expect    models.UpdateKind
		asset     string
		wantError bool
	}{
		{
			name:   "github source",
			flags:  map[string]string{"github": "owner/repo", "asset": "*.AppImage"},
			expect: models.UpdateGitHubRelease,
			asset:  "*.AppImage",
		},
		{
			name:   "gitlab source",
			flags:  map[string]string{"gitlab": "group/project", "asset": "*.AppImage"},
			expect: models.UpdateGitLabRelease,
			asset:  "*.AppImage",
		},
		{
			name:   "github source default asset",
			flags:  map[string]string{"github": "owner/repo"},
			expect: models.UpdateGitHubRelease,
			asset:  "*.AppImage",
		},
		{
			name:   "gitlab source default asset",
			flags:  map[string]string{"gitlab": "group/project"},
			expect: models.UpdateGitLabRelease,
			asset:  "*.AppImage",
		},
		{
			name:   "zsync source",
			flags:  map[string]string{"zsync-url": "https://example.com/MyApp.AppImage.zsync"},
			expect: models.UpdateZsync,
		},
		{
			name:      "manifest no longer supported",
			flags:     map[string]string{"manifest-url": "https://example.com/latest.json"},
			wantError: true,
		},
		{
			name:      "direct url no longer supported",
			flags:     map[string]string{"url": "https://example.com/MyApp.AppImage"},
			wantError: true,
		},
		{
			name:      "sha256 no longer supported",
			flags:     map[string]string{"sha256": strings.Repeat("a", 64)},
			wantError: true,
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
			switch source.Kind {
			case models.UpdateGitHubRelease:
				if source.GitHubRelease == nil || source.GitHubRelease.Asset != tt.asset {
					t.Fatalf("github asset = %q, want %q", source.GitHubRelease.Asset, tt.asset)
				}
			case models.UpdateGitLabRelease:
				if source.GitLabRelease == nil || source.GitLabRelease.Asset != tt.asset {
					t.Fatalf("gitlab asset = %q, want %q", source.GitLabRelease.Asset, tt.asset)
				}
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

func newRootTestCommand() *cli.Command {
	return &cli.Command{
		Name:   "aim",
		Usage:  "Manage AppImages as desktop apps on Linux",
		Action: RootCmd,
		Commands: []*cli.Command{
			{Name: "add", Action: AddCmd},
			{
				Name: "remove",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "unlink",
						Usage: "remove only desktop integration; keep managed AppImage files",
					},
				},
				Action: RemoveCmd,
			},
			{Name: "list", Action: ListCmd},
			{
				Name: "update",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "yes"},
					&cli.BoolFlag{Name: "check-only"},
					&cli.StringFlag{Name: "github"},
					&cli.StringFlag{Name: "asset"},
					&cli.StringFlag{Name: "gitlab"},
					&cli.StringFlag{Name: "zsync-url"},
				},
				Action: UpdateCmd,
			},
			{Name: "upgrade", Action: UpgradeCmd},
		},
	}
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
				ID:   "my-app",
				Name: "My App",
			},
		},
	}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	cmd := &cli.Command{Flags: []cli.Flag{&cli.BoolFlag{Name: "yes"}, &cli.BoolFlag{Name: "check-only"}}}
	originalCheck := runAppUpdateCheck
	t.Cleanup(func() {
		runAppUpdateCheck = originalCheck
	})
	runAppUpdateCheck = func(app *models.App) (*pendingManagedUpdate, error) {
		return &pendingManagedUpdate{
			App:       app,
			Available: false,
			FromKind:  models.UpdateGitHubRelease,
		}, nil
	}

	output := captureStdout(t, func() {
		if err := runManagedUpdate(context.Background(), cmd, "my-app"); err != nil {
			t.Fatalf("runManagedUpdate returned error: %v", err)
		}
	})

	if strings.Contains(output, "You are up-to-date!") {
		t.Fatalf("expected old up-to-date output to be absent, got output:\n%s", output)
	}
	if strings.Count(output, "Up to date: my-app unknown") != 1 {
		t.Fatalf("expected exactly one up-to-date message, got output:\n%s", output)
	}
}

func TestRunManagedUpdateSingleNoSourceConfigured(t *testing.T) {
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

	cmd := &cli.Command{Flags: []cli.Flag{&cli.BoolFlag{Name: "yes"}, &cli.BoolFlag{Name: "check-only"}}}
	originalCheck := runAppUpdateCheck
	t.Cleanup(func() {
		runAppUpdateCheck = originalCheck
	})
	runAppUpdateCheck = func(*models.App) (*pendingManagedUpdate, error) {
		return nil, nil
	}

	output := captureStdout(t, func() {
		if err := runManagedUpdate(context.Background(), cmd, "my-app"); err != nil {
			t.Fatalf("runManagedUpdate returned error: %v", err)
		}
	})

	if !strings.Contains(output, "No update source configured for my-app") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestCheckAppUpdateUnsupportedLegacySource(t *testing.T) {
	_, err := checkAppUpdate(&models.App{
		ID: "my-app",
		Update: &models.UpdateSource{
			Kind: models.UpdateKind("manifest"),
		},
	})
	if err == nil {
		t.Fatal("expected unsupported-source error")
	}
	if !strings.Contains(err.Error(), "Reconfigure with `aim update set`") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunManagedUpdateBatchContinuesOnCheckFailure(t *testing.T) {
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
			"app-a": {ID: "app-a", Name: "App A"},
			"app-b": {ID: "app-b", Name: "App B"},
		},
	}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	originalCheck := runAppUpdateCheck
	t.Cleanup(func() {
		runAppUpdateCheck = originalCheck
	})
	runAppUpdateCheck = func(app *models.App) (*pendingManagedUpdate, error) {
		if app.ID == "app-a" {
			return nil, fmt.Errorf("boom")
		}
		return nil, nil
	}

	cmd := &cli.Command{Flags: []cli.Flag{&cli.BoolFlag{Name: "yes"}, &cli.BoolFlag{Name: "check-only"}}}
	output := captureStdout(t, func() {
		if err := runManagedUpdate(context.Background(), cmd, ""); err != nil {
			t.Fatalf("runManagedUpdate returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Failed to check updates for app-a: boom") {
		t.Fatalf("expected batch failure message, got:\n%s", output)
	}
	if !strings.Contains(output, "No updates applied; some checks failed") {
		t.Fatalf("expected summary message, got:\n%s", output)
	}
}

func TestRunManagedUpdateBatchAllUpToDateSummary(t *testing.T) {
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
			"app-a": {ID: "app-a", Name: "App A"},
			"app-b": {ID: "app-b", Name: "App B"},
		},
	}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	originalCheck := runAppUpdateCheck
	t.Cleanup(func() {
		runAppUpdateCheck = originalCheck
	})
	runAppUpdateCheck = func(app *models.App) (*pendingManagedUpdate, error) {
		return &pendingManagedUpdate{App: app, Available: false}, nil
	}

	cmd := &cli.Command{Flags: []cli.Flag{&cli.BoolFlag{Name: "check-only"}}}
	output := captureStdout(t, func() {
		if err := runManagedUpdate(context.Background(), cmd, ""); err != nil {
			t.Fatalf("runManagedUpdate returned error: %v", err)
		}
	})

	if !strings.Contains(output, "All apps are up to date") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if strings.Contains(output, "Up to date: app-a") || strings.Contains(output, "Up to date: app-b") {
		t.Fatalf("did not expect per-app up-to-date noise:\n%s", output)
	}
}

func TestRunManagedUpdateCheckOnlyShowsDownloadAndAsset(t *testing.T) {
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
			"my-app": {ID: "my-app", Name: "My App", Version: "1.0.0"},
		},
	}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	originalCheck := runAppUpdateCheck
	t.Cleanup(func() {
		runAppUpdateCheck = originalCheck
	})
	runAppUpdateCheck = func(app *models.App) (*pendingManagedUpdate, error) {
		return &pendingManagedUpdate{
			App:       app,
			Label:     "Update available",
			Available: true,
			Latest:    "1.1.0",
			URL:       "https://example.com/MyApp.AppImage",
			Asset:     "MyApp-x86_64.AppImage",
		}, nil
	}

	cmd := &cli.Command{Flags: []cli.Flag{&cli.BoolFlag{Name: "check-only"}}}
	if err := cmd.Set("check-only", "true"); err != nil {
		t.Fatalf("failed to set check-only: %v", err)
	}
	output := captureStdout(t, func() {
		if err := runManagedUpdate(context.Background(), cmd, "my-app"); err != nil {
			t.Fatalf("runManagedUpdate returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Update available: my-app v1.0.0 -> v1.1.0") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if !strings.Contains(output, "  Download: https://example.com/MyApp.AppImage") {
		t.Fatalf("expected download line, got:\n%s", output)
	}
	if !strings.Contains(output, "  Asset: MyApp-x86_64.AppImage") {
		t.Fatalf("expected asset line, got:\n%s", output)
	}
}

func TestRunManagedUpdateSinglePromptText(t *testing.T) {
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
			"my-app": {ID: "my-app", Name: "My App", Version: "1.0.0"},
		},
	}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	originalCheck := runAppUpdateCheck
	t.Cleanup(func() {
		runAppUpdateCheck = originalCheck
	})
	runAppUpdateCheck = func(app *models.App) (*pendingManagedUpdate, error) {
		return &pendingManagedUpdate{
			App:       app,
			Label:     "Update available",
			Available: true,
			Latest:    "1.1.0",
			URL:       "https://example.com/MyApp.AppImage",
		}, nil
	}

	cmd := &cli.Command{Flags: []cli.Flag{&cli.BoolFlag{Name: "yes"}, &cli.BoolFlag{Name: "check-only"}}}
	output := captureStdoutWithInput(t, "n\n", func() {
		if err := runManagedUpdate(context.Background(), cmd, "my-app"); err != nil {
			t.Fatalf("runManagedUpdate returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Apply update for my-app? [y/N]: ") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if !strings.Contains(output, "No updates applied") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestRunManagedUpdateBatchPromptText(t *testing.T) {
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
			"app-a": {ID: "app-a", Name: "App A", Version: "1.0.0"},
			"app-b": {ID: "app-b", Name: "App B", Version: "2.0.0"},
		},
	}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	originalCheck := runAppUpdateCheck
	t.Cleanup(func() {
		runAppUpdateCheck = originalCheck
	})
	runAppUpdateCheck = func(app *models.App) (*pendingManagedUpdate, error) {
		return &pendingManagedUpdate{
			App:       app,
			Label:     "Update available",
			Available: true,
			Latest:    "9.9.9",
			URL:       "https://example.com/MyApp.AppImage",
		}, nil
	}

	cmd := &cli.Command{Flags: []cli.Flag{&cli.BoolFlag{Name: "yes"}, &cli.BoolFlag{Name: "check-only"}}}
	output := captureStdoutWithInput(t, "n\n", func() {
		if err := runManagedUpdate(context.Background(), cmd, ""); err != nil {
			t.Fatalf("runManagedUpdate returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Apply 2 updates? [y/N]: ") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if !strings.Contains(output, "No updates applied") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestCheckAppUpdateGitHubUsesNormalizedVersion(t *testing.T) {
	originalCheck := runGitHubReleaseUpdateCheck
	t.Cleanup(func() {
		runGitHubReleaseUpdateCheck = originalCheck
	})

	runGitHubReleaseUpdateCheck = func(update *models.UpdateSource, currentVersion string) (*core.GitHubReleaseUpdate, error) {
		return &core.GitHubReleaseUpdate{
			Available:         false,
			TagName:           "@standardnotes/desktop@3.201.19",
			NormalizedVersion: "3.201.19",
		}, nil
	}

	result, err := checkAppUpdate(&models.App{
		ID:      "standard-notes",
		Version: "3.201.19",
		Update: &models.UpdateSource{
			Kind: models.UpdateGitHubRelease,
			GitHubRelease: &models.GitHubReleaseUpdateSource{
				Repo:  "standardnotes/app",
				Asset: "*.AppImage",
			},
		},
	})
	if err != nil {
		t.Fatalf("checkAppUpdate returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected pending update result")
	}
	if result.Available {
		t.Fatal("expected no update when normalized versions match")
	}
	if result.Latest != "3.201.19" {
		t.Fatalf("Latest = %q, want %q", result.Latest, "3.201.19")
	}
}

func TestCheckAppUpdateGitHubDisplaysNormalizedLatest(t *testing.T) {
	originalCheck := runGitHubReleaseUpdateCheck
	t.Cleanup(func() {
		runGitHubReleaseUpdateCheck = originalCheck
	})

	runGitHubReleaseUpdateCheck = func(update *models.UpdateSource, currentVersion string) (*core.GitHubReleaseUpdate, error) {
		return &core.GitHubReleaseUpdate{
			Available:         true,
			DownloadUrl:       "https://example.com/StandardNotes-x86_64.AppImage",
			TagName:           "@standardnotes/desktop@3.202.0",
			NormalizedVersion: "3.202.0",
			AssetName:         "StandardNotes-x86_64.AppImage",
		}, nil
	}

	result, err := checkAppUpdate(&models.App{
		ID:      "standard-notes",
		Version: "3.201.19",
		Update: &models.UpdateSource{
			Kind: models.UpdateGitHubRelease,
			GitHubRelease: &models.GitHubReleaseUpdateSource{
				Repo:  "standardnotes/app",
				Asset: "*.AppImage",
			},
		},
	})
	if err != nil {
		t.Fatalf("checkAppUpdate returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected pending update result")
	}
	if result.Latest != "3.202.0" {
		t.Fatalf("Latest = %q, want %q", result.Latest, "3.202.0")
	}

	msg := buildManagedUpdateMessage(*result, false)
	if !strings.Contains(msg, "Update available: standard-notes v3.201.19 -> v3.202.0") {
		t.Fatalf("expected normalized version transition, got:\n%s", msg)
	}
	if strings.Contains(msg, "@standardnotes/desktop@3.202.0") {
		t.Fatalf("did not expect raw decorated tag in message:\n%s", msg)
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

	if err := runAddCommand(context.Background(), []string{"my-app"}); err != nil {
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

	if err := runAddCommand(context.Background(), []string{"my-app", "--post-check"}); err != nil {
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
		Label:  "Update available",
		Latest: "1.11.7",
		URL:    "https://example.com/Obsidian-1.11.7.AppImage",
		Asset:  "Obsidian-1.11.7.AppImage",
	}

	msgManaged := buildManagedUpdateMessage(update, false)
	if strings.Contains(msgManaged, "Download:") {
		t.Fatalf("managed update message should not include manual download hint: %s", msgManaged)
	}
	if !strings.Contains(msgManaged, "Update available: obsidian v1.11.6 -> v1.11.7") {
		t.Fatalf("managed update message should include version transition: %s", msgManaged)
	}

	msgCheckOnly := buildManagedUpdateMessage(update, true)
	if msgCheckOnly != msgManaged {
		t.Fatalf("check-only message should use the same summary line, got %q want %q", msgCheckOnly, msgManaged)
	}
}

func TestUpdateVersionTransitionUnknownLatest(t *testing.T) {
	update := pendingManagedUpdate{
		App:    &models.App{Version: "2.0.0"},
		Latest: "",
	}

	transition := updateVersionTransition(update)
	if transition != "v2.0.0 -> unknown" {
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

func TestUpdateSetPromptText(t *testing.T) {
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
				ID:   "my-app",
				Name: "My App",
				Update: &models.UpdateSource{
					Kind: models.UpdateGitHubRelease,
					GitHubRelease: &models.GitHubReleaseUpdateSource{
						Repo:  "owner/repo",
						Asset: "*.AppImage",
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	output := captureStdoutWithInput(t, "n\n", func() {
		if err := runUpdateSetCommand(context.Background(), []string{"my-app", "--gitlab", "group/project"}); err != nil {
			t.Fatalf("runUpdateSetCommand returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Replace update source for my-app? [y/N]: ") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if !strings.Contains(output, "Update source unchanged") {
		t.Fatalf("unexpected output:\n%s", output)
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

func captureStdoutWithInput(t *testing.T, input string, fn func()) string {
	t.Helper()

	originalStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed creating stdin pipe: %v", err)
	}

	if _, err := io.WriteString(w, input); err != nil {
		t.Fatalf("failed writing stdin input: %v", err)
	}
	_ = w.Close()

	os.Stdin = r
	defer func() {
		os.Stdin = originalStdin
		_ = r.Close()
	}()

	return captureStdout(t, fn)
}

func runAddCommand(ctx context.Context, args []string) error {
	cmd := &cli.Command{
		Name: "add",
		Arguments: []cli.Argument{
			&cli.StringArg{Name: "app"},
		},
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "post-check"},
		},
		Action: AddCmd,
	}

	argv := append([]string{"add"}, args...)
	return cmd.Run(ctx, argv)
}

func runListCommand(ctx context.Context, args []string) error {
	cmd := &cli.Command{
		Name: "list",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "all"},
			&cli.BoolFlag{Name: "integrated"},
			&cli.BoolFlag{Name: "unlinked"},
		},
		Action: ListCmd,
	}

	argv := append([]string{"list"}, args...)
	return cmd.Run(ctx, argv)
}

func runUpdateSetCommand(ctx context.Context, args []string) error {
	cmd := &cli.Command{
		Name: "update",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "yes"},
			&cli.BoolFlag{Name: "check-only"},
			&cli.StringFlag{Name: "github"},
			&cli.StringFlag{Name: "asset"},
			&cli.StringFlag{Name: "gitlab"},
			&cli.StringFlag{Name: "zsync-url"},
		},
		Action: UpdateCmd,
	}

	argv := append([]string{"update", "set"}, args...)
	return cmd.Run(ctx, argv)
}
