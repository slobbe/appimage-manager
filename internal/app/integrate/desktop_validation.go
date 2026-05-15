package integrate

import (
	"context"
	"fmt"
	"strings"
)

func ValidateDesktopEntry(ctx context.Context, desktopPath string) error {
	if strings.TrimSpace(desktopPath) == "" {
		return fmt.Errorf("desktop file path cannot be empty")
	}
	validator, err := requireDesktopEntryValidator()
	if err != nil {
		return err
	}
	return validator.ValidateDesktopEntry(ctx, desktopPath)
}
