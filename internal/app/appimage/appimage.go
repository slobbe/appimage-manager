package appimage

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	models "github.com/slobbe/appimage-manager/internal/domain"
	appimageinfra "github.com/slobbe/appimage-manager/internal/infra/appimage"
	"github.com/slobbe/appimage-manager/internal/infra/desktop"
	fsys "github.com/slobbe/appimage-manager/internal/infra/filesystem"
)

type AppInfo = models.AppInfo

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
	if !fsys.HasExtension(src, ".AppImage") {
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

	if !fsys.HasExtension(src, ".AppImage") {
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
	if !fsys.HasExtension(src, ".desktop") {
		return fmt.Errorf("source file must be a .desktop file")
	}

	if execSrc == "" || !fsys.HasExtension(execSrc, ".AppImage") {
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
	if !fsys.HasExtension(desktopSrc, ".desktop") {
		return nil, fmt.Errorf("source file must be a .desktop file")
	}

	content, err := fsys.ReadTextFile(desktopSrc)
	if err != nil {
		return nil, err
	}

	desktopStem := desktop.SanitizeDesktopStem(desktop.DesktopStemFromPath(desktopSrc))
	return models.ParseDesktopEntryAppInfo(desktopSrc, content, desktopStem), nil
}
