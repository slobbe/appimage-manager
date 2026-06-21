package fileutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/slobbe/appimage-manager/internal/app"
)

type Remover struct{}

func NewRemover() Remover {
	return Remover{}
}

var _ app.ArtifactRemover = Remover{}

func (Remover) Remove(ctx context.Context, path string) error {
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
