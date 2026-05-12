package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

var (
	integrationCacheLookPath       = exec.LookPath
	integrationCacheCommandContext = exec.CommandContext
	integrationCacheWarn           = func(msg string) { fmt.Fprintln(os.Stderr, msg) }
)

func refreshDesktopIntegrationCaches(ctx context.Context) {
	paths, err := requirePaths()
	if err != nil {
		integrationCacheWarn(fmt.Sprintf("Warning: failed to refresh integration caches: %v", err))
		return
	}

	if _, err := runCommandIfAvailable(ctx, "update-desktop-database", paths.DesktopDir); err != nil {
		integrationCacheWarn(fmt.Sprintf("Warning: failed to refresh desktop database: %v", err))
	}

	if err := refreshKDEServiceCache(ctx); err != nil {
		integrationCacheWarn(fmt.Sprintf("Warning: failed to refresh KDE service cache: %v", err))
	}

	ranXDG, err := runCommandIfAvailable(ctx, "xdg-icon-resource", "forceupdate")
	if err != nil {
		integrationCacheWarn(fmt.Sprintf("Warning: failed to refresh icon cache via xdg-icon-resource: %v", err))
		return
	}

	if ranXDG {
		return
	}

	if _, err := runCommandIfAvailable(ctx, "gtk-update-icon-cache", "-f", paths.IconThemeDir); err != nil {
		integrationCacheWarn(fmt.Sprintf("Warning: failed to refresh icon cache via gtk-update-icon-cache: %v", err))
	}
}

func RefreshDesktopIntegrationCaches(ctx context.Context) {
	refreshDesktopIntegrationCaches(ctx)
}

func refreshKDEServiceCache(ctx context.Context) error {
	if ran, err := runCommandIfAvailable(ctx, "kbuildsycoca6"); ran || err != nil {
		return err
	}
	if _, err := runCommandIfAvailable(ctx, "kbuildsycoca5"); err != nil {
		return err
	}
	return nil
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
