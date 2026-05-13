package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	appimageinfra "github.com/slobbe/appimage-manager/internal/infra/appimage"
	"github.com/slobbe/appimage-manager/internal/infra/desktop"
	fsys "github.com/slobbe/appimage-manager/internal/infra/filesystem"
	util "github.com/slobbe/appimage-manager/internal/infra/helpers"
)

type AppInfo struct {
	Name        string
	ID          string
	DesktopStem string
	Version     string
}

type ExtractionData struct {
	Dir              string
	ExecPath         string
	DesktopEntryPath string
	DesktopStem      string
	IconPath         string
}

func ReadAppImageInfo(ctx context.Context, src string) (*AppInfo, error) {
	if _, err := fsys.RequireRegularFile(src, "source path"); err != nil {
		return nil, err
	}
	if !util.HasExtension(src, ".AppImage") {
		return nil, fmt.Errorf("source file must be an .AppImage: %s", filepath.Ext(src))
	}

	src, err := fsys.MakeAbsolute(src)
	if err != nil {
		return nil, fmt.Errorf("failed to make source path absolute: %w", err)
	}

	extraction, cleanup, err := appimageinfra.Inspect(ctx, src)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	desktopEntry, err := fsys.LocateDesktopEntry(extraction.RootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to locate desktop file: %w", err)
	}

	return GetAppInfo(ctx, desktopEntry.Path)
}

func ExtractAppImage(ctx context.Context, src string) (*ExtractionData, error) {
	paths, err := requirePaths()
	if err != nil {
		return nil, err
	}

	if _, err := fsys.RequireRegularFile(src, "source path"); err != nil {
		return nil, err
	}

	srcFileExt := filepath.Ext(src)
	srcFileName := strings.TrimSuffix(filepath.Base(src), srcFileExt)

	if !util.HasExtension(src, ".AppImage") {
		return nil, fmt.Errorf("source file must be an .AppImage: %s", srcFileExt)
	}

	if src, err = fsys.MakeAbsolute(src); err != nil {
		return nil, fmt.Errorf("failed to make source path absolute: %w", err)
	}

	extraction, cleanup, err := appimageinfra.Extract(ctx, src, paths.TempDir)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	tempDesktopEntry, err := fsys.LocateDesktopEntry(extraction.RootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to locate desktop file: %w", err)
	}

	tempIconSrc, err := fsys.LocateIcon(extraction.RootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to locate icon file: %w", err)
	}

	extractDir := filepath.Join(paths.AimDir, "."+srcFileName)
	if err := fsys.EnsureDir(extractDir); err != nil {
		return nil, fmt.Errorf("failed to create temporary extracttion directory %s: %w", extractDir, err)
	}

	execSrc := filepath.Join(extractDir, srcFileName+".AppImage")
	if _, err = fsys.Copy(src, execSrc); err != nil {
		return nil, err
	}

	desktopSrc := filepath.Join(extractDir, srcFileName+".desktop")
	if _, err := fsys.Move(tempDesktopEntry.Path, desktopSrc); err != nil {
		return nil, err
	}

	iconSrc := filepath.Join(extractDir, srcFileName+filepath.Ext(tempIconSrc))
	if _, err := fsys.Move(tempIconSrc, iconSrc); err != nil {
		return nil, err
	}

	extractionData := &ExtractionData{
		Dir:              extractDir,
		ExecPath:         execSrc,
		DesktopEntryPath: desktopSrc,
		DesktopStem:      tempDesktopEntry.Stem,
		IconPath:         iconSrc,
	}

	return extractionData, nil
}

func UpdateDesktopEntry(ctx context.Context, src string, execSrc string, iconSrc string) error {
	if !util.HasExtension(src, ".desktop") {
		return fmt.Errorf("source file must be a .desktop file")
	}

	if execSrc == "" || !util.HasExtension(execSrc, ".AppImage") {
		return fmt.Errorf("exec source file must be a .AppImage file")
	}

	if iconSrc == "" {
		return fmt.Errorf("icon source file cannot be empty")
	}

	if err := desktop.RewriteDesktopEntryFile(src, execSrc, iconSrc); err != nil {
		return fmt.Errorf("failed to write desktop file: %w", err)
	}

	return nil
}

func GetAppInfo(ctx context.Context, desktopSrc string) (*AppInfo, error) {
	if !util.HasExtension(desktopSrc, ".desktop") {
		return nil, fmt.Errorf("source file must be a .desktop file")
	}

	content, err := fsys.ReadTextFile(desktopSrc)
	if err != nil {
		return nil, err
	}

	appInfo := AppInfo{
		DesktopStem: desktop.SanitizeDesktopStem(desktop.DesktopStemFromPath(desktopSrc)),
	}
	inDesktopEntry := false
	for line := range strings.SplitSeq(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inDesktopEntry = trimmed == "[Desktop Entry]"
			continue
		}

		if inDesktopEntry && strings.HasPrefix(trimmed, "[") {
			break
		}

		if !inDesktopEntry {
			continue
		}

		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if strings.Contains(key, "[") {
			continue
		}

		switch key {
		case "Name":
			if appInfo.Name == "" {
				appInfo.Name = value
			}
		case "X-AppImage-Version":
			if appInfo.Version == "" {
				appInfo.Version = sanitizeAppVersion(value)
			}
		}
	}

	if appInfo.Name == "" {
		appInfo.Name = strings.TrimSuffix(filepath.Base(desktopSrc), filepath.Ext(desktopSrc))
	}
	if appInfo.Version == "" {
		appInfo.Version = versionFromFilename(desktopSrc)
	}
	if appInfo.Version == "" {
		appInfo.Version = "unknown"
	}

	appInfo.ID = appInfo.DesktopStem
	if appInfo.ID == "" {
		appInfo.ID = util.Slugify(appInfo.Name)
	}

	return &appInfo, nil
}

func sanitizeAppVersion(raw string) string {
	return normalizeComparableVersion(raw)
}

func versionFromFilename(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if base == "" {
		return ""
	}

	return sanitizeAppVersion(base)
}
