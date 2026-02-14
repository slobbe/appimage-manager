package core

import (
	"context"
	"os/exec"
	"testing"

	"github.com/slobbe/appimage-manager/internal/config"
)

func TestRefreshDesktopIntegrationCachesUsesXDGIconResource(t *testing.T) {
	originalLookPath := integrationCacheLookPath
	originalCommand := integrationCacheCommandContext
	originalWarn := integrationCacheWarn
	t.Cleanup(func() {
		integrationCacheLookPath = originalLookPath
		integrationCacheCommandContext = originalCommand
		integrationCacheWarn = originalWarn
	})

	integrationCacheLookPath = func(name string) (string, error) {
		switch name {
		case "update-desktop-database", "xdg-icon-resource":
			return name, nil
		default:
			return "", exec.ErrNotFound
		}
	}

	var calls [][]string
	integrationCacheCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		call := append([]string{name}, arg...)
		calls = append(calls, call)
		return exec.CommandContext(ctx, "sh", "-c", "exit 0")
	}
	integrationCacheWarn = func(string) {}

	refreshDesktopIntegrationCaches(context.Background())

	if len(calls) != 2 {
		t.Fatalf("expected 2 command calls, got %d", len(calls))
	}
	if calls[0][0] != "update-desktop-database" {
		t.Fatalf("first command = %q, want update-desktop-database", calls[0][0])
	}
	if calls[1][0] != "xdg-icon-resource" {
		t.Fatalf("second command = %q, want xdg-icon-resource", calls[1][0])
	}
	if len(calls[1]) < 2 || calls[1][1] != "forceupdate" {
		t.Fatalf("xdg-icon-resource args = %v, want forceupdate", calls[1])
	}
}

func TestRefreshDesktopIntegrationCachesFallsBackToGtkIconCache(t *testing.T) {
	originalLookPath := integrationCacheLookPath
	originalCommand := integrationCacheCommandContext
	originalWarn := integrationCacheWarn
	originalIconThemeDir := config.IconThemeDir
	t.Cleanup(func() {
		integrationCacheLookPath = originalLookPath
		integrationCacheCommandContext = originalCommand
		integrationCacheWarn = originalWarn
		config.IconThemeDir = originalIconThemeDir
	})

	config.IconThemeDir = "/tmp/hicolor"

	integrationCacheLookPath = func(name string) (string, error) {
		switch name {
		case "update-desktop-database", "gtk-update-icon-cache":
			return name, nil
		case "xdg-icon-resource":
			return "", exec.ErrNotFound
		default:
			return "", exec.ErrNotFound
		}
	}

	var calls [][]string
	integrationCacheCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		call := append([]string{name}, arg...)
		calls = append(calls, call)
		return exec.CommandContext(ctx, "sh", "-c", "exit 0")
	}
	integrationCacheWarn = func(string) {}

	refreshDesktopIntegrationCaches(context.Background())

	if len(calls) != 2 {
		t.Fatalf("expected 2 command calls, got %d", len(calls))
	}
	if calls[1][0] != "gtk-update-icon-cache" {
		t.Fatalf("second command = %q, want gtk-update-icon-cache", calls[1][0])
	}
	if len(calls[1]) < 3 || calls[1][1] != "-f" || calls[1][2] != "/tmp/hicolor" {
		t.Fatalf("gtk-update-icon-cache args = %v, want -f /tmp/hicolor", calls[1])
	}
}

func TestRefreshDesktopIntegrationCachesWarnsOnCommandFailure(t *testing.T) {
	originalLookPath := integrationCacheLookPath
	originalCommand := integrationCacheCommandContext
	originalWarn := integrationCacheWarn
	t.Cleanup(func() {
		integrationCacheLookPath = originalLookPath
		integrationCacheCommandContext = originalCommand
		integrationCacheWarn = originalWarn
	})

	integrationCacheLookPath = func(name string) (string, error) {
		switch name {
		case "update-desktop-database", "xdg-icon-resource":
			return name, nil
		default:
			return "", exec.ErrNotFound
		}
	}

	integrationCacheCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		if name == "xdg-icon-resource" {
			return exec.CommandContext(ctx, "sh", "-c", "exit 1")
		}
		return exec.CommandContext(ctx, "sh", "-c", "exit 0")
	}

	warnings := 0
	integrationCacheWarn = func(string) {
		warnings++
	}

	refreshDesktopIntegrationCaches(context.Background())

	if warnings == 0 {
		t.Fatal("expected at least one warning when icon cache refresh fails")
	}
}
