package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slobbe/appimage-manager/internal/config"
	"github.com/slobbe/appimage-manager/internal/core"
	"github.com/slobbe/appimage-manager/internal/discovery"
	util "github.com/slobbe/appimage-manager/internal/helpers"
	repo "github.com/slobbe/appimage-manager/internal/repository"
	models "github.com/slobbe/appimage-manager/internal/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestResolveIntegrateTarget(t *testing.T) {
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
		name      string
		input     string
		expect    integrateTargetKind
		wantError bool
		errText   string
	}{
		{name: "local appimage path", input: "/tmp/MyApp.AppImage", expect: integrateTargetLocalFile},
		{name: "integrated id", input: "integrated", expect: integrateTargetIntegrated},
		{name: "unlinked id", input: "unlinked", expect: integrateTargetUnlinked},
		{name: "direct url rejected", input: "https://example.com/MyApp.AppImage", wantError: true, errText: "remote sources are added with 'aim add'"},
		{name: "github repo rejected", input: "github:owner/repo", wantError: true, errText: "remote sources are added with 'aim add'"},
		{name: "gitlab repo rejected", input: "gitlab:group/project", wantError: true, errText: "remote sources are added with 'aim add'"},
		{name: "http rejected", input: "http://example.com/MyApp.AppImage", wantError: true, errText: "direct URLs must use https; use 'aim add https://...'"},
		{name: "malformed github treated as remote", input: "github:owner", wantError: true, errText: "remote sources are added with 'aim add'"},
		{name: "malformed gitlab treated as remote", input: "gitlab:group", wantError: true, errText: "remote sources are added with 'aim add'"},
		{name: "unknown id", input: "missing", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveIntegrateTarget(tt.input)
			if tt.wantError {
				if err == nil {
					t.Fatalf("resolveIntegrateTarget(%q) expected error", tt.input)
				}
				if tt.errText != "" && !strings.Contains(err.Error(), tt.errText) {
					t.Fatalf("resolveIntegrateTarget(%q) error = %q, want substring %q", tt.input, err.Error(), tt.errText)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveIntegrateTarget(%q) returned error: %v", tt.input, err)
			}
			if got == nil {
				t.Fatalf("resolveIntegrateTarget(%q) returned nil target", tt.input)
			}
			if got.Kind != tt.expect {
				t.Fatalf("resolveIntegrateTarget(%q) kind = %q, want %q", tt.input, got.Kind, tt.expect)
			}
		})
	}
}

