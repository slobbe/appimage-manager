package appimage

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	models "github.com/slobbe/appimage-manager/internal/domain"
)

type AppInfo = models.AppInfo

type ExtractionData struct {
	Dir              string
	ExecPath         string
	DesktopEntryPath string
	DesktopStem      string
	IconPath         string
}

var defaultService Service

func ReadAppImageInfo(ctx context.Context, src string) (*AppInfo, error) {
	return defaultService.ReadAppImageInfo(ctx, src)
}

func ExtractAppImage(ctx context.Context, src string) (*ExtractionData, error) {
	return defaultService.ExtractAppImage(ctx, src)
}

func UpdateDesktopEntry(ctx context.Context, src string, execSrc string, iconSrc string) error {
	return defaultService.UpdateDesktopEntry(ctx, src, execSrc, iconSrc)
}

func GetAppInfo(ctx context.Context, desktopSrc string) (*AppInfo, error) {
	return defaultService.GetAppInfo(ctx, desktopSrc)
}

func (service Service) ReadAppImageInfo(ctx context.Context, src string) (*AppInfo, error) {
	filesystem, extractor, _, err := service.requireDependencies()
	if err != nil {
		return nil, err
	}
	if _, err := filesystem.RequireRegularFile(src, "source path"); err != nil {
		return nil, err
	}
	if !filesystem.HasExtension(src, ".AppImage") {
		return nil, fmt.Errorf("source file must be an .AppImage: %s", filepath.Ext(src))
	}

	src, err = filesystem.MakeAbsolute(src)
	if err != nil {
		return nil, fmt.Errorf("failed to make source path absolute: %w", err)
	}

	extraction, cleanup, err := extractor.Inspect(ctx, src)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	desktopEntry, err := filesystem.LocateDesktopEntry(extraction.RootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to locate desktop file: %w", err)
	}

	return service.GetAppInfo(ctx, desktopEntry.Path)
}

func (service Service) ExtractAppImage(ctx context.Context, src string) (*ExtractionData, error) {
	paths, err := requirePaths(service.Paths)
	if err != nil {
		return nil, err
	}
	filesystem, extractor, _, err := service.requireDependencies()
	if err != nil {
		return nil, err
	}

	if _, err := filesystem.RequireRegularFile(src, "source path"); err != nil {
		return nil, err
	}

	srcFileExt := filepath.Ext(src)
	srcFileName := strings.TrimSuffix(filepath.Base(src), srcFileExt)

	if !filesystem.HasExtension(src, ".AppImage") {
		return nil, fmt.Errorf("source file must be an .AppImage: %s", srcFileExt)
	}

	if src, err = filesystem.MakeAbsolute(src); err != nil {
		return nil, fmt.Errorf("failed to make source path absolute: %w", err)
	}

	extraction, cleanup, err := extractor.Extract(ctx, src, paths.TempDir)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	tempDesktopEntry, err := filesystem.LocateDesktopEntry(extraction.RootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to locate desktop file: %w", err)
	}

	tempIconSrc, err := filesystem.LocateIcon(extraction.RootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to locate icon file: %w", err)
	}

	extractDir := filepath.Join(paths.AimDir, "."+srcFileName)
	if err := filesystem.EnsureDir(extractDir); err != nil {
		return nil, fmt.Errorf("failed to create temporary extracttion directory %s: %w", extractDir, err)
	}

	execSrc := filepath.Join(extractDir, srcFileName+".AppImage")
	if _, err = filesystem.Copy(src, execSrc); err != nil {
		return nil, err
	}

	desktopSrc := filepath.Join(extractDir, srcFileName+".desktop")
	if _, err := filesystem.Move(tempDesktopEntry.Path, desktopSrc); err != nil {
		return nil, err
	}

	iconSrc := filepath.Join(extractDir, srcFileName+filepath.Ext(tempIconSrc))
	if _, err := filesystem.Move(tempIconSrc, iconSrc); err != nil {
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

func (service Service) UpdateDesktopEntry(ctx context.Context, src string, execSrc string, iconSrc string) error {
	filesystem, _, rewriter, err := service.requireDependencies()
	if err != nil {
		return err
	}
	if !filesystem.HasExtension(src, ".desktop") {
		return fmt.Errorf("source file must be a .desktop file")
	}

	if execSrc == "" || !filesystem.HasExtension(execSrc, ".AppImage") {
		return fmt.Errorf("exec source file must be a .AppImage file")
	}

	if iconSrc == "" {
		return fmt.Errorf("icon source file cannot be empty")
	}

	if err := rewriter.RewriteDesktopEntryFile(src, execSrc, iconSrc); err != nil {
		return fmt.Errorf("failed to write desktop file: %w", err)
	}

	return nil
}

func (service Service) GetAppInfo(ctx context.Context, desktopSrc string) (*AppInfo, error) {
	filesystem, _, rewriter, err := service.requireDependencies()
	if err != nil {
		return nil, err
	}
	if !filesystem.HasExtension(desktopSrc, ".desktop") {
		return nil, fmt.Errorf("source file must be a .desktop file")
	}

	content, err := filesystem.ReadTextFile(desktopSrc)
	if err != nil {
		return nil, err
	}

	desktopStem := rewriter.SanitizeDesktopStem(rewriter.DesktopStemFromPath(desktopSrc))
	return models.ParseDesktopEntryAppInfo(desktopSrc, content, desktopStem), nil
}

func (service Service) requireDependencies() (Filesystem, Extractor, DesktopEntryRewriter, error) {
	if service.Filesystem == nil {
		return nil, nil, nil, fmt.Errorf("appimage filesystem is not configured")
	}
	if service.Extractor == nil {
		return nil, nil, nil, fmt.Errorf("appimage extractor is not configured")
	}
	if service.DesktopEntryRewriter == nil {
		return nil, nil, nil, fmt.Errorf("appimage desktop entry rewriter is not configured")
	}
	return service.Filesystem, service.Extractor, service.DesktopEntryRewriter, nil
}
