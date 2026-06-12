package appimage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"aim/internal/app"
)

const extractedRootDirName = "squashfs-root"

// Extractor extracts AppImages by executing them with --appimage-extract.
type Extractor struct{}

// NewExtractor creates an AppImage extractor.
func NewExtractor() Extractor {
	return Extractor{}
}

var _ app.AppImageExtractor = Extractor{}

// Extract extracts appImagePath into destDir and returns the extracted root.
//
// AppImage extraction creates a squashfs-root directory in the process working
// directory, so the command is run with destDir as its working directory. The
// source AppImage is made owner-executable before extraction because AppImages
// must be executable to run their extraction mode.
func (Extractor) Extract(ctx context.Context, appImagePath string, destDir string) (app.AppImageExtraction, error) {
	if err := ctx.Err(); err != nil {
		return app.AppImageExtraction{}, err
	}
	if appImagePath == "" {
		return app.AppImageExtraction{}, errors.New("appimage path is required")
	}
	if destDir == "" {
		return app.AppImageExtraction{}, errors.New("extraction destination directory is required")
	}

	absoluteAppImagePath, err := filepath.Abs(appImagePath)
	if err != nil {
		return app.AppImageExtraction{}, fmt.Errorf("resolve appimage path %q: %w", appImagePath, err)
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return app.AppImageExtraction{}, fmt.Errorf("create extraction directory %q: %w", destDir, err)
	}

	if err := ensureOwnerExecutable(absoluteAppImagePath); err != nil {
		return app.AppImageExtraction{}, err
	}

	updateInfo, err := appImageUpdateInfo(ctx, absoluteAppImagePath)
	if err != nil {
		return app.AppImageExtraction{}, err
	}

	cmd := exec.CommandContext(ctx, absoluteAppImagePath, "--appimage-extract")
	cmd.Dir = destDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return app.AppImageExtraction{}, ctxErr
		}
		return app.AppImageExtraction{}, fmt.Errorf("extract appimage %q: %w: %s", absoluteAppImagePath, err, string(output))
	}

	rootDir := filepath.Join(destDir, extractedRootDirName)
	info, err := os.Stat(rootDir)
	if err != nil {
		return app.AppImageExtraction{}, fmt.Errorf("find extracted appimage root %q: %w", rootDir, err)
	}
	if !info.IsDir() {
		return app.AppImageExtraction{}, fmt.Errorf("extracted appimage root %q is not a directory", rootDir)
	}

	return app.AppImageExtraction{RootDir: rootDir, UpdateInfo: updateInfo}, nil
}

func appImageUpdateInfo(ctx context.Context, appImagePath string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, appImagePath, "--appimage-updateinformation")
	output, err := cmd.Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		return "", nil
	}

	return strings.TrimSpace(string(output)), nil
}

func ensureOwnerExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat appimage %q: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("appimage path %q is a directory", path)
	}

	mode := info.Mode()
	if mode&0o100 != 0 {
		return nil
	}

	if err := os.Chmod(path, mode|0o100); err != nil {
		return fmt.Errorf("make appimage executable %q: %w", path, err)
	}

	return nil
}
