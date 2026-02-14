package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/slobbe/appimage-manager/internal/config"
	util "github.com/slobbe/appimage-manager/internal/helpers"
)

func InstallDesktopIcon(appID, iconSrc string) (string, string, error) {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return "", "", fmt.Errorf("app id cannot be empty")
	}

	iconSrc = strings.TrimSpace(iconSrc)
	if iconSrc == "" {
		return "", "", fmt.Errorf("icon source cannot be empty")
	}

	ext := strings.ToLower(filepath.Ext(iconSrc))
	if ext == "" {
		return "", "", fmt.Errorf("icon file extension is required")
	}

	desktopIconValue := appID
	destDir := iconInstallDir(ext)
	destName := appID + ext

	if !isThemeLookupExtension(ext) {
		desktopIconValue = filepath.Join(destDir, destName)
	}

	destPath := filepath.Join(destDir, destName)

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
	if ext == ".svg" {
		return filepath.Join(config.IconThemeDir, "scalable", "apps")
	}

	if isThemeLookupExtension(ext) {
		return filepath.Join(config.IconThemeDir, "256x256", "apps")
	}

	return config.PixmapsDir
}

func isThemeLookupExtension(ext string) bool {
	switch strings.ToLower(ext) {
	case ".png", ".svg", ".xpm":
		return true
	default:
		return false
	}
}

func ensureDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("directory cannot be empty")
	}
	return os.MkdirAll(dir, 0o755)
}
