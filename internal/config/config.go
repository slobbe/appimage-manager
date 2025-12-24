package config

import (
	"os"
	"path/filepath"
)

var (
	AppName    = "AppImage Manager"
	AppVersion = "0.1.0"
	AppAuthor  = "Sebastian Lobbe <slobbe@lobbe.cc>"
	AppRepo    = "https://github.com/slobbe/appimage-manager"

	AimDir     string
	DesktopDir string
	TempDir    string

	DbSrc string
)

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		panic("failed to get home directory: " + err.Error())
	}

	AimDir = filepath.Join(home, ".local/share/appimage-manager")
	DesktopDir = filepath.Join(home, ".local/share/applications")

	TempDir = filepath.Join(AimDir, ".tmp")

	DbSrc = filepath.Join(AimDir, "apps.json")
}
