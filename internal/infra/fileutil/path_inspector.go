package fileutil

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/slobbe/appimage-manager/internal/app"
)

type PathInspector struct{}

var _ app.ArtifactPathInspector = PathInspector{}

func (PathInspector) Exists(ctx context.Context, path string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("inspect artifact %q: %w", path, err)
}

func (PathInspector) SameFile(ctx context.Context, first string, second string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	firstInfo, err := os.Stat(first)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("inspect artifact %q: %w", first, err)
	}
	secondInfo, err := os.Stat(second)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("inspect artifact %q: %w", second, err)
	}
	return os.SameFile(firstInfo, secondInfo), nil
}
