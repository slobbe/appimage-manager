package xdg

import (
	"fmt"
	"os"
	"path/filepath"
)

const AppName = "aim"

type Dirs struct {
	DataHome string
}

func Resolve() (Dirs, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Dirs{}, fmt.Errorf("resolve home directory: %w", err)
	}

	return Dirs{
		DataHome: envOrDefault("XDG_DATA_HOME", filepath.Join(home, ".local", "share")),
	}, nil
}

func envOrDefault(name string, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}

	return value
}

func DataDir(dirs Dirs) string {
	return filepath.Join(dirs.DataHome, AppName)
}

func DefaultAppImageDir(dirs Dirs) string {
	return filepath.Join(DataDir(dirs), "appimages")
}

func DesktopDir(dirs Dirs) string {
	return filepath.Join(dirs.DataHome, "applications")
}

func IconDir(dirs Dirs) string {
	return filepath.Join(dirs.DataHome, "icons")
}
