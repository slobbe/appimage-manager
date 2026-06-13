package appimage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aim/internal/app"
)

// Stager copies AppImages into a temporary workspace before extraction.
type Stager struct{}

// NewStager creates an AppImage stager.
func NewStager() Stager {
	return Stager{}
}

var _ app.AppImageStager = Stager{}

// Stage copies sourcePath into workspacePath using the source file's base name.
func (Stager) Stage(ctx context.Context, sourcePath string, workspacePath string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if strings.TrimSpace(sourcePath) == "" {
		return "", errors.New("appimage source path is required")
	}
	if strings.TrimSpace(workspacePath) == "" {
		return "", errors.New("appimage staging workspace is required")
	}
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		return "", fmt.Errorf("create appimage staging workspace %q: %w", workspacePath, err)
	}

	destination := filepath.Join(workspacePath, filepath.Base(sourcePath))
	if err := copyFile(ctx, sourcePath, destination); err != nil {
		return "", fmt.Errorf("stage appimage %q to %q: %w", sourcePath, destination, err)
	}

	return destination, nil
}
