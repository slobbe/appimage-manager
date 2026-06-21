package xdg

import (
	"path/filepath"
	"testing"
)

func TestResolveUsesHomeDefaultsWhenXDGEnvUnset(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")

	dirs, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	want := Dirs{
		ConfigHome: filepath.Join(home, ".config"),
		DataHome:   filepath.Join(home, ".local", "share"),
	}

	if dirs != want {
		t.Fatalf("Resolve() = %#v, want %#v", dirs, want)
	}
}

func TestResolveHonorsXDGEnvOverrides(t *testing.T) {
	home := t.TempDir()
	configHome := filepath.Join(home, "xdg-config")
	dataHome := filepath.Join(home, "xdg-data")

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", dataHome)

	dirs, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	want := Dirs{
		ConfigHome: configHome,
		DataHome:   dataHome,
	}

	if dirs != want {
		t.Fatalf("Resolve() = %#v, want %#v", dirs, want)
	}
}

func TestDerivedPaths(t *testing.T) {
	dirs := Dirs{
		ConfigHome: filepath.Join("root", "config"),
		DataHome:   filepath.Join("root", "data"),
	}

	tests := map[string]struct {
		got  string
		want string
	}{
		"ConfigDir":          {got: ConfigDir(dirs), want: filepath.Join(dirs.ConfigHome, AppName)},
		"ConfigFile":         {got: ConfigFile(dirs), want: filepath.Join(dirs.ConfigHome, AppName, "config.toml")},
		"DataDir":            {got: DataDir(dirs), want: filepath.Join(dirs.DataHome, AppName)},
		"DefaultAppImageDir": {got: DefaultAppImageDir(dirs), want: filepath.Join(dirs.DataHome, AppName, "appimages")},
		"DesktopDir":         {got: DesktopDir(dirs), want: filepath.Join(dirs.DataHome, "applications")},
		"IconDir":            {got: IconDir(dirs), want: filepath.Join(dirs.DataHome, "icons")},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}
