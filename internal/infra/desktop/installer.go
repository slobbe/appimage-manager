package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aim/internal/app"
)

// Installer installs desktop entries into the configured applications directory.
type Installer struct {
	Dir string
}

// NewInstaller creates a desktop entry installer rooted at dir.
func NewInstaller(dir string) Installer {
	return Installer{Dir: dir}
}

var _ app.DesktopEntryInstaller = Installer{}

// Install writes content into the desktop entry directory as <appID>.desktop.
func (i Installer) Install(ctx context.Context, appID string, content []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if strings.TrimSpace(appID) == "" {
		return "", errors.New("app id is required")
	}
	if strings.TrimSpace(i.Dir) == "" {
		return "", errors.New("desktop entry install directory is required")
	}
	if len(content) == 0 {
		return "", errors.New("desktop entry content is required")
	}

	if err := os.MkdirAll(i.Dir, 0o755); err != nil {
		return "", fmt.Errorf("create desktop entry install directory %q: %w", i.Dir, err)
	}

	destination := filepath.Join(i.Dir, appID+".desktop")
	if err := writeDesktopFile(ctx, destination, content); err != nil {
		return "", fmt.Errorf("install desktop entry to %q: %w", destination, err)
	}

	return destination, nil
}

func writeDesktopFile(ctx context.Context, destination string, content []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	temporaryDestination := destination + ".tmp"
	if err := os.WriteFile(temporaryDestination, content, 0o644); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		_ = os.Remove(temporaryDestination)
		return err
	}

	return os.Rename(temporaryDestination, destination)
}
