package appimage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"aim/internal/app"
)

// Installer installs AppImage files into the configured AppImage library.
type Installer struct {
	Dir string
}

// NewInstaller creates an AppImage installer rooted at dir.
func NewInstaller(dir string) Installer {
	return Installer{Dir: dir}
}

var _ app.AppImageInstaller = Installer{}

// Install copies sourcePath into the AppImage library as <appID>.AppImage and
// ensures the installed file is owner-executable.
func (i Installer) Install(ctx context.Context, sourcePath string, appID string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if strings.TrimSpace(sourcePath) == "" {
		return "", errors.New("appimage source path is required")
	}
	if strings.TrimSpace(appID) == "" {
		return "", errors.New("app id is required")
	}
	if strings.TrimSpace(i.Dir) == "" {
		return "", errors.New("appimage install directory is required")
	}

	if err := os.MkdirAll(i.Dir, 0o755); err != nil {
		return "", fmt.Errorf("create appimage install directory %q: %w", i.Dir, err)
	}

	destination := filepath.Join(i.Dir, appID+".AppImage")
	if err := copyFile(ctx, sourcePath, destination); err != nil {
		return "", fmt.Errorf("install appimage %q to %q: %w", sourcePath, destination, err)
	}
	if err := ensureOwnerExecutable(destination); err != nil {
		return "", err
	}

	return destination, nil
}

func copyFile(ctx context.Context, sourcePath string, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	info, err := source.Stat()
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("source path %q is a directory", sourcePath)
	}

	temporaryDestination := destination + ".tmp"
	dest, err := os.OpenFile(temporaryDestination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(dest, source)
	closeErr := dest.Close()
	if copyErr != nil {
		_ = os.Remove(temporaryDestination)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(temporaryDestination)
		return closeErr
	}
	if err := ctx.Err(); err != nil {
		_ = os.Remove(temporaryDestination)
		return err
	}

	return os.Rename(temporaryDestination, destination)
}
