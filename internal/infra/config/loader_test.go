package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/infra/xdg"
)

func TestDefaultAppConfigMapsXDGDirs(t *testing.T) {
	dirs := testDirs(t)

	got := DefaultAppConfig(dirs)
	want := app.Config{
		ConfigFile:             filepath.Join(dirs.ConfigHome, xdg.AppName, "config.toml"),
		AppImageDir:            filepath.Join(dirs.DataHome, xdg.AppName, "appimages"),
		DesktopDir:             filepath.Join(dirs.DataHome, "applications"),
		IconDir:                filepath.Join(dirs.DataHome, "icons"),
		MinimumReleaseAgeHours: 1,
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

func TestLoadMinimumReleaseAgeHours(t *testing.T) {
	for _, tt := range []struct {
		name     string
		contents string
		want     int64
	}{
		{name: "missing", contents: "", want: 1},
		{name: "zero", contents: "minimum_release_age_hours = 0", want: 0},
		{name: "positive", contents: "minimum_release_age_hours = 24", want: 24},
		{name: "beyond duration range", contents: "minimum_release_age_hours = 3000000", want: 3000000},
		{name: "maximum int64", contents: "minimum_release_age_hours = 9223372036854775807", want: 9223372036854775807},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dirs := testDirs(t)
			got, err := Load(writeConfigFile(t, tt.contents), dirs)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			want := DefaultAppConfig(dirs)
			want.MinimumReleaseAgeHours = tt.want
			if got != want {
				t.Fatalf("Load() = %#v, want %#v", got, want)
			}
		})
	}
}

func TestLoadRejectsInvalidMinimumReleaseAgeHours(t *testing.T) {
	for _, value := range []string{
		"-1", "-9223372036854775808", "1.5", "1.0", `"24"`, "true", "[]", "{}", "1979-05-27", "9223372036854775808",
	} {
		t.Run(value, func(t *testing.T) {
			_, err := Load(writeConfigFile(t, "minimum_release_age_hours = "+value), testDirs(t))
			if err == nil {
				t.Fatal("Load() error = nil, want invalid minimum_release_age_hours error")
			}
		})
	}
}

func testDirs(t *testing.T) xdg.Dirs {
	t.Helper()
	root := t.TempDir()
	return xdg.Dirs{
		ConfigHome: filepath.Join(root, "config"),
		DataHome:   filepath.Join(root, "data"),
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
