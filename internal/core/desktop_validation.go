package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var (
	desktopValidateLookPath       = exec.LookPath
	desktopValidateCommandContext = exec.CommandContext
	desktopValidateWarn           = func(msg string) { fmt.Fprintln(os.Stderr, msg) }
)

func ValidateDesktopEntry(ctx context.Context, desktopPath string) error {
	if strings.TrimSpace(desktopPath) == "" {
		return fmt.Errorf("desktop file path cannot be empty")
	}

	binary, err := desktopValidateLookPath("desktop-file-validate")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			desktopValidateWarn(fmt.Sprintf("warning: desktop-file-validate not found; skipping desktop entry validation for %s", desktopPath))
			return nil
		}
		return fmt.Errorf("failed to find desktop-file-validate: %w", err)
	}

	out, err := desktopValidateCommandContext(ctx, binary, desktopPath).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(out))
		if message == "" {
			return fmt.Errorf("desktop entry validation failed for %s: %w", desktopPath, err)
		}
		return fmt.Errorf("desktop entry validation failed for %s: %s", desktopPath, message)
	}

	return nil
}