func TestResolveInstallTarget(t *testing.T) {
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
			"managed": {
				ID: "managed",
			},
		},
	}
	if err := repo.SaveDB(dbPath, db); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	tests := []struct {
		name      string
		input     string
		expect    installTargetKind
		wantError bool
		errText   string
	}{
		{name: "direct url", input: "https://example.com/MyApp.AppImage", expect: installTargetDirectURL},
		{name: "github repo", input: "github:owner/repo", expect: installTargetGitHub},
		{name: "gitlab repo", input: "gitlab:group/project", expect: installTargetGitLab},
		{name: "http rejected", input: "http://example.com/MyApp.AppImage", wantError: true, errText: "direct URLs must use https"},
		{name: "local path rejected", input: "/tmp/MyApp.AppImage", wantError: true, errText: "local AppImages are integrated with 'aim integrate <path-to.AppImage>'"},
		{name: "managed id rejected", input: "managed", wantError: true, errText: "managed app IDs are reintegrated with 'aim integrate <id>'"},
		{name: "malformed github", input: "github:owner", wantError: true, errText: "github install source must be in the form github:owner/repo"},
		{name: "malformed gitlab", input: "gitlab:group", wantError: true, errText: "gitlab install source must be in the form gitlab:namespace/project"},
		{name: "unknown target", input: "missing", wantError: true, errText: "unknown install target"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveInstallTarget(tt.input)
			if tt.wantError {
				if err == nil {
					t.Fatalf("resolveInstallTarget(%q) expected error", tt.input)
				}
				if tt.errText != "" && !strings.Contains(err.Error(), tt.errText) {
					t.Fatalf("resolveInstallTarget(%q) error = %q, want substring %q", tt.input, err.Error(), tt.errText)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveInstallTarget(%q) returned error: %v", tt.input, err)
			}
			if got == nil {
				t.Fatalf("resolveInstallTarget(%q) returned nil target", tt.input)
			}
			if got.Kind != tt.expect {
				t.Fatalf("resolveInstallTarget(%q) kind = %q, want %q", tt.input, got.Kind, tt.expect)
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

	cmd := newUpgradeTestCommand()

	err := executeTestCommand(context.Background(), cmd)
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

	cmd := newUpgradeTestCommand()
	output := captureStdout(t, func() {
		if err := executeTestCommand(context.Background(), cmd); err != nil {
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

	cmd := newUpgradeTestCommand()
	output := captureStdout(t, func() {
		if err := executeTestCommand(context.Background(), cmd); err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "aim is up to date (v0.8.1)") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestRootCommandDoesNotAcceptUpgradeFlag(t *testing.T) {
	cmd := newRootTestCommand()

	err := executeTestCommand(context.Background(), cmd, "--upgrade")
	if err == nil {
		t.Fatal("expected unknown flag error")
	}
	if !strings.Contains(err.Error(), "unknown flag: --upgrade") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRootVersionOutputIsCompact(t *testing.T) {
	cmd := newRootTestCommand()
	cmd.Version = "v0.8.0"

	output := captureStdout(t, func() {
		if err := executeTestCommand(context.Background(), cmd, "--version"); err != nil {
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
		if err := executeTestCommand(context.Background(), cmd); err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	})

	for _, unwanted := range []string{"--upgrade", " pin", " unpin", "completion"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("root help unexpectedly contains %q:\n%s", unwanted, output)
		}
	}
	if !strings.Contains(output, "info") {
		t.Fatalf("expected root help to list info command:\n%s", output)
	}
	if !strings.Contains(output, "integrate") {
		t.Fatalf("expected root help to list integrate command:\n%s", output)
	}
}

func TestRemovedCommandsAreUnavailable(t *testing.T) {
	cmd := newRootTestCommand()

	for _, unwanted := range []string{"pin", "unpin", "completion"} {
		if findSubcommand(cmd, unwanted) != nil {
			t.Fatalf("unexpected command registration for %q", unwanted)
		}
	}
}

func TestRootRegistersPackageCommands(t *testing.T) {
	cmd := newRootTestCommand()

	required := map[string]bool{
		"add":       false,
		"integrate": false,
		"info":      false,
		"show":      false,
		"install":   false,
	}
	for _, subcommand := range cmd.Commands() {
		if _, ok := required[subcommand.Name()]; ok {
			required[subcommand.Name()] = true
		}
	}

	for name, found := range required {
		if !found {
			t.Fatalf("expected command %q to be registered", name)
		}
	}
}

func TestRootPackageCommandFlags(t *testing.T) {
	cmd := newRootTestCommand()

	addCmd := findSubcommand(cmd, "add")
	integrateCmd := findSubcommand(cmd, "integrate")
	installCmd := findSubcommand(cmd, "install")
	if addCmd == nil || integrateCmd == nil || installCmd == nil {
		t.Fatal("expected add, integrate, and install commands")
	}

	if got := countFlags(addCmd.Flags()); got != 2 {
		t.Fatalf("add flags = %d, want 2", got)
	}

	if addCmd.Flags().Lookup("asset") == nil || addCmd.Flags().Lookup("sha256") == nil {
		t.Fatalf("add flags missing asset or sha256")
	}

	if got := countFlags(integrateCmd.Flags()); got != 0 {
		t.Fatalf("integrate flags = %d, want none", got)
	}

	if installCmd.Flags().Lookup("asset") == nil || installCmd.Flags().Lookup("sha256") == nil {
		t.Fatalf("install flags missing asset or sha256")
	}
}

func TestRootPackageCommandAliases(t *testing.T) {
	cmd := newRootTestCommand()

	installCmd := findSubcommand(cmd, "install")
	updateCmd := findSubcommand(cmd, "update")
	if installCmd == nil || updateCmd == nil {
		t.Fatal("expected install and update commands")
	}

	if aliases := installCmd.Aliases; len(aliases) != 1 || aliases[0] != "i" {
		t.Fatalf("install aliases = %v, want [i]", aliases)
	}
	if aliases := updateCmd.Aliases; len(aliases) != 1 || aliases[0] != "u" {
		t.Fatalf("update aliases = %v, want [u]", aliases)
	}
}

func TestRemoveCommandUsesUnlinkFlag(t *testing.T) {
	cmd := newRootTestCommand()

	removeCmd := findSubcommand(cmd, "remove")
	if removeCmd == nil {
		t.Fatal("expected remove command")
	}

	unlinkFlag := removeCmd.Flags().Lookup("unlink")
	if unlinkFlag == nil {
		t.Fatal("expected --unlink flag on remove command")
	}
	if removeCmd.Flags().Lookup("keep") != nil {
		t.Fatal("did not expect --keep flag on remove command")
	}
	if unlinkFlag.Shorthand != "" {
		t.Fatalf("expected no shorthand for --unlink, got %q", unlinkFlag.Shorthand)
	}
	if unlinkFlag.Usage != "remove only desktop integration; keep managed AppImage files" {
		t.Fatalf("unexpected --unlink usage: %q", unlinkFlag.Usage)
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

	output := captureStdout(t, func() {
		if err := runRootCommand(context.Background(), []string{"remove", "my-app"}); err != nil {
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

	output := captureStdout(t, func() {
		if err := runRootCommand(context.Background(), []string{"remove", "--unlink", "my-app"}); err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Unlinked: My App [my-app]") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestUpdateCheckSubcommandRemoved(t *testing.T) {
	err := runRootCommand(context.Background(), []string{"update", "check", "./MyApp.AppImage"})
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

func TestIntegrateCmdAlreadyIntegratedMessage(t *testing.T) {
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
		if err := runIntegrateCommand(context.Background(), []string{"my-app"}); err != nil {
			t.Fatalf("runIntegrateCommand returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Already integrated: My App v1.0.0 [my-app]") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestIntegrateCmdReintegratedMessage(t *testing.T) {
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
		if err := runIntegrateCommand(context.Background(), []string{"my-app"}); err != nil {
			t.Fatalf("runIntegrateCommand returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Reintegrated: My App v1.0.0 [my-app]") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestIntegrateCmdIntegratedMessage(t *testing.T) {
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
		if err := runIntegrateCommand(context.Background(), []string{"/tmp/MyApp.AppImage"}); err != nil {
			t.Fatalf("runIntegrateCommand returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Integrating MyApp.AppImage") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if !strings.Contains(output, "Integrated: My App v1.0.0 [my-app]") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestIntegrateCmdRejectsRemoteSources(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		errText string
	}{
		{name: "direct url", args: []string{"https://example.com/MyApp.AppImage"}, errText: "remote sources are added with 'aim add'"},
		{name: "github", args: []string{"github:owner/repo"}, errText: "remote sources are added with 'aim add'"},
		{name: "gitlab", args: []string{"gitlab:group/project"}, errText: "remote sources are added with 'aim add'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runIntegrateCommand(context.Background(), tt.args)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.errText) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestAddCmdRoutesManagedIDToIntegrateFlow(t *testing.T) {
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

func TestAddCmdRoutesLocalPathToIntegrateFlow(t *testing.T) {
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

	if !strings.Contains(output, "Integrating MyApp.AppImage") || !strings.Contains(output, "Integrated: My App v1.0.0 [my-app]") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestValidateInstallTargetFlags(t *testing.T) {
	tests := []struct {
		name      string
		target    *installTarget
		args      []string
		wantError bool
	}{
		{
			name:      "asset rejected for direct url",
			target:    &installTarget{Kind: installTargetDirectURL},
			args:      []string{"https://example.com/MyApp.AppImage", "--asset", "*.AppImage"},
			wantError: true,
		},
		{
			name:      "sha256 rejected for github",
			target:    &installTarget{Kind: installTargetGitHub},
			args:      []string{"github:owner/repo", "--sha256", strings.Repeat("a", 64)},
			wantError: true,
		},
		{
			name:      "sha256 rejected for gitlab",
			target:    &installTarget{Kind: installTargetGitLab},
			args:      []string{"gitlab:group/project", "--sha256", strings.Repeat("a", 64)},
			wantError: true,
		},
		{
			name:      "invalid sha256 rejected",
			target:    &installTarget{Kind: installTargetDirectURL},
			args:      []string{"https://example.com/MyApp.AppImage", "--sha256", "not-a-hash"},
			wantError: true,
		},
		{
			name:   "valid direct url sha256",
			target: &installTarget{Kind: installTargetDirectURL},
			args:   []string{"https://example.com/MyApp.AppImage", "--sha256", strings.Repeat("a", 64)},
		},
		{
			name:   "valid github asset",
			target: &installTarget{Kind: installTargetGitHub},
			args:   []string{"github:owner/repo", "--asset", "*.AppImage"},
		},
		{
			name:   "valid gitlab asset",
			target: &installTarget{Kind: installTargetGitLab},
			args:   []string{"gitlab:group/project", "--asset", "*.AppImage"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newInstallTestCommand()
			if err := executeTestCommand(context.Background(), cmd, tt.args...); err == nil {
				t.Fatal("expected install action placeholder error")
			}

			err := validateInstallTargetFlags(cmd, tt.target)
			if tt.wantError && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("validateInstallTargetFlags returned error: %v", err)
			}
		})
	}
}

func TestAddCmdRejectsLocalTargetsWithRemoteFlags(t *testing.T) {
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
			"my-app": {ID: "my-app"},
		},
	}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	err := runAddCommand(context.Background(), []string{"./MyApp.AppImage", "--asset", "*.AppImage"})
	if err == nil || !strings.Contains(err.Error(), "--asset is only supported with github: or gitlab: add sources") {
		t.Fatalf("unexpected error: %v", err)
	}

	err = runAddCommand(context.Background(), []string{"my-app", "--sha256", strings.Repeat("a", 64)})
	if err == nil || !strings.Contains(err.Error(), "--sha256 is only supported with direct https:// add sources") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddCmdRejectsUnknownTarget(t *testing.T) {
	err := runAddCommand(context.Background(), []string{"1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown add target") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddCmdRemoteFlagValidation(t *testing.T) {
	err := runAddCommand(context.Background(), []string{"github:owner/repo", "--sha256", strings.Repeat("a", 64)})
	if err == nil || !strings.Contains(err.Error(), "--sha256 is only supported with direct https URLs") {
		t.Fatalf("unexpected error: %v", err)
	}

	err = runAddCommand(context.Background(), []string{"https://example.com/app.AppImage", "--asset", "*.AppImage"})
	if err == nil || !strings.Contains(err.Error(), "--asset is only supported with github: or gitlab: install sources") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddAndIntegrateRejectRemovedPostCheckFlag(t *testing.T) {
	err := runAddCommand(context.Background(), []string{"./MyApp.AppImage", "--post-check"})
	if err == nil || !strings.Contains(err.Error(), "unknown flag: --post-check") {
		t.Fatalf("unexpected add error: %v", err)
	}

	err = runIntegrateCommand(context.Background(), []string{"./MyApp.AppImage", "--post-check"})
	if err == nil || !strings.Contains(err.Error(), "unknown flag: --post-check") {
		t.Fatalf("unexpected integrate error: %v", err)
	}
}

func TestInstallCmdDirectURLWithChecksum(t *testing.T) {
	tempDir := t.TempDir()
	setupAddCommandConfigForTest(t, tempDir)

	originalDownload := downloadRemoteAsset
	originalIntegrate := integrateLocalApp
	originalAddSingle := addSingleApp
	t.Cleanup(func() {
		downloadRemoteAsset = originalDownload
		integrateLocalApp = originalIntegrate
		addSingleApp = originalAddSingle
	})

	payload := []byte("remote-appimage")
	sum := sha256.Sum256(payload)
	expectedSHA256 := hex.EncodeToString(sum[:])

	downloadRemoteAsset = func(ctx context.Context, assetURL, destination string, interactive bool) error {
		_ = ctx
		_ = interactive
		if assetURL != "https://example.com/MyApp.AppImage" {
			t.Fatalf("assetURL = %q", assetURL)
		}
		return os.WriteFile(destination, payload, 0o644)
	}
	integrateLocalApp = func(context.Context, string, core.UpdateOverwritePrompt) (*models.App, error) {
		return &models.App{
			ID:        "my-app",
			Name:      "My App",
			Version:   "1.0.0",
			UpdatedAt: "2026-03-08T12:00:00Z",
		}, nil
	}
	addSingleApp = repo.AddApp

	output := captureStdout(t, func() {
		if err := runInstallCommand(context.Background(), []string{"https://example.com/MyApp.AppImage", "--sha256", expectedSHA256}); err != nil {
			t.Fatalf("runInstallCommand returned error: %v", err)
		}
	})

	if strings.Contains(output, "skipping checksum verification") {
		t.Fatalf("did not expect checksum warning:\n%s", output)
	}

	app, err := repo.GetApp("my-app")
	if err != nil {
		t.Fatalf("failed to load persisted app: %v", err)
	}
	if app.Source.Kind != models.SourceDirectURL {
		t.Fatalf("Source.Kind = %q", app.Source.Kind)
	}
	if app.Source.DirectURL == nil {
		t.Fatal("expected direct URL source")
	}
	if app.Source.DirectURL.URL != "https://example.com/MyApp.AppImage" {
		t.Fatalf("direct URL source URL = %q", app.Source.DirectURL.URL)
	}
	if app.Source.DirectURL.SHA256 != expectedSHA256 {
		t.Fatalf("direct URL source SHA256 = %q", app.Source.DirectURL.SHA256)
	}
	if app.Update == nil || app.Update.Kind != models.UpdateNone {
		t.Fatalf("Update.Kind = %v, want none", app.Update)
	}
}

func TestInstallCmdDirectURLWithoutChecksumWarns(t *testing.T) {
	tempDir := t.TempDir()
	setupAddCommandConfigForTest(t, tempDir)

	originalDownload := downloadRemoteAsset
	originalIntegrate := integrateLocalApp
	originalAddSingle := addSingleApp
	t.Cleanup(func() {
		downloadRemoteAsset = originalDownload
		integrateLocalApp = originalIntegrate
		addSingleApp = originalAddSingle
	})

	downloadRemoteAsset = func(context.Context, string, string, bool) error {
		return nil
	}
	integrateLocalApp = func(context.Context, string, core.UpdateOverwritePrompt) (*models.App, error) {
		return &models.App{
			ID:        "my-app",
			Name:      "My App",
			Version:   "1.0.0",
			UpdatedAt: "2026-03-08T12:00:00Z",
		}, nil
	}
	addSingleApp = repo.AddApp

	output := captureStdout(t, func() {
		if err := runInstallCommand(context.Background(), []string{"https://example.com/MyApp.AppImage"}); err != nil {
			t.Fatalf("runInstallCommand returned error: %v", err)
		}
	})

	if !strings.Contains(output, "No SHA-256 provided; skipping checksum verification") {
		t.Fatalf("expected checksum warning, got:\n%s", output)
	}
}

func TestAddCmdDirectURLWithChecksum(t *testing.T) {
	tempDir := t.TempDir()
	setupAddCommandConfigForTest(t, tempDir)

	originalDownload := downloadRemoteAsset
	originalIntegrate := integrateLocalApp
	originalAddSingle := addSingleApp
	t.Cleanup(func() {
		downloadRemoteAsset = originalDownload
		integrateLocalApp = originalIntegrate
		addSingleApp = originalAddSingle
	})

	payload := []byte("remote-appimage")
	sum := sha256.Sum256(payload)
	expectedSHA256 := hex.EncodeToString(sum[:])

	downloadRemoteAsset = func(ctx context.Context, assetURL, destination string, interactive bool) error {
		_ = ctx
		_ = interactive
		if assetURL != "https://example.com/MyApp.AppImage" {
			t.Fatalf("assetURL = %q", assetURL)
		}
		return os.WriteFile(destination, payload, 0o644)
	}
	integrateLocalApp = func(context.Context, string, core.UpdateOverwritePrompt) (*models.App, error) {
		return &models.App{ID: "my-app", Name: "My App", Version: "1.0.0", UpdatedAt: "2026-03-08T12:00:00Z"}, nil
	}
	addSingleApp = repo.AddApp

	output := captureStdout(t, func() {
		if err := runAddCommand(context.Background(), []string{"https://example.com/MyApp.AppImage", "--sha256", expectedSHA256}); err != nil {
			t.Fatalf("runAddCommand returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Installed: My App v1.0.0 [my-app]") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestInstallCmdDirectURLChecksumMismatch(t *testing.T) {
	tempDir := t.TempDir()
	setupAddCommandConfigForTest(t, tempDir)

	originalDownload := downloadRemoteAsset
	originalIntegrate := integrateLocalApp
	t.Cleanup(func() {
		downloadRemoteAsset = originalDownload
		integrateLocalApp = originalIntegrate
	})

	downloadRemoteAsset = func(ctx context.Context, assetURL, destination string, interactive bool) error {
		_ = ctx
		_ = assetURL
		_ = interactive
		return os.WriteFile(destination, []byte("payload"), 0o644)
	}

	var calls int32
	integrateLocalApp = func(context.Context, string, core.UpdateOverwritePrompt) (*models.App, error) {
		atomic.AddInt32(&calls, 1)
		return &models.App{ID: "my-app"}, nil
	}

	err := runInstallCommand(context.Background(), []string{"https://example.com/MyApp.AppImage", "--sha256", strings.Repeat("a", 64)})
	if err == nil {
		t.Fatal("expected checksum mismatch")
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("integrateLocalApp calls = %d, want 0", calls)
	}
}

func TestInstallCmdGitHubSetsDefaultAssetSourceAndUpdate(t *testing.T) {
	tempDir := t.TempDir()
	setupAddCommandConfigForTest(t, tempDir)

	originalBackends := discoveryBackends
	originalResolve := resolveGitHubReleaseAsset
	originalDownload := downloadRemoteAsset
	originalIntegrate := integrateLocalApp
	originalAddSingle := addSingleApp
	t.Cleanup(func() {
		discoveryBackends = originalBackends
		resolveGitHubReleaseAsset = originalResolve
		downloadRemoteAsset = originalDownload
		integrateLocalApp = originalIntegrate
		addSingleApp = originalAddSingle
	})

	discoveryBackends = func() []discovery.DiscoveryBackend {
		return []discovery.DiscoveryBackend{
			&stubDiscoveryBackend{
				name: "GitHub",
				resolveFn: func(context.Context, discovery.PackageRef, string) (*discovery.PackageMetadata, error) {
					return &discovery.PackageMetadata{
						Name:          "My App",
						Provider:      "GitHub",
						Ref:           discovery.PackageRef{Kind: discovery.ProviderGitHub, ProviderRef: "owner/repo"},
						LatestVersion: "1.2.3",
						AssetName:     "MyApp-x86_64.AppImage",
						AssetPattern:  "*.AppImage",
						DownloadURL:   "https://example.com/MyApp-x86_64.AppImage",
						Installable:   true,
						ReleaseTag:    "v1.2.3",
					}, nil
				},
			},
		}
	}
	resolveGitHubReleaseAsset = func(repoSlug, assetPattern string) (*core.GitHubReleaseAsset, error) {
		if repoSlug != "owner/repo" {
			t.Fatalf("repoSlug = %q", repoSlug)
		}
		if assetPattern != "*.AppImage" {
			t.Fatalf("assetPattern = %q", assetPattern)
		}
		return &core.GitHubReleaseAsset{
			DownloadURL: "https://example.com/MyApp-x86_64.AppImage",
			TagName:     "v1.2.3",
			AssetName:   "MyApp-x86_64.AppImage",
		}, nil
	}
	downloadRemoteAsset = func(context.Context, string, string, bool) error {
		return nil
	}
	integrateLocalApp = func(context.Context, string, core.UpdateOverwritePrompt) (*models.App, error) {
		return &models.App{
			ID:        "my-app",
			Name:      "My App",
			Version:   "1.2.3",
			UpdatedAt: "2026-03-08T12:00:00Z",
		}, nil
	}
	addSingleApp = repo.AddApp

	if err := runInstallCommand(context.Background(), []string{"github:owner/repo"}); err != nil {
		t.Fatalf("runInstallCommand returned error: %v", err)
	}

	app, err := repo.GetApp("my-app")
	if err != nil {
		t.Fatalf("failed to load persisted app: %v", err)
	}
	if app.Source.Kind != models.SourceGitHubRelease {
		t.Fatalf("Source.Kind = %q", app.Source.Kind)
	}
	if app.Source.GitHubRelease == nil || app.Source.GitHubRelease.Asset != "*.AppImage" || app.Source.GitHubRelease.Tag != "v1.2.3" {
		t.Fatalf("unexpected github source: %#v", app.Source.GitHubRelease)
	}
	if app.Update == nil || app.Update.Kind != models.UpdateGitHubRelease || app.Update.GitHubRelease == nil {
		t.Fatalf("unexpected update source: %#v", app.Update)
	}
	if app.Update.GitHubRelease.Asset != "*.AppImage" {
		t.Fatalf("update asset = %q", app.Update.GitHubRelease.Asset)
	}
}

func TestInstallCmdGitHubUsesCustomAsset(t *testing.T) {
	tempDir := t.TempDir()
	setupAddCommandConfigForTest(t, tempDir)

	originalBackends := discoveryBackends
	originalResolve := resolveGitHubReleaseAsset
	originalDownload := downloadRemoteAsset
	originalIntegrate := integrateLocalApp
	originalAddSingle := addSingleApp
	t.Cleanup(func() {
		discoveryBackends = originalBackends
		resolveGitHubReleaseAsset = originalResolve
		downloadRemoteAsset = originalDownload
		integrateLocalApp = originalIntegrate
		addSingleApp = originalAddSingle
	})

	discoveryBackends = func() []discovery.DiscoveryBackend {
		return []discovery.DiscoveryBackend{
			&stubDiscoveryBackend{
				name: "GitHub",
				resolveFn: func(context.Context, discovery.PackageRef, string) (*discovery.PackageMetadata, error) {
					return &discovery.PackageMetadata{
						Name:          "My App",
						Provider:      "GitHub",
						Ref:           discovery.PackageRef{Kind: discovery.ProviderGitHub, ProviderRef: "owner/repo"},
						LatestVersion: "1.2.3",
						AssetName:     "MyApp-x86_64.AppImage",
						AssetPattern:  "MyApp-*-x86_64.AppImage",
						DownloadURL:   "https://example.com/MyApp-x86_64.AppImage",
						Installable:   true,
						ReleaseTag:    "v1.2.3",
					}, nil
				},
			},
		}
	}
	resolveGitHubReleaseAsset = func(repoSlug, assetPattern string) (*core.GitHubReleaseAsset, error) {
		if assetPattern != "MyApp-*-x86_64.AppImage" {
			t.Fatalf("assetPattern = %q", assetPattern)
		}
		return &core.GitHubReleaseAsset{
			DownloadURL: "https://example.com/MyApp-x86_64.AppImage",
			TagName:     "v1.2.3",
			AssetName:   "MyApp-x86_64.AppImage",
		}, nil
	}
	downloadRemoteAsset = func(context.Context, string, string, bool) error { return nil }
	integrateLocalApp = func(context.Context, string, core.UpdateOverwritePrompt) (*models.App, error) {
		return &models.App{ID: "my-app", Name: "My App", Version: "1.2.3", UpdatedAt: "2026-03-08T12:00:00Z"}, nil
	}
	addSingleApp = repo.AddApp

	if err := runInstallCommand(context.Background(), []string{"github:owner/repo", "--asset", "MyApp-*-x86_64.AppImage"}); err != nil {
		t.Fatalf("runInstallCommand returned error: %v", err)
	}

	app, err := repo.GetApp("my-app")
	if err != nil {
		t.Fatalf("failed to load persisted app: %v", err)
	}
	if app.Source.GitHubRelease == nil || app.Source.GitHubRelease.Asset != "MyApp-*-x86_64.AppImage" {
		t.Fatalf("unexpected github source: %#v", app.Source.GitHubRelease)
	}
}

func TestAddCmdGitHubUsesCustomAsset(t *testing.T) {
	tempDir := t.TempDir()
	setupAddCommandConfigForTest(t, tempDir)

	originalBackends := discoveryBackends
	originalResolve := resolveGitHubReleaseAsset
	originalDownload := downloadRemoteAsset
	originalIntegrate := integrateLocalApp
	originalAddSingle := addSingleApp
	t.Cleanup(func() {
		discoveryBackends = originalBackends
		resolveGitHubReleaseAsset = originalResolve
		downloadRemoteAsset = originalDownload
		integrateLocalApp = originalIntegrate
		addSingleApp = originalAddSingle
	})

	discoveryBackends = func() []discovery.DiscoveryBackend {
		return []discovery.DiscoveryBackend{
			&stubDiscoveryBackend{
				name: "GitHub",
				resolveFn: func(context.Context, discovery.PackageRef, string) (*discovery.PackageMetadata, error) {
					return &discovery.PackageMetadata{
						Name:          "My App",
						Provider:      "GitHub",
						Ref:           discovery.PackageRef{Kind: discovery.ProviderGitHub, ProviderRef: "owner/repo"},
						LatestVersion: "1.2.3",
						AssetName:     "MyApp-x86_64.AppImage",
						AssetPattern:  "MyApp-*-x86_64.AppImage",
						DownloadURL:   "https://example.com/MyApp-x86_64.AppImage",
						Installable:   true,
						ReleaseTag:    "v1.2.3",
					}, nil
				},
			},
		}
	}
	resolveGitHubReleaseAsset = func(repoSlug, assetPattern string) (*core.GitHubReleaseAsset, error) {
		if assetPattern != "MyApp-*-x86_64.AppImage" {
			t.Fatalf("assetPattern = %q", assetPattern)
		}
		return &core.GitHubReleaseAsset{
			DownloadURL: "https://example.com/MyApp-x86_64.AppImage",
			TagName:     "v1.2.3",
			AssetName:   "MyApp-x86_64.AppImage",
		}, nil
	}
	downloadRemoteAsset = func(context.Context, string, string, bool) error { return nil }
	integrateLocalApp = func(context.Context, string, core.UpdateOverwritePrompt) (*models.App, error) {
		return &models.App{ID: "my-app", Name: "My App", Version: "1.2.3", UpdatedAt: "2026-03-08T12:00:00Z"}, nil
	}
	addSingleApp = repo.AddApp

	output := captureStdout(t, func() {
		if err := runAddCommand(context.Background(), []string{"github:owner/repo", "--asset", "MyApp-*-x86_64.AppImage"}); err != nil {
			t.Fatalf("runAddCommand returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Installed: My App v1.2.3 [my-app]") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestInstallCmdGitLabSetsDefaultAssetSourceAndUpdate(t *testing.T) {
	tempDir := t.TempDir()
	setupAddCommandConfigForTest(t, tempDir)

	originalBackends := discoveryBackends
	originalResolve := resolveGitLabReleaseAsset
	originalDownload := downloadRemoteAsset
	originalIntegrate := integrateLocalApp
	originalAddSingle := addSingleApp
	t.Cleanup(func() {
		discoveryBackends = originalBackends
		resolveGitLabReleaseAsset = originalResolve
		downloadRemoteAsset = originalDownload
		integrateLocalApp = originalIntegrate
		addSingleApp = originalAddSingle
	})

	discoveryBackends = func() []discovery.DiscoveryBackend {
		return []discovery.DiscoveryBackend{
			&stubDiscoveryBackend{
				name: "GitLab",
				resolveFn: func(context.Context, discovery.PackageRef, string) (*discovery.PackageMetadata, error) {
					return &discovery.PackageMetadata{
						Name:          "My App",
						Provider:      "GitLab",
						Ref:           discovery.PackageRef{Kind: discovery.ProviderGitLab, ProviderRef: "group/project"},
						LatestVersion: "2.0.0",
						AssetName:     "MyApp-x86_64.AppImage",
						AssetPattern:  "*.AppImage",
						DownloadURL:   "https://example.com/MyApp-x86_64.AppImage",
						Installable:   true,
						ReleaseTag:    "v2.0.0",
					}, nil
				},
			},
		}
	}
	resolveGitLabReleaseAsset = func(project, assetPattern string) (*core.GitLabReleaseAsset, error) {
		if project != "group/project" {
			t.Fatalf("project = %q", project)
		}
		if assetPattern != "*.AppImage" {
			t.Fatalf("assetPattern = %q", assetPattern)
		}
		return &core.GitLabReleaseAsset{
			DownloadURL: "https://example.com/MyApp-x86_64.AppImage",
			TagName:     "v2.0.0",
			AssetName:   "MyApp-x86_64.AppImage",
		}, nil
	}
	downloadRemoteAsset = func(context.Context, string, string, bool) error { return nil }
	integrateLocalApp = func(context.Context, string, core.UpdateOverwritePrompt) (*models.App, error) {
		return &models.App{ID: "my-app", Name: "My App", Version: "2.0.0", UpdatedAt: "2026-03-08T12:00:00Z"}, nil
	}
	addSingleApp = repo.AddApp

	if err := runInstallCommand(context.Background(), []string{"gitlab:group/project"}); err != nil {
		t.Fatalf("runInstallCommand returned error: %v", err)
	}

	app, err := repo.GetApp("my-app")
	if err != nil {
		t.Fatalf("failed to load persisted app: %v", err)
	}
	if app.Source.Kind != models.SourceGitLabRelease {
		t.Fatalf("Source.Kind = %q", app.Source.Kind)
	}
	if app.Source.GitLabRelease == nil || app.Source.GitLabRelease.Asset != "*.AppImage" || app.Source.GitLabRelease.Tag != "v2.0.0" {
		t.Fatalf("unexpected gitlab source: %#v", app.Source.GitLabRelease)
	}
	if app.Update == nil || app.Update.Kind != models.UpdateGitLabRelease || app.Update.GitLabRelease == nil {
		t.Fatalf("unexpected update source: %#v", app.Update)
	}
}

func TestInstallCmdGitLabUsesCustomAsset(t *testing.T) {
	tempDir := t.TempDir()
	setupAddCommandConfigForTest(t, tempDir)

	originalBackends := discoveryBackends
	originalResolve := resolveGitLabReleaseAsset
	originalDownload := downloadRemoteAsset
	originalIntegrate := integrateLocalApp
	originalAddSingle := addSingleApp
	t.Cleanup(func() {
		discoveryBackends = originalBackends
		resolveGitLabReleaseAsset = originalResolve
		downloadRemoteAsset = originalDownload
		integrateLocalApp = originalIntegrate
		addSingleApp = originalAddSingle
	})

	discoveryBackends = func() []discovery.DiscoveryBackend {
		return []discovery.DiscoveryBackend{
			&stubDiscoveryBackend{
				name: "GitLab",
				resolveFn: func(context.Context, discovery.PackageRef, string) (*discovery.PackageMetadata, error) {
					return &discovery.PackageMetadata{
						Name:          "My App",
						Provider:      "GitLab",
						Ref:           discovery.PackageRef{Kind: discovery.ProviderGitLab, ProviderRef: "group/project"},
						LatestVersion: "2.0.0",
						AssetName:     "MyApp-x86_64.AppImage",
						AssetPattern:  "MyApp-*-x86_64.AppImage",
						DownloadURL:   "https://example.com/MyApp-x86_64.AppImage",
						Installable:   true,
						ReleaseTag:    "v2.0.0",
					}, nil
				},
			},
		}
	}
	resolveGitLabReleaseAsset = func(project, assetPattern string) (*core.GitLabReleaseAsset, error) {
		if assetPattern != "MyApp-*-x86_64.AppImage" {
			t.Fatalf("assetPattern = %q", assetPattern)
		}
		return &core.GitLabReleaseAsset{
			DownloadURL: "https://example.com/MyApp-x86_64.AppImage",
			TagName:     "v2.0.0",
			AssetName:   "MyApp-x86_64.AppImage",
		}, nil
	}
	downloadRemoteAsset = func(context.Context, string, string, bool) error { return nil }
	integrateLocalApp = func(context.Context, string, core.UpdateOverwritePrompt) (*models.App, error) {
		return &models.App{ID: "my-app", Name: "My App", Version: "2.0.0", UpdatedAt: "2026-03-08T12:00:00Z"}, nil
	}
	addSingleApp = repo.AddApp

	if err := runInstallCommand(context.Background(), []string{"gitlab:group/project", "--asset", "MyApp-*-x86_64.AppImage"}); err != nil {
		t.Fatalf("runInstallCommand returned error: %v", err)
	}

	app, err := repo.GetApp("my-app")
	if err != nil {
		t.Fatalf("failed to load persisted app: %v", err)
	}
	if app.Source.GitLabRelease == nil || app.Source.GitLabRelease.Asset != "MyApp-*-x86_64.AppImage" {
		t.Fatalf("unexpected gitlab source: %#v", app.Source.GitLabRelease)
	}
}

func TestAddCmdGitLabProviderRef(t *testing.T) {
	tempDir := t.TempDir()
	setupAddCommandConfigForTest(t, tempDir)

	originalBackends := discoveryBackends
	originalResolve := resolveGitLabReleaseAsset
	originalDownload := downloadRemoteAsset
	originalIntegrate := integrateLocalApp
	originalAddSingle := addSingleApp
	t.Cleanup(func() {
		discoveryBackends = originalBackends
		resolveGitLabReleaseAsset = originalResolve
		downloadRemoteAsset = originalDownload
		integrateLocalApp = originalIntegrate
		addSingleApp = originalAddSingle
	})

	discoveryBackends = func() []discovery.DiscoveryBackend {
		return []discovery.DiscoveryBackend{
			&stubDiscoveryBackend{
				name: "GitLab",
				resolveFn: func(context.Context, discovery.PackageRef, string) (*discovery.PackageMetadata, error) {
					return &discovery.PackageMetadata{
						Name:          "My App",
						Provider:      "GitLab",
						Ref:           discovery.PackageRef{Kind: discovery.ProviderGitLab, ProviderRef: "group/project"},
						LatestVersion: "2.0.0",
						AssetName:     "MyApp-x86_64.AppImage",
						AssetPattern:  "*.AppImage",
						DownloadURL:   "https://example.com/MyApp-x86_64.AppImage",
						Installable:   true,
						ReleaseTag:    "v2.0.0",
					}, nil
				},
			},
		}
	}
	resolveGitLabReleaseAsset = func(project, assetPattern string) (*core.GitLabReleaseAsset, error) {
		if project != "group/project" || assetPattern != "*.AppImage" {
			t.Fatalf("unexpected install resolution: %s %s", project, assetPattern)
		}
		return &core.GitLabReleaseAsset{
			DownloadURL: "https://example.com/MyApp-x86_64.AppImage",
			TagName:     "v2.0.0",
			AssetName:   "MyApp-x86_64.AppImage",
		}, nil
	}
	downloadRemoteAsset = func(context.Context, string, string, bool) error { return nil }
	integrateLocalApp = func(context.Context, string, core.UpdateOverwritePrompt) (*models.App, error) {
		return &models.App{ID: "my-app", Name: "My App", Version: "2.0.0", UpdatedAt: "2026-03-08T12:00:00Z"}, nil
	}
	addSingleApp = repo.AddApp

	output := captureStdout(t, func() {
		if err := runAddCommand(context.Background(), []string{"gitlab:group/project"}); err != nil {
			t.Fatalf("runAddCommand returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Installed: My App v2.0.0 [my-app]") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

type stubDiscoveryBackend struct {
	name      string
	resolveFn func(context.Context, discovery.PackageRef, string) (*discovery.PackageMetadata, error)
}

func (b *stubDiscoveryBackend) Name() string {
	return b.name
}

func (b *stubDiscoveryBackend) Resolve(ctx context.Context, ref discovery.PackageRef, asset string) (*discovery.PackageMetadata, error) {
	return b.resolveFn(ctx, ref, asset)
}

func TestShowCmdDirectProviderRef(t *testing.T) {
	originalBackends := discoveryBackends
	t.Cleanup(func() {
		discoveryBackends = originalBackends
	})

	discoveryBackends = func() []discovery.DiscoveryBackend {
		return []discovery.DiscoveryBackend{
			&stubDiscoveryBackend{
				name: "GitHub",
				resolveFn: func(context.Context, discovery.PackageRef, string) (*discovery.PackageMetadata, error) {
					return &discovery.PackageMetadata{
						Name:          "My App",
						Provider:      "GitHub",
						Ref:           discovery.PackageRef{Kind: discovery.ProviderGitHub, ProviderRef: "owner/repo"},
						RepoURL:       "https://github.com/owner/repo",
						Summary:       "A test app",
						LatestVersion: "1.2.3",
						AssetName:     "MyApp-x86_64.AppImage",
						AssetPattern:  "*.AppImage",
						Installable:   true,
					}, nil
				},
			},
		}
	}

	output := captureStdout(t, func() {
		if err := runShowCommand(context.Background(), []string{"github:owner/repo"}); err != nil {
			t.Fatalf("runShowCommand returned error: %v", err)
		}
	})

	if !strings.Contains(output, "My App") || !strings.Contains(strings.ToLower(output), "install command") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if !strings.Contains(output, "aim install github:owner/repo") {
		t.Fatalf("expected install preview, got:\n%s", output)
	}
	if !strings.Contains(output, "Managed updates: yes") {
		t.Fatalf("expected managed updates summary, got:\n%s", output)
	}
	if strings.Contains(output, "Notes") {
		t.Fatalf("did not expect Notes section, got:\n%s", output)
	}
	if strings.Contains(output, "Asset pattern:") {
		t.Fatalf("did not expect asset pattern, got:\n%s", output)
	}
	if strings.Contains(output, "github_release: owner/repo, asset: *.AppImage") {
		t.Fatalf("did not expect raw update summary, got:\n%s", output)
	}
}

func TestShowCmdGitLabProviderRefOutput(t *testing.T) {
	originalBackends := discoveryBackends
	t.Cleanup(func() {
		discoveryBackends = originalBackends
	})

	discoveryBackends = func() []discovery.DiscoveryBackend {
		return []discovery.DiscoveryBackend{
			&stubDiscoveryBackend{
				name: "GitLab",
				resolveFn: func(context.Context, discovery.PackageRef, string) (*discovery.PackageMetadata, error) {
					return &discovery.PackageMetadata{
						Name:          "Foo App",
						Provider:      "GitLab",
						Ref:           discovery.PackageRef{Kind: discovery.ProviderGitLab, ProviderRef: "group/project"},
						RepoURL:       "https://gitlab.com/group/project",
						Summary:       "A GitLab test app",
						LatestVersion: "2.0.0",
						AssetName:     "Foo-x86_64.AppImage",
						AssetPattern:  "Foo-*-x86_64.AppImage",
						Installable:   true,
					}, nil
				},
			},
		}
	}

	output := captureStdout(t, func() {
		if err := runShowCommand(context.Background(), []string{"gitlab:group/project"}); err != nil {
			t.Fatalf("runShowCommand returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Foo App") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if !strings.Contains(output, "Managed updates: yes") {
		t.Fatalf("expected managed updates summary, got:\n%s", output)
	}
	if !strings.Contains(output, "aim install gitlab:group/project") {
		t.Fatalf("expected install preview, got:\n%s", output)
	}
	if strings.Contains(output, "Notes") {
		t.Fatalf("did not expect Notes section, got:\n%s", output)
	}
	if strings.Contains(output, "Asset pattern:") {
		t.Fatalf("did not expect asset pattern, got:\n%s", output)
	}
	if strings.Contains(output, "gitlab_release: group/project, asset: Foo-*-x86_64.AppImage") {
		t.Fatalf("did not expect raw update summary, got:\n%s", output)
	}
}

func TestShowCmdUninstallablePackageOmitsInstallPreview(t *testing.T) {
	originalBackends := discoveryBackends
	t.Cleanup(func() {
		discoveryBackends = originalBackends
	})

	discoveryBackends = func() []discovery.DiscoveryBackend {
		return []discovery.DiscoveryBackend{
			&stubDiscoveryBackend{
				name: "GitHub",
				resolveFn: func(context.Context, discovery.PackageRef, string) (*discovery.PackageMetadata, error) {
					return &discovery.PackageMetadata{
						Name:          "My App",
						Provider:      "GitHub",
						Ref:           discovery.PackageRef{Kind: discovery.ProviderGitHub, ProviderRef: "owner/repo"},
						RepoURL:       "https://github.com/owner/repo",
						Summary:       "A test app",
						Installable:   false,
						InstallReason: "no matching release asset",
					}, nil
				},
			},
		}
	}

	output := captureStdout(t, func() {
		if err := runShowCommand(context.Background(), []string{"github:owner/repo"}); err != nil {
			t.Fatalf("runShowCommand returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Installable: no") {
		t.Fatalf("expected non-installable status, got:\n%s", output)
	}
	if !strings.Contains(output, "Reason: no matching release asset") {
		t.Fatalf("expected install reason, got:\n%s", output)
	}
	if strings.Contains(output, "Install Command") {
		t.Fatalf("did not expect install preview, got:\n%s", output)
	}
	if strings.Contains(output, "Managed updates:") {
		t.Fatalf("did not expect managed updates summary, got:\n%s", output)
	}
	if strings.Contains(output, "Latest release:") {
		t.Fatalf("did not expect latest release, got:\n%s", output)
	}
	if strings.Contains(output, "Selected asset:") {
		t.Fatalf("did not expect selected asset, got:\n%s", output)
	}
}

func TestInfoCmdGitHubPackageRef(t *testing.T) {
	originalBackends := discoveryBackends
	t.Cleanup(func() {
		discoveryBackends = originalBackends
	})

	discoveryBackends = func() []discovery.DiscoveryBackend {
		return []discovery.DiscoveryBackend{
			&stubDiscoveryBackend{
				name: "GitHub",
				resolveFn: func(context.Context, discovery.PackageRef, string) (*discovery.PackageMetadata, error) {
					return &discovery.PackageMetadata{
						Name:          "My App",
						Provider:      "GitHub",
						Ref:           discovery.PackageRef{Kind: discovery.ProviderGitHub, ProviderRef: "owner/repo"},
						RepoURL:       "https://github.com/owner/repo",
						Summary:       "A test app",
						LatestVersion: "1.2.3",
						AssetName:     "MyApp-x86_64.AppImage",
						AssetPattern:  "*.AppImage",
						Installable:   true,
					}, nil
				},
			},
		}
	}

	output := captureStdout(t, func() {
		if err := runInfoCommand(context.Background(), []string{"github:owner/repo"}); err != nil {
			t.Fatalf("runInfoCommand returned error: %v", err)
		}
	})

	if !strings.Contains(output, "My App") || !strings.Contains(output, "Managed updates: yes") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if !strings.Contains(output, "aim install github:owner/repo") {
		t.Fatalf("expected install preview, got:\n%s", output)
	}
}

func TestInfoCmdGitLabPackageRef(t *testing.T) {
	originalBackends := discoveryBackends
	t.Cleanup(func() {
		discoveryBackends = originalBackends
	})

	discoveryBackends = func() []discovery.DiscoveryBackend {
		return []discovery.DiscoveryBackend{
			&stubDiscoveryBackend{
				name: "GitLab",
				resolveFn: func(context.Context, discovery.PackageRef, string) (*discovery.PackageMetadata, error) {
					return &discovery.PackageMetadata{
						Name:          "Foo App",
						Provider:      "GitLab",
						Ref:           discovery.PackageRef{Kind: discovery.ProviderGitLab, ProviderRef: "group/project"},
						RepoURL:       "https://gitlab.com/group/project",
						Summary:       "A GitLab test app",
						LatestVersion: "2.0.0",
						AssetName:     "Foo-x86_64.AppImage",
						AssetPattern:  "Foo-*-x86_64.AppImage",
						Installable:   true,
					}, nil
				},
			},
		}
	}

	output := captureStdout(t, func() {
		if err := runInfoCommand(context.Background(), []string{"gitlab:group/project"}); err != nil {
			t.Fatalf("runInfoCommand returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Foo App") || !strings.Contains(output, "Managed updates: yes") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if !strings.Contains(output, "aim install gitlab:group/project") {
		t.Fatalf("expected install preview, got:\n%s", output)
	}
}

func TestInstallCmdDirectProviderRefDelegatesToExistingAddFlow(t *testing.T) {
	originalBackends := discoveryBackends
	originalResolve := resolveGitHubReleaseAsset
	originalDownload := downloadRemoteAsset
	originalIntegrate := integrateLocalApp
	originalAddSingle := addSingleApp
	t.Cleanup(func() {
		discoveryBackends = originalBackends
		resolveGitHubReleaseAsset = originalResolve
		downloadRemoteAsset = originalDownload
		integrateLocalApp = originalIntegrate
		addSingleApp = originalAddSingle
	})

	tempDir := t.TempDir()
	setupAddCommandConfigForTest(t, tempDir)

	discoveryBackends = func() []discovery.DiscoveryBackend {
		return []discovery.DiscoveryBackend{
			&stubDiscoveryBackend{
				name: "GitHub",
				resolveFn: func(context.Context, discovery.PackageRef, string) (*discovery.PackageMetadata, error) {
					return &discovery.PackageMetadata{
						Name:          "My App",
						Provider:      "GitHub",
						Ref:           discovery.PackageRef{Kind: discovery.ProviderGitHub, ProviderRef: "owner/repo"},
						LatestVersion: "1.2.3",
						AssetName:     "MyApp-x86_64.AppImage",
						AssetPattern:  "*.AppImage",
						DownloadURL:   "https://example.com/MyApp-x86_64.AppImage",
						Installable:   true,
						ReleaseTag:    "v1.2.3",
					}, nil
				},
			},
		}
	}
	resolveGitHubReleaseAsset = func(repoSlug, assetPattern string) (*core.GitHubReleaseAsset, error) {
		if repoSlug != "owner/repo" || assetPattern != "*.AppImage" {
			t.Fatalf("unexpected install resolution: %s %s", repoSlug, assetPattern)
		}
		return &core.GitHubReleaseAsset{
			DownloadURL: "https://example.com/MyApp-x86_64.AppImage",
			TagName:     "v1.2.3",
			AssetName:   "MyApp-x86_64.AppImage",
		}, nil
	}
	downloadRemoteAsset = func(context.Context, string, string, bool) error { return nil }
	integrateLocalApp = func(context.Context, string, core.UpdateOverwritePrompt) (*models.App, error) {
		return &models.App{ID: "my-app", Name: "My App", Version: "1.2.3", UpdatedAt: "2026-03-08T12:00:00Z"}, nil
	}
	addSingleApp = repo.AddApp

	if err := runInstallCommand(context.Background(), []string{"github:owner/repo"}); err != nil {
		t.Fatalf("runInstallCommand returned error: %v", err)
	}

	app, err := repo.GetApp("my-app")
	if err != nil {
		t.Fatalf("failed to load installed app: %v", err)
	}
	if app.Source.Kind != models.SourceGitHubRelease {
		t.Fatalf("Source.Kind = %q", app.Source.Kind)
	}
}

func TestInstallCmdDirectProviderRefAssetOverride(t *testing.T) {
	originalBackends := discoveryBackends
	originalResolve := resolveGitLabReleaseAsset
	originalDownload := downloadRemoteAsset
	originalIntegrate := integrateLocalApp
	originalAddSingle := addSingleApp
	t.Cleanup(func() {
		discoveryBackends = originalBackends
		resolveGitLabReleaseAsset = originalResolve
		downloadRemoteAsset = originalDownload
		integrateLocalApp = originalIntegrate
		addSingleApp = originalAddSingle
	})

	tempDir := t.TempDir()
	setupAddCommandConfigForTest(t, tempDir)

	discoveryBackends = func() []discovery.DiscoveryBackend {
		return []discovery.DiscoveryBackend{
			&stubDiscoveryBackend{
				name: "GitLab",
				resolveFn: func(context.Context, discovery.PackageRef, string) (*discovery.PackageMetadata, error) {
					return &discovery.PackageMetadata{
						Name:          "Foo App",
						Provider:      "GitLab",
						Ref:           discovery.PackageRef{Kind: discovery.ProviderGitLab, ProviderRef: "group/project"},
						LatestVersion: "2.0.0",
						AssetName:     "Foo-x86_64.AppImage",
						AssetPattern:  "Foo-*-x86_64.AppImage",
						DownloadURL:   "https://example.com/Foo-x86_64.AppImage",
						Installable:   true,
						ReleaseTag:    "v2.0.0",
					}, nil
				},
			},
		}
	}
	resolveGitLabReleaseAsset = func(project, assetPattern string) (*core.GitLabReleaseAsset, error) {
		if assetPattern != "Foo-*-x86_64.AppImage" {
			t.Fatalf("assetPattern = %q", assetPattern)
		}
		return &core.GitLabReleaseAsset{
			DownloadURL: "https://example.com/Foo-x86_64.AppImage",
			TagName:     "v2.0.0",
			AssetName:   "Foo-x86_64.AppImage",
		}, nil
	}
	downloadRemoteAsset = func(context.Context, string, string, bool) error { return nil }
	integrateLocalApp = func(context.Context, string, core.UpdateOverwritePrompt) (*models.App, error) {
		return &models.App{ID: "foo-app", Name: "Foo App", Version: "2.0.0", UpdatedAt: "2026-03-08T12:00:00Z"}, nil
	}
	addSingleApp = repo.AddApp

	if err := runInstallCommand(context.Background(), []string{"gitlab:group/project", "--asset", "Foo-*-x86_64.AppImage"}); err != nil {
		t.Fatalf("runInstallCommand returned error: %v", err)
	}

	app, err := repo.GetApp("foo-app")
	if err != nil {
		t.Fatalf("failed to load installed app: %v", err)
	}
	if app.Source.GitLabRelease == nil || app.Source.GitLabRelease.Asset != "Foo-*-x86_64.AppImage" {
		t.Fatalf("unexpected gitlab source: %#v", app.Source.GitLabRelease)
	}
}

func TestInstallCmdFailsForUninstallablePackage(t *testing.T) {
	originalBackends := discoveryBackends
	t.Cleanup(func() {
		discoveryBackends = originalBackends
	})

	discoveryBackends = func() []discovery.DiscoveryBackend {
		return []discovery.DiscoveryBackend{
			&stubDiscoveryBackend{
				name: "GitHub",
				resolveFn: func(context.Context, discovery.PackageRef, string) (*discovery.PackageMetadata, error) {
					return &discovery.PackageMetadata{
						Provider:      "GitHub",
						Ref:           discovery.PackageRef{Kind: discovery.ProviderGitHub, ProviderRef: "owner/repo"},
						Installable:   false,
						InstallReason: "no matching release asset",
					}, nil
				},
			},
		}
	}

	err := runInstallCommand(context.Background(), []string{"github:owner/repo"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "package is not installable") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestShowCmdRejectsNumericRef(t *testing.T) {
	err := runShowCommand(context.Background(), []string{"1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported package ref") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInfoCmdRejectsUnknownTarget(t *testing.T) {
	err := runInfoCommand(context.Background(), []string{"1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown info target") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "expected github:owner/repo, gitlab:namespace/project, <id>, or <path-to.AppImage>") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddCmdRejectsHTTPRemoteInput(t *testing.T) {
	err := runAddCommand(context.Background(), []string{"http://example.com/MyApp.AppImage"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "direct URLs must use https") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddCmdRejectsMalformedGitHubRef(t *testing.T) {
	err := runAddCommand(context.Background(), []string{"github:owner"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "github install source must be in the form github:owner/repo") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInstallCmdRejectsNumericRef(t *testing.T) {
	err := runInstallCommand(context.Background(), []string{"1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown install target") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInstallCmdRejectsLocalPath(t *testing.T) {
	err := runInstallCommand(context.Background(), []string{"./MyApp.AppImage"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "local AppImages are integrated with 'aim integrate <path-to.AppImage>'") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInstallCmdRejectsManagedID(t *testing.T) {
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
			"my-app": {ID: "my-app"},
		},
	}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	err := runInstallCommand(context.Background(), []string{"my-app"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "managed app IDs are reintegrated with 'aim integrate <id>'") {
		t.Fatalf("unexpected error: %v", err)
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

func newUpdateSetTestCommand(t *testing.T, values map[string]string) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{Use: "update"}
	addUpdateSharedFlags(cmd)

	for key, value := range values {
		if err := cmd.PersistentFlags().Set(key, value); err != nil {
			t.Fatalf("failed to set %s: %v", key, err)
		}
	}

	return cmd
}

func TestInspectCmdManagedShowsEmbeddedSource(t *testing.T) {
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
				ID:              "my-app",
				Name:            "My App",
				Version:         "1.0.0",
				ExecPath:        "/tmp/MyApp.AppImage",
				Update:          &models.UpdateSource{Kind: models.UpdateNone},
				UpdateAvailable: true,
				LatestVersion:   "1.1.0",
				LastCheckedAt:   "2026-03-16T12:00:00Z",
				Source: models.Source{
					Kind: models.SourceGitHubRelease,
					GitHubRelease: &models.GitHubReleaseSource{
						Repo:  "owner/repo",
						Asset: "*.AppImage",
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	originalUpdateInfo := getAppImageUpdateInfo
	t.Cleanup(func() {
		getAppImageUpdateInfo = originalUpdateInfo
	})
	getAppImageUpdateInfo = func(path string) (*core.UpdateInfo, error) {
		if path != "/tmp/MyApp.AppImage" {
			t.Fatalf("path = %q", path)
		}
		return &core.UpdateInfo{
			Kind:       models.UpdateZsync,
			UpdateInfo: "zsync|https://example.com/MyApp.AppImage.zsync",
			Transport:  "zsync",
		}, nil
	}

	output := captureStdout(t, func() {
		if err := runInspectCommand(context.Background(), []string{"my-app"}); err != nil {
			t.Fatalf("runInspectCommand returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Configured update source: none") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if !strings.Contains(output, "Embedded update source: zsync: zsync|https://example.com/MyApp.AppImage.zsync") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if strings.Contains(output, "Commands") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if strings.Contains(output, "Active update source:") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if strings.Contains(output, "Installed via:") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if strings.Contains(output, "Embedded source status:") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if strings.Contains(output, "Embedded source active:") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestInspectCmdManagedShowsMissingEmbeddedSource(t *testing.T) {
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
				ID:       "my-app",
				Name:     "My App",
				Version:  "1.0.0",
				ExecPath: "/tmp/MyApp.AppImage",
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

	originalUpdateInfo := getAppImageUpdateInfo
	t.Cleanup(func() {
		getAppImageUpdateInfo = originalUpdateInfo
	})
	getAppImageUpdateInfo = func(string) (*core.UpdateInfo, error) {
		return nil, fmt.Errorf("no update information found in ELF headers")
	}

	output := captureStdout(t, func() {
		if err := runInspectCommand(context.Background(), []string{"my-app"}); err != nil {
			t.Fatalf("runInspectCommand returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Embedded update source: none") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if strings.Contains(output, "Active update source:") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if strings.Contains(output, "Embedded source status:") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestInspectCmdLocalAppImage(t *testing.T) {
	originalRead := readAppImageInfo
	originalUpdateInfo := getAppImageUpdateInfo
	t.Cleanup(func() {
		readAppImageInfo = originalRead
		getAppImageUpdateInfo = originalUpdateInfo
	})

	readAppImageInfo = func(context.Context, string) (*core.AppInfo, error) {
		return &core.AppInfo{Name: "My App", ID: "my-app", Version: "1.2.3"}, nil
	}
	getAppImageUpdateInfo = func(path string) (*core.UpdateInfo, error) {
		if path != "./MyApp.AppImage" {
			t.Fatalf("path = %q", path)
		}
		return &core.UpdateInfo{
			Kind:       models.UpdateZsync,
			UpdateInfo: "gh-releases-zsync|owner|repo|latest|MyApp-*.AppImage.zsync",
			Transport:  "gh-releases",
		}, nil
	}

	output := captureStdout(t, func() {
		if err := runInspectCommand(context.Background(), []string{"./MyApp.AppImage"}); err != nil {
			t.Fatalf("runInspectCommand returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Path: ./MyApp.AppImage") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if !strings.Contains(output, "Name: My App") || !strings.Contains(output, "ID: my-app") || !strings.Contains(output, "Version: v1.2.3") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if !strings.Contains(output, "Embedded update source: zsync: gh-releases-zsync|owner|repo|latest|MyApp-*.AppImage.zsync") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if strings.Contains(output, "Embedded source status:") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestInfoCmdManagedApp(t *testing.T) {
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
				ID:              "my-app",
				Name:            "My App",
				Version:         "1.0.0",
				ExecPath:        "/tmp/MyApp.AppImage",
				UpdateAvailable: true,
				LatestVersion:   "1.1.0",
				LastCheckedAt:   "2026-03-17T12:00:00Z",
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

	originalUpdateInfo := getAppImageUpdateInfo
	t.Cleanup(func() {
		getAppImageUpdateInfo = originalUpdateInfo
	})
	getAppImageUpdateInfo = func(path string) (*core.UpdateInfo, error) {
		if path != "/tmp/MyApp.AppImage" {
			t.Fatalf("path = %q", path)
		}
		return &core.UpdateInfo{
			Kind:       models.UpdateZsync,
			UpdateInfo: "zsync|https://example.com/MyApp.AppImage.zsync",
			Transport:  "zsync",
		}, nil
	}

	output := captureStdout(t, func() {
		if err := runInfoCommand(context.Background(), []string{"my-app"}); err != nil {
			t.Fatalf("runInfoCommand returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Configured update source: github: owner/repo, asset: *.AppImage") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if !strings.Contains(output, "Embedded update source: zsync: zsync|https://example.com/MyApp.AppImage.zsync") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if !strings.Contains(output, "Latest known version: v1.1.0") || !strings.Contains(output, "Last checked: 2026-03-17T12:00:00Z") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestInfoCmdLocalAppImage(t *testing.T) {
	originalRead := readAppImageInfo
	originalUpdateInfo := getAppImageUpdateInfo
	t.Cleanup(func() {
		readAppImageInfo = originalRead
		getAppImageUpdateInfo = originalUpdateInfo
	})

	readAppImageInfo = func(context.Context, string) (*core.AppInfo, error) {
		return &core.AppInfo{Name: "My App", ID: "my-app", Version: "1.2.3"}, nil
	}
	getAppImageUpdateInfo = func(path string) (*core.UpdateInfo, error) {
		if path != "./MyApp.AppImage" {
			t.Fatalf("path = %q", path)
		}
		return &core.UpdateInfo{
			Kind:       models.UpdateZsync,
			UpdateInfo: "gh-releases-zsync|owner|repo|latest|MyApp-*.AppImage.zsync",
			Transport:  "gh-releases",
		}, nil
	}

	output := captureStdout(t, func() {
		if err := runInfoCommand(context.Background(), []string{"./MyApp.AppImage"}); err != nil {
			t.Fatalf("runInfoCommand returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Path: ./MyApp.AppImage") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if !strings.Contains(output, "Embedded update source: zsync: gh-releases-zsync|owner|repo|latest|MyApp-*.AppImage.zsync") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestInfoCmdArgumentValidation(t *testing.T) {
	err := runInfoCommand(context.Background(), nil)
	if err == nil {
		t.Fatal("expected missing argument error")
	}
	if !strings.Contains(err.Error(), "missing required argument <target>") {
		t.Fatalf("unexpected error: %v", err)
	}

	err = runInfoCommand(context.Background(), []string{"a", "b"})
	if err == nil {
		t.Fatal("expected too many arguments error")
	}
	if !strings.Contains(err.Error(), "too many arguments") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateSetEmbeddedSetsEmbeddedSource(t *testing.T) {
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
				ID:       "my-app",
				Name:     "My App",
				ExecPath: "/tmp/MyApp.AppImage",
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

	originalUpdateInfo := getAppImageUpdateInfo
	t.Cleanup(func() {
		getAppImageUpdateInfo = originalUpdateInfo
	})
	getAppImageUpdateInfo = func(string) (*core.UpdateInfo, error) {
		return &core.UpdateInfo{
			Kind:       models.UpdateZsync,
			UpdateInfo: "zsync|https://example.com/MyApp.AppImage.zsync",
			Transport:  "zsync",
		}, nil
	}

	output := captureStdoutWithInput(t, "y\n", func() {
		if err := runUpdateSetCommand(context.Background(), []string{"my-app", "--embedded"}); err != nil {
			t.Fatalf("runUpdateSetCommand returned error: %v", err)
		}
	})

	app, err := repo.GetApp("my-app")
	if err != nil {
		t.Fatalf("failed to reload app: %v", err)
	}
	if app.Update == nil || app.Update.Kind != models.UpdateZsync {
		t.Fatalf("unexpected update source: %#v", app.Update)
	}
	if !strings.Contains(output, "Update source set: zsync: zsync|https://example.com/MyApp.AppImage.zsync") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestUpdateSetEmbeddedMissingPromptsToUnsetOrKeep(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "apps.json")

	originalDbSrc := config.DbSrc
	config.DbSrc = dbPath
	t.Cleanup(func() {
		config.DbSrc = originalDbSrc
	})

	originalUpdateInfo := getAppImageUpdateInfo
	t.Cleanup(func() {
		getAppImageUpdateInfo = originalUpdateInfo
	})
	getAppImageUpdateInfo = func(string) (*core.UpdateInfo, error) {
		return nil, fmt.Errorf("no update information found in ELF headers")
	}

	writeDB := func(update *models.UpdateSource) {
		t.Helper()
		if err := repo.SaveDB(dbPath, &repo.DB{
			SchemaVersion: 1,
			Apps: map[string]*models.App{
				"my-app": {
					ID:       "my-app",
					Name:     "My App",
					ExecPath: "/tmp/MyApp.AppImage",
					Update:   update,
				},
			},
		}); err != nil {
			t.Fatalf("failed to write test DB: %v", err)
		}
	}

	writeDB(&models.UpdateSource{
		Kind: models.UpdateGitHubRelease,
		GitHubRelease: &models.GitHubReleaseUpdateSource{
			Repo:  "owner/repo",
			Asset: "*.AppImage",
		},
	})

	outputKeep := captureStdoutWithInput(t, "n\n", func() {
		if err := runUpdateSetCommand(context.Background(), []string{"my-app", "--embedded"}); err != nil {
			t.Fatalf("runUpdateSetCommand returned error: %v", err)
		}
	})

	appKeep, err := repo.GetApp("my-app")
	if err != nil {
		t.Fatalf("failed to reload app: %v", err)
	}
	if appKeep.Update == nil || appKeep.Update.Kind != models.UpdateGitHubRelease {
		t.Fatalf("unexpected update source after keep: %#v", appKeep.Update)
	}
	if !strings.Contains(outputKeep, "No embedded update source found in the current AppImage.") {
		t.Fatalf("unexpected output:\n%s", outputKeep)
	}
	if !strings.Contains(outputKeep, "Unset current update source github: owner/repo, asset: *.AppImage for my-app? [y/N]: ") {
		t.Fatalf("unexpected output:\n%s", outputKeep)
	}
	if !strings.Contains(outputKeep, "Update source unchanged") {
		t.Fatalf("unexpected output:\n%s", outputKeep)
	}

	writeDB(&models.UpdateSource{
		Kind: models.UpdateGitHubRelease,
		GitHubRelease: &models.GitHubReleaseUpdateSource{
			Repo:  "owner/repo",
			Asset: "*.AppImage",
		},
	})

	outputUnset := captureStdoutWithInput(t, "y\n", func() {
		if err := runUpdateSetCommand(context.Background(), []string{"my-app", "--embedded"}); err != nil {
			t.Fatalf("runUpdateSetCommand returned error: %v", err)
		}
	})

	appUnset, err := repo.GetApp("my-app")
	if err != nil {
		t.Fatalf("failed to reload app: %v", err)
	}
	if appUnset.Update == nil || appUnset.Update.Kind != models.UpdateNone {
		t.Fatalf("unexpected update source after unset: %#v", appUnset.Update)
	}
	if !strings.Contains(outputUnset, "Update source unset") {
		t.Fatalf("unexpected output:\n%s", outputUnset)
	}
}

func TestUpdateSetEmbeddedMissingWithoutConfiguredSource(t *testing.T) {
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
				ID:       "my-app",
				Name:     "My App",
				ExecPath: "/tmp/MyApp.AppImage",
				Update:   &models.UpdateSource{Kind: models.UpdateNone},
			},
		},
	}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	originalUpdateInfo := getAppImageUpdateInfo
	t.Cleanup(func() {
		getAppImageUpdateInfo = originalUpdateInfo
	})
	getAppImageUpdateInfo = func(string) (*core.UpdateInfo, error) {
		return nil, fmt.Errorf("no update information found in ELF headers")
	}

	output := captureStdout(t, func() {
		if err := runUpdateSetCommand(context.Background(), []string{"my-app", "--embedded"}); err != nil {
			t.Fatalf("runUpdateSetCommand returned error: %v", err)
		}
	})

	if !strings.Contains(output, "No embedded update source found in the current AppImage.") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if strings.Contains(output, "Update source unchanged") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestUpdateUnsetCommand(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "apps.json")

	originalDbSrc := config.DbSrc
	config.DbSrc = dbPath
	t.Cleanup(func() {
		config.DbSrc = originalDbSrc
	})

	writeDB := func(update *models.UpdateSource) {
		t.Helper()
		if err := repo.SaveDB(dbPath, &repo.DB{
			SchemaVersion: 1,
			Apps: map[string]*models.App{
				"my-app": {
					ID:     "my-app",
					Name:   "My App",
					Update: update,
				},
			},
		}); err != nil {
			t.Fatalf("failed to write test DB: %v", err)
		}
	}

	writeDB(&models.UpdateSource{
		Kind: models.UpdateGitHubRelease,
		GitHubRelease: &models.GitHubReleaseUpdateSource{
			Repo:  "owner/repo",
			Asset: "*.AppImage",
		},
	})

	outputKeep := captureStdoutWithInput(t, "n\n", func() {
		if err := runUpdateUnsetCommand(context.Background(), []string{"my-app"}); err != nil {
			t.Fatalf("runUpdateUnsetCommand returned error: %v", err)
		}
	})
	if !strings.Contains(outputKeep, "Unset update source for my-app? [y/N]: ") {
		t.Fatalf("unexpected output:\n%s", outputKeep)
	}
	if !strings.Contains(outputKeep, "Update source unchanged") {
		t.Fatalf("unexpected output:\n%s", outputKeep)
	}

	writeDB(&models.UpdateSource{
		Kind: models.UpdateGitHubRelease,
		GitHubRelease: &models.GitHubReleaseUpdateSource{
			Repo:  "owner/repo",
			Asset: "*.AppImage",
		},
	})

	outputUnset := captureStdoutWithInput(t, "y\n", func() {
		if err := runUpdateUnsetCommand(context.Background(), []string{"my-app"}); err != nil {
			t.Fatalf("runUpdateUnsetCommand returned error: %v", err)
		}
	})
	app, err := repo.GetApp("my-app")
	if err != nil {
		t.Fatalf("failed to reload app: %v", err)
	}
	if app.Update == nil || app.Update.Kind != models.UpdateNone {
		t.Fatalf("unexpected update source: %#v", app.Update)
	}
	if !strings.Contains(outputUnset, "Update source unset") {
		t.Fatalf("unexpected output:\n%s", outputUnset)
	}

	writeDB(&models.UpdateSource{Kind: models.UpdateNone})
	outputNoSource := captureStdout(t, func() {
		if err := runUpdateUnsetCommand(context.Background(), []string{"my-app"}); err != nil {
			t.Fatalf("runUpdateUnsetCommand returned error: %v", err)
		}
	})
	if !strings.Contains(outputNoSource, "No update source configured for my-app") {
		t.Fatalf("unexpected output:\n%s", outputNoSource)
	}
}

func newRootTestCommand() *cobra.Command {
	cmd := newRootCommand("test")
	cmd.SetVersionTemplate("{{.Name}} {{.Version}}\n")
	return cmd
}

func TestNewRootCommandMetadata(t *testing.T) {
	cmd := newRootCommand("v1.2.3")

	if strings.TrimSpace(cmd.Long) == "" {
		t.Fatal("expected root command description")
	}
	if got := rootCommandAuthor; got != "Sebastian Lobbe <slobbe@lobbe.cc>" {
		t.Fatalf("unexpected root command author: %q", got)
	}
	if strings.TrimSpace(rootCommandCopyright) == "" {
		t.Fatal("expected root command copyright")
	}
}

func TestRenderManPageIncludesMetadata(t *testing.T) {
	got, err := renderManPage(newRootCommand("v1.2.3"), 1)
	if err != nil {
		t.Fatalf("failed to generate man page: %v", err)
	}

	for _, expected := range []string{
		".SH VERSION",
		".SH AUTHOR",
		".SH COPYRIGHT",
		"v1.2.3",
		"Sebastian Lobbe <slobbe@lobbe.cc>",
		"Copyright (c) 2025 Sebastian Lobbe",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("generated man page missing %q:\n%s", expected, got)
		}
	}
}

func TestGeneratedManPageIsCurrent(t *testing.T) {
	got, err := renderManPage(newRootCommand(version), 1)
	if err != nil {
		t.Fatalf("failed to generate man page: %v", err)
	}

	wantBytes, err := os.ReadFile(filepath.Join("..", "..", "docs", "aim.1"))
	if err != nil {
		t.Fatalf("failed to read committed man page: %v", err)
	}

	if got != string(wantBytes) {
		t.Fatal("generated man page is stale; run `go run -tags docgen ./cmd/aim`")
	}
}

func TestRenderShellCompletions(t *testing.T) {
	root := newRootCommand("v1.2.3")

	bashCompletion, err := renderBashCompletion(root)
	if err != nil {
		t.Fatalf("failed to render bash completion: %v", err)
	}
	if !strings.Contains(bashCompletion, "aim") {
		t.Fatalf("unexpected bash completion output:\n%s", bashCompletion)
	}

	zshCompletion, err := renderZshCompletion(root)
	if err != nil {
		t.Fatalf("failed to render zsh completion: %v", err)
	}
	if !strings.Contains(zshCompletion, "#compdef aim") {
		t.Fatalf("unexpected zsh completion output:\n%s", zshCompletion)
	}

	fishCompletion, err := renderFishCompletion(root)
	if err != nil {
		t.Fatalf("failed to render fish completion: %v", err)
	}
	if !strings.Contains(fishCompletion, "complete -c aim") {
		t.Fatalf("unexpected fish completion output:\n%s", fishCompletion)
	}
}

func TestWriteCompletionFiles(t *testing.T) {
	outputDir := t.TempDir()

	if err := writeCompletionFiles(newRootCommand(version), outputDir); err != nil {
		t.Fatalf("writeCompletionFiles returned error: %v", err)
	}

	tests := []struct {
		path            string
		expectedSnippet string
	}{
		{path: bashCompletionRelativePath, expectedSnippet: "aim"},
		{path: zshCompletionRelativePath, expectedSnippet: "#compdef aim"},
		{path: fishCompletionRelativePath, expectedSnippet: "complete -c aim"},
	}

	for _, tt := range tests {
		content, err := os.ReadFile(filepath.Join(outputDir, tt.path))
		if err != nil {
			t.Fatalf("failed to read generated completion %s: %v", tt.path, err)
		}
		if !strings.Contains(string(content), tt.expectedSnippet) {
			t.Fatalf("generated completion %s missing %q:\n%s", tt.path, tt.expectedSnippet, string(content))
		}
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

	cmd := newManagedUpdateTestCommand(t, nil)
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

	cmd := newManagedUpdateTestCommand(t, nil)
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

	cmd := newManagedUpdateTestCommand(t, nil)
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

	cmd := newManagedUpdateTestCommand(t, map[string]string{"check-only": "true"})
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

	cmd := newManagedUpdateTestCommand(t, map[string]string{"check-only": "true"})
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

	cmd := newManagedUpdateTestCommand(t, nil)
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

	cmd := newManagedUpdateTestCommand(t, nil)
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

	runGitHubReleaseUpdateCheck = func(update *models.UpdateSource, currentVersion, localSHA1 string) (*core.GitHubReleaseUpdate, error) {
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

	runGitHubReleaseUpdateCheck = func(update *models.UpdateSource, currentVersion, localSHA1 string) (*core.GitHubReleaseUpdate, error) {
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

func TestCheckAppUpdateGitHubPropagatesZsyncTransport(t *testing.T) {
	originalCheck := runGitHubReleaseUpdateCheck
	t.Cleanup(func() {
		runGitHubReleaseUpdateCheck = originalCheck
	})

	runGitHubReleaseUpdateCheck = func(update *models.UpdateSource, currentVersion, localSHA1 string) (*core.GitHubReleaseUpdate, error) {
		if currentVersion != "3.201.19" {
			t.Fatalf("currentVersion = %q", currentVersion)
		}
		if localSHA1 != strings.Repeat("a", 40) {
			t.Fatalf("localSHA1 = %q", localSHA1)
		}

		return &core.GitHubReleaseUpdate{
			Available:         true,
			DownloadUrl:       "https://example.com/StandardNotes-x86_64.AppImage",
			TagName:           "@standardnotes/desktop@3.202.0",
			NormalizedVersion: "3.202.0",
			AssetName:         "StandardNotes-x86_64.AppImage",
			Transport:         "zsync",
			ZsyncURL:          "https://example.com/StandardNotes-x86_64.AppImage.zsync",
			ExpectedSHA1:      strings.Repeat("b", 40),
		}, nil
	}

	result, err := checkAppUpdate(&models.App{
		ID:      "standard-notes",
		Version: "3.201.19",
		SHA1:    strings.Repeat("a", 40),
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
	if result.Transport != "zsync" {
		t.Fatalf("Transport = %q, want %q", result.Transport, "zsync")
	}
	if result.ZsyncURL != "https://example.com/StandardNotes-x86_64.AppImage.zsync" {
		t.Fatalf("ZsyncURL = %q", result.ZsyncURL)
	}
	if result.ExpectedSHA1 != strings.Repeat("b", 40) {
		t.Fatalf("ExpectedSHA1 = %q", result.ExpectedSHA1)
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

func TestManagedApplyWorkerCount(t *testing.T) {
	tests := []struct {
		input  int
		expect int
	}{
		{input: 0, expect: 0},
		{input: 1, expect: 1},
		{input: 3, expect: 3},
		{input: 5, expect: 5},
		{input: 9, expect: 5},
	}

	for _, tt := range tests {
		got := managedApplyWorkerCount(tt.input)
		if got != tt.expect {
			t.Fatalf("managedApplyWorkerCount(%d) = %d, want %d", tt.input, got, tt.expect)
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
			Version: "1.11.6-linux-x86_64",
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

func TestFormatAppRefNormalizesPlatformSuffixedVersion(t *testing.T) {
	app := &models.App{
		ID:      "localsend",
		Name:    "LocalSend",
		Version: "1.17.0-linux-x86-64",
	}

	got := formatAppRef(app)
	if got != "LocalSend v1.17.0 [localsend]" {
		t.Fatalf("formatAppRef(app) = %q, want %q", got, "LocalSend v1.17.0 [localsend]")
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

func TestManagedApplyStatusText(t *testing.T) {
	tests := []struct {
		name   string
		row    managedApplyRowState
		expect string
	}{
		{name: "queued", row: managedApplyRowState{stage: managedApplyStageQueued}, expect: "queued"},
		{name: "zsync", row: managedApplyRowState{stage: managedApplyStageZsync}, expect: "delta update"},
		{name: "download known", row: managedApplyRowState{stage: managedApplyStageDownload, downloaded: 512, downloadTotal: 1024}, expect: "downloading 50.0% (512B/1.0KB)"},
		{name: "download unknown", row: managedApplyRowState{stage: managedApplyStageDownload, downloaded: 2048}, expect: "downloading 2.0KB"},
		{name: "verify", row: managedApplyRowState{stage: managedApplyStageVerify}, expect: "verifying"},
		{name: "integrate", row: managedApplyRowState{stage: managedApplyStageIntegrate}, expect: "integrating"},
		{name: "done", row: managedApplyRowState{stage: managedApplyStageDone, version: "2.0.0"}, expect: "updated -> v2.0.0"},
		{name: "failed", row: managedApplyRowState{stage: managedApplyStageFailed}, expect: "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := managedApplyStatusText(tt.row)
			if got != tt.expect {
				t.Fatalf("managedApplyStatusText(%s) = %q, want %q", tt.name, got, tt.expect)
			}
		})
	}
}

func TestManagedApplyRendererTTYRendersStableRows(t *testing.T) {
	withTerminalOutput(t, true)

	output := captureStdout(t, func() {
		renderer := newManagedApplyRenderer(&cobra.Command{}, []pendingManagedUpdate{
			{App: &models.App{ID: "app-a"}},
			{App: &models.App{ID: "app-b"}},
		})
		renderer.Event(managedApplyEvent{Index: 0, AppID: "app-a", Stage: managedApplyStageDownload, Downloaded: 1024, DownloadTotal: 2048})
		renderer.Event(managedApplyEvent{Index: 1, AppID: "app-b", Stage: managedApplyStageDone, Version: "2.0.0"})
		renderer.Finish([]managedApplyResult{
			{index: 0, app: &models.App{ID: "app-a"}, updatedApp: &models.App{ID: "app-a", Version: "1.1.0"}},
			{index: 1, app: &models.App{ID: "app-b"}, updatedApp: &models.App{ID: "app-b", Version: "2.0.0"}},
		})
	})

	if !strings.Contains(output, "Applying 2 updates concurrently (max 5 workers)") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if !strings.Contains(output, "\033[2K\r[1/2] app-a") {
		t.Fatalf("expected tty redraw output, got:\n%s", output)
	}
	if !strings.Contains(output, "[2/2] app-b") {
		t.Fatalf("expected second row, got:\n%s", output)
	}
}

func TestRunManagedUpdateUsesUnifiedApplyUIForSingleApp(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "apps.json")

	originalDbSrc := config.DbSrc
	originalCheck := runAppUpdateCheck
	originalApply := runManagedApply
	config.DbSrc = dbPath
	t.Cleanup(func() {
		config.DbSrc = originalDbSrc
		runAppUpdateCheck = originalCheck
		runManagedApply = originalApply
	})

	if err := repo.SaveDB(dbPath, &repo.DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"my-app": {ID: "my-app", Name: "My App", Version: "1.0.0"},
		},
	}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	runAppUpdateCheck = func(app *models.App) (*pendingManagedUpdate, error) {
		return &pendingManagedUpdate{
			App:       app,
			Label:     "Update available",
			Available: true,
			Latest:    "1.1.0",
			URL:       "https://example.com/MyApp.AppImage",
		}, nil
	}
	runManagedApply = func(ctx context.Context, update pendingManagedUpdate, reporter managedApplyReporter) (*models.App, error) {
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageDownload, Downloaded: 1024, DownloadTotal: 2048})
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageVerify})
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageIntegrate})
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageDone, Version: "1.1.0"})
		return &models.App{ID: update.App.ID, Version: "1.1.0"}, nil
	}

	cmd := newManagedUpdateTestCommand(t, map[string]string{"yes": "true"})
	output := captureStdout(t, func() {
		if err := runManagedUpdate(context.Background(), cmd, "my-app"); err != nil {
			t.Fatalf("runManagedUpdate returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Applying 1 update") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if !strings.Contains(output, "[1/1] my-app updated -> v1.1.0") {
		t.Fatalf("expected final row, got:\n%s", output)
	}
	if strings.Contains(output, "Updating my-app") {
		t.Fatalf("expected old serial apply output to be absent:\n%s", output)
	}
	if strings.Contains(output, "\033[") {
		t.Fatalf("expected non-tty output without ansi codes:\n%s", output)
	}
	if !strings.Contains(output, "Updated 1 app(s); 0 failed") {
		t.Fatalf("unexpected summary:\n%s", output)
	}
}

func TestRunManagedUpdateAppliesConcurrentlyWithMaxFiveWorkers(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "apps.json")

	originalDbSrc := config.DbSrc
	originalCheck := runAppUpdateCheck
	originalApply := runManagedApply
	config.DbSrc = dbPath
	t.Cleanup(func() {
		config.DbSrc = originalDbSrc
		runAppUpdateCheck = originalCheck
		runManagedApply = originalApply
	})

	apps := make(map[string]*models.App)
	for idx := 0; idx < 7; idx++ {
		id := fmt.Sprintf("app-%d", idx)
		apps[id] = &models.App{ID: id, Name: id, Version: "1.0.0"}
	}
	if err := repo.SaveDB(dbPath, &repo.DB{SchemaVersion: 1, Apps: apps}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	runAppUpdateCheck = func(app *models.App) (*pendingManagedUpdate, error) {
		return &pendingManagedUpdate{
			App:       app,
			Label:     "Update available",
			Available: true,
			Latest:    "2.0.0",
			URL:       "https://example.com/" + app.ID + ".AppImage",
		}, nil
	}

	var current int32
	var observedMax int32
	runManagedApply = func(ctx context.Context, update pendingManagedUpdate, reporter managedApplyReporter) (*models.App, error) {
		active := atomic.AddInt32(&current, 1)
		for {
			max := atomic.LoadInt32(&observedMax)
			if active <= max {
				break
			}
			if atomic.CompareAndSwapInt32(&observedMax, max, active) {
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
		atomic.AddInt32(&current, -1)
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageDone, Version: "2.0.0"})
		return &models.App{ID: update.App.ID, Version: "2.0.0"}, nil
	}

	cmd := newManagedUpdateTestCommand(t, map[string]string{"yes": "true"})
	output := captureStdout(t, func() {
		if err := runManagedUpdate(context.Background(), cmd, ""); err != nil {
			t.Fatalf("runManagedUpdate returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&observedMax) != 5 {
		t.Fatalf("observed concurrency = %d, want 5", observedMax)
	}
	if !strings.Contains(output, "Applying 7 updates concurrently (max 5 workers)") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestRunManagedUpdatePersistsSuccessesInPendingOrder(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "apps.json")

	originalDbSrc := config.DbSrc
	originalCheck := runAppUpdateCheck
	originalApply := runManagedApply
	originalBatch := addAppsBatch
	originalSingle := addSingleApp
	config.DbSrc = dbPath
	t.Cleanup(func() {
		config.DbSrc = originalDbSrc
		runAppUpdateCheck = originalCheck
		runManagedApply = originalApply
		addAppsBatch = originalBatch
		addSingleApp = originalSingle
	})

	if err := repo.SaveDB(dbPath, &repo.DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"app-a": {ID: "app-a", Version: "1.0.0"},
			"app-b": {ID: "app-b", Version: "1.0.0"},
			"app-c": {ID: "app-c", Version: "1.0.0"},
		},
	}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	runAppUpdateCheck = func(app *models.App) (*pendingManagedUpdate, error) {
		return &pendingManagedUpdate{App: app, Available: true, URL: "https://example.com/" + app.ID + ".AppImage"}, nil
	}
	runManagedApply = func(ctx context.Context, update pendingManagedUpdate, reporter managedApplyReporter) (*models.App, error) {
		switch update.App.ID {
		case "app-a":
			time.Sleep(30 * time.Millisecond)
		case "app-b":
			time.Sleep(10 * time.Millisecond)
		}
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageDone, Version: "2.0.0"})
		return &models.App{ID: update.App.ID, Version: "2.0.0"}, nil
	}

	var persisted []string
	addAppsBatch = func(apps []*models.App, overwrite bool) error {
		for _, app := range apps {
			persisted = append(persisted, app.ID)
		}
		return nil
	}
	addSingleApp = func(*models.App, bool) error {
		t.Fatal("single app persistence should not be used")
		return nil
	}

	cmd := newManagedUpdateTestCommand(t, map[string]string{"yes": "true"})
	if err := runManagedUpdate(context.Background(), cmd, ""); err != nil {
		t.Fatalf("runManagedUpdate returned error: %v", err)
	}

	got := strings.Join(persisted, ",")
	if got != "app-a,app-b,app-c" {
		t.Fatalf("persisted order = %q, want %q", got, "app-a,app-b,app-c")
	}
}

func TestRunManagedUpdateContinuesAfterApplyFailure(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "apps.json")

	originalDbSrc := config.DbSrc
	originalCheck := runAppUpdateCheck
	originalApply := runManagedApply
	originalBatch := addAppsBatch
	config.DbSrc = dbPath
	t.Cleanup(func() {
		config.DbSrc = originalDbSrc
		runAppUpdateCheck = originalCheck
		runManagedApply = originalApply
		addAppsBatch = originalBatch
	})

	if err := repo.SaveDB(dbPath, &repo.DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"app-a": {ID: "app-a", Version: "1.0.0"},
			"app-b": {ID: "app-b", Version: "1.0.0"},
		},
	}); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	runAppUpdateCheck = func(app *models.App) (*pendingManagedUpdate, error) {
		return &pendingManagedUpdate{App: app, Available: true, URL: "https://example.com/" + app.ID + ".AppImage"}, nil
	}
	runManagedApply = func(ctx context.Context, update pendingManagedUpdate, reporter managedApplyReporter) (*models.App, error) {
		if update.App.ID == "app-a" {
			emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageFailed, Message: "boom"})
			return nil, fmt.Errorf("boom")
		}
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageDone, Version: "2.0.0"})
		return &models.App{ID: update.App.ID, Version: "2.0.0"}, nil
	}

	var persisted []string
	addAppsBatch = func(apps []*models.App, overwrite bool) error {
		for _, app := range apps {
			persisted = append(persisted, app.ID)
		}
		return nil
	}

	cmd := newManagedUpdateTestCommand(t, map[string]string{"yes": "true"})
	output := captureStdout(t, func() {
		err := runManagedUpdate(context.Background(), cmd, "")
		if err == nil {
			t.Fatal("expected aggregated apply error")
		}
		if err.Error() != "1 update(s) failed" {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if strings.Join(persisted, ",") != "app-b" {
		t.Fatalf("persisted ids = %q, want %q", strings.Join(persisted, ","), "app-b")
	}
	if !strings.Contains(output, "[1/2] app-a failed") {
		t.Fatalf("expected failed row, got:\n%s", output)
	}
	if !strings.Contains(output, "Failed: app-a: boom") {
		t.Fatalf("expected failure detail, got:\n%s", output)
	}
	if !strings.Contains(output, "[2/2] app-b updated -> v2.0.0") {
		t.Fatalf("expected success row, got:\n%s", output)
	}
	if !strings.Contains(output, "Updated 1 app(s); 1 failed") {
		t.Fatalf("unexpected summary:\n%s", output)
	}
}

func TestApplyManagedUpdateUsesZsyncWhenAvailable(t *testing.T) {
	originalLookPath := zsyncLookPath
	originalCommand := zsyncCommandContext
	originalDownload := downloadManagedRemoteAsset
	originalIntegrate := integrateManagedUpdate
	t.Cleanup(func() {
		zsyncLookPath = originalLookPath
		zsyncCommandContext = originalCommand
		downloadManagedRemoteAsset = originalDownload
		integrateManagedUpdate = originalIntegrate
	})

	currentPath := filepath.Join(t.TempDir(), "current.AppImage")
	if err := os.WriteFile(currentPath, []byte("current"), 0o755); err != nil {
		t.Fatalf("failed to write current appimage: %v", err)
	}

	payload := []byte("updated-by-zsync")
	expectedSHA1 := sha1Hex(payload)

	zsyncLookPath = func(string) (string, error) {
		return "zsync", nil
	}

	var call []string
	zsyncCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		call = append([]string{name}, arg...)

		var outputPath string
		for i := 0; i < len(arg)-1; i++ {
			if arg[i] == "-o" {
				outputPath = arg[i+1]
				break
			}
		}
		if outputPath == "" {
			t.Fatal("missing -o argument")
		}
		if err := os.WriteFile(outputPath, payload, 0o644); err != nil {
			t.Fatalf("failed to write zsync output: %v", err)
		}

		return exec.CommandContext(ctx, "sh", "-c", "exit 0")
	}

	downloadManagedRemoteAsset = func(context.Context, string, string, bool, func(int64, int64)) error {
		t.Fatal("download should not be called when zsync succeeds")
		return nil
	}

	integrateManagedUpdate = func(ctx context.Context, src string, confirm core.UpdateOverwritePrompt) (*models.App, error) {
		if _, err := os.Stat(src); err != nil {
			t.Fatalf("expected zsync output file: %v", err)
		}
		overwrite, err := confirm(&models.UpdateSource{Kind: models.UpdateGitHubRelease}, &models.UpdateSource{Kind: models.UpdateZsync})
		if err != nil {
			t.Fatalf("confirm returned error: %v", err)
		}
		if overwrite {
			t.Fatal("expected update source overwrite callback to reject replacement")
		}
		return &models.App{ID: "my-app", Version: "2.0.0"}, nil
	}

	reporter := &recordedManagedApplyReporter{}
	_, err := applyManagedUpdate(context.Background(), pendingManagedUpdate{
		App:          &models.App{ID: "my-app", ExecPath: currentPath},
		URL:          "https://example.com/MyApp.AppImage",
		Asset:        "MyApp.AppImage",
		ZsyncURL:     "https://example.com/MyApp.AppImage.zsync",
		ExpectedSHA1: expectedSHA1,
	}, reporter)
	if err != nil {
		t.Fatalf("applyManagedUpdate returned error: %v", err)
	}

	if len(call) < 7 {
		t.Fatalf("unexpected zsync call: %v", call)
	}
	if call[0] != "zsync" {
		t.Fatalf("command = %q, want zsync", call[0])
	}
	if !containsString(call, currentPath) {
		t.Fatalf("expected zsync call to include input path, got %v", call)
	}
	if !containsString(call, "https://example.com/MyApp.AppImage.zsync") {
		t.Fatalf("expected zsync call to include zsync url, got %v", call)
	}
	assertManagedApplyStages(t, reporter.events,
		managedApplyStageQueued,
		managedApplyStageZsync,
		managedApplyStageVerify,
		managedApplyStageIntegrate,
		managedApplyStageDone,
	)
}

func TestApplyManagedUpdateFallsBackWhenZsyncMissing(t *testing.T) {
	originalLookPath := zsyncLookPath
	originalDownload := downloadManagedRemoteAsset
	originalIntegrate := integrateManagedUpdate
	t.Cleanup(func() {
		zsyncLookPath = originalLookPath
		downloadManagedRemoteAsset = originalDownload
		integrateManagedUpdate = originalIntegrate
	})

	payload := []byte("downloaded-fallback")
	expectedSHA1 := sha1Hex(payload)

	zsyncLookPath = func(string) (string, error) {
		return "", exec.ErrNotFound
	}

	var downloadCalls int32
	downloadManagedRemoteAsset = func(ctx context.Context, assetURL, destination string, interactive bool, onProgress func(int64, int64)) error {
		atomic.AddInt32(&downloadCalls, 1)
		if onProgress != nil {
			onProgress(int64(len(payload)), int64(len(payload)))
		}
		return os.WriteFile(destination, payload, 0o644)
	}

	integrateManagedUpdate = func(context.Context, string, core.UpdateOverwritePrompt) (*models.App, error) {
		return &models.App{ID: "my-app", Version: "2.0.0"}, nil
	}

	reporter := &recordedManagedApplyReporter{}
	_, err := applyManagedUpdate(context.Background(), pendingManagedUpdate{
		App:          &models.App{ID: "my-app", ExecPath: "/tmp/current.AppImage"},
		URL:          "https://example.com/MyApp.AppImage",
		Asset:        "MyApp.AppImage",
		ZsyncURL:     "https://example.com/MyApp.AppImage.zsync",
		ExpectedSHA1: expectedSHA1,
	}, reporter)
	if err != nil {
		t.Fatalf("applyManagedUpdate returned error: %v", err)
	}
	if atomic.LoadInt32(&downloadCalls) != 1 {
		t.Fatalf("download calls = %d, want 1", downloadCalls)
	}
	assertManagedApplyStages(t, reporter.events,
		managedApplyStageQueued,
		managedApplyStageZsync,
		managedApplyStageDownload,
		managedApplyStageDownload,
		managedApplyStageVerify,
		managedApplyStageIntegrate,
		managedApplyStageDone,
	)
}

func TestApplyManagedUpdateFallsBackWhenZsyncFails(t *testing.T) {
	originalLookPath := zsyncLookPath
	originalCommand := zsyncCommandContext
	originalDownload := downloadManagedRemoteAsset
	originalIntegrate := integrateManagedUpdate
	t.Cleanup(func() {
		zsyncLookPath = originalLookPath
		zsyncCommandContext = originalCommand
		downloadManagedRemoteAsset = originalDownload
		integrateManagedUpdate = originalIntegrate
	})

	payload := []byte("downloaded-fallback")
	expectedSHA1 := sha1Hex(payload)

	zsyncLookPath = func(string) (string, error) {
		return "zsync", nil
	}
	zsyncCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "exit 1")
	}

	var downloadCalls int32
	downloadManagedRemoteAsset = func(ctx context.Context, assetURL, destination string, interactive bool, onProgress func(int64, int64)) error {
		atomic.AddInt32(&downloadCalls, 1)
		if onProgress != nil {
			onProgress(int64(len(payload)), int64(len(payload)))
		}
		return os.WriteFile(destination, payload, 0o644)
	}

	integrateManagedUpdate = func(context.Context, string, core.UpdateOverwritePrompt) (*models.App, error) {
		return &models.App{ID: "my-app", Version: "2.0.0"}, nil
	}

	reporter := &recordedManagedApplyReporter{}
	_, err := applyManagedUpdate(context.Background(), pendingManagedUpdate{
		App:          &models.App{ID: "my-app", ExecPath: "/tmp/current.AppImage"},
		URL:          "https://example.com/MyApp.AppImage",
		Asset:        "MyApp.AppImage",
		ZsyncURL:     "https://example.com/MyApp.AppImage.zsync",
		ExpectedSHA1: expectedSHA1,
	}, reporter)
	if err != nil {
		t.Fatalf("applyManagedUpdate returned error: %v", err)
	}
	if atomic.LoadInt32(&downloadCalls) != 1 {
		t.Fatalf("download calls = %d, want 1", downloadCalls)
	}
	assertManagedApplyStages(t, reporter.events,
		managedApplyStageQueued,
		managedApplyStageZsync,
		managedApplyStageDownload,
		managedApplyStageDownload,
		managedApplyStageVerify,
		managedApplyStageIntegrate,
		managedApplyStageDone,
	)
}

func TestApplyManagedUpdateWithoutZsyncUsesFullDownload(t *testing.T) {
	originalLookPath := zsyncLookPath
	originalDownload := downloadManagedRemoteAsset
	originalIntegrate := integrateManagedUpdate
	t.Cleanup(func() {
		zsyncLookPath = originalLookPath
		downloadManagedRemoteAsset = originalDownload
		integrateManagedUpdate = originalIntegrate
	})

	zsyncLookPath = func(string) (string, error) {
		t.Fatal("zsync should not be probed when no zsync url is present")
		return "", nil
	}

	var downloadCalls int32
	downloadManagedRemoteAsset = func(ctx context.Context, assetURL, destination string, interactive bool, onProgress func(int64, int64)) error {
		atomic.AddInt32(&downloadCalls, 1)
		if onProgress != nil {
			onProgress(10, 10)
		}
		return os.WriteFile(destination, []byte("downloaded"), 0o644)
	}

	integrateManagedUpdate = func(context.Context, string, core.UpdateOverwritePrompt) (*models.App, error) {
		return &models.App{ID: "my-app", Version: "2.0.0"}, nil
	}

	reporter := &recordedManagedApplyReporter{}
	_, err := applyManagedUpdate(context.Background(), pendingManagedUpdate{
		App:   &models.App{ID: "my-app", ExecPath: "/tmp/current.AppImage"},
		URL:   "https://example.com/MyApp.AppImage",
		Asset: "MyApp.AppImage",
	}, reporter)
	if err != nil {
		t.Fatalf("applyManagedUpdate returned error: %v", err)
	}
	if atomic.LoadInt32(&downloadCalls) != 1 {
		t.Fatalf("download calls = %d, want 1", downloadCalls)
	}
	assertManagedApplyStages(t, reporter.events,
		managedApplyStageQueued,
		managedApplyStageDownload,
		managedApplyStageDownload,
		managedApplyStageVerify,
		managedApplyStageIntegrate,
		managedApplyStageDone,
	)
}

func TestDisplayVersion(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "numeric", input: "1.2.3", expect: "v1.2.3"},
		{name: "already prefixed", input: "v1.2.3", expect: "v1.2.3"},
		{name: "platform suffixed", input: "1.17.0-linux-x86-64", expect: "v1.17.0"},
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

type recordedManagedApplyReporter struct {
	events []managedApplyEvent
}

func (r *recordedManagedApplyReporter) Event(event managedApplyEvent) {
	r.events = append(r.events, event)
}

func assertManagedApplyStages(t *testing.T, events []managedApplyEvent, want ...managedApplyStage) {
	t.Helper()

	got := make([]managedApplyStage, 0, len(events))
	for _, event := range events {
		got = append(got, event.Stage)
	}

	if len(got) != len(want) {
		t.Fatalf("stage count = %d, want %d (%v)", len(got), len(want), got)
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("stage[%d] = %q, want %q (all=%v)", idx, got[idx], want[idx], got)
		}
	}
}

func withTerminalOutput(t *testing.T, value bool) {
	t.Helper()

	original := terminalOutputChecker
	terminalOutputChecker = func() bool {
		return value
	}
	t.Cleanup(func() {
		terminalOutputChecker = original
	})
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

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sha1Hex(value []byte) string {
	sum := sha1.Sum(value)
	return hex.EncodeToString(sum[:])
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

func newUpgradeTestCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "upgrade",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          UpgradeCmd,
	}
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stdout)
	return cmd
}

func newInstallTestCommand() *cobra.Command {
	cmd := newInstallCommand()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.RunE = func(*cobra.Command, []string) error {
		return fmt.Errorf("test sentinel")
	}
	return cmd
}

func newManagedUpdateTestCommand(t *testing.T, values map[string]string) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{Use: "update"}
	cmd.Flags().Bool("yes", false, "")
	cmd.Flags().Bool("check-only", false, "")
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stdout)

	for key, value := range values {
		if err := cmd.Flags().Set(key, value); err != nil {
			t.Fatalf("failed to set %s: %v", key, err)
		}
	}

	return cmd
}

func executeTestCommand(ctx context.Context, cmd *cobra.Command, args ...string) error {
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stdout)
	cmd.SetArgs(args)
	return cmd.ExecuteContext(ctx)
}

func findSubcommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, subcommand := range cmd.Commands() {
		if subcommand.Name() == name {
			return subcommand
		}
	}
	return nil
}

func countFlags(flags interface{ VisitAll(func(*pflag.Flag)) }) int {
	count := 0
	flags.VisitAll(func(*pflag.Flag) {
		count++
	})
	return count
}

func setupAddCommandConfigForTest(t *testing.T, tmp string) {
	t.Helper()

	originalDbSrc := config.DbSrc
	originalTempDir := config.TempDir
	t.Cleanup(func() {
		config.DbSrc = originalDbSrc
		config.TempDir = originalTempDir
	})

	config.DbSrc = filepath.Join(tmp, "state", "aim", "apps.json")
	config.TempDir = filepath.Join(tmp, "cache", "aim", "tmp")

	for _, dir := range []string{filepath.Dir(config.DbSrc), config.TempDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create test dir %q: %v", dir, err)
		}
	}
}

func runAddCommand(ctx context.Context, args []string) error {
	return runRootCommand(ctx, append([]string{"add"}, args...))
}

func runIntegrateCommand(ctx context.Context, args []string) error {
	return runRootCommand(ctx, append([]string{"integrate"}, args...))
}

func runListCommand(ctx context.Context, args []string) error {
	return runRootCommand(ctx, append([]string{"list"}, args...))
}

func runShowCommand(ctx context.Context, args []string) error {
	return runRootCommand(ctx, append([]string{"show"}, args...))
}

func runInfoCommand(ctx context.Context, args []string) error {
	return runRootCommand(ctx, append([]string{"info"}, args...))
}

func runInspectCommand(ctx context.Context, args []string) error {
	return runRootCommand(ctx, append([]string{"inspect"}, args...))
}

func runInstallCommand(ctx context.Context, args []string) error {
	return runRootCommand(ctx, append([]string{"install"}, args...))
}

func runUpdateSetCommand(ctx context.Context, args []string) error {
	return runRootCommand(ctx, append([]string{"update", "set"}, args...))
}

func runUpdateUnsetCommand(ctx context.Context, args []string) error {
	return runRootCommand(ctx, append([]string{"update", "unset"}, args...))
}

func runRootCommand(ctx context.Context, args []string) error {
	cmd := newRootTestCommand()
	return executeTestCommand(ctx, cmd, args...)
}
