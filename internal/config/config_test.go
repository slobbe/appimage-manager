package config

import (
	"path/filepath"
	"testing"
)

func TestResolvePathsDefaults(t *testing.T) {
	home := "/home/alice"

	paths := resolvePaths(home, func(string) string { return "" })

	if paths.AimDir != filepath.Join(home, ".local", "share", "appimage-manager") {
		t.Fatalf("AimDir = %q", paths.AimDir)
	}
	if paths.DesktopDir != filepath.Join(home, ".local", "share", "applications") {
		t.Fatalf("DesktopDir = %q", paths.DesktopDir)
	}
	if paths.TempDir != filepath.Join(home, ".cache", "appimage-manager", "tmp") {
		t.Fatalf("TempDir = %q", paths.TempDir)
	}
	if paths.DbSrc != filepath.Join(home, ".local", "state", "appimage-manager", "apps.json") {
		t.Fatalf("DbSrc = %q", paths.DbSrc)
	}
}

func TestResolvePathsXDGOverrides(t *testing.T) {
	home := "/home/alice"
	env := map[string]string{
		"XDG_DATA_HOME":  "/xdg/data",
		"XDG_CACHE_HOME": "/xdg/cache",
		"XDG_STATE_HOME": "/xdg/state",
	}

	paths := resolvePaths(home, func(key string) string {
		return env[key]
	})

	if paths.AimDir != "/xdg/data/appimage-manager" {
		t.Fatalf("AimDir = %q", paths.AimDir)
	}
	if paths.DesktopDir != "/xdg/data/applications" {
		t.Fatalf("DesktopDir = %q", paths.DesktopDir)
	}
	if paths.TempDir != "/xdg/cache/appimage-manager/tmp" {
		t.Fatalf("TempDir = %q", paths.TempDir)
	}
	if paths.DbSrc != "/xdg/state/appimage-manager/apps.json" {
		t.Fatalf("DbSrc = %q", paths.DbSrc)
	}
}

func TestResolvePathsIgnoresRelativeXDGPaths(t *testing.T) {
	home := "/home/alice"
	env := map[string]string{
		"XDG_DATA_HOME":  "relative/data",
		"XDG_CACHE_HOME": "relative/cache",
		"XDG_STATE_HOME": "relative/state",
	}

	paths := resolvePaths(home, func(key string) string {
		return env[key]
	})

	if paths.AimDir != filepath.Join(home, ".local", "share", "appimage-manager") {
		t.Fatalf("AimDir = %q", paths.AimDir)
	}
	if paths.TempDir != filepath.Join(home, ".cache", "appimage-manager", "tmp") {
		t.Fatalf("TempDir = %q", paths.TempDir)
	}
	if paths.DbSrc != filepath.Join(home, ".local", "state", "appimage-manager", "apps.json") {
		t.Fatalf("DbSrc = %q", paths.DbSrc)
	}
}
