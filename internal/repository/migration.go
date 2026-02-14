package repo

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/slobbe/appimage-manager/internal/config"
	models "github.com/slobbe/appimage-manager/internal/types"
)

func MigrateLegacyToXDG() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	legacyAimDir := filepath.Join(home, ".appimage-manager")
	legacyDBPath := filepath.Join(legacyAimDir, "apps.json")

	if filepath.Clean(config.DbSrc) == filepath.Clean(legacyDBPath) {
		return nil
	}

	if _, err := os.Stat(config.DbSrc); err == nil {
		return repairExistingXDGDB(legacyAimDir)
	} else if !os.IsNotExist(err) {
		return err
	}

	if _, err := os.Stat(legacyDBPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	legacyDB, err := LoadDB(legacyDBPath)
	if err != nil {
		return fmt.Errorf("failed to load legacy database: %w", err)
	}

	if err := os.MkdirAll(config.AimDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(config.DesktopDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(config.DbSrc), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(config.IconThemeDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(config.PixmapsDir, 0o755); err != nil {
		return err
	}

	for _, app := range legacyDB.Apps {
		if app == nil || strings.TrimSpace(app.ID) == "" {
			continue
		}

		if err := migrateAppAssets(app, legacyAimDir); err != nil {
			return err
		}
	}

	if err := SaveDB(config.DbSrc, legacyDB); err != nil {
		return fmt.Errorf("failed to write migrated database: %w", err)
	}

	return nil
}

func repairExistingXDGDB(legacyAimDir string) error {
	db, err := LoadDB(config.DbSrc)
	if err != nil {
		return fmt.Errorf("failed to load existing xdg database: %w", err)
	}

	changed := false
	for _, app := range db.Apps {
		if app == nil || strings.TrimSpace(app.ID) == "" {
			continue
		}

		legacyAppDir := filepath.Join(legacyAimDir, app.ID)
		newAppDir := filepath.Join(config.AimDir, app.ID)
		if err := copyDirMissing(legacyAppDir, newAppDir); err != nil {
			return err
		}

		if updated := rewriteAppPathsToXDG(app, legacyAimDir); updated {
			changed = true
		}

		desktopIconValue := app.IconPath
		if strings.TrimSpace(app.IconPath) != "" {
			legacyIconPath := deriveLegacyPath(app.IconPath, legacyAimDir)
			installedIconPath, iconValue, err := installMigratedIcon(app.ID, app.IconPath, legacyIconPath)
			if err != nil {
				return err
			}
			if installedIconPath != app.IconPath {
				app.IconPath = installedIconPath
				changed = true
			}
			desktopIconValue = iconValue
		}

		if _, err := os.Stat(app.DesktopEntryPath); err == nil {
			if err := rewriteDesktopEntryFile(app.DesktopEntryPath, app.ExecPath, desktopIconValue); err != nil {
				return err
			}
		}

		if strings.TrimSpace(app.DesktopEntryLink) != "" {
			linkName := filepath.Base(app.DesktopEntryLink)
			if linkName == "" || linkName == "." || linkName == string(filepath.Separator) {
				app.DesktopEntryLink = ""
				changed = true
				continue
			}

			expectedLink := filepath.Join(config.DesktopDir, linkName)
			if app.DesktopEntryLink != expectedLink {
				app.DesktopEntryLink = expectedLink
				changed = true
			}

			if _, err := os.Stat(app.DesktopEntryPath); err == nil {
				if err := ensureDesktopLink(app.DesktopEntryPath, app.DesktopEntryLink); err != nil {
					return err
				}
			} else {
				app.DesktopEntryLink = ""
				changed = true
			}
		}
	}

	if changed {
		if err := SaveDB(config.DbSrc, db); err != nil {
			return fmt.Errorf("failed to update xdg database: %w", err)
		}
	}

	return nil
}

func migrateAppAssets(app *models.App, legacyAimDir string) error {
	legacyExecPath := app.ExecPath
	legacyDesktopPath := app.DesktopEntryPath
	legacyIconPath := app.IconPath

	legacyAppDir := filepath.Join(legacyAimDir, app.ID)
	newAppDir := filepath.Join(config.AimDir, app.ID)

	if err := copyDirMissing(legacyAppDir, newAppDir); err != nil {
		return err
	}

	_ = rewriteAppPathsToXDG(app, legacyAimDir)

	desktopIconValue := app.IconPath
	if strings.TrimSpace(app.IconPath) != "" {
		installedIconPath, iconValue, err := installMigratedIcon(app.ID, app.IconPath, legacyIconPath)
		if err != nil {
			return err
		}
		app.IconPath = installedIconPath
		desktopIconValue = iconValue
	}

	if strings.TrimSpace(app.DesktopEntryPath) == "" {
		app.DesktopEntryPath = rewritePrefix(legacyDesktopPath, legacyAimDir, config.AimDir)
	}
	if strings.TrimSpace(app.ExecPath) == "" {
		app.ExecPath = rewritePrefix(legacyExecPath, legacyAimDir, config.AimDir)
	}

	if _, err := os.Stat(app.DesktopEntryPath); err == nil {
		if err := rewriteDesktopEntryFile(app.DesktopEntryPath, app.ExecPath, desktopIconValue); err != nil {
			return err
		}
	}

	if strings.TrimSpace(app.DesktopEntryLink) != "" {
		linkName := filepath.Base(app.DesktopEntryLink)
		if linkName == "" || linkName == "." || linkName == string(filepath.Separator) {
			app.DesktopEntryLink = ""
			return nil
		}

		app.DesktopEntryLink = filepath.Join(config.DesktopDir, linkName)

		if _, err := os.Stat(app.DesktopEntryPath); err == nil {
			if err := ensureDesktopLink(app.DesktopEntryPath, app.DesktopEntryLink); err != nil {
				return err
			}
		} else {
			app.DesktopEntryLink = ""
		}
	}

	return nil
}

func rewriteAppPathsToXDG(app *models.App, legacyAimDir string) bool {
	changed := false

	newExecPath := rewritePrefix(app.ExecPath, legacyAimDir, config.AimDir)
	if newExecPath != app.ExecPath {
		app.ExecPath = newExecPath
		changed = true
	}

	newDesktopPath := rewritePrefix(app.DesktopEntryPath, legacyAimDir, config.AimDir)
	if newDesktopPath != app.DesktopEntryPath {
		app.DesktopEntryPath = newDesktopPath
		changed = true
	}

	newIconPath := rewritePrefix(app.IconPath, legacyAimDir, config.AimDir)
	if newIconPath != app.IconPath {
		app.IconPath = newIconPath
		changed = true
	}

	if app.Source.LocalFile != nil {
		newOriginalPath := rewritePrefix(app.Source.LocalFile.OriginalPath, legacyAimDir, config.AimDir)
		if newOriginalPath != app.Source.LocalFile.OriginalPath {
			app.Source.LocalFile.OriginalPath = newOriginalPath
			changed = true
		}
	}

	return changed
}

func deriveLegacyPath(path, legacyAimDir string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(filepath.Clean(trimmed), filepath.Clean(legacyAimDir)+string(filepath.Separator)) {
		return trimmed
	}

	rel, err := filepath.Rel(filepath.Clean(config.AimDir), filepath.Clean(trimmed))
	if err != nil {
		return ""
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}

	return filepath.Join(legacyAimDir, rel)
}

func installMigratedIcon(appID, newIconPath, legacyIconPath string) (string, string, error) {
	sourceIconPath := selectExistingPath(newIconPath, legacyIconPath)
	if sourceIconPath == "" {
		return newIconPath, newIconPath, nil
	}

	ext := strings.ToLower(filepath.Ext(sourceIconPath))
	if ext == "" {
		return newIconPath, newIconPath, nil
	}

	desktopIconValue := appID
	destinationDir := iconInstallDir(ext)
	destinationName := appID + ext
	if !isThemeLookupExtension(ext) {
		desktopIconValue = filepath.Join(destinationDir, destinationName)
	}

	destinationPath := filepath.Join(destinationDir, destinationName)
	if err := os.MkdirAll(destinationDir, 0o755); err != nil {
		return "", "", err
	}

	if filepath.Clean(sourceIconPath) != filepath.Clean(destinationPath) {
		if err := moveFile(sourceIconPath, destinationPath); err != nil {
			return "", "", err
		}
	}

	return destinationPath, desktopIconValue, nil
}

func selectExistingPath(preferred, fallback string) string {
	if preferred != "" {
		if _, err := os.Stat(preferred); err == nil {
			return preferred
		}
	}
	if fallback != "" {
		if _, err := os.Stat(fallback); err == nil {
			return fallback
		}
	}
	return ""
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

func moveFile(srcPath, dstPath string) error {
	if err := os.Rename(srcPath, dstPath); err == nil {
		return nil
	}

	info, err := os.Stat(srcPath)
	if err != nil {
		return err
	}
	if err := copyFile(srcPath, dstPath, info.Mode().Perm()); err != nil {
		return err
	}
	_ = os.Remove(srcPath)
	return nil
}

func rewriteDesktopEntryFile(path, execPath, iconValue string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	inDesktopEntryGroup := false
	inDesktopActionGroup := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inDesktopEntryGroup = trimmed == "[Desktop Entry]"
			inDesktopActionGroup = strings.HasPrefix(trimmed, "[Desktop Action ") && strings.HasSuffix(trimmed, "]")
			continue
		}

		if !inDesktopEntryGroup && !inDesktopActionGroup {
			continue
		}

		if strings.HasPrefix(trimmed, "Exec=") {
			lines[i] = rewriteExecLine(trimmed, execPath)
		}

		if inDesktopEntryGroup && iconValue != "" && strings.HasPrefix(trimmed, "Icon=") {
			lines[i] = "Icon=" + iconValue
		}
	}

	if len(lines) > 0 && lines[len(lines)-1] != "" {
		lines = append(lines, "")
	}

	fileInfo, statErr := os.Stat(path)
	perm := os.FileMode(0o644)
	if statErr == nil {
		perm = fileInfo.Mode().Perm() & 0o666
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), perm)
}

func rewriteExecLine(execLine, execPath string) string {
	value := strings.TrimPrefix(execLine, "Exec=")
	value = strings.TrimSpace(value)
	_, args := splitDesktopExec(value)
	return "Exec=" + quoteDesktopExecArg(execPath) + args
}

func splitDesktopExec(value string) (string, string) {
	if value == "" {
		return "", ""
	}

	if value[0] == '"' {
		escaped := false
		for i := 1; i < len(value); i++ {
			if escaped {
				escaped = false
				continue
			}
			if value[i] == '\\' {
				escaped = true
				continue
			}
			if value[i] == '"' {
				return value[:i+1], value[i+1:]
			}
		}
		return value, ""
	}

	if idx := strings.IndexAny(value, " \t"); idx >= 0 {
		return value[:idx], value[idx:]
	}

	return value, ""
}

func quoteDesktopExecArg(value string) string {
	if value == "" {
		return ""
	}
	if strings.ContainsAny(value, " \t\n\r\"") {
		return strconv.Quote(value)
	}
	return value
}

func rewritePrefix(value, oldBase, newBase string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return value
	}

	cleanValue := filepath.Clean(trimmed)
	cleanOld := filepath.Clean(oldBase)

	rel, err := filepath.Rel(cleanOld, cleanValue)
	if err != nil {
		return value
	}

	if rel == "." {
		return filepath.Clean(newBase)
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return value
	}

	return filepath.Join(newBase, rel)
}

func ensureDesktopLink(target, linkPath string) error {
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		return err
	}

	_ = os.Remove(linkPath)
	if err := os.Symlink(target, linkPath); err != nil {
		return err
	}

	return nil
}

func copyDirMissing(srcDir, dstDir string) error {
	if filepath.Clean(srcDir) == filepath.Clean(dstDir) {
		return nil
	}

	info, err := os.Stat(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}

	if err := os.MkdirAll(dstDir, info.Mode().Perm()); err != nil {
		return err
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())

		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}

		if entry.IsDir() {
			if err := copyDirMissing(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}

		if entryInfo.Mode()&os.ModeSymlink != 0 {
			if _, err := os.Lstat(dstPath); err == nil {
				continue
			} else if !os.IsNotExist(err) {
				return err
			}

			target, err := os.Readlink(srcPath)
			if err != nil {
				return err
			}
			if err := os.Symlink(target, dstPath); err != nil {
				return err
			}
			continue
		}

		if _, err := os.Stat(dstPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}

		if err := copyFile(srcPath, dstPath, entryInfo.Mode().Perm()); err != nil {
			return err
		}
	}

	return nil
}

func copyFile(srcPath, dstPath string, perm os.FileMode) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}

	return nil
}
