package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/slobbe/appimage-manager/internal/config"
)

var (
	integrationCacheLookPath       = exec.LookPath
	integrationCacheCommandContext = exec.CommandContext
	integrationCacheWarn           = func(msg string) { fmt.Fprintln(os.Stderr, msg) }
)

func refreshDesktopIntegrationCaches(ctx context.Context) {
	if _, err := runCommandIfAvailable(ctx, "update-desktop-database", config.DesktopDir); err != nil {
		integrationCacheWarn(fmt.Sprintf("warning: failed to refresh desktop database: %v", err))
	}

	ranXDG, err := runCommandIfAvailable(ctx, "xdg-icon-resource", "forceupdate")
	if err != nil {
		integrationCacheWarn(fmt.Sprintf("warning: failed to refresh icon cache via xdg-icon-resource: %v", err))
		return
	}

	if ranXDG {
		return
	}

	if _, err := runCommandIfAvailable(ctx, "gtk-update-icon-cache", "-f", config.IconThemeDir); err != nil {
		integrationCacheWarn(fmt.Sprintf("warning: failed to refresh icon cache via gtk-update-icon-cache: %v", err))
	}
}

func runCommandIfAvailable(ctx context.Context, name string, args ...string) (bool, error) {
	binary, err := integrationCacheLookPath(name)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return false, nil
		}
		return false, err
	}

	if err := integrationCacheCommandContext(ctx, binary, args...).Run(); err != nil {
		return true, err
	}

	return true, nil
}
