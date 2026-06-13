package desktop

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/slobbe/appimage-manager/internal/app"
)

// Refresher refreshes desktop environment caches after installing desktop
// entries and icons. Missing refresh tools are ignored because they vary by
// distribution and desktop environment.
type Refresher struct {
	DesktopDir string
	IconDir    string
	LookPath   func(string) (string, error)
	Run        func(context.Context, string, ...string) error
}

// NewRefresher creates a desktop integration refresher.
func NewRefresher(desktopDir string, iconDir string) Refresher {
	return Refresher{DesktopDir: desktopDir, IconDir: iconDir}
}

var _ app.DesktopIntegrationRefresher = Refresher{}

// Refresh runs available desktop and icon cache refresh commands. Command
// failures are returned to the app layer, which may decide whether refresh
// failures should be fatal for a use case.
func (r Refresher) Refresh(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	commands := []refreshCommand{
		{name: "update-desktop-database", args: []string{r.DesktopDir}, requiredPath: &r.DesktopDir},
		{name: "gtk-update-icon-cache", args: []string{"-f", "-t", filepath.Join(r.IconDir, "hicolor")}, requiredPath: &r.IconDir},
		{name: "xdg-desktop-menu", args: []string{"forceupdate"}},
		{name: "xdg-icon-resource", args: []string{"forceupdate"}},
	}

	var errs []error
	for _, command := range commands {
		if command.requiredPath != nil && *command.requiredPath == "" {
			continue
		}
		if err := r.runIfAvailable(ctx, command.name, command.args...); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

type refreshCommand struct {
	name         string
	args         []string
	requiredPath *string
}

func (r Refresher) runIfAvailable(ctx context.Context, name string, args ...string) error {
	path, err := r.lookPath(name)
	if err != nil {
		return nil
	}
	if err := r.run(ctx, path, args...); err != nil {
		return fmt.Errorf("refresh desktop integration with %s: %w", name, err)
	}
	return nil
}

func (r Refresher) lookPath(name string) (string, error) {
	if r.LookPath != nil {
		return r.LookPath(name)
	}
	return exec.LookPath(name)
}

func (r Refresher) run(ctx context.Context, name string, args ...string) error {
	if r.Run != nil {
		return r.Run(ctx, name, args...)
	}
	return exec.CommandContext(ctx, name, args...).Run()
}
