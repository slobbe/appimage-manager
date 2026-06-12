package icon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"aim/internal/app"
)

// Remover removes installed icon files.
type Remover struct{}

// NewRemover creates an icon remover.
func NewRemover() Remover {
	return Remover{}
}

var _ app.IconRemover = Remover{}

func (Remover) Remove(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(path) == "" {
		return errors.New("icon path is required")
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove icon %q: %w", path, err)
	}

	return nil
}
