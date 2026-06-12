package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"aim/internal/app"
)

// Remover removes installed desktop entry files.
type Remover struct{}

// NewRemover creates a desktop entry remover.
func NewRemover() Remover {
	return Remover{}
}

var _ app.DesktopEntryRemover = Remover{}

func (Remover) Remove(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(path) == "" {
		return errors.New("desktop entry path is required")
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove desktop entry %q: %w", path, err)
	}

	return nil
}
