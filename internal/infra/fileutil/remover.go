package fileutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

func RemoveArtifact(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(path) == "" {
		return errors.New("artifact path is required")
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove artifact %q: %w", path, err)
	}

	return nil
}
