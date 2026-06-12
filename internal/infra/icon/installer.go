package icon

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

// Installer installs icon files into the configured icon theme root.
type Installer struct {
	Dir string
}

// NewInstaller creates an icon installer rooted at dir.
func NewInstaller(dir string) Installer {
	return Installer{Dir: dir}
}

var _ app.IconInstaller = Installer{}

// Install copies sourcePath into the hicolor icon theme as <appID><source extension>.
func (i Installer) Install(ctx context.Context, sourcePath string, appID string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if strings.TrimSpace(sourcePath) == "" {
		return "", errors.New("icon source path is required")
	}
	if strings.TrimSpace(appID) == "" {
		return "", errors.New("app id is required")
	}
	if strings.TrimSpace(i.Dir) == "" {
		return "", errors.New("icon install directory is required")
	}
	if !isSupportedIconPath(sourcePath) && !isDirIcon(sourcePath) {
		return "", fmt.Errorf("icon source path %q has unsupported extension", sourcePath)
	}

	extension := installedIconExtension(sourcePath)
	destinationDir := filepath.Join(i.Dir, "hicolor", iconThemeSizeDir(extension), "apps")
	if err := os.MkdirAll(destinationDir, 0o755); err != nil {
		return "", fmt.Errorf("create icon install directory %q: %w", destinationDir, err)
	}

	destination := filepath.Join(destinationDir, appID+extension)
	if err := copyIconFile(ctx, sourcePath, destination); err != nil {
		return "", fmt.Errorf("install icon %q to %q: %w", sourcePath, destination, err)
	}

	return destination, nil
}

func installedIconExtension(sourcePath string) string {
	if isDirIcon(sourcePath) {
		return ".png"
	}

	return strings.ToLower(filepath.Ext(sourcePath))
}

func iconThemeSizeDir(extension string) string {
	switch extension {
	case ".svg", ".svgz":
		return "scalable"
	default:
		return "256x256"
	}
}

func copyIconFile(ctx context.Context, sourcePath string, destination string) error {
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
