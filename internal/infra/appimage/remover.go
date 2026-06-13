package appimage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/slobbe/appimage-manager/internal/app"
)

// Remover removes installed AppImage files.
type Remover struct{}

// NewRemover creates an AppImage remover.
func NewRemover() Remover {
	return Remover{}
}

var _ app.AppImageRemover = Remover{}

func (Remover) Remove(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(path) == "" {
		return errors.New("appimage path is required")
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove appimage %q: %w", path, err)
	}

	return nil
}
