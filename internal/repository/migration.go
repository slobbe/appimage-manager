package repo

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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
		return nil
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

func migrateAppAssets(app *models.App, legacyAimDir string) error {
	legacyAppDir := filepath.Join(legacyAimDir, app.ID)
	newAppDir := filepath.Join(config.AimDir, app.ID)

	if err := copyDirMissing(legacyAppDir, newAppDir); err != nil {
		return err
	}

	app.ExecPath = rewritePrefix(app.ExecPath, legacyAimDir, config.AimDir)
	app.DesktopEntryPath = rewritePrefix(app.DesktopEntryPath, legacyAimDir, config.AimDir)
	app.IconPath = rewritePrefix(app.IconPath, legacyAimDir, config.AimDir)

	if app.Source.LocalFile != nil {
		app.Source.LocalFile.OriginalPath = rewritePrefix(app.Source.LocalFile.OriginalPath, legacyAimDir, config.AimDir)
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
