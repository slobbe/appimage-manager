package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	util "github.com/slobbe/appimage-manager/internal/infra/helpers"
)

func InstallDesktopIcon(iconID, iconSrc string) (string, string, error) {
	iconID = strings.TrimSpace(iconID)
	if iconID == "" {
		return "", "", fmt.Errorf("icon id cannot be empty")
	}

	iconSrc = strings.TrimSpace(iconSrc)
	if iconSrc == "" {
		return "", "", fmt.Errorf("icon source cannot be empty")
	}

	ext := strings.ToLower(filepath.Ext(iconSrc))
	if ext == "" {
		return "", "", fmt.Errorf("icon file extension is required")
	}

	destDir := iconInstallDir(ext)
	destName := iconID + ext

	destPath := filepath.Join(destDir, destName)
	desktopIconValue := destPath

	if filepath.Clean(iconSrc) == filepath.Clean(destPath) {
		return destPath, desktopIconValue, nil
	}

	if err := ensureDir(destDir); err != nil {
		return "", "", err
	}

	if _, err := util.Move(iconSrc, destPath); err != nil {
		return "", "", err
	}

	return destPath, desktopIconValue, nil
}

func iconInstallDir(ext string) string {
	paths, err := requirePaths()
	if err != nil {
		return ""
	}
	if ext == ".svg" {
		return filepath.Join(paths.IconThemeDir, "scalable", "apps")
	}

	return filepath.Join(paths.IconThemeDir, "256x256", "apps")
}

func ensureDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("directory cannot be empty")
	}
	return os.MkdirAll(dir, 0o755)
}
