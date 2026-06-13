package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aim/internal/app"
	"aim/internal/infra/xdg"
)

func TestDefaultAppConfigMapsXDGDirs(t *testing.T) {
	dirs := testDirs(t)

	got := DefaultAppConfig(dirs)
	want := app.Config{
		ConfigFile:  filepath.Join(dirs.ConfigHome, xdg.AppName, "config.toml"),
		AppImageDir: filepath.Join(dirs.DataHome, xdg.AppName, "appimages"),
		CacheDir:    filepath.Join(dirs.CacheHome, xdg.AppName),
		DesktopDir:  filepath.Join(dirs.DataHome, "applications"),
		IconDir:     filepath.Join(dirs.DataHome, "icons"),
	}

	if got != want {
		t.Fatalf("DefaultAppConfig() = %#v, want %#v", got, want)
	}
}

func TestLoadReturnsDefaultsWhenConfigFileDoesNotExist(t *testing.T) {
	dirs := testDirs(t)
	missingPath := filepath.Join(t.TempDir(), "missing.toml")

	got, err := Load(missingPath, dirs)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := DefaultAppConfig(dirs)
	if got != want {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestLoadReturnsDefaultsForEmptyConfigFile(t *testing.T) {
	dirs := testDirs(t)
	path := writeConfigFile(t, "")

	got, err := Load(path, dirs)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := DefaultAppConfig(dirs)
	if got != want {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestLoadOverridesOnlyAppImageDir(t *testing.T) {
	dirs := testDirs(t)
	path := writeConfigFile(t, "appimage_dir = \"/custom/appimages\"\n")

	got, err := Load(path, dirs)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := DefaultAppConfig(dirs)
	want.AppImageDir = "/custom/appimages"
	if got != want {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestLoadExpandsHomeRelativeAppImageDir(t *testing.T) {
	dirs := testDirs(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := writeConfigFile(t, "appimage_dir = \"~/Apps\"\n")

	got, err := Load(path, dirs)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := filepath.Join(home, "Apps")
	if got.AppImageDir != want {
		t.Fatalf("AppImageDir = %q, want %q", got.AppImageDir, want)
	}
}

func TestLoadExpandsHomeAppImageDir(t *testing.T) {
	dirs := testDirs(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := writeConfigFile(t, "appimage_dir = \"~\"\n")

	got, err := Load(path, dirs)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got.AppImageDir != home {
		t.Fatalf("AppImageDir = %q, want %q", got.AppImageDir, home)
	}
}

func TestLoadTrimsWhitespaceAndCleansNonHomeAppImageDir(t *testing.T) {
	dirs := testDirs(t)
	dirtyPath := filepath.Join(t.TempDir(), "apps", "..", "appimages")
	path := writeConfigFile(t, "appimage_dir = \"  "+dirtyPath+"  \"\n")

	got, err := Load(path, dirs)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := filepath.Clean(dirtyPath)
	if got.AppImageDir != want {
		t.Fatalf("AppImageDir = %q, want %q", got.AppImageDir, want)
	}
}

func TestLoadMalformedTOMLReturnsParseError(t *testing.T) {
	dirs := testDirs(t)
	path := writeConfigFile(t, "appimage_dir = [\n")

	_, err := Load(path, dirs)
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "parse config file") {
		t.Fatalf("Load() error = %q, want message containing %q", err.Error(), "parse config file")
	}
}

func testDirs(t *testing.T) xdg.Dirs {
	t.Helper()
	root := t.TempDir()
	return xdg.Dirs{
		ConfigHome: filepath.Join(root, "config"),
		DataHome:   filepath.Join(root, "data"),
		CacheHome:  filepath.Join(root, "cache"),
		StateHome:  filepath.Join(root, "state"),
	}
}

func writeConfigFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	return path
}
